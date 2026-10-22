package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// EnforcementMaxOutput bounds the minimized-record stream of one full
// source revision. The pinned revision emits ~128 MB of records; the
// input workbook itself stays under MaxBytes.
const EnforcementMaxOutput = 200 << 20

const (
	enforcementManifestSchema = "enforcement-native-manifest-v1"
	enforcementSourceProfile  = "anp-enforcement-xlsx-2019-v1"
	enforcementRecordSchema   = "enforcement-source-record-v1"
	enforcementRecordSource   = "anp-enforcement-xlsx"
	enforcementRecordsFile    = "enforcement-source-records.jsonl"
)

// EnforcementBatch is the verified typed boundary of one native
// enforcement revision. Records stay source-global: no station, company
// or site binding is assigned here. No resolution/publication occurs
// until a later stage consumes this batch.
type EnforcementBatch struct {
	Records       []application.EnforcementSourceRecord
	Counts        domain.Counts
	ManifestHash  string
	SourceFile    string
	SourceProfile string
	Context       application.EnforcementSourceContext
}
type enforcementCensus struct {
	Rows            *int64           `json:"rows"`
	DistinctDocs    *int64           `json:"distinct_documents"`
	DistinctTuples  *int64           `json:"distinct_subject_date_document_procedure"`
	ResultVocab     *int64           `json:"result_vocabulary_size"`
	ResultMaxChars  *int64           `json:"result_max_chars"`
	ResultsOver64   *int64           `json:"results_over_64_chars"`
	Segments        map[string]int64 `json:"segments"`
	SubjectShapes   map[string]int64 `json:"subject_shapes"`
	Admitted        *int64           `json:"subjects_admitted"`
	NonnumericDates *int64           `json:"nonnumeric_dates"`
	DateSystem      string           `json:"date_system"`
	DatePrecision   string           `json:"date_precision"`
	FirstDate       *string          `json:"first_date"`
	LastDate        *string          `json:"last_date"`
	Members         *int64           `json:"members"`
	ExpandedBytes   *int64           `json:"expanded_bytes"`
}
type enforcementManifest struct {
	Schema       string            `json:"schema_version"`
	Profile      string            `json:"source_profile"`
	SourceFile   string            `json:"source_file"`
	InputBytes   int64             `json:"input_bytes"`
	InputHash    string            `json:"input_sha256"`
	ManifestHash string            `json:"manifest_hash"`
	Counts       counts            `json:"counts"`
	Census       enforcementCensus `json:"census"`
	Outputs      []output          `json:"outputs"`
}

// Field order documents the Rust minimized-content identity; Go
// recomputes the record hash from parsed values, never from raw JSON.
type enforcementRecord struct {
	Schema    string  `json:"schema_version"`
	Source    string  `json:"source"`
	Locator   string  `json:"row_locator"`
	Hash      string  `json:"record_hash"`
	Tuple     string  `json:"tuple_hash"`
	Subject   *string `json:"subject_cnpj"`
	Kind      string  `json:"subject_kind"`
	UF        string  `json:"uf"`
	City      string  `json:"municipality"`
	Segment   string  `json:"segment"`
	Date      string  `json:"document_date"`
	Document  string  `json:"document_number"`
	Procedure string  `json:"procedure"`
	Result    string  `json:"result"`
	Repeat    bool    `json:"exact_repeat"`
}

