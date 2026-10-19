package application

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
