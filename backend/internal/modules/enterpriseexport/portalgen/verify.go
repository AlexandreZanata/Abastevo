package portalgen

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/analytics"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const (
	MaxSitemapURLs        = 50_000
	MaxSitemapBytes       = int64(50 << 20)
	DefaultMaxHTMLGzip    = int64(100 << 10)
	DefaultMaxScriptGzip  = int64(100 << 10)
	DefaultMaxShardGzip   = int64(250 << 10)
	DefaultMaxFilesMargin = 1_000
)

var (
	forbiddenJSONKeys = map[string]bool{
		"cpf": true, "password": true, "passwd": true, "secret": true,
		"token": true, "access_token": true, "refresh_token": true,
		"authorization": true, "latitude": true, "longitude": true,
		"coordinates": true, "lat": true, "lon": true, "email": true,
		"phone": true, "owner_id": true, "account_id": true,
		"internal_notes": true, "unlabelled_field": true,
		"history_revision_id": true, "paid_text": true, "paid_minor": true,
		"department": true,
	}
	allowedExtensions = map[string]bool{
		".html": true, ".css": true, ".js": true, ".json": true,
		".jsonl": true, ".csv": true, ".txt": true, ".xml": true,
	}
	searchStationKeys = map[string]bool{
		"i": true, "n": true, "s": true, "c": true, "m": true,
		"mn": true, "u": true, "st": true, "sl": true, "p": true,
	}
	cpfTextPattern  = regexp.MustCompile(`(^|[^0-9A-Za-z])([0-9]{11}|[0-9]{3}\.[0-9]{3}\.[0-9]{3}-[0-9]{2})([^0-9A-Za-z]|$)`)
	rawScriptTag    = regexp.MustCompile(`<script\b[^>]*>`)
	allowedScript   = regexp.MustCompile(`^<script type="module" src="/assets/portal\.[0-9a-f]{8}\.js" defer>$`)
	unsafeAttribute = regexp.MustCompile(`(?i)<[^>]*\sonerror\s*=`)
	unsafeURL       = regexp.MustCompile(`(?i)(?:href|src)="\s*javascript:`)
	assetRef        = regexp.MustCompile(`(?:href|src)="(/assets/[^"]+)"`)
	htmlRef         = regexp.MustCompile(`(?:href|src)="([^"]+)"`)
	metaRobots      = regexp.MustCompile(`<meta name="robots" content="([^"]+)">`)
	shaPrefixName   = regexp.MustCompile(`^portal\.([0-9a-f]{8})\.(css|js)$`)
)

type VerifyOptions struct {
	SiteURL            string
	CompareDir         string
	MaxFiles           int
	MaxBytes           int64
	MaxHTMLGzipBytes   int64
	MaxScriptGzipBytes int64
	MaxShardGzipBytes  int64
	// OfficialOnly requires an official-only artifact (fiscalizacao pages,
	// metodologia and 404 only) and refuses any price, station, search,
	// insight or download route.
	OfficialOnly bool
}

type VerifyResult struct {
	Pages                  int
	Files                  int
	Bytes                  int64
	ContentSHA256          string
	SitemapURLs            int
	LargestHTMLGzipBytes   int64
	LargestScriptGzipBytes int64
	LargestShardGzipBytes  int64
	LargestCSSGzipBytes    int64
	InitialViewGzipBytes   int64
}

// Verify checks one complete portal artifact against its manifest, public-field
// allowlists, content-addressed assets, sitemap, compression/file/byte budgets
// and optional second build. It never writes to the artifact.
func Verify(dir string, opts VerifyOptions) (VerifyResult, error) {
	first, firstManifest, err := verifyOne(dir, opts)
	if err != nil {
		return VerifyResult{}, err
	}
	if opts.CompareDir == "" {
		return first, nil
	}
	compareOpts := opts
	compareOpts.CompareDir = ""
	second, secondManifest, err := verifyOne(opts.CompareDir, compareOpts)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("portalcheck: compare artifact: %w", err)
	}
	if first.ContentSHA256 != second.ContentSHA256 || first.Pages != second.Pages || first.Files != second.Files || first.Bytes != second.Bytes {
		return VerifyResult{}, fmt.Errorf("portalcheck: builds differ in manifest/counts")
	}
	if err := compareFileDigests(firstManifest.Files, secondManifest.Files); err != nil {
		return VerifyResult{}, err
	}
	return first, nil
}