func enforcementText(s string, max int) bool {
	return len(s) <= max && utf8.ValidString(s) &&
		strings.IndexFunc(s, func(r rune) bool { return r == 0 || r == 0x1f }) < 0
}
func enforcementHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func parseEnforcementManifest(raw []byte, identity InputIdentity) (enforcementManifest, error) {
	var m enforcementManifest
	if len(raw) > 1<<20 || !utf8.Valid(raw) || httpapi.DecodeJSON(raw, &m) != nil ||
		m.Schema != enforcementManifestSchema || m.Profile != enforcementSourceProfile {
		return m, refused("enforcement manifest contract")
	}
	if len(m.SourceFile) == 0 || len(m.SourceFile) > 255 || m.SourceFile == "." || m.SourceFile == ".." ||
		strings.ContainsAny(m.SourceFile, "/\\") || strings.IndexFunc(m.SourceFile, unicode.IsControl) >= 0 {
		return m, refused("enforcement source name")
	}
	if !validHash(identity.SHA256) || identity.Bytes <= 0 || identity.Bytes > MaxBytes ||
		m.InputHash != identity.SHA256 || m.InputBytes != identity.Bytes || !validHash(m.ManifestHash) {
		return m, refused("enforcement input identity")
	}
	c := m.Counts
	for _, n := range []*int64{c.Input, c.Accepted, c.Duplicates, c.Quarantined} {
		if n == nil || *n < 0 || *n > MaxRows {
			return m, refused("enforcement count budget")
		}
	}
	if *c.Input != *c.Accepted+*c.Duplicates+*c.Quarantined {
		return m, refused("enforcement loss accounting")
	}
	if len(m.Outputs) != 1 {
		return m, refused("enforcement output set")
	}
	o := m.Outputs[0]
	if o.File != enforcementRecordsFile || o.Bytes == nil || *o.Bytes < 0 || *o.Bytes > EnforcementMaxOutput || !validHash(o.SHA256) {
		return m, refused("enforcement artifact identity")
	}
	s := m.Census
	for _, n := range []*int64{s.Rows, s.DistinctDocs, s.DistinctTuples, s.ResultVocab, s.ResultMaxChars, s.ResultsOver64, s.Admitted, s.NonnumericDates, s.Members} {
		if n == nil || *n < 0 || *n > MaxRows {
			return m, refused("enforcement census budget")
		}
	}
	if s.ExpandedBytes == nil || *s.ExpandedBytes <= 0 || *s.ExpandedBytes > EnforcementMaxOutput {
		return m, refused("enforcement census expansion")
	}
	if s.Segments == nil || s.SubjectShapes == nil {
		return m, refused("enforcement census shape")
	}
	if *s.Rows != *c.Input || *s.DistinctDocs > *s.Rows || *s.DistinctTuples > *s.Rows ||
		*s.ResultVocab > *s.Rows || *s.ResultsOver64 > *s.Rows || *s.Admitted > *s.Rows ||
		*s.NonnumericDates > *c.Quarantined || *s.Members <= 0 || *s.ExpandedBytes <= 0 {
		return m, refused("enforcement census accounting")
	}
	if s.DateSystem != "EXCEL_1900" && s.DateSystem != "EXCEL_1904" || s.DatePrecision != "DAY" {
		return m, refused("enforcement census clock")
	}
	if *c.Input > 0 && (s.FirstDate == nil || s.LastDate == nil || !domain.ValidDay(*s.FirstDate) || !domain.ValidDay(*s.LastDate) || *s.FirstDate > *s.LastDate) {
		return m, refused("enforcement census dates")
	}
	return m, nil
}

func validateEnforcementRecord(line []byte, m enforcementManifest) (enforcementRecord, error) {
	var r enforcementRecord
	if !utf8.Valid(line) || httpapi.DecodeJSON(line, &r) != nil ||
		r.Schema != enforcementRecordSchema || r.Source != enforcementRecordSource {
		return r, refused("enforcement record schema")
	}
	if r.Subject != nil {
		id, e := kernel.ParseCNPJ(*r.Subject)
		if e != nil || id.Normalized() != *r.Subject || r.Kind != "company-shaped" {
			return r, refused("enforcement subject identity")
		}
	} else if r.Kind != "personal-shaped" && r.Kind != "other" {
		return r, refused("enforcement subject kind")
	}
	if len(r.UF) != 2 || r.UF[0] < 'A' || r.UF[0] > 'Z' || r.UF[1] < 'A' || r.UF[1] > 'Z' {
		return r, refused("enforcement geography")
	}
	if !enforcementText(r.City, 256) || r.City == "" || !enforcementText(r.Segment, 256) ||
		!enforcementText(r.Document, 128) || r.Document == "" || !enforcementText(r.Procedure, 64<<10) ||
		!enforcementText(r.Result, 64<<10) || !domain.ValidDay(r.Date) {
		return r, refused("enforcement retained field")
	}
	if _, ok := strings.CutPrefix(r.Locator, m.SourceFile+"#row:"); !ok {
		return r, refused("enforcement source locator")
	}
	if !validHash(r.Hash) || !validTupleHash(r.Tuple) {
		return r, refused("enforcement content identity")
	}
	subject := ""
	if r.Subject != nil {
		subject = *r.Subject
	}
	if r.Hash != enforcementHash(subject, r.UF, r.City, r.Segment, r.Date, r.Document, r.Procedure, r.Result) {
		return r, refused("enforcement content hash")
	}
	return r, nil
}

