//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	expensepg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/adapters"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	fuelapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

// fuelingState is the SELF intake fixture (ACTIVE implement with a meter, the
// FLEET-006 assignment source and the ADMIN/ORG device member) plus an ACTIVE
// DIESEL_S10 vehicle with its own meter and the real fueling.submit owner
// with its real expense projection (EMP-EXP-002).
type fuelingState struct {
	selfIntakeState
	authority *orgpg.FuelingAuthority
	store     *fuelpg.Store
	expenses  *expensepg.Store
	fueling   syncapp.Handler
	fuelings  *syncpg.Store
	vehicle   fa.MeterInstallResult
	vehicleAt int64
}

func fuelingGrants(t *testing.T, fx legalFixtureState) {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_fuelings,org_fueling_warnings,org_fueling_review_entries TO "+role+";GRANT SELECT ON org_employee_identities,org_asset_unit_periods,org_asset_cost_centre_periods,org_asset_tank_products,org_meters,org_meter_readings TO "+role); err != nil {
		t.Fatal(err)
	}
	var heldSchema bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_held_fuelings') IS NOT NULL`).Scan(&heldSchema); err != nil {
		t.Fatal(err)
	}
	if heldSchema {
		if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT ON org_held_fuelings TO "+role); err != nil {
			t.Fatal(err)
		}
	}
	expenseGrants(t, fx)
}

// expenseGrants provisions the signed intake principal for EMP-EXP-002 once
// migration 000244 is installed: every accepted fueling inserts its expense.
func expenseGrants(t *testing.T, fx legalFixtureState) {
	t.Helper()
	var installed bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_expenses') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_expenses TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
			t.Fatal(err)
		}
	}
}

func fuelingIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := meterReviewIntakeConfig(fx)
	config.NoAsyncPlans[syncapp.OperationKey{Type: fuelapp.OperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup}
	return config
}

func fuelingFixture(t *testing.T) fuelingState {
	t.Helper()
	return fuelingFixtureOn(t, selfIntakeFixture(t), nil)
}

// fuelingFixtureOn builds the fueling fixture on a SELF intake fixture. A nil
// costs port composes the real expense projection, as the runtime does; an
// earlier migration image (before 000244) supplies its own.
func fuelingFixtureOn(t *testing.T, base selfIntakeState, costs fuelapp.CostProjection) fuelingState {
	t.Helper()
	x := fuelingState{selfIntakeState: base}
	fuelingGrants(t, x.legalFixtureState)
	meterAuthority, ok := x.meters.Authority.(*orgpg.MeterAuthority)
	if !ok {
		t.Fatal("fixture meter authority is not the real one")
	}
	x.authority = (*orgpg.FuelingAuthority)(meterAuthority)
	x.store = &fuelpg.Store{Allocations: x.assets}
	x.expenses = &expensepg.Store{Allocations: x.assets, NewID: jobs.NewUUIDv4}
	if costs == nil {
		costs = x.expenses
	}
	handler, err := fuelpg.NewIntakeHandler(x.authority, x.meters, x.store, costs)
	if err != nil {
		t.Fatal(err)
	}
	x.fueling, x.fuelings = handler, syncpg.NewStore(fuelingIntakeConfig(x.syncFixtureState))
	input := fleetCreateInput()
	input.Profile.Kind, input.Profile.UsageBasis, input.UnitID = fleet.Vehicle, fleet.Distance, x.unit
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	input.Profile.Tanks = []fleet.Tank{tank}
	created, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	x.vehicle, _, err = meterInstall(t, x.legalFixtureState, x.meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, lifecycleDBNow(t, x.legalFixtureState).Add(-6*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	active, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: x.vehicle.AssetRevision}))
	if err != nil || active.State != fleet.Active {
		t.Fatal("tanked vehicle not active", err)
	}
	x.vehicleAt = active.Revision
	return x
}