func verifyOne(dir string, opts VerifyOptions) (VerifyResult, Manifest, error) {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: artifact directory unavailable")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: artifact directory is not private")
	}
	manifestPath := filepath.Join(dir, "portal-manifest.json")
	manifestInfo, err := os.Lstat(manifestPath)
	if err != nil || !manifestInfo.Mode().IsRegular() || manifestInfo.Mode().Perm()&0o077 != 0 {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: portal manifest unavailable")
	}
	rawManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: portal manifest unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(rawManifest))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: invalid portal manifest")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: trailing portal manifest data")
	}
	if manifest.SchemaVersion != SchemaVersion || manifest.InputContentSHA256 == "" ||
		manifest.MetricsVersion != analytics.MetricsVersion || manifest.Pages <= 0 ||
		len(manifest.Files) == 0 || !validSHA256Prefix(manifest.ContentSHA256) {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: invalid portal manifest fields")
	}
	if opts.OfficialOnly != manifest.OfficialOnly {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: official-only mode mismatch")
	}
	if _, err := time.Parse(time.RFC3339, manifest.PublishedAt); err != nil {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: invalid publication time")
	}
	if err := checkOfficialPin(manifest.OfficialRecords); err != nil {
		return VerifyResult{}, Manifest{}, err
	}
	contentSHA256, err := manifestContentDigest(manifest)
	if err != nil || contentSHA256 != manifest.ContentSHA256 {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: portal manifest digest mismatch")
	}

	files := make(map[string]FileDigest, len(manifest.Files))
	previous := ""
	var totalBytes int64
	for _, digest := range manifest.Files {
		if err := safeRelPath(digest.Path); err != nil || !allowedExtensions[strings.ToLower(filepath.Ext(digest.Path))] {
			return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: invalid file path")
		}
		if digest.Path <= previous {
			return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: manifest files are not sorted/unique")
		}
		previous = digest.Path
		if !validSHA256Prefix(digest.SHA256) || digest.Bytes < 0 {
			return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: invalid file digest")
		}
		if _, exists := files[digest.Path]; exists {
			return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: duplicate file digest")
		}
		fileInfo, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(digest.Path)))
		if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode()&fs.ModeSymlink != 0 || fileInfo.Mode().Perm()&0o077 != 0 {
			return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: missing/unsafe file %s", digest.Path)
		}
		if fileInfo.Size() != digest.Bytes {
			return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: size mismatch %s", digest.Path)
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(digest.Path)))
		if err != nil {
			return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: unreadable file %s", digest.Path)
		}
		sum := sha256.Sum256(body)
		if "sha256:"+hex.EncodeToString(sum[:]) != digest.SHA256 {
			return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: hash mismatch %s", digest.Path)
		}
		files[digest.Path] = digest
		totalBytes += digest.Bytes
	}
	totalBytes += int64(len(rawManifest))
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = manifest.Pages + DefaultMaxFilesMargin
	}
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	if manifest.Pages > defaultMaxPages || len(files)+1 > maxFiles || totalBytes > maxBytes {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: file/byte/page budget exceeded")
	}
	onDisk, err := walkArtifact(dir)
	if err != nil {
		return VerifyResult{}, Manifest{}, err
	}
	if len(onDisk) != len(files)+1 {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: unexpected artifact files")
	}
	for _, rel := range onDisk {
		if rel == "portal-manifest.json" {
			continue
		}
		if _, ok := files[rel]; !ok {
			return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: unlisted artifact file %s", rel)
		}
	}
	if err := checkPrivacy(dir, files); err != nil {
		return VerifyResult{}, Manifest{}, err
	}
	pageCount := 0
	for path := range files {
		if strings.HasSuffix(path, ".html") {
			pageCount++
		}
	}
	if pageCount != manifest.Pages {
		return VerifyResult{}, Manifest{}, fmt.Errorf("portalcheck: page count mismatch")
	}
	if opts.OfficialOnly {
		if err := checkOfficialOnlyScope(files); err != nil {
			return VerifyResult{}, Manifest{}, err
		}
	} else {
		if err := checkSearchArtifacts(dir, files); err != nil {
			return VerifyResult{}, Manifest{}, err
		}
		if err := checkDownloadArtifacts(dir, files); err != nil {
			return VerifyResult{}, Manifest{}, err
		}
	}
	result, err := checkBudgetsAndHTML(dir, files, manifest, opts)
	if err != nil {
		return VerifyResult{}, Manifest{}, err
	}
	if err := checkOfficialPages(dir, files, manifest.OfficialRecords, opts.OfficialOnly); err != nil {
		return VerifyResult{}, Manifest{}, err
	}
	result.Pages = manifest.Pages
	result.Files = len(files) + 1
	result.Bytes = totalBytes
	result.ContentSHA256 = manifest.ContentSHA256
	return result, manifest, nil
}

