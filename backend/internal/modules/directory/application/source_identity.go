package application

// SourceIdentity contains only canonical identity evidence. It deliberately
// excludes operational timestamps, names, addresses and location coordinates.
type SourceIdentity struct {
	StationID              string
	Current, StationActive bool
}
type SourceBinding struct{ CNPJ, StationID string }