// op builds a signed fueling.submit of member d on the vehicle: 150 L of
// DIESEL_S10 at R$ 6.490 paid R$ 973.50 with an embedded odometer reading.
func (x fuelingState) op(t *testing.T, d syncFixtureState, declared time.Time, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	payload := map[string]any{
		"asset_id": x.vehicle.AssetID, "responsible_employee_id": d.person.employee, "product": "DIESEL_S10", "unit": "L",
		"quantity": "150000", "paid_total": "97350", "discount": nil, "unit_price": "6490", "unit_price_text": nil,
		"fill": "FULL", "payer": "COMPANY", "payment_method": "FUEL_CARD",
		"reading":                map[string]any{"meter_id": x.vehicle.MeterID, "segment_id": x.vehicle.SegmentID, "reading_id": onboardingID(t), "kind": "ODOMETER", "value": "184190000"},
		"missing_reading_reason": nil, "place": nil, "trip_id": nil, "contracted_supplier_id": nil,
	}
	return syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = fuelapp.OperationType
		raw["entity"] = map[string]any{"type": fuelapp.EntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(declared)
		raw["entry_zone"] = "America/Cuiaba"
		raw["device_offset_seconds"] = -14400
		raw["payload"] = payload
		if edit != nil {
			edit(raw, payload)
		}
	})
}

// missing turns an operation into the missing-reading-reason form.
func missing(reason string) func(_, p map[string]any) {
	return func(_, p map[string]any) { p["reading"], p["missing_reading_reason"] = nil, reason }
}

func (x fuelingState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	return x.fuelings.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.fueling)
}

type fuelingCounts struct{ fuelings, readings, warnings, entries, effects, claims, receipts, audits, expenses int }

// count includes the operation's expenses once migration 000244 exists
// (EMP-EXP-002: an accepted fueling has exactly one).
func (x fuelingState) count(t *testing.T, op protocol.Operation) fuelingCounts {
	t.Helper()
	var c fuelingCounts
	var installed bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT to_regclass('public.org_expenses') IS NOT NULL`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_expenses WHERE org_id=$1 AND operation_id=$2`, x.scope.OrgID().String(), op.ID()).Scan(&c.expenses); err != nil {
			t.Fatal(err)
		}
	}
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_fuelings WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_meter_readings WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_fueling_warnings w JOIN org_fuelings f ON(f.org_id,f.id)=(w.org_id,w.fueling_id) WHERE f.org_id=$1 AND f.operation_id=$2),
(SELECT count(*) FROM org_fueling_review_entries WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&c.fuelings, &c.readings, &c.warnings, &c.entries, &c.effects, &c.claims, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x fuelingState) accepted(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, warnings, readings int) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 || len(ack.Warnings()) != 0 {
		t.Fatal("legitimate fueling was not accepted", err, ack.Result(), ack.Code())
	}
	if c := x.count(t, op); c != (fuelingCounts{fuelings: 1, readings: readings, warnings: warnings, entries: 1, effects: 1, claims: 1, receipts: 1, audits: 1, expenses: 1}) {
		t.Fatalf("accepted fueling not stored exactly once: %+v", c)
	}
}

// nothing proves a refusal or retry left no claim, receipt, audit or fact.
func (x fuelingState) nothing(t *testing.T, op protocol.Operation) {
	t.Helper()
	if c := x.count(t, op); c != (fuelingCounts{}) {
		t.Fatalf("refused fueling left state: %+v", c)
	}
}

func (x fuelingState) malformed(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
		t.Fatal("expected a final MALFORMED receipt", err, ack.Result(), ack.Code())
	}
	if c := x.count(t, op); c != (fuelingCounts{claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("malformed fueling stored a fact: %+v", c)
	}
}

func (x fuelingState) unavailable(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
		t.Fatal("expected RETRY/UNAVAILABLE with nothing stored", err)
	}
	x.nothing(t, op)
}

func (x fuelingState) refused(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, want orgapi.Code) {
	t.Helper()
	if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
		t.Fatalf("expected %s refusal, got %v", want, err)
	}
	x.nothing(t, op)
}

func (x fuelingState) declaredNow(t *testing.T) time.Time {
	t.Helper()
	return lifecycleDBNow(t, x.legalFixtureState).Truncate(time.Microsecond).Add(123 * time.Nanosecond)
}