func walkArtifact(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("portalcheck: symlink in artifact")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("portalcheck: non-regular artifact file")
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if err := safeRelPath(rel); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("portalcheck: artifact file is not private")
		}
		out = append(out, rel)
		return nil
	})
	sort.Strings(out)
	return out, err
}

// checkOfficialOnlyScope refuses any price, station, search, insight or
// download route in an official-only artifact. Only fiscalizacao pages,
// metodologia, 404, shared assets, robots, sitemap and the manifest may exist.
func checkOfficialOnlyScope(files map[string]FileDigest) error {
	for _, prefix := range []string{"precos/", "postos/", "dados/", "consultar/", "curiosidades/"} {
		for path := range files {
			if strings.HasPrefix(path, prefix) {
				return fmt.Errorf("portalcheck: official-only artifact contains %s", path)
			}
		}
	}
	return nil
}

func safeRelPath(path string) error {
	if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path ||
		strings.Contains(path, "\\") || strings.Contains(path, "\x00") ||
		path == ".." || strings.HasPrefix(path, "../") ||
		strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return errors.New("portalcheck: unsafe path")
	}
	return nil
}

func validSHA256Prefix(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func compareFileDigests(left, right []FileDigest) error {
	if len(left) != len(right) {
		return fmt.Errorf("portalcheck: build file sets differ")
	}
	for i := range left {
		if left[i] != right[i] {
			return fmt.Errorf("portalcheck: build file digest differs")
		}
	}
	return nil
}

func checkPrivacy(dir string, files map[string]FileDigest) error {
	for path := range files {
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("portalcheck: unreadable text artifact")
		}
		if cpfTextPattern.Match(body) {
			return fmt.Errorf("portalcheck: CPF-shaped value in %s", path)
		}
		if strings.HasSuffix(path, ".html") {
			if err := checkHTMLPrivacy(body); err != nil {
				return err
			}
		}
		if strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".jsonl") {
			if err := checkJSONKeys(body, strings.HasSuffix(path, ".jsonl")); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkHTMLPrivacy(body []byte) error {
	for _, tag := range rawScriptTag.FindAll(body, -1) {
		if !allowedScript.Match(tag) {
			return fmt.Errorf("portalcheck: unexpected script element")
		}
	}
	if unsafeAttribute.Match(body) || unsafeURL.Match(body) {
		return fmt.Errorf("portalcheck: unsafe HTML attribute")
	}
	return nil
}

func checkJSONKeys(body []byte, jsonl bool) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if jsonl {
		scanner := bufio.NewScanner(bytes.NewReader(body))
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			var value any
			lineDecoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
			lineDecoder.UseNumber()
			if err := lineDecoder.Decode(&value); err != nil {
				return fmt.Errorf("portalcheck: invalid JSONL")
			}
			if err := checkJSONValue(value); err != nil {
				return err
			}
		}
		return scanner.Err()
	}
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("portalcheck: invalid JSON artifact")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("portalcheck: trailing JSON artifact")
	}
	return checkJSONValue(value)
}

