// Package enterpriseexport freezes a bounded, read-only snapshot of the
// Directory and Official public projections. It never writes canonical data;
// the output is a private candidate directory with deterministic JSONL files
// and a manifest binding the pinned source revisions and public-field policy.
package enterpriseexport

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	officialapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
)

const (
	SchemaVersion     = "enterprise-export-v1"
	ContentVersion    = "1"
	PublicFieldPolicy = "esp-02"

	defaultPageSize     = 500
	defaultMaxStations  = 100_000
	defaultMaxRevisions = 2_000
	defaultMaxPrices    = 5_000_000
)

type StationSource interface {
	ExportPage(ctx context.Context, afterID string, limit int) ([]directoryapp.ExportStation, string, error)
}

type PriceSource interface {
	ExportRevisions(ctx context.Context, fromSurvey, toSurvey, afterID string, limit int) ([]officialapp.ExportRevision, string, error)
	ExportPrices(ctx context.Context, revisionID, afterID string, limit int) ([]officialapp.ExportPrice, string, error)
}

type Builder struct {
	Stations     StationSource
	Prices       PriceSource
	Now          func() time.Time
	PageSize     int
	MaxStations  int
	MaxRevisions int
	MaxPrices    int
}

type Options struct {
	FromSurvey       string
	ToSurvey         string
	OutputDir        string
	DatabaseSnapshot string
}

type Scope struct {
	FromSurvey   string `json:"from_survey"`
	ToSurvey     string `json:"to_survey"`
	PageSize     int    `json:"page_size"`
	MaxStations  int    `json:"max_stations"`
	MaxRevisions int    `json:"max_revisions"`
	MaxPrices    int    `json:"max_prices"`
}

type DirectoryPin struct {
	Source           string         `json:"source"`
	Kind             string         `json:"kind"`
	DatabaseSnapshot string         `json:"database_snapshot,omitempty"`
	StationStatuses  map[string]int `json:"station_statuses"`
}

type RevisionPin struct {
	RevisionID           string `json:"revision_id"`
	SurveyStart          string `json:"survey_start"`
	SurveyEnd            string `json:"survey_end"`
	PublishedAt          string `json:"published_at"`
	SupersedesRevisionID string `json:"supersedes_revision_id,omitempty"`
	SourceURL            string `json:"source_url"`
	SourceChecksum       string `json:"source_checksum"`
	ParserVersion        string `json:"parser_version"`
}

type Sources struct {
	Directory DirectoryPin  `json:"directory"`
	Official  []RevisionPin `json:"official"`
}

type Counts struct {
	Stations  int `json:"stations"`
	Revisions int `json:"revisions"`
	Prices    int `json:"prices"`
}

type FileDigest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Rows   int    `json:"rows"`
}

type Manifest struct {
	SchemaVersion     string       `json:"schema_version"`
	ContentVersion    string       `json:"content_version"`
	PublicFieldPolicy string       `json:"public_field_policy"`
	BuiltAt           string       `json:"built_at"`
	Scope             Scope        `json:"scope"`
	Sources           Sources      `json:"sources"`
	Counts            Counts       `json:"counts"`
	Files             []FileDigest `json:"files"`
	ContentSHA256     string       `json:"content_sha256"`
}

type Station struct {
	StationID          string               `json:"station_id"`
	CNPJ               string               `json:"cnpj,omitempty"`
	DisplayName        string               `json:"display_name"`
	Address            map[string]string    `json:"address,omitempty"`
	MunicipalityCode   string               `json:"municipality_code,omitempty"`
	State              string               `json:"state,omitempty"`
	LocationQuality    string               `json:"location_quality"`
	Coordinates        *directoryapp.LatLon `json:"coordinates,omitempty"`
	OperationStatus    string               `json:"operation_status"`
	IdentityRevisionID string               `json:"identity_revision_id,omitempty"`
}

type Revision struct {
	RevisionID           string `json:"revision_id"`
	SurveyStart          string `json:"survey_start"`
	SurveyEnd            string `json:"survey_end"`
	PublishedAt          string `json:"published_at"`
	SupersedesRevisionID string `json:"supersedes_revision_id,omitempty"`
	SourceURL            string `json:"source_url"`
	SourceChecksum       string `json:"source_checksum"`
	ParserVersion        string `json:"parser_version"`
}

type Price struct {
	PriceID        string `json:"price_id"`
	RevisionID     string `json:"revision_id"`
	StationID      string `json:"station_id"`
	FuelProduct    string `json:"fuel_product"`
	Unit           string `json:"unit"`
	AmountMilliBRL int64  `json:"amount_milli_brl"`
	Currency       string `json:"currency"`
	CollectedOn    string `json:"collected_on"`
}

