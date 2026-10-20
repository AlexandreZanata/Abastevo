package prepared

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// FineHistoryBatch retains source uncertainty and metadata explicitly. It is
// not a2025 FineBatch, and no legacy publisher accepts this type implicitly.
type FineHistoryBatch struct {
	Observations                                          []application.HistoricalFineObservation
	Notes                                                 []application.FineHistoryNote
	SourcePeriod, InputContract, SourceFile, ManifestHash string
	Clock                                                 application.FineHistoryClock
	HeaderRecord                                          int
	Counts                                                domain.Counts
	MetadataRecords                                       int
}
type fineHistoryManifest struct {
	manifest
	Period   string                       `json:"source_period"`
	Clock    application.FineHistoryClock `json:"source_clock"`
	Header   *int64                       `json:"header_record"`
	Metadata *int64                       `json:"metadata_records"`
}

func historyManifest(raw []byte, identity InputIdentity, period string) (fineHistoryManifest, error) {
	var m fineHistoryManifest
	p, ok := application.HistoricalFineProfile(period)
	if !ok || len(raw) > 1<<20 || !utf8.Valid(raw) || httpapi.DecodeJSON(raw, &m) != nil || m.Schema != "fine-history-prepared-v1" || m.Period != period || m.Contract != p.Contract || m.SourceFile != p.SourceFile || !m.Clock.Valid(period) || m.Header == nil || *m.Header != int64(p.HeaderRecord) || m.Metadata == nil || *m.Metadata < 0 || *m.Metadata > MaxRows {
		return m, refused("historical fine manifest contract")
	}
	if !validHash(identity.SHA256) || identity.Bytes <= 0 || identity.Bytes > MaxBytes || m.InputHash != identity.SHA256 || m.InputBytes != identity.Bytes || !validHash(m.ManifestHash) {
		return m, refused("historical fine input identity")
	}
	c := m.Counts
	for _, n := range []*int64{c.Input, c.Accepted, c.Duplicates, c.Quarantined} {
		if n == nil || *n < 0 || *n > MaxRows {
			return m, refused("historical fine count budget")
		}
	}
	if *c.Input != *c.Accepted+*c.Duplicates+*c.Quarantined || *m.Metadata > *c.Quarantined || ((period != "2020" && period != "2021") && *m.Metadata != 0) || len(m.Outputs) != 4 {
		return m, refused("historical fine loss accounting")
	}
	seen := map[string]bool{}
	var total int64
	for _, o := range m.Outputs {
		if (o.File != "history.jsonl" && o.File != "duplicates.jsonl" && o.File != "quarantine.jsonl" && o.File != "source_notes.jsonl") || seen[o.File] || o.Bytes == nil || *o.Bytes < 0 || *o.Bytes > MaxBytes || !validHash(o.SHA256) {
			return m, refused("historical fine artifact identity")
		}
		seen[o.File] = true
		total += *o.Bytes
	}
	if total > MaxBytes {
		return m, refused("historical fine aggregate budget")
	}
	return m, nil
}