func checkJSONValue(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if forbiddenJSONKeys[key] {
				return fmt.Errorf("portalcheck: private JSON key %q", key)
			}
			if err := checkJSONValue(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := checkJSONValue(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkSearchArtifacts(dir string, files map[string]FileDigest) error {
	release := ""
	shards := 0
	for path := range files {
		if !strings.HasPrefix(path, "dados/busca/") || !strings.HasSuffix(path, ".json") {
			continue
		}
		parts := strings.Split(path, "/")
		if len(parts) != 4 || parts[3] == "all.json" || parts[3] == "nacional.json" {
			return fmt.Errorf("portalcheck: invalid search shard path")
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("portalcheck: unreadable search shard")
		}
		var decoded struct {
			Schema   string            `json:"schema"`
			Release  string            `json:"release"`
			UF       string            `json:"uf"`
			Count    int               `json:"count"`
			Stations []json.RawMessage `json:"stations"`
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&decoded); err != nil {
			return fmt.Errorf("portalcheck: invalid search shard")
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return fmt.Errorf("portalcheck: trailing search shard data")
		}
		if decoded.Schema != SearchSchema || decoded.Release != parts[2] || decoded.UF != strings.TrimSuffix(parts[3], ".json") || decoded.Count != len(decoded.Stations) {
			return fmt.Errorf("portalcheck: search shard identity mismatch")
		}
		if release == "" {
			release = decoded.Release
		} else if release != decoded.Release {
			return fmt.Errorf("portalcheck: mixed search release")
		}
		for _, raw := range decoded.Stations {
			var row map[string]json.RawMessage
			if err := json.Unmarshal(raw, &row); err != nil {
				return fmt.Errorf("portalcheck: invalid search row")
			}
			for key := range row {
				if !searchStationKeys[key] {
					return fmt.Errorf("portalcheck: search row key %q not allowed", key)
				}
			}
			for _, required := range []string{"i", "n", "s"} {
				if len(row[required]) == 0 {
					return fmt.Errorf("portalcheck: search row missing %s", required)
				}
			}
			var id, name, searchName, cnpj, uf string
			if err := json.Unmarshal(row["i"], &id); err != nil || !isUUID(id) {
				return fmt.Errorf("portalcheck: invalid search station id")
			}
			if err := json.Unmarshal(row["n"], &name); err != nil || strings.TrimSpace(name) == "" {
				return fmt.Errorf("portalcheck: invalid search station name")
			}
			if err := json.Unmarshal(row["s"], &searchName); err != nil || strings.TrimSpace(searchName) == "" {
				return fmt.Errorf("portalcheck: invalid search station key")
			}
			if raw, ok := row["c"]; ok {
				if err := json.Unmarshal(raw, &cnpj); err != nil {
					return fmt.Errorf("portalcheck: invalid search CNPJ")
				}
				if cnpj != "" {
					parsed, err := kernel.ParseCNPJ(cnpj)
					if err != nil || parsed.Normalized() != cnpj {
						return fmt.Errorf("portalcheck: invalid search CNPJ")
					}
				}
			}
			if raw, ok := row["u"]; ok {
				if err := json.Unmarshal(raw, &uf); err != nil || (uf != "" && uf != "sem-uf" && !isUF(uf)) {
					return fmt.Errorf("portalcheck: invalid search UF")
				}
			}
		}
		shards++
	}
	if shards == 0 {
		return fmt.Errorf("portalcheck: no search shards")
	}
	consultar, err := os.ReadFile(filepath.Join(dir, "consultar", "index.html"))
	if err != nil {
		return fmt.Errorf("portalcheck: consultar page unavailable")
	}
	if !bytes.Contains(consultar, []byte(`data-release="`+release+`"`)) || !bytes.Contains(consultar, []byte(`data-shard-base="/dados/busca/"`)) {
		return fmt.Errorf("portalcheck: search release reference mismatch")
	}
	return nil
}

func checkDownloadArtifacts(dir string, files map[string]FileDigest) error {
	release := ""
	found := 0
	for path := range files {
		if !strings.HasPrefix(path, "dados/") || strings.HasPrefix(path, "dados/busca/") || !strings.HasSuffix(path, "/manifest.json") {
			continue
		}
		found++
		parts := strings.Split(path, "/")
		if len(parts) != 3 {
			return fmt.Errorf("portalcheck: invalid download manifest path")
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("portalcheck: unreadable download manifest")
		}
		var decoded map[string]json.RawMessage
		if err := json.Unmarshal(body, &decoded); err != nil {
			return fmt.Errorf("portalcheck: invalid download manifest")
		}
		for _, key := range []string{"schema", "input_content_sha256", "release", "period", "published_at", "attribution", "notes", "files"} {
			if len(decoded[key]) == 0 {
				return fmt.Errorf("portalcheck: download manifest missing %s", key)
			}
		}
		if len(decoded) != 8 {
			return fmt.Errorf("portalcheck: unexpected download manifest key")
		}
		var releaseValue string
		if err := json.Unmarshal(decoded["release"], &releaseValue); err != nil || releaseValue != parts[1] {
			return fmt.Errorf("portalcheck: download release mismatch")
		}
		if release == "" {
			release = releaseValue
		} else if release != releaseValue {
			return fmt.Errorf("portalcheck: mixed download release")
		}
		var entries []downloadEntry
		if err := json.Unmarshal(decoded["files"], &entries); err != nil || len(entries) != 3 {
			return fmt.Errorf("portalcheck: invalid download file census")
		}
		byName := map[string]downloadEntry{}
		for _, entry := range entries {
			byName[entry.Name] = entry
		}
		for _, name := range []string{"estacoes.csv", "precos.csv", "resumos.csv"} {
			filePath := "dados/" + releaseValue + "/" + name
			digest, ok := files[filePath]
			if !ok {
				return fmt.Errorf("portalcheck: download file %s missing", name)
			}
			entry, ok := byName[name]
			if !ok || entry.SHA256 != digest.SHA256 || entry.Bytes != digest.Bytes {
				return fmt.Errorf("portalcheck: download manifest digest mismatch")
			}
			if err := checkCSVHeader(dir, filePath, name, digest); err != nil {
				return err
			}
		}
	}
	if found != 1 {
		return fmt.Errorf("portalcheck: expected one download manifest")
	}
	return nil
}

func checkCSVHeader(dir, path, name string, digest FileDigest) error {
	body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if err != nil {
		return fmt.Errorf("portalcheck: unreadable download CSV")
	}
	reader := csv.NewReader(bytes.NewReader(body))
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("portalcheck: invalid download CSV")
	}
	expected := map[string][]string{
		"estacoes.csv": {"station_id", "display_name", "cnpj", "municipality_code", "municipality_name", "state", "operation_status", "location_quality", "address_street", "address_number", "address_district", "address_postal_code", "latest_fuel_count"},
		"precos.csv":   {"station_id", "display_name", "cnpj", "state", "municipality_code", "fuel", "unit", "condition", "amount_milli_brl", "amount_brl", "collected_on", "survey_start", "survey_end", "revision_id", "source_url", "source_checksum"},
		"resumos.csv":  {"revision_id", "survey_start", "survey_end", "geography_level", "geography_code", "fuel", "unit", "condition", "station_count", "source_observations", "min_milli_brl", "max_milli_brl", "spread_milli_brl", "method"},
	}
	want, ok := expected[name]
	if !ok || len(header) != len(want) {
		return fmt.Errorf("portalcheck: unexpected download CSV header")
	}
	for i := range want {
		if header[i] != want[i] {
			return fmt.Errorf("portalcheck: download CSV header field %q not allowed", header[i])
		}
	}
	if digest.Bytes == 0 {
		return fmt.Errorf("portalcheck: empty download CSV")
	}
	return nil
}

func checkBudgetsAndHTML(dir string, files map[string]FileDigest, manifest Manifest, opts VerifyOptions) (VerifyResult, error) {
	result := VerifyResult{}
	maxHTML := opts.MaxHTMLGzipBytes
	if maxHTML <= 0 {
		maxHTML = DefaultMaxHTMLGzip
	}
	maxScript := opts.MaxScriptGzipBytes
	if maxScript <= 0 {
		maxScript = DefaultMaxScriptGzip
	}
	maxShard := opts.MaxShardGzipBytes
	if maxShard <= 0 {
		maxShard = DefaultMaxShardGzip
	}
	indexable := map[string]bool{}
	sitemapSeen := false
	for path := range files {
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return VerifyResult{}, fmt.Errorf("portalcheck: unreadable artifact")
		}
		switch {
		case strings.HasSuffix(path, ".html"):
			compressed, err := gzipSize(body)
			if err != nil {
				return VerifyResult{}, err
			}
			if compressed > maxHTML {
				return VerifyResult{}, fmt.Errorf("portalcheck: HTML gzip budget exceeded by %s", path)
			}
			if compressed > result.LargestHTMLGzipBytes {
				result.LargestHTMLGzipBytes = compressed
			}
			robots := metaRobots.FindSubmatch(body)
			if len(robots) != 2 {
				return VerifyResult{}, fmt.Errorf("portalcheck: page lacks robots meta")
			}
			if string(robots[1]) == "index,follow" {
				indexable[path] = true
			} else if string(robots[1]) != "noindex,nofollow" {
				return VerifyResult{}, fmt.Errorf("portalcheck: unexpected robots value")
			}
			if err := checkHTMLReferences(dir, files, path, body); err != nil {
				return VerifyResult{}, err
			}
		case strings.HasSuffix(path, ".js"):
			compressed, err := gzipSize(body)
			if err != nil {
				return VerifyResult{}, err
			}
			if compressed > maxScript {
				return VerifyResult{}, fmt.Errorf("portalcheck: JavaScript gzip budget exceeded by %s", path)
			}
			if compressed > result.LargestScriptGzipBytes {
				result.LargestScriptGzipBytes = compressed
			}
		case strings.HasPrefix(path, "dados/busca/") && strings.HasSuffix(path, ".json"):
			compressed, err := gzipSize(body)
			if err != nil {
				return VerifyResult{}, err
			}
			if compressed > maxShard {
				return VerifyResult{}, fmt.Errorf("portalcheck: search shard gzip budget exceeded by %s", path)
			}
			if compressed > result.LargestShardGzipBytes {
				result.LargestShardGzipBytes = compressed
			}
		case strings.HasSuffix(path, ".css"):
			compressed, err := gzipSize(body)
			if err != nil {
				return VerifyResult{}, err
			}
			if compressed > result.LargestCSSGzipBytes {
				result.LargestCSSGzipBytes = compressed
			}
		case path == "sitemap.xml":
			sitemapSeen = true
			if int64(len(body)) > MaxSitemapBytes {
				return VerifyResult{}, fmt.Errorf("portalcheck: sitemap bytes budget exceeded")
			}
		}
	}
	sitemapURLs, err := checkSitemap(dir, files, indexable, sitemapSeen, opts.SiteURL)
	if err != nil {
		return VerifyResult{}, err
	}
	result.SitemapURLs = sitemapURLs
	if err := checkMixedVersionAssets(dir, files); err != nil {
		return VerifyResult{}, err
	}
	initialPage := filepath.Join(dir, "consultar", "index.html")
	initialMissing := "portalcheck: consultar page unavailable"
	if opts.OfficialOnly {
		initialPage = filepath.Join(dir, "fiscalizacao", "index.html")
		initialMissing = "portalcheck: fiscalizacao page unavailable"
	}
	initialBody, err := os.ReadFile(initialPage)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("%s", initialMissing)
	}
	initial, err := gzipSize(initialBody)
	if err != nil {
		return VerifyResult{}, err
	}
	for _, ref := range assetRef.FindAllSubmatch(initialBody, -1) {
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(strings.TrimPrefix(string(ref[1]), "/"))))
		if err != nil {
			return VerifyResult{}, fmt.Errorf("portalcheck: initial asset unavailable")
		}
		compressed, err := gzipSize(body)
		if err != nil {
			return VerifyResult{}, err
		}
		initial += compressed
	}
	result.InitialViewGzipBytes = initial
	return result, nil
}