var publicAddressKeys = []string{"street", "number", "complement", "district", "postal_code", "municipio", "uf"}

func (b Builder) pageSize() int {
	if b.PageSize <= 0 {
		return defaultPageSize
	}
	if b.PageSize > directoryapp.MaxExportLimit {
		return directoryapp.MaxExportLimit
	}
	return b.PageSize
}

func (b Builder) maxStations() int {
	if b.MaxStations <= 0 {
		return defaultMaxStations
	}
	return b.MaxStations
}

func (b Builder) maxRevisions() int {
	if b.MaxRevisions <= 0 {
		return defaultMaxRevisions
	}
	return b.MaxRevisions
}

func (b Builder) maxPrices() int {
	if b.MaxPrices <= 0 {
		return defaultMaxPrices
	}
	return b.MaxPrices
}

func (b Builder) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Build writes a complete candidate snapshot and returns its manifest. The
// output directory must not exist; a temporary directory is renamed only after
// every payload and the manifest are complete.
func (b Builder) Build(ctx context.Context, opts Options) (Manifest, error) {
	if b.Stations == nil || b.Prices == nil {
		return Manifest{}, fmt.Errorf("export: sources are required")
	}
	if opts.FromSurvey == "" || opts.ToSurvey == "" {
		return Manifest{}, fmt.Errorf("export: survey range is required")
	}
	if opts.OutputDir == "" {
		return Manifest{}, fmt.Errorf("export: output directory is required")
	}
	if _, err := os.Stat(opts.OutputDir); err == nil {
		return Manifest{}, fmt.Errorf("export: output directory already exists")
	} else if !os.IsNotExist(err) {
		return Manifest{}, err
	}
	parent := filepath.Dir(opts.OutputDir)
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(opts.OutputDir)+".tmp-")
	if err != nil {
		return Manifest{}, err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(tmp)
		}
	}()

	stationsFile, err := newHashedFile(tmp, "stations.jsonl")
	if err != nil {
		return Manifest{}, err
	}
	stationsEnc := json.NewEncoder(stationsFile)
	stationsEnc.SetEscapeHTML(false)
	statusCounts := map[string]int{}
	stationCount := 0
	afterID := ""
	for {
		page, next, err := b.Stations.ExportPage(ctx, afterID, b.pageSize())
		if err != nil {
			return Manifest{}, err
		}
		for _, station := range page {
			if station.ID == "" {
				return Manifest{}, fmt.Errorf("export: station without id")
			}
			status := strings.TrimSpace(station.Status)
			if status == "" {
				return Manifest{}, fmt.Errorf("export: station %s has no status", station.ID)
			}
			statusCounts[status]++
			row := Station{
				StationID:          station.ID,
				CNPJ:               station.CNPJ,
				DisplayName:        publicDisplayName(station.DisplayName, station.CNPJ),
				Address:            publicAddress(station.Address),
				MunicipalityCode:   station.MunicipalityCode,
				State:              station.State,
				LocationQuality:    station.LocationQuality,
				Coordinates:        station.Coordinates,
				OperationStatus:    status,
				IdentityRevisionID: station.CurrentRevisionID,
			}
			if err := stationsEnc.Encode(row); err != nil {
				return Manifest{}, err
			}
			stationCount++
			if stationCount > b.maxStations() {
				return Manifest{}, fmt.Errorf("export: station budget exceeded")
			}
		}
		if next == "" {
			break
		}
		if next == afterID {
			return Manifest{}, fmt.Errorf("export: station cursor did not advance")
		}
		afterID = next
	}
	stationsDigest, err := stationsFile.Close()
	if err != nil {
		return Manifest{}, err
	}
	stationsDigest.Rows = stationCount

	revisions := make([]officialapp.ExportRevision, 0)
	afterID = ""
	for {
		page, next, err := b.Prices.ExportRevisions(ctx, opts.FromSurvey, opts.ToSurvey, afterID, b.pageSize())
		if err != nil {
			return Manifest{}, err
		}
		revisions = append(revisions, page...)
		if len(revisions) > b.maxRevisions() {
			return Manifest{}, fmt.Errorf("export: revision budget exceeded")
		}
		if next == "" {
			break
		}
		if next == afterID {
			return Manifest{}, fmt.Errorf("export: revision cursor did not advance")
		}
		afterID = next
	}
	sort.Slice(revisions, func(i, j int) bool {
		if revisions[i].SurveyStart != revisions[j].SurveyStart {
			return revisions[i].SurveyStart < revisions[j].SurveyStart
		}
		return revisions[i].ID < revisions[j].ID
	})
	seenRevisions := map[string]bool{}
	for _, revision := range revisions {
		if revision.ID == "" || seenRevisions[revision.ID] {
			return Manifest{}, fmt.Errorf("export: invalid or duplicate revision")
		}
		seenRevisions[revision.ID] = true
	}
	revisionsFile, err := newHashedFile(tmp, "revisions.jsonl")
	if err != nil {
		return Manifest{}, err
	}
	revisionsEnc := json.NewEncoder(revisionsFile)
	revisionsEnc.SetEscapeHTML(false)
	for _, revision := range revisions {
		row := Revision{
			RevisionID:           revision.ID,
			SurveyStart:          revision.SurveyStart,
			SurveyEnd:            revision.SurveyEnd,
			PublishedAt:          revision.PublishedAt,
			SupersedesRevisionID: revision.SupersedesRevisionID,
			SourceURL:            revision.SourceURL,
			SourceChecksum:       revision.SourceChecksum,
			ParserVersion:        revision.ParserVersion,
		}
		if err := revisionsEnc.Encode(row); err != nil {
			return Manifest{}, err
		}
	}
	revisionsDigest, err := revisionsFile.Close()
	if err != nil {
		return Manifest{}, err
	}
	revisionsDigest.Rows = len(revisions)

	pricesFile, err := newHashedFile(tmp, "prices.jsonl")
	if err != nil {
		return Manifest{}, err
	}
	pricesEnc := json.NewEncoder(pricesFile)
	pricesEnc.SetEscapeHTML(false)
	priceCount := 0
	for _, revision := range revisions {
		afterID = ""
		for {
			page, next, err := b.Prices.ExportPrices(ctx, revision.ID, afterID, b.pageSize())
			if err != nil {
				return Manifest{}, err
			}
			for _, price := range page {
				if price.RevisionID != revision.ID {
					return Manifest{}, fmt.Errorf("export: price revision mismatch")
				}
				row := Price{
					PriceID:        price.ID,
					RevisionID:     price.RevisionID,
					StationID:      price.StationID,
					FuelProduct:    price.FuelProduct,
					Unit:           price.Unit,
					AmountMilliBRL: price.AmountMilliBRL,
					Currency:       "BRL",
					CollectedOn:    price.CollectedOn,
				}
				if err := pricesEnc.Encode(row); err != nil {
					return Manifest{}, err
				}
				priceCount++
				if priceCount > b.maxPrices() {
					return Manifest{}, fmt.Errorf("export: price budget exceeded")
				}
			}
			if next == "" {
				break
			}
			if next == afterID {
				return Manifest{}, fmt.Errorf("export: price cursor did not advance")
			}
			afterID = next
		}
	}
	pricesDigest, err := pricesFile.Close()
	if err != nil {
		return Manifest{}, err
	}
	pricesDigest.Rows = priceCount

	revisionPins := make([]RevisionPin, 0, len(revisions))
	for _, revision := range revisions {
		revisionPins = append(revisionPins, RevisionPin{
			RevisionID:           revision.ID,
			SurveyStart:          revision.SurveyStart,
			SurveyEnd:            revision.SurveyEnd,
			PublishedAt:          revision.PublishedAt,
			SupersedesRevisionID: revision.SupersedesRevisionID,
			SourceURL:            revision.SourceURL,
			SourceChecksum:       revision.SourceChecksum,
			ParserVersion:        revision.ParserVersion,
		})
	}
	files := []FileDigest{stationsDigest, revisionsDigest, pricesDigest}
	counts := Counts{Stations: stationCount, Revisions: len(revisions), Prices: priceCount}
	scope := Scope{
		FromSurvey: opts.FromSurvey, ToSurvey: opts.ToSurvey,
		PageSize: b.pageSize(), MaxStations: b.maxStations(),
		MaxRevisions: b.maxRevisions(), MaxPrices: b.maxPrices(),
	}
	manifest := Manifest{
		SchemaVersion: SchemaVersion, ContentVersion: ContentVersion,
		PublicFieldPolicy: PublicFieldPolicy,
		BuiltAt:           b.now().UTC().Format(time.RFC3339),
		Scope:             scope,
		Sources: Sources{
			Directory: DirectoryPin{
				Source: "S-ESP-01", Kind: "directory",
				DatabaseSnapshot: opts.DatabaseSnapshot,
				StationStatuses:  statusCounts,
			},
			Official: revisionPins,
		},
		Counts: counts,
		Files:  files,
	}
	manifest.ContentSHA256, err = contentDigest(manifest)
	if err != nil {
		return Manifest{}, err
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(filepath.Join(tmp, "manifest.json"), encoded, 0o600); err != nil {
		return Manifest{}, err
	}
	if err := os.Rename(tmp, opts.OutputDir); err != nil {
		return Manifest{}, err
	}
	success = true
	return manifest, nil
}

