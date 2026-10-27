package domain

import "math/big"

type DistanceObservation struct {
	ID, Segment, Basis string
	Window             Window
	Metres             *int64
	Complete, HasGap   bool
}
type DistanceResult struct {
	Metres         *int64
	Basis, Unknown string
}

func SelectDistance(w Window, basis string, observations []DistanceObservation) (DistanceResult, error) {
	out := DistanceResult{Basis: basis}
	if w.Validate() != nil || len(observations) > MaxItems {
		return out, ErrInvalid
	}
	if basis == "ROUTE_PREDICTION" {
		out.Unknown = "prediction-not-measured"
		return out, nil
	}
	if basis != "ODOMETER" && basis != "GPS_ESTIMATE" && basis != "PAID_REPORTED" && basis != "MANUAL_TRIP" {
		return out, ErrInvalid
	}
	var selected []DistanceObservation
	seen := map[string]bool{}
	for _, o := range observations {
		if o.ID == "" || o.Segment == "" || seen[o.ID] || (o.Metres != nil && !validAmount(*o.Metres)) {
			return out, ErrInvalid
		}
		seen[o.ID] = true
		if o.Window != w {
			out.Unknown = "unmatched-period-window"
			return out, nil
		}
		if o.Basis == basis {
			selected = append(selected, o)
		}
	}
	if len(selected) != 1 {
		out.Unknown = "distance-source-ambiguous-or-missing"
		return out, nil
	}
	o := selected[0]
	if o.HasGap || !o.Complete || o.Metres == nil {
		out.Unknown = "distance-coverage-incomplete"
		return out, nil
	}
	out.Metres = o.Metres
	return out, nil
}
func OdometerDistance(start, end int64, startSegment, endSegment string) (DistanceResult, error) {
	out := DistanceResult{Basis: "ODOMETER"}
	if !validAmount(start) || !validAmount(end) || startSegment == "" || endSegment == "" {
		return out, ErrInvalid
	}
	if startSegment != endSegment {
		out.Unknown = "odometer-segment-boundary"
		return out, nil
	}
	if end < start {
		out.Unknown = "reversed-odometer"
		return out, nil
	}
	n := end - start
	out.Metres = &n
	return out, nil
}

type DistancePartition struct {
	EmptyMetres          *int64
	PaidPercent, Unknown string
}

func PartitionDistance(total int64, work, paid, personal *int64, complete bool) (DistancePartition, error) {
	out := DistancePartition{}
	if !validAmount(total) {
		return out, ErrInvalid
	}
	for _, v := range []*int64{work, paid, personal} {
		if v != nil && (!validAmount(*v) || *v > total) {
			return out, ErrInvalid
		}
	}
	if work != nil && paid != nil && *paid > *work {
		return out, ErrInvalid
	}
	if !complete || work == nil || paid == nil || personal == nil {
		out.Unknown = "distance-classification-incomplete"
		return out, nil
	}
	if new(big.Int).Add(big.NewInt(*work), big.NewInt(*personal)).Cmp(big.NewInt(total)) != 0 {
		return out, ErrInvalid
	}
	n := *work - *paid
	out.EmptyMetres = &n
	if *work == 0 {
		out.Unknown = "work-distance-nonpositive"
		return out, nil
	}
	out.PaidPercent = new(big.Rat).SetFrac(new(big.Int).Mul(big.NewInt(*paid), big.NewInt(100)), big.NewInt(*work)).RatString()
	return out, nil
}