func gzipSize(body []byte) (int64, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(body); err != nil {
		return 0, err
	}
	if err := writer.Close(); err != nil {
		return 0, err
	}
	return int64(buffer.Len()), nil
}

func checkHTMLReferences(dir string, files map[string]FileDigest, path string, body []byte) error {
	for _, match := range htmlRef.FindAllSubmatch(body, -1) {
		ref := strings.TrimSpace(string(match[1]))
		if ref == "" || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "mailto:") || strings.HasPrefix(ref, "tel:") || strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
			continue
		}
		ref = strings.SplitN(ref, "#", 2)[0]
		ref = strings.SplitN(ref, "?", 2)[0]
		if !strings.HasPrefix(ref, "/") || ref == "/" {
			continue
		}
		target := strings.TrimPrefix(ref, "/")
		if strings.HasSuffix(target, "/") {
			target += "index.html"
		}
		if _, ok := files[target]; !ok {
			return fmt.Errorf("portalcheck: broken link %s in %s", ref, path)
		}
	}
	return nil
}

func checkMixedVersionAssets(dir string, files map[string]FileDigest) error {
	for path := range files {
		if !strings.HasPrefix(path, "assets/") {
			continue
		}
		base := filepath.Base(path)
		match := shaPrefixName.FindStringSubmatch(base)
		if match == nil {
			return fmt.Errorf("portalcheck: asset is not content-addressed: %s", path)
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("portalcheck: unreadable asset")
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:4]) != match[1] {
			return fmt.Errorf("portalcheck: asset hash prefix mismatch: %s", path)
		}
	}
	return nil
}

