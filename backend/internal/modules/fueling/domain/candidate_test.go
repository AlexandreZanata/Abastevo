package domain_test

import (
	"fmt"
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func uuid(n int) string { return fmt.Sprintf("9d000000-0000-4000-8000-%012d", n) }
func scope(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	id, e := kernel.ParseOrgID(uuid(n))
	if e != nil {
		t.Fatal(e)
	}
	s, e := kernel.NewOrgScope(id)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func asset(t *testing.T, s kernel.OrgScope, n int) fleet.AssetRef {
	t.Helper()
	id, e := fleet.ParseAssetID(uuid(n))
	if e != nil {
		t.Fatal(e)
	}
	a, e := fleet.NewAssetRef(s, id)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func candidateInput(t *testing.T, s kernel.OrgScope) fuel.CandidateInput {
	t.Helper()
	ref, e := fuel.NewFuelingRef(s, uuid(10))
	if e != nil {
		t.Fatal(e)
	}
	emp, e := fuel.NewEmployeeRef(s, uuid(11))
	if e != nil {
		t.Fatal(e)
	}
	author, e := fuel.NewMembershipRef(s, uuid(12))
	if e != nil {
		t.Fatal(e)
	}
	q, e := fuel.NewQuantity(fuel.Electricity, fleet.KilowattHour, "30000")
	if e != nil {
		t.Fatal(e)
	}
	amounts, e := fuel.NewAmounts("5850", nil, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	missing, e := meter.NewMissing("Display inaccessible")
	if e != nil {
		t.Fatal(e)
	}
	observation, e := fuel.Missing(missing)
	if e != nil {
		t.Fatal(e)
	}
	at, e := orgtime.ParseInstant("2026-11-14T12:00:00Z")
	if e != nil {
		t.Fatal(e)
	}
	return fuel.CandidateInput{Reference: ref, Asset: asset(t, s, 20), Responsible: emp, Author: author, Declared: &orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}, Quantity: q, Amounts: amounts, Fill: fuel.Unknown, Payer: fuel.Employee, Payment: fuel.PIX, Observation: observation}
}
func TestReadyCandidateDoesNotInferFullOrPayment(t *testing.T) {
	s := scope(t, 1)
	c, e := fuel.NewCandidate(s, candidateInput(t, s))
	if e != nil {
		t.Fatal(e)
	}
	in := c.Snapshot()
	if in.Fill != fuel.Unknown || c.EffectiveFill() != fuel.Partial || in.Payer != fuel.Employee || in.Amounts.Snapshot().Paid != 5850 || in.Quantity.Product().Unit() != fleet.KilowattHour {
		t.Fatal("declared evidence rewritten")
	}
}
func TestIndependentRolesAndIssuerProjection(t *testing.T) {
	s := scope(t, 1)
	in := candidateInput(t, s)
	station, e := fuel.NewPublicStationRef(uuid(40))
	if e != nil {
		t.Fatal(e)
	}
	place, e := fuel.AtStation(station)
	if e != nil {
		t.Fatal(e)
	}
	contract, e := fuel.NewSupplierRef(s, uuid(41))
	if e != nil {
		t.Fatal(e)
	}
	in.Establishment = &place
	in.ContractedSupplier = &contract
	c, e := fuel.NewCandidate(s, in)
	if e != nil {
		t.Fatal(e)
	}
	doc, e := fuel.NewDocumentRef(s, uuid(42))
	if e != nil {
		t.Fatal(e)
	}
	issuer, e := fuel.NewIssuerProjection(s, doc, 2, "Independent DOC issuer")
	if e != nil {
		t.Fatal(e)
	}
	if c.Snapshot().Establishment.Station().ID() != uuid(40) || c.Snapshot().ContractedSupplier.ID() != uuid(41) || issuer.Document().ID() != uuid(42) || issuer.Revision() != 2 {
		t.Fatal("independent roles replaced")
	}
}
func TestCompatiblePurchasedTotalsKeepProductsAndAdditiveSeparate(t *testing.T) {
	q1, e := fuel.NewQuantity(fuel.DieselS10, fleet.Liter, "1000000000")
	if e != nil {
		t.Fatal(e)
	}
	q2, e := fuel.NewQuantity(fuel.DieselS500, fleet.Liter, "1000000000")
	if e != nil {
		t.Fatal(e)
	}
	total, e := fuel.SumCompatible([]fuel.Quantity{q1, q2, q1})
	if e != nil || total.Amount().String() != "3000000000" {
		t.Fatal("purchased sum incorrect", e)
	}
	products, e := fuel.SumByProduct([]fuel.Quantity{q1, q2, q1})
	if e != nil || len(products) != 2 {
		t.Fatal("per-product totals lost", e)
	}
	additive, e := fuel.NewQuantity(fuel.ARLA32, fleet.Liter, "1")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = fuel.SumCompatible([]fuel.Quantity{q1, additive}); e == nil {
		t.Fatal("additive merged into fuel volume")
	}
}
