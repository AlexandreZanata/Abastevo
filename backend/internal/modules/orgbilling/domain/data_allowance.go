package domain

// DataAllowance comes from one current scoped subscription and its reviewed
// catalog. Operator capability flags do not establish a metering term.
type DataAllowance struct {
	Plan  PlanKey
	Term  Period
	Calls int64
}