func validTupleHash(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// LoadEnforcement verifies trusted native enforcement artifacts and returns
// the typed batch. Hashes prove content identity, not origin: callers must
// also pin the native preparer and the original source. A late tamper,
// truncation, cancellation, close, oversize or unknown profile returns no
// partial batch.
func LoadEnforcement(ctx context.Context, raw []byte, open OpenStream, identity InputIdentity) (EnforcementBatch, error) {
	if e := ctx.Err(); e != nil {
		return EnforcementBatch{}, e
	}
	m, e := parseEnforcementManifest(raw, identity)
	if e != nil {
		return EnforcementBatch{}, e
	}
	if open == nil {
		return EnforcementBatch{}, refused("enforcement stream port")
	}
	out := EnforcementBatch{
		SourceFile: m.SourceFile, SourceProfile: m.Profile, ManifestHash: m.ManifestHash,
		Context: application.EnforcementSourceContext{Profile: m.Profile, SourceFile: m.SourceFile, DateSystem: m.Census.DateSystem},
		Counts:  domain.Counts{Input: int(*m.Counts.Input), Accepted: int(*m.Counts.Accepted), Duplicates: int(*m.Counts.Duplicates), Quarantined: int(*m.Counts.Quarantined)},
	}
	if m.Census.FirstDate != nil {
		out.Context.FirstDocumentDay = *m.Census.FirstDate
	}
	if m.Census.LastDate != nil {
		out.Context.LastDocumentDay = *m.Census.LastDate
	}
	outputs := map[string]output{}
	for _, o := range m.Outputs {
		outputs[o.File] = o
	}
	seen := map[string]bool{}
	twins := map[string]string{}
	locators := map[int]bool{}
	documents := map[string]bool{}
	tuples := map[string]bool{}
	results := map[string]bool{}
	segments := map[string]int64{}
	shapes := map[string]int64{}
	var admitted, maxChars, over64 int64
	var first, last string
	out.Records = make([]application.EnforcementSourceRecord, 0, int(*m.Counts.Accepted))
	e = scan(ctx, open, outputs[enforcementRecordsFile], *m.Counts.Input, func(line []byte) error {
		r, err := validateEnforcementRecord(line, m)
		if err != nil {
			return err
		}
		prefix := m.SourceFile + "#row:"
		text, ok := strings.CutPrefix(r.Locator, prefix)
		number, err := strconv.Atoi(text)
		if !ok || err != nil || strconv.Itoa(number) != text ||
			number < 2 || number > 1+int(*m.Counts.Input) || locators[number] {
			return refused("enforcement locator census")
		}
		locators[number] = true
		content := string(canonicalRecord(r))
		if r.Repeat {
			if twins[r.Hash] != content {
				return refused("enforcement repeat without twin")
			}
		} else {
			if seen[r.Hash] {
				return refused("enforcement duplicate content")
			}
			seen[r.Hash] = true
			twins[r.Hash] = content
		}
		documents[r.Document] = true
		tuples[r.Tuple] = true
		results[r.Result] = true
		if n := int64(len([]rune(r.Result))); n > maxChars {
			maxChars = n
		}
		if len([]rune(r.Result)) > 64 {
			over64++
		}
		segments[r.Segment]++
		shapes[r.Kind]++
		if r.Subject != nil {
			admitted++
		}
		if first == "" || r.Date < first {
			first = r.Date
		}
		if last == "" || r.Date > last {
			last = r.Date
		}
		if !r.Repeat {
			subject := ""
			if r.Subject != nil {
				subject = *r.Subject
			}
			out.Records = append(out.Records, application.EnforcementSourceRecord{
				RecordKey: "enforcement-source:" + r.Hash, SourceCNPJ: subject, SubjectKind: r.Kind,
				UF: r.UF, Municipality: r.City, Segment: r.Segment, DocumentDate: r.Date,
				DocumentNumber: r.Document, Procedure: r.Procedure, Result: r.Result,
				EvidenceRef: r.Locator, Fingerprint: r.Hash,
			})
		}
		return nil
	})
	if e != nil {
		return EnforcementBatch{}, e
	}
	s := m.Census
	if len(out.Records) != out.Counts.Accepted || out.Counts.Input-len(out.Records) != out.Counts.Duplicates || out.Counts.Quarantined != 0 || int64(len(locators)) != *m.Counts.Input || int64(len(documents)) != *s.DistinctDocs ||
		int64(len(tuples)) != *s.DistinctTuples || int64(len(results)) != *s.ResultVocab ||
		maxChars != *s.ResultMaxChars || over64 != *s.ResultsOver64 || admitted != *s.Admitted ||
		!equalCounts(segments, s.Segments) || !equalCounts(shapes, s.SubjectShapes) {
		return EnforcementBatch{}, refused("enforcement census mismatch")
	}
	if *m.Counts.Input > 0 && (first != *s.FirstDate || last != *s.LastDate) {
		return EnforcementBatch{}, refused("enforcement census dates")
	}
	if e = ctx.Err(); e != nil {
		return EnforcementBatch{}, e
	}
	return out, nil
}

func canonicalRecord(r enforcementRecord) []byte {
	subject := ""
	if r.Subject != nil {
		subject = *r.Subject
	}
	return []byte(strings.Join([]string{
		subject, r.UF, r.City, r.Segment, r.Date, r.Document, r.Procedure, r.Result,
	}, "\x1f"))
}

func equalCounts(a map[string]int64, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