// cpfShapedToken matches a standalone 11-digit or NNN.NNN.NNN-NN token, the
// same shape the portal privacy check refuses.
var cpfShapedToken = regexp.MustCompile(`(^|[^0-9A-Za-z])([0-9]{11}|[0-9]{3}\.[0-9]{3}\.[0-9]{3}-[0-9]{2})([^0-9A-Za-z]|$)`)

// publicDisplayName minimizes a station name for public exports (policy
// esp-02): official MEI legal names often carry the owner's CPF, which is
// never public. A name left empty falls back to the establishment CNPJ.
func publicDisplayName(name, cnpj string) string {
	for cpfShapedToken.MatchString(name) {
		name = cpfShapedToken.ReplaceAllString(name, "$1$3")
	}
	name = strings.Join(strings.Fields(name), " ")
	if name == "" && cnpj != "" {
		return "Posto " + cnpj
	}
	return name
}

func publicAddress(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(publicAddressKeys))
	for _, key := range publicAddressKeys {
		if value := strings.TrimSpace(in[key]); value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type contentSources struct {
	Directory struct {
		Source          string         `json:"source"`
		Kind            string         `json:"kind"`
		StationStatuses map[string]int `json:"station_statuses"`
	} `json:"directory"`
	Official []RevisionPin `json:"official"`
}

type contentScope struct {
	FromSurvey string `json:"from_survey"`
	ToSurvey   string `json:"to_survey"`
}

type contentDescriptor struct {
	SchemaVersion     string         `json:"schema_version"`
	ContentVersion    string         `json:"content_version"`
	PublicFieldPolicy string         `json:"public_field_policy"`
	Scope             contentScope   `json:"scope"`
	Sources           contentSources `json:"sources"`
	Counts            Counts         `json:"counts"`
	Files             []FileDigest   `json:"files"`
}

func contentDigest(manifest Manifest) (string, error) {
	var descriptor contentDescriptor
	descriptor.SchemaVersion = manifest.SchemaVersion
	descriptor.ContentVersion = manifest.ContentVersion
	descriptor.PublicFieldPolicy = manifest.PublicFieldPolicy
	descriptor.Scope = contentScope{
		FromSurvey: manifest.Scope.FromSurvey,
		ToSurvey:   manifest.Scope.ToSurvey,
	}
	descriptor.Sources.Directory.Source = manifest.Sources.Directory.Source
	descriptor.Sources.Directory.Kind = manifest.Sources.Directory.Kind
	descriptor.Sources.Directory.StationStatuses = manifest.Sources.Directory.StationStatuses
	descriptor.Sources.Official = manifest.Sources.Official
	descriptor.Counts = manifest.Counts
	descriptor.Files = manifest.Files
	encoded, err := json.Marshal(descriptor)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type hashedFile struct {
	name   string
	file   *os.File
	buf    *bufio.Writer
	hash   hash.Hash
	bytes  int64
	closed bool
}

func newHashedFile(dir, name string) (*hashedFile, error) {
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return &hashedFile{
		name: name, file: file, buf: bufio.NewWriter(file), hash: sha256.New(),
	}, nil
}

func (h *hashedFile) Write(p []byte) (int, error) {
	n, err := h.buf.Write(p)
	if n > 0 {
		_, _ = h.hash.Write(p[:n])
		h.bytes += int64(n)
	}
	return n, err
}

func (h *hashedFile) Close() (FileDigest, error) {
	if h.closed {
		return FileDigest{}, fmt.Errorf("export: file already closed")
	}
	h.closed = true
	if err := h.buf.Flush(); err != nil {
		_ = h.file.Close()
		return FileDigest{}, err
	}
	if err := h.file.Close(); err != nil {
		return FileDigest{}, err
	}
	return FileDigest{
		Name: h.name, SHA256: "sha256:" + hex.EncodeToString(h.hash.Sum(nil)),
		Bytes: h.bytes,
	}, nil
}

var _ io.Writer = (*hashedFile)(nil)