// AC: a matching resend returns the stored result; the paid total survives
// an arithmetic mismatch, which is stored as a warning (B-BR-EMP-263..267).
func TestFuelingIntakeStoresOneFactWithItsReadingAndReplaysTheOriginalAck(t *testing.T) {
	x := fuelingFixture(t)
	d := x.syncFixtureState
	op := x.op(t, d, x.declaredNow(t), func(_, p map[string]any) { p["paid_total"], p["unit_price_text"] = "97000", "6,490" })
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
	org := x.scope.OrgID().String()
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT f.paid_centavos=97000 AND f.gross_centavos=97000 AND f.expected_centavos=97350 AND f.difference_centavos=350 AND f.derived_price_milli_brl IS NULL
 AND f.discount_centavos IS NULL AND f.unit_price_milli_brl=6490 AND f.unit_price_text='6,490' AND f.quantity=150000 AND f.product='DIESEL_S10' AND f.unit='L'
 AND f.fill='FULL' AND f.payer='COMPANY' AND f.payment_method='FUEL_CARD' AND f.origin='APP' AND f.place_kind IS NULL
 AND f.review_state='SUBMITTED' AND f.revision=1 AND f.unit_id=$3 AND f.cost_centre_id IS NULL AND f.occurred_submicro=123 AND f.entry_zone='America/Cuiaba' AND f.device_offset_seconds=-14400
 AND f.author_membership_id=$4 AND f.author_employee_id=$5 AND f.responsible_employee_id=$5 AND f.enrollment_id=$6 AND f.payload_digest=$7 AND f.id=$8 AND f.asset_id=$9
 AND f.missing_reading_reason IS NULL AND f.reading_id IS NOT NULL
 AND(SELECT array_agg(code) FROM org_fueling_warnings w WHERE w.org_id=f.org_id AND w.fueling_id=f.id)=ARRAY['ARITHMETIC_MISMATCH']
 AND EXISTS(SELECT 1 FROM org_fueling_review_entries r WHERE r.org_id=f.org_id AND r.fueling_id=f.id AND r.sequence=1 AND r.kind='SUBMIT' AND r.state='SUBMITTED' AND r.revision=1 AND r.actor_membership_id=$4)
 AND EXISTS(SELECT 1 FROM org_meter_readings r WHERE r.org_id=f.org_id AND r.id=f.reading_id AND r.origin='FUELING' AND r.operation_id=f.operation_id AND r.value=184190000 AND r.state='VALID' AND r.method='TYPED')
 AND f.received_at<=(SELECT received_at FROM org_sync_acknowledgements a WHERE a.org_id=f.org_id AND a.operation_id=f.operation_id)
 FROM org_fuelings f WHERE f.org_id=$1 AND f.operation_id=$2`, org, op.ID(), x.unit, d.source.MembershipID, d.person.employee, d.source.EnrollmentID, op.PayloadDigest(), op.Snapshot().Entity.ID, x.vehicle.AssetID).Scan(&exact); err != nil || !exact {
		t.Fatal("stored fueling is not the exact original fact", err)
	}
	// Two resends: byte-identical ACK, no second fact, reading, warning or audit.
	for range 2 {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK", err)
		}
	}
	x.accepted(t, op, ack, nil, 1, 1)
	// Same operation id with another digest: refused, nothing rewritten.
	reused := x.op(t, d, x.declaredNow(t), func(raw, _ map[string]any) {
		raw["operation_id"], raw["entity"] = op.ID(), map[string]any{"type": fuelapp.EntityType, "id": op.Snapshot().Entity.ID, "expected_revision": nil}
	})
	if reused.ID() != op.ID() || reused.PayloadDigest() == op.PayloadDigest() {
		t.Fatal("digest-reuse fixture")
	}
	refusal, err := x.process(t, d, reused)
	if err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same operation was not refused", err, refusal.Code())
	}
	x.accepted(t, op, ack, nil, 1, 1)
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT paid_centavos=97000 AND payload_digest=$3 FROM org_fuelings WHERE org_id=$1 AND operation_id=$2`, org, op.ID(), op.PayloadDigest()).Scan(&exact); err != nil || !exact {
		t.Fatal("digest reuse rewrote the stored fueling", err)
	}
	again, err := x.process(t, d, op)
	if err != nil || !bytes.Equal(again.CanonicalBytes(), ack.CanonicalBytes()) {
		t.Fatal("original ACK lost after a refused reuse", err)
	}
}

