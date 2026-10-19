package application

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// FineObservation is an official SICOM observation, deliberately separate from
// synthetic SanctionInput: a GRM due day is not a decision day, extraction month
// has no invented exact day, and content identity is not a source-issued fine ID.
// SourceCNPJ is a validated full company identifier; names/CPF are never staged.
type FineObservation struct {
	ObservationKey  string
	SourceCNPJ      string
	ProceedingRef   string
	DUFRef          string
	Department      string
	Status          string
	DueOn           string
	AmountText      string
	AmountMinor     int64
	PaidText        string
	PaidMinor       int64
	ReportYear      int
	ExtractionMonth string
	EvidenceRef     string
}

// ObservationIdentity reproduces the native minimized-content identity. It is
// not a source-issued fine number. Locator and derived key slots remain empty.
func (r FineObservation) ObservationIdentity() string {
	dto := struct {
		Schema     string `json:"schema_version"`
		Source     string `json:"source"`
		Key        string `json:"observation_key"`
		Company    string `json:"source_key"`
		Proceeding string `json:"proceeding_ref"`
		DUF        string `json:"duf_ref"`
		Department string `json:"department"`
		Status     string `json:"status"`
		Due        string `json:"due_on"`
		AmountText string `json:"amount_text"`
		Amount     int64  `json:"amount_minor"`
		PaidText   string `json:"paid_text"`
		Paid       int64  `json:"paid_minor"`
		Year       int    `json:"report_year"`
		Month      string `json:"extraction_month"`
		Evidence   string `json:"evidence_ref"`
	}{"anp-fine-observation-v1", "sanctions", "", r.SourceCNPJ, r.ProceedingRef, r.DUFRef, r.Department, r.Status, r.DueOn, r.AmountText, r.AmountMinor, r.PaidText, r.PaidMinor, r.ReportYear, r.ExtractionMonth, ""}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(dto)
	sum := sha256.Sum256(bytes.TrimSuffix(b.Bytes(), []byte("\n")))
	return "observation:" + hex.EncodeToString(sum[:])
}
