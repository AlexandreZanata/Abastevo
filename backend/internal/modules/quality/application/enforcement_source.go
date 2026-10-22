package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

const EnforcementSourceProfile = "anp-enforcement-xlsx-2019-v1"

// EnforcementSourceContext preserves the workbook calendar and document bounds.
// The source declares no extraction, occurrence or finality clock.
type EnforcementSourceContext struct {
	Profile          string
	SourceFile       string
	DateSystem       string
	FirstDocumentDay string
	LastDocumentDay  string
}

type EnforcementSourceInput struct {
	Context      EnforcementSourceContext
	Resources    []Resource
	Counts       domain.Counts
	ManifestHash string
	Predecessor  string
	Records      []EnforcementSourceRecord
}

func (r EnforcementSourceRecord) ContentHash() string {
	sum := sha256.Sum256([]byte(strings.Join([]string{r.SourceCNPJ, r.UF, r.Municipality, r.Segment, r.DocumentDate, r.DocumentNumber, r.Procedure, r.Result}, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func (in EnforcementSourceInput) Validate() error {
	refuse := errors.New("quality: invalid native enforcement publication")
	c := in.Context
	if c.Profile != EnforcementSourceProfile || len(c.SourceFile) == 0 || len(c.SourceFile) > 255 || c.SourceFile == "." || c.SourceFile == ".." || strings.ContainsAny(c.SourceFile, "/\\") || strings.IndexFunc(c.SourceFile, unicode.IsControl) >= 0 || (c.DateSystem != "EXCEL_1900" && c.DateSystem != "EXCEL_1904") {
		return refuse
	}
	n := in.Counts
	if n.Accepted != len(in.Records) || n.Quarantined != 0 || len(in.Resources) != 1 {
		return refuse
	}
	for _, v := range []int{n.Input, n.Accepted, n.Duplicates, n.Quarantined} {
		if v < 0 || v > 500000 {
			return refuse
		}
	}
	m := domain.Manifest{Source: "enforcement", Stream: domain.StreamEnforcement, Edition: "2019+", Counts: n}
	r := in.Resources[0]
	if r.ValidateMember() != nil || r.Member != "" || r.URL == "" || len(r.URL) > 1024 || r.Bytes == 0 || r.Bytes > 100<<20 || len(r.SHA256) != 64 || !isHex(r.SHA256) || r.SHA256 != strings.ToLower(r.SHA256) {
		return refuse
	}
	m.Resources = []domain.Resource{{URL: r.URL, SHA256: r.SHA256, Bytes: r.Bytes}}
	if m.Reconcile() != nil || len(in.ManifestHash) != 64 || !isHex(in.ManifestHash) || in.ManifestHash != strings.ToLower(in.ManifestHash) {
		return refuse
	}
	if n.Input == 0 {
		if c.FirstDocumentDay != "" || c.LastDocumentDay != "" {
			return refuse
		}
	} else if !domain.ValidDay(c.FirstDocumentDay) || !domain.ValidDay(c.LastDocumentDay) || c.FirstDocumentDay > c.LastDocumentDay {
		return refuse
	}
	text := func(s string, max int) bool {
		return len(s) <= max && utf8.ValidString(s) && !strings.ContainsAny(s, "\x00\x1f")
	}
	seen := map[string]bool{}
	locators := map[int]bool{}
	first, last := "", ""
	for _, r := range in.Records {
		if seen[r.RecordKey] || r.Fingerprint != r.ContentHash() || r.RecordKey != "enforcement-source:"+r.Fingerprint {
			return refuse
		}
		seen[r.RecordKey] = true
		if r.SourceCNPJ != "" {
			id, e := kernel.ParseCNPJ(r.SourceCNPJ)
			if e != nil || id.Normalized() != r.SourceCNPJ || r.SubjectKind != "company-shaped" {
				return refuse
			}
		} else if r.SubjectKind != "personal-shaped" && r.SubjectKind != "other" {
			return refuse
		}
		if len(r.UF) != 2 || r.UF[0] < 'A' || r.UF[0] > 'Z' || r.UF[1] < 'A' || r.UF[1] > 'Z' || !domain.ValidDay(r.DocumentDate) || r.DocumentDate < c.FirstDocumentDay || r.DocumentDate > c.LastDocumentDay {
			return refuse
		}
		if r.Municipality == "" || !text(r.Municipality, 256) || !text(r.Segment, 256) || r.DocumentNumber == "" || !text(r.DocumentNumber, 128) || !text(r.Procedure, 64<<10) || !text(r.Result, 64<<10) {
			return refuse
		}
		locator, ok := strings.CutPrefix(r.EvidenceRef, c.SourceFile+"#row:")
		index, e := strconv.Atoi(locator)
		if !ok || e != nil || strconv.Itoa(index) != locator || index < 2 || index > n.Input+1 || locators[index] {
			return refuse
		}
		locators[index] = true
		if first == "" || r.DocumentDate < first {
			first = r.DocumentDate
		}
		if last == "" || r.DocumentDate > last {
			last = r.DocumentDate
		}
	}
	if first != c.FirstDocumentDay || last != c.LastDocumentDay {
		return refuse
	}
	return nil
}

// EnforcementSourceRecord is one verified XLSX enforcement source record,
// deliberately separate from the CSV EnforcementInput: the workbook carries
// source geography, segment, document identity, procedure and result wording
// that the CSV stream does not state. A document date is the source DATA DO
// DF value, not an occurrence, payment, due or finality date. SourceCNPJ is
// a validated full company identifier and stays empty when the source
// subject is not an admitted company; names, CPF, neighbourhoods and
// addresses are never staged. Distinct results that share a document stay
// distinct records; no record is bound to a current company or station here.
type EnforcementSourceRecord struct {
	RecordKey      string
	SourceCNPJ     string
	SubjectKind    string
	UF             string
	Municipality   string
	Segment        string
	DocumentDate   string
	DocumentNumber string
	Procedure      string
	Result         string
	EvidenceRef    string
	Fingerprint    string
}
