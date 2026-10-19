// Package prepared verifies file-backed Rust quality artifacts before resolution
// or publication. It has no filesystem, network or database write capability.
package prepared

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

const MaxBytes int64 = 100 << 20
const MaxRows int64 = 500000
const MaxLine = 1 << 20

type InputIdentity struct {
	SHA256 string
	Bytes  int64
}
type OpenStream func(string) (io.ReadCloser, error)
type ResolveSample func(context.Context, domain.SourceRecord) (domain.Decision, error)
type Batch struct {
	Assays       []application.AssayInput
	Links        []application.LinkInput
	Counts       domain.Counts
	ManifestHash string
}
type counts struct {
	Input       *int64 `json:"input"`
	Accepted    *int64 `json:"accepted"`
	Duplicates  *int64 `json:"duplicates"`
	Quarantined *int64 `json:"quarantined"`
}
type output struct {
	File   string `json:"file"`
	Bytes  *int64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type manifest struct {
	Schema       string   `json:"schema_version"`
	Contract     string   `json:"input_contract"`
	SourceFile   string   `json:"source_file"`
	InputBytes   int64    `json:"input_bytes"`
	InputHash    string   `json:"input_sha256"`
	ManifestHash string   `json:"manifest_hash"`
	Counts       counts   `json:"counts"`
	Outputs      []output `json:"outputs"`
}
type assay struct {
	Schema     string  `json:"schema_version"`
	Source     string  `json:"source"`
	SourceKey  string  `json:"source_key"`
	SampleRef  string  `json:"sample_ref"`
	ObservedAt string  `json:"observed_at"`
	Fuel       string  `json:"fuel"`
	AssayRef   string  `json:"assay_ref"`
	Result     *string `json:"result_text"`
	Unit       *string `json:"unit"`
	Conformity string  `json:"conformity"`
	Evidence   string  `json:"evidence_ref"`
}
type quarantined struct {
	Schema    string  `json:"schema_version"`
	Source    string  `json:"source"`
	Locator   string  `json:"row_locator"`
	Reason    string  `json:"reason"`
	Detail    string  `json:"detail"`
	SourceKey *string `json:"source_key"`
}

func refused(code string) error { return fmt.Errorf("quality preparation: %s", code) }
func validHash(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func field(s string, min, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) >= min && utf8.RuneCountInString(s) <= max && !strings.ContainsAny(s, "\x00\x1f")
}
func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func parseManifest(raw []byte, expected InputIdentity) (manifest, error) {
	var m manifest
	if len(raw) > 1<<20 || !utf8.Valid(raw) || httpapi.DecodeJSON(raw, &m) != nil {
		return m, refused("invalid manifest")
	}
	if m.Schema != "quality-prepared-v1" || (m.Contract != "pmqc-native-19-v2026-10" && m.Contract != "pmqc-native-19-v2026-10b" && m.Contract != "pmqc-prepared-8-v1") || !field(m.SourceFile, 1, 255) || path.Base(m.SourceFile) != m.SourceFile || strings.Contains(m.SourceFile, "\\") || m.SourceFile == "." || m.SourceFile == ".." || strings.IndexFunc(m.SourceFile, unicode.IsControl) >= 0 {
		return m, refused("unknown preparation contract")
	}
	if !validHash(expected.SHA256) || expected.Bytes <= 0 || expected.Bytes > MaxBytes || m.InputHash != expected.SHA256 || m.InputBytes != expected.Bytes || !validHash(m.ManifestHash) {
		return m, refused("input identity mismatch")
	}
	c := m.Counts
	if c.Input == nil || c.Accepted == nil || c.Duplicates == nil || c.Quarantined == nil {
		return m, refused("missing counts")
	}
	for _, n := range []*int64{c.Input, c.Accepted, c.Duplicates, c.Quarantined} {
		if *n < 0 || *n > MaxRows {
			return m, refused("count budget")
		}
	}
	if *c.Input != *c.Accepted+*c.Duplicates+*c.Quarantined {
		return m, refused("unbalanced counts")
	}
	if len(m.Outputs) != 3 {
		return m, refused("output set")
	}
	seen := map[string]bool{}
	var total int64
	for _, o := range m.Outputs {
		if (o.File != "assays.jsonl" && o.File != "duplicates.jsonl" && o.File != "quarantine.jsonl") || seen[o.File] || o.Bytes == nil || *o.Bytes < 0 || *o.Bytes > MaxBytes || !validHash(o.SHA256) {
			return m, refused("output identity")
		}
		seen[o.File] = true
		total += *o.Bytes
	}
	if total > MaxBytes {
		return m, refused("aggregate output budget")
	}
	return m, nil
}
func validateAssay(raw []byte, contract string) (assay, error) {
	var a assay
	if !utf8.Valid(raw) || httpapi.DecodeJSON(raw, &a) != nil || a.Schema != "quality-assay-v1" || a.Source != "pmqc-assay" || a.Result == nil || a.Unit == nil {
		return a, refused("assay schema")
	}
	id, e := kernel.ParseCNPJ(a.SourceKey)
	if e != nil || id.Normalized() != a.SourceKey {
		return a, refused("assay identifier")
	}
	_, validClock := domain.SourceCollectionDay(a.ObservedAt)
	if !field(a.SampleRef, 1, 128) || !validClock || (len(a.ObservedAt) != 10 && contract != "pmqc-native-19-v2026-10b") || !field(a.AssayRef, 1, 128) || !field(*a.Result, 0, 1024) || !field(*a.Unit, 0, 64) || !field(a.Evidence, 1, 512) || (contract == "pmqc-prepared-8-v1" && *a.Result == "") {
		return a, refused("assay field")
	}
	fuel := false
	for _, f := range domain.AllFuels {
		if string(f) == a.Fuel {
			fuel = true
		}
	}
	if !fuel {
		return a, refused("assay fuel")
	}
	switch a.Conformity {
	case "CONFORMING", "NONCONFORMING", "INCONCLUSIVE", "UNKNOWN":
	default:
		return a, refused("assay conformity")
	}
	return a, nil
}
func fingerprint(a assay) string {
	return digest(a.SampleRef, a.SourceKey, a.ObservedAt, a.Fuel, a.AssayRef, *a.Result, *a.Unit, a.Conformity)
}
func sampleID(a assay) string {
	return "s-" + digest(a.Source, a.SampleRef, a.SourceKey, a.ObservedAt, a.Fuel)
}
func sortFields(a assay) []string {
	return []string{a.SourceKey, a.ObservedAt, a.SampleRef, a.AssayRef, *a.Result, *a.Unit, a.Conformity}
}
func ordered(before, after assay) bool {
	a, b := sortFields(before), sortFields(after)
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return true
}

type census struct {
	hash hash.Hash
	n    int64
	last byte
}

func (c *census) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	if len(p) > 0 {
		c.last = p[len(p)-1]
	}
	return c.hash.Write(p)
}
func scan(ctx context.Context, open OpenStream, o output, expectedRows int64, consume func([]byte) error) (err error) {
	stream, e := open(o.File)
	if e != nil || stream == nil {
		if stream != nil {
			_ = stream.Close()
		}
		return refused("stream unavailable")
	}
	defer func() {
		if e := stream.Close(); e != nil && err == nil {
			err = refused("stream close")
		}
	}()
	c := &census{hash: sha256.New()}
	scanner := bufio.NewScanner(io.TeeReader(io.LimitReader(stream, *o.Bytes+1), c))
	scanner.Buffer(make([]byte, 4096), MaxLine+1)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			return i + 1, data[:i], nil
		}
		if atEOF && len(data) > 0 {
			return 0, nil, refused("truncated line")
		}
		return 0, nil, nil
	})
	var rows int64
	for scanner.Scan() {
		if e = ctx.Err(); e != nil {
			return e
		}
		rows++
		if rows > expectedRows {
			return refused("stream row count")
		}
		line := scanner.Bytes()
		if len(line) == 0 || bytes.IndexByte(line, '\r') >= 0 {
			return refused("noncanonical line")
		}
		if e = consume(line); e != nil {
			return e
		}
	}
	if scanner.Err() != nil {
		return refused("stream read")
	}
	if c.n != *o.Bytes || hex.EncodeToString(c.hash.Sum(nil)) != o.SHA256 || rows != expectedRows || (c.n > 0 && c.last != '\n') {
		return refused("stream integrity")
	}
	return nil
}

