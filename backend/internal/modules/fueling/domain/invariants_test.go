package domain_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
)

func TestClosedDeclaredEnumsHaveNoDefaults(t *testing.T) {
	parsers := map[string]func(string) error{
		"fill":    func(raw string) error { _, e := fuel.ParseFill(raw); return e },
		"payer":   func(raw string) error { _, e := fuel.ParsePayer(raw); return e },
		"payment": func(raw string) error { _, e := fuel.ParsePaymentMethod(raw); return e },
		"origin":  func(raw string) error { _, e := fuel.ParseOrigin(raw); return e },
		"place":   func(raw string) error { _, e := fuel.ParsePlaceKind(raw); return e },
	}
	valid := map[string][]string{"fill": {"FULL", "PARTIAL", "UNKNOWN"}, "payer": {"COMPANY", "EMPLOYEE"}, "payment": {"CASH", "PIX", "DEBIT_CARD", "CREDIT_CARD", "FUEL_CARD", "SUPPLIER_ACCOUNT", "AGREEMENT_VOUCHER", "INTERNAL_STOCK", "OTHER"}, "origin": {"APP", "PANEL", "IMPORT", "AGREEMENT", "INTERNAL_TANK"}, "place": {"CATALOG_STATION", "ORG_SUPPLIER", "INTERNAL_TANK", "NOT_FOUND"}}
	for kind, parse := range parsers {
		t.Run(kind, func(t *testing.T) {
			for _, raw := range valid[kind] {
				if e := parse(raw); e != nil {
					t.Error("documented enum refused", e)
				}
			}
			for _, raw := range []string{"", "other", " UNKNOWN", "UNKNOWN ", "MISSING"} {
				if e := parse(raw); e == nil {
					t.Fatal("undeclared enum accepted")
				}
			}
		})
	}
	if f, e := fuel.Unknown.Effective(); e != nil || f != fuel.Partial {
		t.Fatal("unknown fill must retain partial semantics", e)
	}
}

func TestCandidatePrivateReferencesAreScopedToExactAsset(t *testing.T) {
	s, foreign := scope(t, 1), scope(t, 2)
	for name, alter := range map[string]func(*fuel.CandidateInput){
		"identity": func(i *fuel.CandidateInput) { i.Reference, _ = fuel.NewFuelingRef(foreign, uuid(10)) },
		"asset":    func(i *fuel.CandidateInput) { i.Asset = asset(t, foreign, 20) },
		"employee": func(i *fuel.CandidateInput) { i.Responsible, _ = fuel.NewEmployeeRef(foreign, uuid(11)) },
		"author":   func(i *fuel.CandidateInput) { i.Author, _ = fuel.NewMembershipRef(foreign, uuid(12)) },
		"trip":     func(i *fuel.CandidateInput) { v, _ := fuel.NewTripRef(foreign, uuid(31)); i.Trip = &v },
		"contract": func(i *fuel.CandidateInput) {
			v, _ := fuel.NewSupplierRef(foreign, uuid(32))
			i.ContractedSupplier = &v
		},
		"place-supplier": func(i *fuel.CandidateInput) {
			r, _ := fuel.NewSupplierRef(foreign, uuid(33))
			v, _ := fuel.AtSupplier(r)
			i.Establishment = &v
		},
		"place-source": func(i *fuel.CandidateInput) {
			r, _ := fuel.NewInternalSourceTankRef(foreign, uuid(34))
			v, _ := fuel.AtInternalTank(r)
			i.Establishment = &v
		},
		"tank-other-asset": func(i *fuel.CandidateInput) {
			v, _ := fuel.NewDestinationTankRef(s, asset(t, s, 99), uuid(35))
			i.Tank = &v
		},
		"group-other-asset": func(i *fuel.CandidateInput) {
			v, _ := fuel.NewTankGroupRef(s, asset(t, s, 99), uuid(36))
			i.TankGroup = &v
		},
		"reading-other-asset": func(i *fuel.CandidateInput) {
			m, _ := meter.NewMeterRef(s, asset(t, s, 99), uuid(37), meter.Odometer, nil)
			seg, _ := meter.NewSegmentRef(s, m, uuid(38))
			r, _ := meter.NewReadingRef(s, seg, uuid(39))
			i.Observation, _ = fuel.Observed(r)
		},
	} {
		t.Run(name, func(t *testing.T) {
			in := candidateInput(t, s)
			alter(&in)
			if _, e := fuel.NewCandidate(s, in); !errors.Is(e, fuel.ErrScope) {
				t.Fatal("foreign private evidence accepted", e)
			}
		})
	}
	// A public selector has no organization ownership and creates no station.
	in := candidateInput(t, s)
	public, e := fuel.NewPublicStationRef(uuid(40))
	if e != nil {
		t.Fatal(e)
	}
	place, e := fuel.AtStation(public)
	if e != nil {
		t.Fatal(e)
	}
	in.Establishment = &place
	if _, e = fuel.NewCandidate(s, in); e != nil {
		t.Fatal("public station was assigned private ownership", e)
	}
	doc, e := fuel.NewDocumentRef(foreign, uuid(41))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = fuel.NewIssuerProjection(s, doc, 1, "DOC-supplied issuer"); !errors.Is(e, fuel.ErrScope) {
		t.Fatal("foreign issuer document accepted", e)
	}
}

