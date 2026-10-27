// Package domain owns private individual-driver rules. No I/O or provider access.
package domain

import (
	"errors"
	"math/big"
	"strings"
)

const MaxAmount int64 = 1_000_000_000_000
const MaxItems = 1000

var ErrInvalid = errors.New("driver: invalid input")
var ErrConflict = errors.New("driver: revision or coverage conflict")

type Earnings struct {
	Basis  string `json:"basis"`
	Gross  *int64 `json:"gross,omitempty"`
	Fees   *int64 `json:"fees,omitempty"`
	Tips   *int64 `json:"tips,omitempty"`
	Bonus  *int64 `json:"bonus,omitempty"`
	Refund *int64 `json:"refund,omitempty"`
	Net    *int64 `json:"net,omitempty"`
}

func validAmount(v int64) bool { return v >= 0 && v <= MaxAmount }
func bounded(n *big.Int) (int64, error) {
	if !n.IsInt64() || n.Cmp(big.NewInt(MaxAmount)) > 0 || n.Cmp(big.NewInt(-MaxAmount)) < 0 {
		return 0, ErrInvalid
	}
	return n.Int64(), nil
}
func (e Earnings) Earned() (int64, error) {
	components := []*int64{e.Gross, e.Fees, e.Tips, e.Bonus, e.Refund}
	switch e.Basis {
	case "INCLUSIVE_NET":
		if e.Net == nil || !validAmount(*e.Net) {
			return 0, ErrInvalid
		}
		for _, v := range components {
			if v != nil {
				return 0, ErrInvalid
			}
		}
		return *e.Net, nil
	case "DETAILED":
		if e.Net != nil {
			return 0, ErrInvalid
		}
		for _, v := range components {
			if v == nil || !validAmount(*v) {
				return 0, ErrInvalid
			}
		}
		n := big.NewInt(*e.Gross)
		n.Sub(n, big.NewInt(*e.Fees))
		n.Add(n, big.NewInt(*e.Tips))
		n.Add(n, big.NewInt(*e.Bonus))
		n.Sub(n, big.NewInt(*e.Refund))
		return bounded(n)
	default:
		return 0, ErrInvalid
	}
}

type Adjustment struct {
	ID, TargetID, Kind    string
	TargetRevision, Delta int64
}

func (a Adjustment) Validate() error {
	if a.ID == "" || a.TargetID == "" || a.TargetRevision < 1 || a.Delta == 0 || a.Delta < -MaxAmount || a.Delta > MaxAmount {
		return ErrInvalid
	}
	if a.Kind != "CORRECTION" && (a.Kind != "REVERSAL" || a.Delta > 0) {
		return ErrInvalid
	}
	return nil
}

type Window struct {
	Owner, Vehicle, Currency string
	FromMillis, ToMillis     int64
}

func (w Window) Validate() error {
	if w.Owner == "" || w.Vehicle == "" || w.Currency != "BRL" || w.ToMillis <= w.FromMillis {
		return ErrInvalid
	}
	return nil
}

type CashMovement struct {
	ID, Kind, Account string
	Amount            int64
}
type MoneyViews struct {
	Earned   int64
	Received map[string]int64
}

// Payouts and transfers are cash-account facts, never another work earning.
func Views(earned []int64, cash []CashMovement) (MoneyViews, error) {
	if len(earned) > MaxItems || len(cash) > MaxItems {
		return MoneyViews{}, ErrInvalid
	}
	out := MoneyViews{Received: map[string]int64{}}
	n := new(big.Int)
	for _, v := range earned {
		if v < -MaxAmount || v > MaxAmount {
			return out, ErrInvalid
		}
		n.Add(n, big.NewInt(v))
	}
	var err error
	out.Earned, err = bounded(n)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, c := range cash {
		if c.ID == "" || seen[c.ID] || !validAmount(c.Amount) || (c.Kind != "PAYOUT" && c.Kind != "WALLET_TRANSFER" && c.Kind != "CASH_RECEIPT") || (c.Account != "BANK" && c.Account != "CASH" && c.Account != "WALLET") {
			return out, ErrInvalid
		}
		seen[c.ID] = true
		total, err := bounded(new(big.Int).Add(big.NewInt(out.Received[c.Account]), big.NewInt(c.Amount)))
		if err != nil {
			return out, err
		}
		out.Received[c.Account] = total
	}
	return out, nil
}