// Load verifies all three streams before invoking read-only resolution. The
// returned batch is safe to pass to the existing atomic publisher; no partial
// batch is returned on failure. The caller must trust the preparer provenance.
func Load(ctx context.Context, raw []byte, open OpenStream, expected InputIdentity, resolve ResolveSample) (Batch, error) {
	return load(ctx, raw, open, expected, resolve, false)
}

// LoadNative refuses the internal eight-column compatibility contract before
// resolution. A real official archive must carry the verified native contract.
func LoadNative(ctx context.Context, raw []byte, open OpenStream, expected InputIdentity, resolve ResolveSample) (Batch, error) {
	return load(ctx, raw, open, expected, resolve, true)
}
func load(ctx context.Context, raw []byte, open OpenStream, expected InputIdentity, resolve ResolveSample, nativeOnly bool) (Batch, error) {
	var out Batch
	if e := ctx.Err(); e != nil {
		return out, e
	}
	m, e := parseManifest(raw, expected)
	if e != nil {
		return out, e
	}
	if nativeOnly && m.Contract != "pmqc-native-19-v2026-10" && m.Contract != "pmqc-native-19-v2026-10b" {
		return out, refused("native contract required")
	}
	if open == nil || resolve == nil {
		return out, refused("ports unavailable")
	}
	outputs := map[string]output{}
	for _, o := range m.Outputs {
		outputs[o.File] = o
	}
	accepted := make(map[string]bool)
	twins := make(map[string]bool)
	groups := make(map[string]domain.SourceRecord)
	order := make([]string, 0)
	manifestHash := sha256.New()
	var previous assay
	havePrevious := false
	out.Assays = make([]application.AssayInput, 0, int(*m.Counts.Accepted))
	e = scan(ctx, open, outputs["assays.jsonl"], *m.Counts.Accepted, func(line []byte) error {
		a, e := validateAssay(line, m.Contract)
		if e != nil {
			return e
		}
		fp := fingerprint(a)
		if accepted[fp] || (havePrevious && !ordered(previous, a)) {
			return refused("accepted order or duplicate")
		}
		accepted[fp] = true
		sum := sha256.Sum256(line)
		twins[hex.EncodeToString(sum[:])] = true
		if havePrevious {
			_, _ = manifestHash.Write([]byte("\n"))
		}
		_, _ = manifestHash.Write(line)
		previous = a
		havePrevious = true
		sample := sampleID(a)
		if _, ok := groups[sample]; !ok {
			groups[sample] = domain.SourceRecord{RecordKey: "pmqc-assay-v1:" + sample, CNPJ: a.SourceKey, ObservedAt: sourceDay(a.ObservedAt)}
			order = append(order, sample)
		}
		out.Assays = append(out.Assays, application.AssayInput{SampleRef: sample, AssayRef: a.AssayRef, SourceCNPJ: a.SourceKey, ObservedOn: sourceDay(a.ObservedAt), SourceObservedAt: sourceClock(a.ObservedAt), Fuel: a.Fuel, ResultText: *a.Result, Unit: *a.Unit, Conformity: a.Conformity, EvidenceRef: a.Evidence, Fingerprint: fp})
		return nil
	})
	if e != nil {
		return Batch{}, e
	}
	if havePrevious {
		_, _ = manifestHash.Write([]byte("\n"))
	}
	_, _ = fmt.Fprintf(manifestHash, "counts:%d:%d:%d:%d", *m.Counts.Input, *m.Counts.Accepted, *m.Counts.Duplicates, *m.Counts.Quarantined)
	if hex.EncodeToString(manifestHash.Sum(nil)) != m.ManifestHash {
		return Batch{}, refused("logical manifest mismatch")
	}
	e = scan(ctx, open, outputs["duplicates.jsonl"], *m.Counts.Duplicates, func(line []byte) error {
		sum := sha256.Sum256(line)
		if !twins[hex.EncodeToString(sum[:])] {
			return refused("duplicate has no accepted twin")
		}
		return nil
	})
	if e != nil {
		return Batch{}, e
	}
	e = scan(ctx, open, outputs["quarantine.jsonl"], *m.Counts.Quarantined, func(line []byte) error {
		var q quarantined
		if !utf8.Valid(line) || httpapi.DecodeJSON(line, &q) != nil || q.Schema != "station-quarantine-v1" || q.Source != "pmqc-assay" || !strings.HasPrefix(q.Locator, m.SourceFile+":") || !field(q.Reason, 1, 128) || !field(q.Detail, 0, MaxLine) {
			return refused("quarantine schema")
		}
		return nil
	})
	if e != nil {
		return Batch{}, e
	}
	out.Links = make([]application.LinkInput, 0, len(order))
	for _, sample := range order {
		if e = ctx.Err(); e != nil {
			return Batch{}, e
		}
		record := groups[sample]
		d, e := resolve(ctx, record)
		if e != nil {
			return Batch{}, refused("resolution unavailable")
		}
		link := application.LinkInput{RecordKey: record.RecordKey, Reason: d.Reason}
		switch d.Kind {
		case domain.ResolutionBound:
			station, e := application.ValidateStationID(d.Link.StationID)
			if e != nil || d.Link.RecordKey != record.RecordKey || !d.Link.Active {
				return Batch{}, refused("resolution binding")
			}
			link.StationID = station
			link.Active = true
			link.Automatic = d.Link.Automatic
		case domain.ResolutionUnmatched, domain.ResolutionAmbiguous, domain.ResolutionWrongBranch, domain.ResolutionSuccession:
			if d.Link.Active {
				return Batch{}, refused("resolution review")
			}
		default:
			return Batch{}, refused("resolution kind")
		}
		if !field(link.Reason, 1, 128) {
			return Batch{}, refused("resolution reason")
		}
		out.Links = append(out.Links, link)
	}
	out.Counts = domain.Counts{Input: int(*m.Counts.Input), Accepted: int(*m.Counts.Accepted), Duplicates: int(*m.Counts.Duplicates), Quarantined: int(*m.Counts.Quarantined)}
	out.ManifestHash = m.ManifestHash
	return out, nil
}

func sourceDay(raw string) string { day, _ := domain.SourceCollectionDay(raw); return day }
func sourceClock(raw string) string {
	if len(raw) == 19 {
		return raw
	}
	return ""
}