func TestCandidateAndAmountsCopyMutableInputsAndSnapshots(t *testing.T) {
	s := scope(t, 1)
	input := candidateInput(t, s)
	discount, price, text := "500", "6490", "\n5.89999\t"
	amounts, e := fuel.NewAmounts("97350", &discount, &price, &text)
	if e != nil {
		t.Fatal(e)
	}
	discount, price, text = "999", "1", "changed"
	input.Amounts = amounts
	tank, e := fuel.NewDestinationTankRef(s, input.Asset, uuid(50))
	if e != nil {
		t.Fatal(e)
	}
	input.Tank = &tank
	trip, _ := fuel.NewTripRef(s, uuid(51))
	input.Trip = &trip
	origin := fuel.Import
	input.Origin = &origin
	contract, _ := fuel.NewSupplierRef(s, uuid(52))
	input.ContractedSupplier = &contract
	source, _ := fuel.NewInternalSourceTankRef(s, uuid(53))
	place, _ := fuel.AtInternalTank(source)
	input.Establishment = &place
	c, e := fuel.NewCandidate(s, input)
	if e != nil {
		t.Fatal(e)
	}
	input.Declared.EntryZone = "Invalid/Zone"
	*input.Tank = fuel.DestinationTankRef{}
	*input.Trip = fuel.TripRef{}
	*input.Origin = "INVALID"
	*input.ContractedSupplier = fuel.SupplierRef{}
	*input.Establishment = fuel.Establishment{}
	copy := c.Snapshot()
	if copy.Declared.EntryZone != "America/Cuiaba" || copy.Tank.ID() != uuid(50) || copy.Trip.ID() != uuid(51) || *copy.Origin != fuel.Import || copy.ContractedSupplier.ID() != uuid(52) || copy.Establishment.InternalSource().ID() != uuid(53) {
		t.Fatal("candidate input pointer escaped")
	}
	a := copy.Amounts.Snapshot()
	if a.Paid != 97350 || *a.Discount != 500 || *a.UnitPrice != 6490 || *a.SourcePriceText != "\n5.89999\t" {
		t.Fatal("entered amounts or opaque source changed")
	}
	*a.Discount = 0
	*a.UnitPrice = 0
	*a.SourcePriceText = "changed"
	copy.Declared.EntryZone = "Invalid/Zone"
	*copy.Tank = fuel.DestinationTankRef{}
	*copy.Origin = "INVALID"
	*copy.Establishment.InternalSource() = fuel.InternalSourceTankRef{}
	again := c.Snapshot()
	if again.Declared.EntryZone != "America/Cuiaba" || again.Tank.ID() != uuid(50) || *again.Origin != fuel.Import || again.Establishment.InternalSource().ID() != uuid(53) || *again.Amounts.Snapshot().SourcePriceText != "\n5.89999\t" {
		t.Fatal("snapshot escaped")
	}
}

func TestNamesAreUncappedMetadataAndPriceIsOpaque(t *testing.T) {
	long := strings.Repeat("😀 Fonte declarada ", 100)
	place, e := fuel.AtNamedPlace(long)
	if e != nil || place.Name() != long {
		t.Fatal("invented place-label cap", e)
	}
	s := scope(t, 1)
	doc, _ := fuel.NewDocumentRef(s, uuid(40))
	p, e := fuel.NewIssuerProjection(s, doc, 9223372036854775807, long)
	if e != nil || p.Label() != long {
		t.Fatal("invented source-label/revision cap", e)
	}
	for _, bad := range []string{"", "\u00a0", "A\u0085B", "A\nB", string([]byte{0xff})} {
		if _, e = fuel.AtNamedPlace(bad); !errors.Is(e, fuel.ErrInvalid) {
			t.Fatal("invalid place metadata accepted")
		}
		if _, e = fuel.NewIssuerProjection(s, doc, 1, bad); !errors.Is(e, fuel.ErrInvalid) {
			t.Fatal("invalid DOC metadata accepted")
		}
	}
	for _, raw := range []string{"", "\n\t\u0000", long, "R$ 5,89999 (original)"} {
		a, e := fuel.NewAmounts("0", nil, nil, &raw)
		if e != nil || a.Snapshot().SourcePriceText == nil || *a.Snapshot().SourcePriceText != raw {
			t.Fatal("opaque price text interpreted", e)
		}
	}
	bad := string([]byte{0xff})
	if _, e = fuel.NewAmounts("0", nil, nil, &bad); !errors.Is(e, fuel.ErrInvalid) {
		t.Fatal("invalid source Unicode accepted")
	}
}

