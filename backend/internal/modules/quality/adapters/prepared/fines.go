package prepared

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// FineBatch cannot be sent to the legacy SanctionInput publisher by accident.
// No resolution/publication occurs until every stream and locator is verified.
type FineBatch struct {
	Observations    []application.FineObservation
	Counts          domain.Counts
	ManifestHash    string
	ReportYear      int
	ExtractionMonth string
}
type fineManifest struct {
	manifest
	Year  *int   `json:"report_year"`
	Month string `json:"extraction_month"`
}

// Field order is the frozen Rust serialization order used by observation hashes.
type fineObservation struct {
	Schema     string `json:"schema_version"`
	Source     string `json:"source"`
	Key        string `json:"observation_key"`
	SourceKey  string `json:"source_key"`
	Proceeding string `json:"proceeding_ref"`
	DUF        string `json:"duf_ref"`
	Department string `json:"department"`
	Status     string `json:"status"`
	Due        string `json:"due_on"`
	AmountText string `json:"amount_text"`
	Amount     *int64 `json:"amount_minor"`
	PaidText   string `json:"paid_text"`
	Paid       *int64 `json:"paid_minor"`
	Year       *int   `json:"report_year"`
	Month      string `json:"extraction_month"`
	Evidence   string `json:"evidence_ref"`
}

func canonicalFine(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // Only the internal finite DTOs below reach this encoder.
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}
func fineText(s string) bool {
	return len(s) > 0 && len(s) <= 64<<10 && utf8.ValidString(s) && strings.TrimSpace(s) != "" &&
		strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) || r == '\u2028' || r == '\u2029' }) < 0
}

var fineMoneyShape = regexp.MustCompile(`^(0|[1-9][0-9]*|[1-9][0-9]{0,2}(\.[0-9]{3})+),[0-9]{2}$`)

func fineMoney(s string) (int64, bool) {
	if !fineMoneyShape.MatchString(s) {
		return 0, false
	}
	value, e := strconv.ParseInt(strings.NewReplacer(".", "", ",", "").Replace(s), 10, 64)
	return value, e == nil && value >= 0
}
func validFineMonth(s string, year int) bool {
	m, e := time.Parse("2006-01", s)
	return e == nil && m.Format("2006-01") == s && m.Year() >= year
}
func parseFineManifest(raw []byte, identity InputIdentity, year int) (fineManifest, error) {
	var m fineManifest
	if len(raw) > 1<<20 || !utf8.Valid(raw) || httpapi.DecodeJSON(raw, &m) != nil || m.Schema != "sanctions-prepared-v1" || m.Contract != "anp-sicom-cp850-9-v1" || m.Year == nil || *m.Year != year || year < 2016 || year > 9999 || !validFineMonth(m.Month, year) {
		return m, refused("fine manifest contract")
	}
	if len(m.SourceFile) == 0 || len(m.SourceFile) > 128 || m.SourceFile == "." || m.SourceFile == ".." || strings.IndexFunc(m.SourceFile, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r))
	}) >= 0 {
		return m, refused("fine source name")
	}
	if !validHash(identity.SHA256) || identity.Bytes <= 0 || identity.Bytes > MaxBytes || m.InputHash != identity.SHA256 || m.InputBytes != identity.Bytes || !validHash(m.ManifestHash) {
		return m, refused("fine input identity")
	}
	c := m.Counts
	for _, n := range []*int64{c.Input, c.Accepted, c.Duplicates, c.Quarantined} {
		if n == nil || *n < 0 || *n > MaxRows {
			return m, refused("fine count budget")
		}
	}
	if *c.Input != *c.Accepted+*c.Duplicates+*c.Quarantined || len(m.Outputs) != 3 {
		return m, refused("fine loss accounting")
	}
	seen := map[string]bool{}
	var total int64
	for _, o := range m.Outputs {
		if (o.File != "sanctions.jsonl" && o.File != "duplicates.jsonl" && o.File != "quarantine.jsonl") || seen[o.File] || o.Bytes == nil || *o.Bytes < 0 || *o.Bytes > MaxBytes || !validHash(o.SHA256) {
			return m, refused("fine artifact identity")
		}
		total += *o.Bytes
		seen[o.File] = true
	}
	if total > MaxBytes {
		return m, refused("fine aggregate budget")
	}
	return m, nil
}
func validateFine(line []byte, m fineManifest) (fineObservation, error) {
	var a fineObservation
	if !utf8.Valid(line) || httpapi.DecodeJSON(line, &a) != nil || a.Schema != "anp-fine-observation-v1" || a.Source != "sanctions" || a.Year == nil || *a.Year != *m.Year || a.Month != m.Month || a.Amount == nil || a.Paid == nil {
		return a, refused("fine observation schema")
	}
	id, e := kernel.ParseCNPJ(a.SourceKey)
	if e != nil || id.Normalized() != a.SourceKey {
		return a, refused("fine company identity")
	}
	for _, s := range []string{a.Proceeding, a.DUF, a.Department, a.Status, a.AmountText, a.PaidText, a.Evidence} {
		if !fineText(s) {
			return a, refused("fine retained field")
		}
	}
	amount, ok := fineMoney(a.AmountText)
	if !ok || amount != *a.Amount {
		return a, refused("fine amount precision")
	}
	paid, ok := fineMoney(a.PaidText)
	if !ok || paid != *a.Paid {
		return a, refused("fine paid precision")
	}
	if !domain.ValidDay(a.Due) {
		return a, refused("fine due calendar")
	}
	base := a
	base.Key = ""
	base.Evidence = ""
	sum := sha256.Sum256(canonicalFine(base))
	if a.Key != "observation:"+hex.EncodeToString(sum[:]) || !bytes.Equal(line, canonicalFine(a)) {
		return a, refused("fine canonical identity")
	}
	return a, nil
}