func checkSitemap(dir string, files map[string]FileDigest, indexable map[string]bool, sitemapSeen bool, siteURL string) (int, error) {
	robotsRaw, err := os.ReadFile(filepath.Join(dir, "robots.txt"))
	if err != nil {
		return 0, fmt.Errorf("portalcheck: robots.txt unavailable")
	}
	robots := string(robotsRaw)
	if sitemapSeen {
		if !strings.Contains(robots, "Allow: /") || strings.Contains(robots, "Disallow: /") {
			return 0, fmt.Errorf("portalcheck: sitemap with disallow robots")
		}
	} else {
		if !strings.Contains(robots, "Disallow: /") || strings.Contains(robots, "Allow: /") {
			return 0, fmt.Errorf("portalcheck: noindex artifact lacks disallow robots")
		}
		if len(indexable) != 0 {
			return 0, fmt.Errorf("portalcheck: indexable page in noindex artifact")
		}
		if strings.Contains(robots, "Sitemap:") {
			return 0, fmt.Errorf("portalcheck: sitemap declared without artifact")
		}
		return 0, nil
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sitemap.xml"))
	if err != nil {
		return 0, fmt.Errorf("portalcheck: sitemap unavailable")
	}
	var set sitemapSet
	if err := xml.Unmarshal(raw, &set); err != nil || set.XMLNS != "http://www.sitemaps.org/schemas/sitemap/0.9" {
		return 0, fmt.Errorf("portalcheck: invalid sitemap")
	}
	if len(set.URLs) == 0 || len(set.URLs) > MaxSitemapURLs {
		return 0, fmt.Errorf("portalcheck: invalid sitemap URL count")
	}
	origin := ""
	if siteURL != "" {
		parsed, err := url.Parse(strings.TrimRight(siteURL, "/"))
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return 0, fmt.Errorf("portalcheck: invalid expected site URL")
		}
		origin = parsed.Scheme + "://" + parsed.Host
	}
	seen := map[string]bool{}
	mapped := map[string]bool{}
	for _, entry := range set.URLs {
		parsed, err := url.Parse(entry.Loc)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return 0, fmt.Errorf("portalcheck: invalid sitemap URL")
		}
		currentOrigin := parsed.Scheme + "://" + parsed.Host
		if origin == "" {
			origin = currentOrigin
		} else if origin != currentOrigin {
			return 0, fmt.Errorf("portalcheck: mixed sitemap origins")
		}
		if seen[entry.Loc] {
			return 0, fmt.Errorf("portalcheck: duplicate sitemap URL")
		}
		seen[entry.Loc] = true
		if entry.LastMod != "" {
			if _, err := time.Parse("2006-01-02", entry.LastMod); err != nil {
				return 0, fmt.Errorf("portalcheck: invalid sitemap lastmod")
			}
		}
		target := strings.TrimPrefix(parsed.Path, "/")
		if target == "" || strings.HasSuffix(target, "/") {
			target += "index.html"
		}
		if _, ok := files[target]; !ok || !strings.HasSuffix(target, ".html") {
			return 0, fmt.Errorf("portalcheck: sitemap URL has no artifact page")
		}
		if !indexable[target] {
			return 0, fmt.Errorf("portalcheck: sitemap lists a noindex page")
		}
		mapped[target] = true
	}
	if !strings.Contains(robots, "Sitemap: "+origin+"/sitemap.xml") {
		return 0, fmt.Errorf("portalcheck: robots sitemap declaration mismatch")
	}
	for page := range indexable {
		if !mapped[page] {
			return 0, fmt.Errorf("portalcheck: indexable page absent from sitemap")
		}
	}
	return len(set.URLs), nil
}