func LoadFineHistory(ctx context.Context, raw []byte, open OpenStream, identity InputIdentity, period string) (FineHistoryBatch, error) {
	if e := ctx.Err(); e != nil {
		return FineHistoryBatch{}, e
	}
	m, e := historyManifest(raw, identity, period)
	if e != nil {
		return FineHistoryBatch{}, e
	}
	if open == nil {
		return FineHistoryBatch{}, refused("historical fine stream port")
	}
	out := FineHistoryBatch{SourcePeriod: period, InputContract: m.Contract, SourceFile: m.SourceFile, ManifestHash: m.ManifestHash, Clock: m.Clock, HeaderRecord: int(*m.Header), MetadataRecords: int(*m.Metadata), Counts: domain.Counts{Input: int(*m.Counts.Input), Accepted: int(*m.Counts.Accepted), Duplicates: int(*m.Counts.Duplicates), Quarantined: int(*m.Counts.Quarantined)}}
	outputs := map[string]output{}
	for _, o := range m.Outputs {
		outputs[o.File] = o
	}
	locators := map[int]bool{}
	number := func(locator string) (int, error) {
		s, ok := strings.CutPrefix(locator, m.SourceFile+":")
		n, e := strconv.Atoi(s)
		if !ok || e != nil || strconv.Itoa(n) != s || n <= int(*m.Header) || n > int(*m.Header)+out.Counts.Input {
			return 0, refused("historical fine source locator")
		}
		return n, nil
	}
	census := func(locator string) error {
		n, e := number(locator)
		if e != nil {
			return e
		}
		if locators[n] {
			return refused("historical fine duplicate locator")
		}
		locators[n] = true
		return nil
	}
	twins := map[string]string{}
	metadata := map[int]bool{}
	noted := map[int]bool{}
	previous := ""
	lastObservationRecord := 0
	logical := sha256.New()
	_, _ = fmt.Fprintf(logical, "[%s,%s,%s,%d,", canonicalFine(m.Contract), canonicalFine(m.Period), canonicalFine(m.Clock), *m.Header)
	expected := []int64{*m.Counts.Accepted, *m.Counts.Duplicates, *m.Counts.Quarantined, *m.Metadata}
	for index, name := range []string{"history.jsonl", "duplicates.jsonl", "quarantine.jsonl", "source_notes.jsonl"} {
		if index > 0 {
			_, _ = logical.Write([]byte(","))
		}
		_, _ = logical.Write([]byte("["))
		first := true
		e = scan(ctx, open, outputs[name], expected[index], func(line []byte) error {
			switch index {
			case 0, 1:
				var a application.HistoricalFineObservation
				if !utf8.Valid(line) || httpapi.DecodeJSON(line, &a) != nil || !a.Valid() || a.SourcePeriod != period || a.Clock != m.Clock || !bytes.Equal(line, canonicalFine(a)) {
					return refused("historical fine canonical observation")
				}
				if e := census(a.EvidenceRef); e != nil {
					return e
				}
				n, _ := number(a.EvidenceRef)
				lastObservationRecord = max(lastObservationRecord, n)
				content := a
				content.EvidenceRef = ""
				key := string(canonicalFine(content))
				if index == 0 {
					if a.ObservationKey <= previous {
						return refused("historical fine duplicate or order")
					}
					previous = a.ObservationKey
					twins[a.ObservationKey] = key
					out.Observations = append(out.Observations, a)
				} else if twins[a.ObservationKey] != key {
					return refused("historical fine duplicate without twin")
				}
			case 2:
				var q quarantined
				if !utf8.Valid(line) || httpapi.DecodeJSON(line, &q) != nil || q.Schema != "station-quarantine-v1" || q.Source != "sanctions" || q.SourceKey != nil || q.Detail != "historical fine row classified; raw fields redacted" || (q.Reason != "invalid_cnpj" && q.Reason != "invalid_field" && q.Reason != "record_too_large" && q.Reason != "source_metadata") || !bytes.Equal(line, canonicalFine(q)) {
					return refused("historical fine redacted quarantine")
				}
				if e := census(q.Locator); e != nil {
					return e
				}
				if q.Reason == "source_metadata" {
					n, _ := number(q.Locator)
					metadata[n] = true
				}
			case 3:
				var note application.FineHistoryNote
				if !utf8.Valid(line) || httpapi.DecodeJSON(line, &note) != nil || !note.Valid(period) || !bytes.Equal(line, canonicalFine(note)) {
					return refused("historical fine safe source note")
				}
				n, e := number(note.EvidenceRef)
				if e != nil || !metadata[n] || noted[n] {
					return refused("historical fine metadata census")
				}
				noted[n] = true
				out.Notes = append(out.Notes, note)
			}
			if !first {
				_, _ = logical.Write([]byte(","))
			}
			first = false
			_, _ = logical.Write(line)
			return nil
		})
		if e != nil {
			return FineHistoryBatch{}, e
		}
		_, _ = logical.Write([]byte("]"))
	}
	_, _ = fmt.Fprintf(logical, ",%d,%d,%d,%d]", out.Counts.Input, out.Counts.Accepted, out.Counts.Duplicates, out.Counts.Quarantined)
	for n := range metadata {
		if n <= lastObservationRecord {
			return FineHistoryBatch{}, refused("historical fine source footer order")
		}
	}
	if len(locators) != out.Counts.Input || len(metadata) != out.MetadataRecords || len(noted) != len(metadata) || hex.EncodeToString(logical.Sum(nil)) != out.ManifestHash {
		return FineHistoryBatch{}, refused("historical fine manifest or census mismatch")
	}
	if e = ctx.Err(); e != nil {
		return FineHistoryBatch{}, e
	}
	return out, nil
}