type Ratios struct{ PerKm, PerHour, Unknown string }

func ExactRatios(amount int64, distanceMetres, workSeconds *int64) (Ratios, error) {
	if amount < -MaxAmount || amount > MaxAmount {
		return Ratios{}, ErrInvalid
	}
	out := Ratios{}
	var reasons []string
	for _, d := range []struct {
		value *int64
		name  string
		scale int64
		out   *string
	}{{distanceMetres, "distance", 1000, &out.PerKm}, {workSeconds, "time", 3600, &out.PerHour}} {
		if d.value == nil {
			reasons = append(reasons, d.name+"-missing")
			continue
		}
		if *d.value <= 0 {
			reasons = append(reasons, d.name+"-nonpositive")
			continue
		}
		if *d.value > MaxAmount {
			return out, ErrInvalid
		}
		n := new(big.Int).Mul(big.NewInt(amount), big.NewInt(d.scale))
		*d.out = new(big.Rat).SetFrac(n, big.NewInt(*d.value)).RatString()
	}
	out.Unknown = strings.Join(reasons, ",")
	return out, nil
}

type Cost struct {
	ID     string
	Amount int64
	Window Window
}
type OperatingResult struct {
	Recorded, Modeled             *int64
	RecordedRatios, ModeledRatios Ratios
	Unknown                       string
}

func Result(earned int64, earningsWindow, reportWindow Window, costs []Cost, complete bool, estimates []Cost, distance, seconds *int64) (OperatingResult, error) {
	out := OperatingResult{}
	if earned < -MaxAmount || earned > MaxAmount || earningsWindow.Validate() != nil || reportWindow.Validate() != nil || len(costs) > MaxItems || len(estimates) > MaxItems {
		return out, ErrInvalid
	}
	if earningsWindow != reportWindow {
		out.Unknown = "unmatched-period-window"
		return out, nil
	}
	if !complete {
		out.Unknown = "cost-coverage-incomplete"
		return out, nil
	}
	seen := map[string]bool{}
	sum := big.NewInt(earned)
	subtract := func(items []Cost) error {
		for _, c := range items {
			if c.ID == "" || seen[c.ID] || !validAmount(c.Amount) {
				return ErrInvalid
			}
			seen[c.ID] = true
			if c.Window != reportWindow {
				return ErrConflict
			}
			sum.Sub(sum, big.NewInt(c.Amount))
		}
		return nil
	}
	if err := subtract(costs); err != nil {
		return out, err
	}
	v, err := bounded(sum)
	if err != nil {
		return out, err
	}
	out.Recorded = &v
	out.RecordedRatios, err = ExactRatios(v, distance, seconds)
	if err != nil {
		return out, err
	}
	if estimates == nil {
		out.Unknown = "model-assumptions-missing"
		return out, nil
	}
	if err := subtract(estimates); err != nil {
		return out, err
	}
	m, err := bounded(sum)
	if err != nil {
		return out, err
	}
	out.Modeled = &m
	out.ModeledRatios, err = ExactRatios(m, distance, seconds)
	return out, err
}

// Sum first; averaging already-computed ratios would weight small windows incorrectly.
func WeightedRatios(amounts, distances, seconds []int64) (Ratios, error) {
	if len(amounts) == 0 || len(amounts) > MaxItems || len(amounts) != len(distances) || len(amounts) != len(seconds) {
		return Ratios{}, ErrInvalid
	}
	a, d, s := new(big.Int), new(big.Int), new(big.Int)
	for i, v := range amounts {
		if v < -MaxAmount || v > MaxAmount || distances[i] < 0 || distances[i] > MaxAmount || seconds[i] < 0 || seconds[i] > MaxAmount {
			return Ratios{}, ErrInvalid
		}
		a.Add(a, big.NewInt(v))
		d.Add(d, big.NewInt(distances[i]))
		s.Add(s, big.NewInt(seconds[i]))
	}
	av, e := bounded(a)
	if e != nil {
		return Ratios{}, e
	}
	dv, e := bounded(d)
	if e != nil {
		return Ratios{}, e
	}
	sv, e := bounded(s)
	if e != nil {
		return Ratios{}, e
	}
	return ExactRatios(av, &dv, &sv)
}
