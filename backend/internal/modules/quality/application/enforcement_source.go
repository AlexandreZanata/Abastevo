package application

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
