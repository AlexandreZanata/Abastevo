package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Enforcement case correlation (MEX-22, FQ-07). A source document groups
// the observations the source states under it; nothing else links records.
// Distinct results that share a document stay distinct observations in one
// case — they are never merged, averaged or deduplicated. Where issued-case
// identity is absent, callers count labelled source records instead of
// inventing cases.

// CaseBasisSameDocument is the only documented enforcement case link: the
// source states these observations under one document number. No name,
// company, address, procedure or proximity inference ever forms a case.
const CaseBasisSameDocument = "same-document"

// DateBasisDocumentDay declares that a date filter uses the source document
// day (DATA DO DF). DateBasisUnknown declares that no event date is
// available; such observations are counted but never dated. The source
// states no extraction, occurrence or finality clock for these records.
const (
	DateBasisDocumentDay = "document_day"
	DateBasisUnknown     = "UNKNOWN"
)

// EnforcementObservation is the minimal correlation view of one source
// record: its stable content identity, its source-stated document and its
// document day (empty when the source states none).
type EnforcementObservation struct {
	RecordKey      string
	DocumentNumber string
	DocumentDay    string
}

// EnforcementCase groups the observations one source document states.
// CaseKey is stable across revisions as long as the document number is
// stable; a corrected result changes the observation identity but not the
// case. Observations are retained in first-seen order, all of them.
type EnforcementCase struct {
	CaseKey        string
	DocumentNumber string
	Basis          string
	Observations   []string
}

// ObservationCounts keeps issued observations and cases explicit and
// separate. Cases is the number of distinct documents; Observations is the
// number of distinct retained rows. A repeated snapshot must not raise
// either count.
type ObservationCounts struct {
	Observations int
	Cases        int
}

// StableCaseKey derives the cross-revision case identity from the source
// profile and the source-stated document number. An empty document number
// has no case identity and must be counted as an observation only.
func StableCaseKey(profile, document string) (string, bool) {
	if strings.TrimSpace(profile) == "" || strings.TrimSpace(document) == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(profile + "\x1f" + document))
	return "enforcement-case:" + hex.EncodeToString(sum[:]), true
}

// DateBasis resolves the clock basis of one observation for filters. A
// valid document day uses the document basis; anything else is UNKNOWN,
// never a guessed or defaulted date.
func DateBasis(documentDay string) string {
	if ValidDay(documentDay) {
		return DateBasisDocumentDay
	}
	return DateBasisUnknown
}

// GroupEnforcementRecords groups observations by their source-stated
// document number with loss accounting: every input observation lands in
// exactly one case, and every distinct observation is retained. An empty
// document number forms no case; such observations are counted but not
// grouped. Repeated observations (same record key) do not raise any count.
// The output order is deterministic: cases in first-seen document order,
// observations in first-seen record order.
func GroupEnforcementRecords(profile string, observations []EnforcementObservation) ([]EnforcementCase, ObservationCounts) {
	cases := []EnforcementCase{}
	byDocument := map[string]int{}
	seen := map[string]bool{}
	counts := ObservationCounts{}
	for _, observation := range observations {
		if observation.RecordKey == "" || seen[observation.RecordKey] {
			continue
		}
		seen[observation.RecordKey] = true
		counts.Observations++
		if observation.DocumentNumber == "" {
			continue
		}
		if index, ok := byDocument[observation.DocumentNumber]; ok {
			cases[index].Observations = append(cases[index].Observations, observation.RecordKey)
			continue
		}
		key, ok := StableCaseKey(profile, observation.DocumentNumber)
		if !ok {
			continue
		}
		byDocument[observation.DocumentNumber] = len(cases)
		cases = append(cases, EnforcementCase{
			CaseKey:        key,
			DocumentNumber: observation.DocumentNumber,
			Basis:          CaseBasisSameDocument,
			Observations:   []string{observation.RecordKey},
		})
	}
	counts.Cases = len(cases)
	return cases, counts
}
