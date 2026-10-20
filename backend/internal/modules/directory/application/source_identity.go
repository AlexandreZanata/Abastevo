package application

// SourceIdentity contains only canonical identity evidence. It deliberately
// excludes operational timestamps, names, addresses and location coordinates.
type SourceIdentity struct {
	StationID              string
	Current, StationActive bool
}
type SourceBinding struct{ CNPJ, StationID string }

// SourceEvidence is the bounded batch form of SourceIdentity: one canonical
// station candidate for one source CNPJ. The same two-candidate policy as the
// single lookup applies, so ambiguity is preserved without unbounded history.
type SourceEvidence struct {
	CNPJ                   string
	StationID              string
	Current, StationActive bool
}