// LoadFines verifies trusted native artifacts. Hashes prove content identity,
// not origin: callers must also pin the native preparer and original source.
func LoadFines(ctx context.Context, raw []byte, open OpenStream, identity InputIdentity, year int) (FineBatch, error) {
	if e := ctx.Err(); e != nil {
		return FineBatch{}, e
	}
	m, e := parseFineManifest(raw, identity, year)
	if e != nil {
		return FineBatch{}, e
	}
	if open == nil {
		return FineBatch{}, refused("fine stream port")
	}
	out := FineBatch{ReportYear: year, ExtractionMonth: m.Month, ManifestHash: m.ManifestHash,
		Counts: domain.Counts{Input: int(*m.Counts.Input), Accepted: int(*m.Counts.Accepted), Duplicates: int(*m.Counts.Duplicates), Quarantined: int(*m.Counts.Quarantined)}}
	outputs := map[string]output{}
	for _, o := range m.Outputs {
		outputs[o.File] = o
	}
	locators := make(map[int]bool)
	locator := func(raw string) error {
		text, ok := strings.CutPrefix(raw, m.SourceFile+":")
		if !ok {
			return refused("fine source locator")
		}
		n, e := strconv.Atoi(text)
		if e != nil || strconv.Itoa(n) != text || n < 8 || n > 7+int(*m.Counts.Input) || locators[n] {
			return refused("fine locator census")
		}
		locators[n] = true
		return nil
	}
	logical := sha256.New()
	_, _ = fmt.Fprintf(logical, "[%d,%s,", year, canonicalFine(m.Month))
	twins := map[string]string{}
	previous := ""
	for index, name := range []string{"sanctions.jsonl", "duplicates.jsonl", "quarantine.jsonl"} {
		if index > 0 {
			_, _ = logical.Write([]byte(","))
		}
		_, _ = logical.Write([]byte("["))
		first := true
		expected := []*int64{m.Counts.Accepted, m.Counts.Duplicates, m.Counts.Quarantined}[index]
		e = scan(ctx, open, outputs[name], *expected, func(line []byte) error {
			if index < 2 {
				a, err := validateFine(line, m)
				if err != nil {
					return err
				}
				if err = locator(a.Evidence); err != nil {
					return err
				}
				content := a
				content.Evidence = ""
				key := string(canonicalFine(content))
				if index == 0 {
					if a.Key <= previous {
						return refused("fine duplicate/order")
					}
					previous = a.Key
					twins[a.Key] = key
					out.Observations = append(out.Observations, application.FineObservation{ObservationKey: a.Key, SourceCNPJ: a.SourceKey, ProceedingRef: a.Proceeding, DUFRef: a.DUF, Department: a.Department, Status: a.Status, DueOn: a.Due, AmountText: a.AmountText, AmountMinor: *a.Amount, PaidText: a.PaidText, PaidMinor: *a.Paid, ReportYear: year, ExtractionMonth: a.Month, EvidenceRef: a.Evidence})
				} else if twins[a.Key] != key {
					return refused("fine duplicate without twin")
				}
			} else {
				var q quarantined
				if !utf8.Valid(line) || httpapi.DecodeJSON(line, &q) != nil || q.Schema != "station-quarantine-v1" || q.Source != "sanctions" || q.SourceKey != nil || q.Detail != "official fine row refused; raw fields redacted" || (q.Reason != "invalid_cnpj" && q.Reason != "invalid_field" && q.Reason != "record_too_large") || !bytes.Equal(line, canonicalFine(q)) {
					return refused("fine redacted quarantine")
				}
				if err := locator(q.Locator); err != nil {
					return err
				}
			}
			if !first {
				_, _ = logical.Write([]byte(","))
			}
			first = false
			_, _ = logical.Write(line)
			return nil
		})
		if e != nil {
			return FineBatch{}, e
		}
		_, _ = logical.Write([]byte("]"))
	}
	_, _ = fmt.Fprintf(logical, ",%d,%d,%d,%d]", out.Counts.Input, out.Counts.Accepted, out.Counts.Duplicates, out.Counts.Quarantined)
	if hex.EncodeToString(logical.Sum(nil)) != m.ManifestHash || len(locators) != out.Counts.Input {
		return FineBatch{}, refused("fine manifest/census mismatch")
	}
	if e = ctx.Err(); e != nil {
		return FineBatch{}, e
	}
	return out, nil
}
