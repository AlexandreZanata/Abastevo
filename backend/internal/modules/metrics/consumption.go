// Package metrics implements the frozen full-to-full consumption and
// purchase-cost engine (PRM06-T01). Exact integer and rational math only:
// no floats, no sums across units, no consumption guessed from volume.
package metrics

import (
	"errors"
	"fmt"
)

const (
	MaxQuantityMilli  = 1000000000
	MaxTotalCentavos  = 1000000000000
	MaxDistanceMetres = 1000000000000
)

type Fill struct {
	QuantityMilli int64
	TotalCentavos int64
	Full          bool
	Fuel          string
	Unit          string
}

type Interval struct {
	StartingFillExcluded Fill
	Fills                []Fill
	DistanceMetres       int64
	HasGap               bool
	CrossesSegment       bool
	OdometerMissing      bool
}

type Result struct {
	EligibleQuantityMilli int64
	EligibleCostCentavos  int64
	DistanceMetres        int64
	KmPerUnit             string
	CentavosPerKm         string
	Unknown               string
}

func gcd(a, b int64) int64 {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

func rational(num, den int64) string {
	if den <= 0 {
		return ""
	}
	g := gcd(num, den)
	num, den = num/g, den/g
	if den == 1 {
		return fmt.Sprintf("%d", num)
	}
	return fmt.Sprintf("%d/%d", num, den)
}

var errBounds = errors.New("metrics: input-out-of-bounds")

func (in Interval) Compute() (Result, error) {
	for _, f := range append(append([]Fill{}, in.StartingFillExcluded), in.Fills...) {
		if f.QuantityMilli < 0 || f.QuantityMilli > MaxQuantityMilli || f.TotalCentavos < 0 || f.TotalCentavos > MaxTotalCentavos {
			return Result{Unknown: "input-out-of-bounds"}, errBounds
		}
	}
	if in.DistanceMetres < 0 || in.DistanceMetres > MaxDistanceMetres {
		return Result{Unknown: "input-out-of-bounds"}, errBounds
	}
	if in.OdometerMissing {
		return Result{Unknown: "odometer-missing"}, nil
	}
	if in.HasGap {
		return Result{Unknown: "declared-fill-gap"}, nil
	}
	if in.CrossesSegment {
		return Result{Unknown: "odometer-segment-boundary"}, nil
	}
	if in.DistanceMetres <= 0 {
		return Result{Unknown: "distance-nonpositive"}, nil
	}
	units := map[string]bool{}
	fuels := map[string]bool{}
	for _, f := range in.Fills {
		units[f.Unit] = true
		fuels[f.Fuel] = true
	}
	if len(units) > 1 {
		return Result{Unknown: "incompatible-unit"}, nil
	}
	for fuel := range fuels {
		if fuel == "CNG" {
			return Result{Unknown: "unsupported-gnv-method"}, nil
		}
	}
	if len(fuels) > 1 {
		return Result{Unknown: "unsupported-mixed-method"}, nil
	}
	fullAnchors := 0
	if in.StartingFillExcluded.Full {
		fullAnchors++
	}
	for _, f := range in.Fills {
		if f.Full {
			fullAnchors++
		}
	}
	if fullAnchors < 2 {
		return Result{Unknown: "full-anchors-required"}, nil
	}
	var quantity, cost int64
	for _, f := range in.Fills {
		quantity += f.QuantityMilli
		cost += f.TotalCentavos
	}
	if quantity <= 0 {
		return Result{Unknown: "full-anchors-required"}, nil
	}
	return Result{
		EligibleQuantityMilli: quantity,
		EligibleCostCentavos:  cost,
		DistanceMetres:        in.DistanceMetres,
		KmPerUnit:             rational(in.DistanceMetres, quantity),
		CentavosPerKm:         rational(cost*1000, in.DistanceMetres),
	}, nil
}