func TestTotalsAreExactPermutationStableAndDefensivelyCopied(t *testing.T) {
	q1, _ := fuel.NewQuantity(fuel.DieselS10, fleet.Liter, "1000000000")
	q2, _ := fuel.NewQuantity(fuel.DieselS500, fleet.Liter, "999999999")
	q3, _ := fuel.NewQuantity(fuel.DieselS10, fleet.Liter, "1")
	rows := []fuel.Quantity{q1, q2, q3}
	count := 0
	var visit func(int)
	visit = func(at int) {
		if at == len(rows) {
			count++
			sum, e := fuel.SumCompatible(rows)
			if e != nil || sum.Amount().String() != "2000000000" {
				t.Fatal("exact sum changed with order", e)
			}
			groups, e := fuel.SumByProduct(rows)
			if e != nil || len(groups) != 2 || groups[0].Amount().String() != "1000000001" {
				t.Fatal("per-product duplicate rows were discarded", e)
			}
			sum.Amount().SetInt64(0)
			groups[0].Amount().SetInt64(0)
			if sum.Amount().String() != "2000000000" || groups[0].Amount().String() != "1000000001" {
				t.Fatal("BigInt getter escaped")
			}
			return
		}
		for i := at; i < len(rows); i++ {
			rows[at], rows[i] = rows[i], rows[at]
			visit(at + 1)
			rows[at], rows[i] = rows[i], rows[at]
		}
	}
	visit(0)
	if count != 6 {
		t.Fatal("missing permutation")
	}
	products := fuel.Products()
	products[0] = fuel.ProductDescriptor{}
	if fuel.Products()[0].Code() == "" {
		t.Fatal("catalog collection escaped")
	}
	if groups, e := fuel.SumByProduct(nil); e != nil || groups == nil || len(groups) != 0 {
		t.Fatal("empty product groups incorrectly invented")
	}
	if _, e := fuel.SumCompatible(nil); !errors.Is(e, fuel.ErrInvalid) {
		t.Fatal("empty compatible group has invented units")
	}
	if _, e := fuel.SumByProduct([]fuel.Quantity{{}}); !errors.Is(e, fuel.ErrInvalid) {
		t.Fatal("zero typed quantity accepted")
	}
	var group sync.WaitGroup
	problems := make(chan error, 16)
	for range 16 {
		group.Go(func() {
			sum, e := fuel.SumCompatible(rows)
			if e != nil || sum.Amount().String() != "2000000000" {
				problems <- errors.New("parallel total changed")
			}
		})
	}
	group.Wait()
	close(problems)
	for e := range problems {
		t.Fatal(e)
	}
}

func TestCandidateDiagnosticsRedactAllPrivateSurfaces(t *testing.T) {
	s := scope(t, 1)
	in := candidateInput(t, s)
	c, e := fuel.NewCandidate(s, in)
	if e != nil {
		t.Fatal(e)
	}
	doc, _ := fuel.NewDocumentRef(s, uuid(40))
	issuer, _ := fuel.NewIssuerProjection(s, doc, 1, "private issuer")
	place, _ := fuel.AtNamedPlace("private establishment")
	total, _ := fuel.SumCompatible([]fuel.Quantity{in.Quantity})
	for _, v := range []any{in.Reference, in.Responsible, in.Author, in.Quantity, in.Amounts, in.Amounts.Snapshot(), in.Observation, in, c, issuer, place, total} {
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		var out strings.Builder
		slog.New(slog.NewJSONHandler(&out, nil)).Info("value", "value", v)
		for _, text := range []string{fmt.Sprint(v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), string(raw), out.String()} {
			if !strings.Contains(text, "redacted") || strings.Contains(text, "private") || strings.Contains(text, uuid(40)) || strings.Contains(text, "Cuiaba") {
				t.Fatalf("private content leaked for %T", v)
			}
		}
	}
}

func TestMissingCandidateFieldsAreInvalidRatherThanInvented(t *testing.T) {
	s := scope(t, 1)
	for name, alter := range map[string]func(*fuel.CandidateInput){"identity": func(i *fuel.CandidateInput) { i.Reference = fuel.FuelingRef{} }, "asset": func(i *fuel.CandidateInput) { i.Asset = fleet.AssetRef{} }, "employee": func(i *fuel.CandidateInput) { i.Responsible = fuel.EmployeeRef{} }, "author": func(i *fuel.CandidateInput) { i.Author = fuel.MembershipRef{} }, "declaration": func(i *fuel.CandidateInput) { i.Declared = nil }, "quantity": func(i *fuel.CandidateInput) { i.Quantity = fuel.Quantity{} }, "amounts": func(i *fuel.CandidateInput) { i.Amounts = fuel.Amounts{} }, "fill": func(i *fuel.CandidateInput) { i.Fill = "" }, "payer": func(i *fuel.CandidateInput) { i.Payer = "" }, "payment": func(i *fuel.CandidateInput) { i.Payment = "" }, "reading": func(i *fuel.CandidateInput) { i.Observation = fuel.Observation{} }} {
		t.Run(name, func(t *testing.T) {
			in := candidateInput(t, s)
			alter(&in)
			if _, e := fuel.NewCandidate(s, in); !errors.Is(e, fuel.ErrInvalid) {
				t.Fatal("missing field did not fail invalid", e)
			}
		})
	}
}