// B-BR-EMP-264/265: warnings never refuse a legitimate fact; a missing
// reading reason replaces the reading; an ORG member may name another
// responsible employee; the derived price is informative only.
func TestFuelingIntakeKeepsWarningsAsFactsAndTheMissingReasonPath(t *testing.T) {
	x := fuelingFixture(t)
	d := x.syncFixtureState
	org := x.scope.OrgID().String()
	other := assignmentEmployeeIn(t, x.legalFixtureState, "ACTIVE")

	electric := x.op(t, d, x.declaredNow(t), func(e, p map[string]any) {
		missing("Painel do hodômetro apagado")(e, p)
		p["product"], p["unit"], p["quantity"], p["paid_total"], p["unit_price"] = "ELECTRICITY", "KWH", "30000", "5850", "1950"
		p["place"] = map[string]any{"kind": "NOT_FOUND", "name": "Eletroposto sem cadastro"}
		p["responsible_employee_id"], p["payer"], p["payment_method"], p["fill"] = other, "EMPLOYEE", "PIX", "UNKNOWN"
	})
	ack, err := x.process(t, d, electric)
	x.accepted(t, electric, ack, err, 1, 0)
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT f.reading_id IS NULL AND f.meter_id IS NULL AND f.missing_reading_reason='Painel do hodômetro apagado' AND f.place_kind='NOT_FOUND' AND f.place_name='Eletroposto sem cadastro'
 AND f.unit='KWH' AND f.expected_centavos=5850 AND f.difference_centavos=0 AND f.responsible_employee_id=$3 AND f.author_employee_id<>$3 AND f.fill='UNKNOWN' AND f.payer='EMPLOYEE'
 AND(SELECT array_agg(code) FROM org_fueling_warnings w WHERE w.org_id=f.org_id AND w.fueling_id=f.id)=ARRAY['PRODUCT_NOT_ACCEPTED']
 FROM org_fuelings f WHERE f.org_id=$1 AND f.operation_id=$2`, org, electric.ID(), other).Scan(&exact); err != nil || !exact {
		t.Fatal("warning or missing reason not stored as the original fact", err)
	}

	derived := x.op(t, d, x.declaredNow(t), func(_, p map[string]any) {
		p["product"], p["unit"], p["quantity"], p["paid_total"], p["unit_price"] = "CNG", "M3", "40000", "23600", nil
		p["reading"].(map[string]any)["value"] = "184190500"
	})
	ack, err = x.process(t, d, derived)
	x.accepted(t, derived, ack, err, 1, 1)
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT derived_price_milli_brl='5900' AND expected_centavos IS NULL AND difference_centavos IS NULL AND unit_price_milli_brl IS NULL AND paid_centavos=23600 FROM org_fuelings WHERE org_id=$1 AND operation_id=$2`, org, derived.ID()).Scan(&exact); err != nil || !exact {
		t.Fatal("derived price became a recorded price", err)
	}

	clean := x.op(t, d, x.declaredNow(t), func(_, p map[string]any) { p["paid_total"], p["discount"] = "96850", "500" })
	ack, err = x.process(t, d, clean)
	x.accepted(t, clean, ack, err, 0, 1)
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT gross_centavos=97350 AND discount_centavos=500 AND paid_centavos=96850 AND difference_centavos=0 FROM org_fuelings WHERE org_id=$1 AND operation_id=$2`, org, clean.ID()).Scan(&exact); err != nil || !exact {
		t.Fatal("discount reconciliation changed the entered amounts", err)
	}
}

// B-BR-EMP-265/267: invalid evidence is a final MALFORMED receipt with no
// fact; recognised future forms and missing dependencies keep no claim.
func TestFuelingIntakeRefusesMalformedFormsWithoutAFact(t *testing.T) {
	x := fuelingFixture(t)
	d := x.syncFixtureState
	now := x.declaredNow(t)
	draft, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	first := x.op(t, d, now, nil)
	ack, err := x.process(t, d, first)
	x.accepted(t, first, ack, err, 0, 1)

	for name, edit := range map[string]func(e, p map[string]any){
		"neither-reading-nor-reason": func(_, p map[string]any) { p["reading"] = nil },
		"reading-and-reason":         func(_, p map[string]any) { p["missing_reading_reason"] = "Painel apagado" },
		"unit-not-of-product":        func(_, p map[string]any) { p["unit"] = "M3" },
		"unknown-responsible":        func(_, p map[string]any) { p["responsible_employee_id"] = onboardingID(t) },
		"draft-asset":                func(e, p map[string]any) { missing("Ativo sem medidor")(e, p); p["asset_id"] = draft.AssetID },
		"fueling-id-of-another-operation": func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": fuelapp.EntityType, "id": first.Snapshot().Entity.ID, "expected_revision": nil}
		},
		"reading-before-meter-installation": func(e, _ map[string]any) {
			e["occurred_at"] = orgtime.FormatInstant(now.Add(-7 * time.Hour))
		},
	} {
		t.Run(name, func(t *testing.T) {
			op := x.op(t, d, now, edit)
			ack, err := x.process(t, d, op)
			x.malformed(t, op, ack, err)
			replay, err := x.process(t, d, op)
			if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
				t.Fatal("malformed receipt not replayed", err)
			}
		})
	}
	for name, edit := range map[string]func(e, p map[string]any){
		"trip-owner-unavailable": func(_, p map[string]any) { p["trip_id"] = onboardingID(t) },
		"station-owner-unavailable": func(_, p map[string]any) {
			p["place"] = map[string]any{"kind": "CATALOG_STATION", "station_id": onboardingID(t)}
		},
		"receipt-bytes-unavailable": func(e, _ map[string]any) {
			e["attachments"] = []any{map[string]any{"id": onboardingID(t), "sha256": strings.Repeat("a", 64), "bytes": 10, "mime": "image/jpeg"}}
		},
		"supplier-owner-unavailable": func(_, p map[string]any) { p["contracted_supplier_id"] = onboardingID(t) },
	} {
		t.Run(name, func(t *testing.T) {
			op := x.op(t, d, now, edit)
			ack, err := x.process(t, d, op)
			x.unavailable(t, op, ack, err)
		})
	}
	for name, meter := range map[string]string{"unknown-meter": onboardingID(t), "meter-of-another-asset": x.installed.MeterID} {
		t.Run(name, func(t *testing.T) {
			op := x.op(t, d, now, func(_, p map[string]any) { p["reading"].(map[string]any)["meter_id"] = meter })
			ack, err := x.process(t, d, op)
			if err != nil || ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
				t.Fatal("missing reading target was not a pending dependency", err, ack.Result(), ack.Code())
			}
			x.nothing(t, op)
		})
	}
}

// foreignAsset is a real asset of a second organization of the same owner.
func foreignAsset(t *testing.T, x fuelingState) string {
	t.Helper()
	other, _, err := x.onboarding.Create(t.Context(), x.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	scope := scopeFor(t, other.OrgID.String())
	if _, err := x.flags.Set(t.Context(), scope, cap.Actor{AccountID: x.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	ox := x.legalFixtureState
	ox.scope, ox.member = scope, other.MembershipID
	created, _, err := fleetWrite(t, ox, x.assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal("foreign asset fixture", err)
	}
	return created.AssetID
}

// B-BR-EMP-266: authority at receipt through the real signed intake.
func TestFuelingIntakeAuthorityAtReceipt(t *testing.T) {
	x := fuelingFixture(t)
	asset := x.installed.AssetID
	self := func(t *testing.T, d syncFixtureState, declared time.Time, responsible string) protocol.Operation {
		return x.op(t, d, declared, func(_, p map[string]any) {
			p["asset_id"], p["responsible_employee_id"] = asset, responsible
			p["reading"].(map[string]any)["meter_id"], p["reading"].(map[string]any)["segment_id"] = x.installed.MeterID, x.installed.SegmentID
		})
	}
	t.Run("foreign-asset-not-found", func(t *testing.T) {
		foreign := foreignAsset(t, x)
		op := x.op(t, x.syncFixtureState, x.declaredNow(t), func(e, p map[string]any) { missing("Ativo de outra organização")(e, p); p["asset_id"] = foreign })
		ack, err := x.process(t, x.syncFixtureState, op)
		x.refused(t, op, ack, err, orgapi.NotFound)
	})
	t.Run("self-never-assigned-forbidden", func(t *testing.T) {
		d := x.driver(t)
		op := self(t, d, x.declaredNow(t), d.person.employee)
		ack, err := x.process(t, d, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("self-assigned-own-fueling-and-replay", func(t *testing.T) {
		d := x.driver(t)
		_, row := x.assign(t, d.person.employee, asset, fleet.PrimaryAssignment, nil, nil)
		op := self(t, d, row.from, d.person.employee)
		ack, err := x.process(t, d, op)
		x.accepted(t, op, ack, err, 1, 1)
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("SELF receipt replay", err)
		}
		t.Run("another-responsible-forbidden", func(t *testing.T) {
			other := self(t, d, x.declaredNow(t), x.person.employee)
			ack, err := x.process(t, d, other)
			x.refused(t, other, ack, err, orgapi.Forbidden)
		})
		t.Run("declared-before-assignment-held-unavailable", func(t *testing.T) {
			declared := row.from.Add(-time.Nanosecond)
			var disposition fuelapp.Disposition
			if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
				var err error
				disposition, err = x.authority.ClassifyFuelingTx(t.Context(), syncapp.Borrow(tx), x.scope, d.source, asset, true, declared)
				return err
			}); err != nil || disposition != fuelapp.HoldPermissionChanged {
				t.Fatal("changed assignment is not a retained hold", disposition, err)
			}
			before := self(t, d, declared, d.person.employee)
			ack, err := x.process(t, d, before)
			x.unavailable(t, before, ack, err)
		})
	})
	t.Run("other-asset-assigned-forbidden", func(t *testing.T) {
		d := x.driver(t)
		x.assign(t, d.person.employee, x.vehicle.AssetID, fleet.PrimaryAssignment, nil, nil)
		op := self(t, d, x.declaredNow(t), d.person.employee)
		ack, err := x.process(t, d, op)
		x.refused(t, op, ack, err, orgapi.Forbidden)
	})
}

// A fueling declared at or after the asset's disposal is held by the meter
// rule; with no held-fueling owner it stays unavailable and stores nothing.
// One declared before the disposal is recorded unchanged.
func TestFuelingIntakeDisposalStoresNothingAtOrAfterTheInstant(t *testing.T) {
	x := fuelingFixture(t)
	d := x.syncFixtureState
	disposal := lifecycleDBNow(t, x.legalFixtureState).Truncate(time.Microsecond).Add(-time.Microsecond).Add(500 * time.Nanosecond)
	before := disposal.Add(-time.Millisecond)
	dispose := lifecycleInputFor(t, x.legalFixtureState, fleet.Dispose, x.vehicle.AssetID, x.vehicleAt)
	dispose.At = disposal
	if _, _, err := lifecycleWrite(t, x.legalFixtureState, x.assets, lifecycleCommand(t, dispose)); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{disposal, disposal.Add(time.Nanosecond)} {
		op := x.op(t, d, at, missing("Veículo baixado"))
		ack, err := x.process(t, d, op)
		x.unavailable(t, op, ack, err)
	}
	op := x.op(t, d, before, missing("Abastecimento anterior à baixa"))
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 0, 0)
}

// Concurrent identical operations leave exactly one fueling (real 12-way
// contention on the ledger claim and the asset locks).
func TestFuelingIntakeConcurrentDuplicatesLeaveOneFueling(t *testing.T) {
	x := fuelingFixture(t)
	d := x.syncFixtureState
	config := fuelingIntakeConfig(x.syncFixtureState)
	config.Pool = x.f.RuntimePool(t, 12)
	store := syncpg.NewStore(config)
	op := x.op(t, d, x.declaredNow(t), nil)
	var wg sync.WaitGroup
	acks := make([]protocol.Acknowledgement, 12)
	errs := make([]error, 12)
	for i := range 12 {
		wg.Go(func() {
			acks[i], errs[i] = store.Process(context.Background(), d.source, op, protocol.DefaultLimits(), x.fueling)
		})
	}
	wg.Wait()
	var original []byte
	for i := range 12 {
		if errs[i] != nil {
			if !errors.Is(errs[i], syncapp.ErrUnavailable) || errors.Is(errs[i], syncapp.ErrSourceDenied) {
				t.Fatal("concurrent duplicate failed outside RETRY", errs[i])
			}
			continue
		}
		if acks[i].Result() != protocol.Accepted || original != nil && !bytes.Equal(original, acks[i].CanonicalBytes()) {
			t.Fatal("concurrent duplicates produced different results")
		}
		original = acks[i].CanonicalBytes()
	}
	if original == nil {
		t.Fatal("no concurrent duplicate was accepted")
	}
	x.accepted(t, op, acks[0], nil, 0, 1)
	replay, err := x.process(t, d, op)
	if err != nil || !bytes.Equal(replay.CanonicalBytes(), original) {
		t.Fatal("replay after contention", err)
	}
}

type fuelingFailingStore struct{ *fuelpg.Store }

func (s fuelingFailingStore) InsertFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source syncapp.Source, command fuelapp.Command, prepared fuelapp.Prepared, arithmetic fuel.Arithmetic) error {
	if err := s.Store.InsertFuelingTx(ctx, db, scope, source, command, prepared, arithmetic); err != nil {
		return err
	}
	return errors.New("synthetic failure after the real fueling insert")
}

type fuelingFailingReadings struct{ *fleetpg.MeterStore }

func (r fuelingFailingReadings) RecordEmbedded(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, source fa.MeterSource, member string, command fa.MeterReadingCommand) (fa.MeterReadingResult, bool, error) {
	if _, _, err := r.MeterStore.RecordEmbedded(ctx, db, scope, source, member, command); err != nil {
		return fa.MeterReadingResult{}, false, err
	}
	return fa.MeterReadingResult{}, false, errors.New("synthetic failure after the real reading insert")
}

// B-BR-EMP-265: the reading and the fueling share one transaction; a failure
// after either real insert leaves neither, and the same operation then
// records both.
func TestFuelingIntakeRollsBackTheReadingAndTheFuelingTogether(t *testing.T) {
	x := fuelingFixture(t)
	d := x.syncFixtureState
	op := x.op(t, d, x.declaredNow(t), nil)
	failingStore, err := fuelpg.NewIntakeHandler(x.authority, x.meters, fuelingFailingStore{x.store}, x.expenses)
	if err != nil {
		t.Fatal(err)
	}
	failingReadings, err := fuelpg.NewIntakeHandler(x.authority, fuelingFailingReadings{x.meters}, x.store, x.expenses)
	if err != nil {
		t.Fatal(err)
	}
	for _, handler := range []syncapp.Handler{failingStore, failingReadings} {
		ack, err := x.fuelings.Process(t.Context(), d.source, op, protocol.DefaultLimits(), handler)
		x.unavailable(t, op, ack, err)
	}
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 0, 1)
}

// Forced RLS, organization isolation and insert-only history for the
// ordinary runtime role.
func TestFuelingSchemaIsForcedRLSAndInsertOnly(t *testing.T) {
	x := fuelingFixture(t)
	d := x.syncFixtureState
	op := x.op(t, d, x.declaredNow(t), nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 0, 1)
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*)=3 AND bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid=ANY(ARRAY['org_fuelings'::regclass,'org_fueling_warnings'::regclass,'org_fueling_review_entries'::regclass])`).Scan(&forced); err != nil || !forced {
		t.Fatal("fueling tables are not forced-RLS", err)
	}
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_fuelings)+(SELECT count(*) FROM org_fueling_warnings)+(SELECT count(*) FROM org_fueling_review_entries)`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("fueling rows visible without an organization setting", visible, err)
	}
	if _, err := x.f.Runtime.Exec(t.Context(), `INSERT INTO org_fueling_review_entries(org_id,fueling_id,sequence,kind,revision,state,reason,actor_membership_id,operation_id) SELECT $1,$2,1,'SUBMIT',1,'SUBMITTED','',$3,$4`, x.scope.OrgID().String(), op.Snapshot().Entity.ID, d.source.MembershipID, op.ID()); err == nil {
		t.Fatal("insert accepted without an organization setting")
	}
	other := foreignAsset(t, x)
	var otherOrg string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT org_id::text FROM org_assets WHERE id=$1`, other).Scan(&otherOrg); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, scopeFor(t, otherOrg), func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_fuelings`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatal("fueling visible to another organization", visible, err)
	}
	var runtimeRole string
	if err := x.f.Runtime.QueryRow(t.Context(), "SELECT current_user").Scan(&runtimeRole); err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{runtimeRole}.Sanitize()
	// EMP-EXP-002: TRUNCATE ... CASCADE also reaches org_expenses, which keeps
	// its own insert-only guard. Document links and card reconciliation also
	// keep separate guards on the cascade; grant TRUNCATE to reach those guards.
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_fuelings,org_fueling_warnings,org_fueling_review_entries,org_expenses,org_document_links TO "+role); err != nil {
		t.Fatal(err)
	}
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT TRUNCATE ON org_card_statement_lines TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`UPDATE org_fuelings SET paid_centavos=97350`, `DELETE FROM org_fuelings`, `TRUNCATE org_fuelings CASCADE`, `UPDATE org_fueling_review_entries SET state='SUBMITTED'`, `DELETE FROM org_fueling_warnings`, `TRUNCATE org_fueling_review_entries`} {
		err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), statement)
			return err
		})
		var pe interface{ SQLState() string }
		if !errors.As(err, &pe) || pe.SQLState() != "55000" {
			t.Fatal("fueling history accepted ordinary maintenance", statement, err)
		}
	}
	x.accepted(t, op, ack, nil, 0, 1)
}

// The default enterprise runtime registers the real fueling.submit owner: a
// signed TLS request records the fueling with its reading, warnings and its
// one expense (EMP-EXP-002), and a resend returns the same canonical ACK
// (EDR-175).
func TestFuelingIntakeDefaultRuntimeSignedTLSAcceptsAndReplays(t *testing.T) {
	fx := meterIntakeHTTP(t)
	fuelingGrants(t, fx.legalFixtureState)
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	assets := fleetStore(t, fx.legalFixtureState)
	input := fleetCreateInput()
	input.UnitID = unit
	created, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	meters := meterStore(t, fx.legalFixtureState)
	installed, _, err := meterInstall(t, fx.legalFixtureState, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision})); err != nil {
		t.Fatal(err)
	}
	op := syncHTTPOperation(t, fx.deviceFixtureState, fx.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = fuelapp.OperationType
		raw["entity"] = map[string]any{"type": fuelapp.EntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		raw["payload"] = map[string]any{"asset_id": created.AssetID, "responsible_employee_id": fx.person.employee, "product": "DIESEL_S10", "unit": "L",
			"quantity": "150000", "paid_total": "97000", "discount": nil, "unit_price": "6490", "unit_price_text": nil, "fill": "PARTIAL", "payer": "COMPANY", "payment_method": "CASH",
			"reading":                map[string]any{"meter_id": installed.MeterID, "segment_id": installed.SegmentID, "reading_id": onboardingID(t), "kind": "ODOMETER", "value": "42"},
			"missing_reading_reason": nil, "place": nil, "trip_id": nil, "contracted_supplier_id": nil}
	})
	ack := fx.dispatch(t, op, 200)
	if ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 {
		t.Fatal("default runtime did not accept the signed fueling", ack.Result(), ack.Code())
	}
	replay := fx.dispatch(t, op, 200)
	if !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
		t.Fatal("default runtime resend changed the ACK")
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*)=1 AND bool_and(f.paid_centavos=97000 AND f.unit_id=$3
 AND(SELECT array_agg(code ORDER BY code) FROM org_fueling_warnings w WHERE w.org_id=f.org_id AND w.fueling_id=f.id)=ARRAY['ARITHMETIC_MISMATCH','PRODUCT_NOT_ACCEPTED']
 AND EXISTS(SELECT 1 FROM org_meter_readings r WHERE r.org_id=f.org_id AND r.id=f.reading_id AND r.origin='FUELING' AND r.value=42)
 AND(SELECT count(*) FROM org_expenses x WHERE x.org_id=f.org_id AND x.fueling_id=f.id AND x.origin_kind='FUELING' AND x.category='FUEL' AND x.paid_centavos=97000 AND x.company_centavos=97000)=1)
 FROM org_fuelings f WHERE f.org_id=$1 AND f.operation_id=$2`, fx.scope.OrgID().String(), op.ID(), unit).Scan(&exact); err != nil || !exact {
		t.Fatal("default runtime fueling not stored exactly once", err)
	}
}