func checkOfficialPin(pin *officialPin) error {
	if pin == nil {
		return nil
	}
	if pin.SchemaVersion != officialSchema || pin.PublicFieldPolicy != "fq-public-v1" || pin.Source != "sanctions" || pin.Stream != "sanction-v1" ||
		pin.Status != "published" || pin.ProvenanceState != "VERIFIED" ||
		pin.Rows <= 0 || pin.Counts.Accepted != pin.Rows ||
		pin.AssociationCensus.Total != pin.Rows ||
		pin.AssociationCensus.Total != pin.AssociationCensus.Linked+pin.AssociationCensus.Unmatched+pin.AssociationCensus.Ambiguous+pin.AssociationCensus.WrongBranch+pin.AssociationCensus.SuccessionReview ||
		pin.RevisionID == "" || pin.LeaseID == "" || !isHexString(pin.PayloadSHA256) ||
		!isHexString(pin.SourceChecksum) || pin.AssociationPolicy != "current-company-v1" ||
		pin.AssociationScope != "CURRENT_COMPANY_ONLY" || pin.AssociationDecisionDay == "" ||
		pin.SourceURL == "" || pin.SourcePeriod == "" {
		return fmt.Errorf("portalcheck: invalid official pin")
	}
	return nil
}

func checkOfficialPages(dir string, files map[string]FileDigest, pin *officialPin, officialOnly bool) error {
	records := 0
	indexes := 0
	for path := range files {
		if strings.HasPrefix(path, "fiscalizacao/registro/") && strings.HasSuffix(path, "/index.html") {
			records++
		}
		if path == "fiscalizacao/index.html" || (strings.HasPrefix(path, "fiscalizacao/pagina/") && strings.HasSuffix(path, "/index.html")) {
			indexes++
		}
	}
	if pin == nil {
		if records != 0 || indexes != 1 {
			return fmt.Errorf("portalcheck: official pages without pin")
		}
		return nil
	}
	expectedIndexes := (pin.Rows + officialPageSize - 1) / officialPageSize
	if records != pin.Rows || indexes != expectedIndexes {
		return fmt.Errorf("portalcheck: official page census mismatch")
	}
	for path := range files {
		if !strings.HasPrefix(path, "fiscalizacao/registro/") || !strings.HasSuffix(path, "/index.html") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("portalcheck: unreadable official page")
		}
		if !bytes.Contains(body, []byte("não é certidão negativa")) {
			return fmt.Errorf("portalcheck: official page lacks negative-certificate limit")
		}
		linked := bytes.Contains(body, []byte("Vinculado a posto"))
		unlinked := bytes.Contains(body, []byte("Sem vínculo de posto")) || bytes.Contains(body, []byte("Vínculo ambíguo"))
		hasStationHref := bytes.Contains(body, []byte("href=\"/postos/"))
		if officialOnly && hasStationHref {
			return fmt.Errorf("portalcheck: official-only page links a station page")
		}
		switch {
		case linked && unlinked:
			return fmt.Errorf("portalcheck: contradictory official association")
		case linked:
			if !officialOnly && !hasStationHref {
				return fmt.Errorf("portalcheck: linked official page lacks station link")
			}
		case unlinked:
			if hasStationHref {
				return fmt.Errorf("portalcheck: unlinked official page has station link")
			}
		default:
			return fmt.Errorf("portalcheck: official page lacks association state")
		}
	}
	return nil
}

func isHexString(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
