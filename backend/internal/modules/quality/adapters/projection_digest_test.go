package adapters

import (
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func TestProjectionDigestSeparatesOpaqueAssayText(t *testing.T) {
	facts := domain.SampleFacts{SampleKey: "sample", StationID: "station", ObservedOn: "2026-09-01", Fuel: domain.FuelDieselS10}
	a := []linkedSample{{facts: facts, assays: []domain.AssayObservation{{Ensaio: "a\x1eb", Result: "c", Conformity: domain.Conforming}}}}
	b := []linkedSample{{facts: facts, assays: []domain.AssayObservation{{Ensaio: "a", Result: "b\x1ec", Conformity: domain.Conforming}}}}
	if digestSamples(a) == digestSamples(b) {
		t.Fatal("opaque text collided across field boundaries")
	}
}
