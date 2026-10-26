package domain

import (
	"math/big"
	"sort"
	"time"
)

type ProfileCost struct {
	ID, Kind, Provenance, LedgerSourceID, From, Until string
	SourceRevision                                    int64
	Amount                                            *int64
}
type VehicleCostProfile struct {
	Vehicle, Ownership, Timezone, From, Until string
	Version                                   int64
	Costs                                     []ProfileCost
}
type ProfileBuckets struct {
	Operating, Capital, Modeled []ProfileCost
	Unknown                     string
}

func civil(s string) (time.Time, error) {
	d, e := time.Parse("2006-01-02", s)
	if e != nil || d.Year() < 1 || d.Format("2006-01-02") != s {
		return time.Time{}, ErrInvalid
	}
	return d, nil
}
func validPeriod(from, until string) bool {
	a, e := civil(from)
	if e != nil {
		return false
	}
	if until == "" {
		return true
	}
	b, e := civil(until)
	return e == nil && b.After(a)
}
func (p VehicleCostProfile) Validate() error {
	if p.Vehicle == "" || p.Version < 1 || len(p.Costs) > MaxItems || !validPeriod(p.From, p.Until) || (p.Ownership != "OWNED" && p.Ownership != "LEASED" && p.Ownership != "FINANCED") || p.Timezone == "" || p.Timezone == "Local" {
		return ErrInvalid
	}
	if _, e := time.LoadLocation(p.Timezone); e != nil {
		return ErrInvalid
	}
	ids := map[string]bool{}
	ledger := map[string]bool{}
	for _, c := range p.Costs {
		if !stableID.MatchString(c.ID) || ids[c.ID] || !validPeriod(c.From, c.Until) || c.From < p.From || (p.Until != "" && (c.Until == "" || c.Until > p.Until)) || (c.Amount != nil && !validAmount(*c.Amount)) {
			return ErrInvalid
		}
		ids[c.ID] = true
		modeled := c.Kind == "RESERVE" || c.Kind == "DEPRECIATION"
		if modeled {
			if c.Provenance != "MODELED" || c.LedgerSourceID != "" || c.SourceRevision != 0 {
				return ErrInvalid
			}
		} else {
			if c.Provenance != "ACTUAL" || !stableID.MatchString(c.LedgerSourceID) || c.SourceRevision < 1 || ledger[c.LedgerSourceID] {
				return ErrInvalid
			}
			ledger[c.LedgerSourceID] = true
		}
		switch c.Kind {
		case "LEASE":
			if p.Ownership != "LEASED" {
				return ErrInvalid
			}
		case "DEPRECIATION":
			if p.Ownership == "LEASED" {
				return ErrInvalid
			}
		case "PRINCIPAL", "INTEREST":
			if p.Ownership != "FINANCED" {
				return ErrInvalid
			}
		case "FEE", "MAINTENANCE", "RESERVE":
		default:
			return ErrInvalid
		}
	}
	return nil
}
func (p VehicleCostProfile) At(day string) (ProfileBuckets, error) {
	out := ProfileBuckets{}
	if p.Validate() != nil {
		return out, ErrInvalid
	}
	if _, e := civil(day); e != nil {
		return out, e
	}
	if day < p.From || (p.Until != "" && day >= p.Until) {
		out.Unknown = "profile-not-effective"
		return out, nil
	}
	for _, c := range p.Costs {
		if day < c.From || (c.Until != "" && day >= c.Until) {
			continue
		}
		switch {
		case c.Kind == "PRINCIPAL":
			out.Capital = append(out.Capital, c)
		case c.Provenance == "MODELED":
			out.Modeled = append(out.Modeled, c)
		default:
			out.Operating = append(out.Operating, c)
		}
	}
	return out, nil
}
func SumProfileCosts(items []ProfileCost) (*int64, error) {
	n := new(big.Int)
	for _, c := range items {
		if c.Amount == nil {
			return nil, nil
		}
		if !validAmount(*c.Amount) {
			return nil, ErrInvalid
		}
		n.Add(n, big.NewInt(*c.Amount))
	}
	v, e := bounded(n)
	return &v, e
}

type Reserve struct {
	ID     string
	Amount *int64
	Window Window
}
type ReserveReplacement struct{ ReserveID, ActualID string }
type ReserveResult struct {
	AdditionalModeled *int64
	ActualRecorded    int64
	Replaced          []string
	Unknown           string
}

func ReconcileReserves(w Window, reserves []Reserve, actual []Cost, links []ReserveReplacement) (ReserveResult, error) {
	out := ReserveResult{}
	if w.Validate() != nil || len(reserves) > MaxItems || len(actual) > MaxItems || len(links) > MaxItems {
		return out, ErrInvalid
	}
	rmap := map[string]Reserve{}
	amap := map[string]bool{}
	sum := new(big.Int)
	for _, r := range reserves {
		if !stableID.MatchString(r.ID) || rmap[r.ID].ID != "" || r.Window != w || (r.Amount != nil && !validAmount(*r.Amount)) {
			return out, ErrInvalid
		}
		rmap[r.ID] = r
	}
	for _, a := range actual {
		if !stableID.MatchString(a.ID) || amap[a.ID] || rmap[a.ID].ID != "" || a.Window != w || !validAmount(a.Amount) {
			return out, ErrInvalid
		}
		amap[a.ID] = true
		sum.Add(sum, big.NewInt(a.Amount))
	}
	var e error
	out.ActualRecorded, e = bounded(sum)
	if e != nil {
		return out, e
	}
	replaced := map[string]bool{}
	usedActual := map[string]bool{}
	for _, l := range links {
		if rmap[l.ReserveID].ID == "" || !amap[l.ActualID] || replaced[l.ReserveID] || usedActual[l.ActualID] {
			return out, ErrInvalid
		}
		replaced[l.ReserveID] = true
		usedActual[l.ActualID] = true
		out.Replaced = append(out.Replaced, l.ReserveID)
	}
	sort.Strings(out.Replaced)
	sum.SetInt64(0)
	missing := false
	for _, r := range reserves {
		if replaced[r.ID] {
			continue
		}
		if r.Amount == nil {
			missing = true
			continue
		}
		sum.Add(sum, big.NewInt(*r.Amount))
	}
	if missing {
		out.Unknown = "model-assumptions-missing"
		return out, nil
	}
	v, e := bounded(sum)
	if e != nil {
		return out, e
	}
	out.AdditionalModeled = &v
	return out, nil
}
