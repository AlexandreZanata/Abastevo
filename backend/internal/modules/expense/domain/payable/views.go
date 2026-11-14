package payable

import (
	"math/big"

	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// Views holds the cost view and the separate settlement view (BR-FIN-001).
// No figure combines them.
type Views struct {
	redacted
	cost                                        expense.CostTotals
	payables, open, partial, settled, cancelled int
	obligation, settledSum, remaining           big.Int
}

// ComputeViews computes the cost view with the unchanged EMP-EXP-001 totals
// over the costs alone, returning their failures unchanged, then the
// settlement view over the payables in input order (INVALID, SCOPE,
// DUPLICATE_ENTRY, DUPLICATE_SOURCE) and last refuses a cost entry whose
// expense identity or origin source is a payable, settlement or statement
// identity (NOT_COST_ORIGIN). Sums are widened exact integers.
func ComputeViews(scope kernel.OrgScope, period expense.ReportingPeriod, basis expense.Basis, costs []expense.Entry,
	payables []Payable) (Views, error) {
	if scope.Validate() != nil {
		return Views{}, ErrScope
	}
	cost, e := expense.Totals(scope, period, basis, costs)
	if e != nil {
		return Views{}, e
	}
	v := Views{cost: cost}
	entries, posing := map[string]bool{}, map[string]bool{}
	liveIDs, liveKeys := map[string]bool{}, map[expense.OriginKey]bool{}
	for _, p := range payables {
		if !p.valid {
			return Views{}, ErrInvalid
		}
		if p.ref.org != scope.OrgID() {
			return Views{}, ErrScope
		}
		identities := []string{p.ref.id}
		for _, s := range p.settlements {
			identities = append(identities, s.ref.id)
		}
		for _, id := range identities {
			if entries[id] {
				return Views{}, ErrDuplicateEntry
			}
			entries[id], posing[id] = true, true
		}
		if p.statement != nil {
			posing[p.statement.id] = true
		}
		if !p.cancelled {
			for _, l := range p.lines {
				if liveIDs[l.expense.ID()] || liveKeys[l.key] {
					return Views{}, ErrDuplicateSource
				}
				liveIDs[l.expense.ID()], liveKeys[l.key] = true, true
			}
			v.obligation.Add(&v.obligation, big.NewInt(p.obligation))
		}
		v.payables++
		switch p.State() {
		case Open:
			v.open++
		case PartiallySettled:
			v.partial++
		case Settled:
			v.settled++
		default:
			v.cancelled++
		}
		v.settledSum.Add(&v.settledSum, big.NewInt(p.settled))
		v.remaining.Add(&v.remaining, big.NewInt(p.RemainingCentavos()))
	}
	for _, c := range costs {
		if posing[c.Expense.Reference().ID()] || posing[sourceID(c.Expense)] {
			return Views{}, ErrSecondCost
		}
	}
	return v, nil
}

// Cost is the cost view: exactly the EMP-EXP-001 totals of the costs alone.
func (v Views) Cost() expense.CostTotals { return v.cost }
func (v Views) PayableCount() int        { return v.payables }
func (v Views) OpenCount() int           { return v.open }

func (v Views) PartiallySettledCount() int { return v.partial }
func (v Views) SettledCount() int          { return v.settled }
func (v Views) CancelledCount() int        { return v.cancelled }

// ObligationCentavos sums the obligations of non-cancelled payables.
func (v Views) ObligationCentavos() *big.Int { return new(big.Int).Set(&v.obligation) }

// SettledCentavos sums every settlement entry.
func (v Views) SettledCentavos() *big.Int { return new(big.Int).Set(&v.settledSum) }

// RemainingCentavos sums the derived remaining amounts.
func (v Views) RemainingCentavos() *big.Int { return new(big.Int).Set(&v.remaining) }
