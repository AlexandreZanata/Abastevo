//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	maintpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/adapters"
	maintapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// incidentState is the EMP-TRIP-003 fixture (trip owners, logbook, the SELF
// intake member and an ACTIVE vehicle allocated to a unit) plus the real
// check.incident.report owner on the same intake (EMP-CHECK-003).
type incidentState struct {
	eventState
	reports *maintpg.IncidentStore
	report  syncapp.Handler
}

// incidentGrants are the runtime privileges of the EMP-CHECK-003 owners
// (README proposal in the evidence): the reports and their history, the
// columns the step trigger updates and the rows the guards re-prove.
func incidentGrants(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_incident_reports,org_incident_report_transitions TO "+role+
		";GRANT UPDATE(state,revision,updated_at) ON org_incident_reports TO "+role+
		";GRANT SELECT ON org_trips,org_trip_finishes,org_asset_unit_periods,org_asset_cost_centre_periods,org_memberships,org_employee_identities,org_device_enrollments,org_sync_operations,org_document_links,org_documents TO "+role); err != nil {
		t.Fatal(err)
	}
}

func incidentIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := eventIntakeConfig(fx)
	config.NoAsyncPlans[syncapp.OperationKey{Type: maintapp.ReportOperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup}
	return config
}

func incidentFixtureOn(t *testing.T, e eventState) incidentState {
	t.Helper()
	incidentGrants(t, e.f)
	x := incidentState{eventState: e, reports: &maintpg.IncidentStore{}}
	x.events.Reports = x.reports
	meterAuthority, ok := x.meters.Authority.(*orgpg.MeterAuthority)
	if !ok {
		t.Fatal("fixture meter authority is not the real one")
	}
	var err error
	if x.report, err = maintpg.NewReportHandler((*orgpg.ReportAuthority)(meterAuthority), x.meters, x.assets, x.store, x.reports); err != nil {
		t.Fatal(err)
	}
	x.trips = syncpg.NewStore(incidentIntakeConfig(x.syncFixtureState))
	return x
}

func incidentFixture(t *testing.T) incidentState { return incidentFixtureOn(t, eventFixture(t)) }

func (x incidentState) process(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	if op.Snapshot().Type == maintapp.ReportOperationType {
		return x.trips.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.report)
	}
	return x.eventState.process(t, d, op)
}

// damagePayload is a synthetic damage of the asset, optionally inside a trip.
func damagePayload(asset string, trip any) map[string]any {
	return map[string]any{"report_type": "DAMAGE", "incident_kind": nil, "asset_id": asset, "trip_id": trip, "description": "Lanterna traseira trincada ao manobrar na doca",
		"location": "Lanterna traseira esquerda", "blocks_use": false}
}

// reportOp is member d's check.incident.report of a new report.
func (x incidentState) reportOp(t *testing.T, d syncFixtureState, payload map[string]any, declared time.Time, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = maintapp.ReportOperationType
		raw["entity"] = map[string]any{"type": maintapp.ReportEntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(declared)
		raw["entry_zone"] = "America/Cuiaba"
		raw["device_offset_seconds"] = -14400
		raw["payload"] = payload
		if edit != nil {
			edit(raw, payload)
		}
	})
}

func reportOf(op protocol.Operation) string { return op.Snapshot().Entity.ID }

type reportCounts struct{ reports, effects, claims, receipts, audits int }

func (x incidentState) countReport(t *testing.T, op protocol.Operation) reportCounts {
	t.Helper()
	var c reportCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_incident_reports WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&c.reports, &c.effects, &c.claims, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x incidentState) reportAccepted(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 || len(ack.Warnings()) != 0 {
		t.Fatal("legitimate report was not accepted", err, ack.Result(), ack.Code())
	}
	if c := x.countReport(t, op); c != (reportCounts{reports: 1, effects: 1, claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("accepted report not stored exactly once: %+v", c)
	}
}

func (x incidentState) reportMalformed(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
		t.Fatal("expected a final MALFORMED receipt", err, ack.Result(), ack.Code())
	}
	if c := x.countReport(t, op); c != (reportCounts{claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("malformed report stored a fact: %+v", c)
	}
}

func (x incidentState) reportNothing(t *testing.T, op protocol.Operation) {
	t.Helper()
	if c := x.countReport(t, op); c != (reportCounts{}) {
		t.Fatalf("refused report left state: %+v", c)
	}
}

func (x incidentState) reportRefused(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, want orgapi.Code) {
	t.Helper()
	if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
		t.Fatalf("expected %s refusal, got %v", want, err)
	}
	x.reportNothing(t, op)
}

// costSnapshot counts every row of every organization table that could hold
// a cost, a payable, a budget effect, an allocation or a fueling, and the
// document rows (BR-CHECK-018: a report books nothing).
func (x incidentState) costSnapshot(t *testing.T) map[string]int64 {
	t.Helper()
	rows, err := x.f.Admin.Query(t.Context(), `SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE'
 AND table_name ~ '^org_.*(expense|payable|budget|fueling|reconcil|allocation_line|cost_entr|charge)' ORDER BY table_name`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	rows.Close()
	names = append(names, "org_documents", "org_document_links")
	out := map[string]int64{}
	for _, name := range names {
		var n int64
		if err := x.f.Admin.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{name}.Sanitize()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		out[name] = n
	}
	if len(out) < 3 {
		t.Fatal("cost snapshot found no cost table", out)
	}
	return out
}

func sameCounts(a, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// foreignAsset plants an ACTIVE asset of another organization.
func (x incidentState) foreignAsset(t *testing.T) string {
	t.Helper()
	other, _, err := x.onboarding.Create(t.Context(), x.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	id := onboardingID(t)
	if err := pgx.BeginFunc(t.Context(), x.f.Admin, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role=replica`); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO org_assets(org_id,id,kind,category_id,name,fleet_number,fleet_key,usage_basis,state,revision) VALUES($1,$2,'VEHICLE',$3,'Foreign vehicle','FRN-1','FRN1','DISTANCE','ACTIVE',1)`,
			other.OrgID.String(), id, onboardingID(t))
		return err
	}); err != nil {
		t.Fatal("foreign asset fixture", err)
	}
	return id
}

// BUC-EMP-186 / B-BR-EMP-369/370 on real PostGIS through the real owner: an
// accepted report stores exactly the declared fact with the allocation at the
// declared instant and the reporter's identity; resends replay byte for byte,
// another digest is OPERATION_ID_REUSED; no cost or document row appears.
func TestIncidentReportIntakeStoresTheFactAndReplays(t *testing.T) {
	x := incidentFixture(t)
	d := x.syncFixtureState
	before := x.costSnapshot(t)
	at := x.declaredNow(t)
	damage := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, nil), at, nil)
	ack, err := x.process(t, d, damage)
	x.reportAccepted(t, damage, ack, err)
	org := x.scope.OrgID().String()
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT r.report_type='DAMAGE' AND r.incident_kind IS NULL AND r.asset_id=$3 AND r.unit_id=$4 AND r.cost_centre_id IS NULL AND r.trip_id IS NULL
 AND r.description='Lanterna traseira trincada ao manobrar na doca' AND r.location='Lanterna traseira esquerda' AND NOT r.blocks_use AND r.reporter_membership_id=$5
 AND r.reporter_employee_id=$6 AND r.reporter_account_id=$7 AND r.enrollment_id=$8 AND r.payload_digest=$9 AND r.occurred_submicro=123 AND r.entry_zone='America/Cuiaba'
 AND r.device_offset_seconds=-14400 AND r.state='REPORTED' AND r.revision=1 AND r.updated_at=r.received_at
 AND r.received_at<=(SELECT received_at FROM org_sync_acknowledgements a WHERE a.org_id=r.org_id AND a.operation_id=r.operation_id)
 FROM org_incident_reports r WHERE r.org_id=$1 AND r.id=$2`, org, reportOf(damage), x.vehicle.AssetID, x.unit, d.source.MembershipID, d.person.employee,
		d.source.Actor.AccountID, d.source.EnrollmentID, damage.PayloadDigest()).Scan(&exact); err != nil || !exact {
		t.Fatal("stored report is not the exact declared fact", err)
	}
	for range 2 {
		replay, err := x.process(t, d, damage)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK", err)
		}
	}
	x.reportAccepted(t, damage, ack, nil)
	reused := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, nil), at, func(raw, payload map[string]any) {
		raw["operation_id"], raw["entity"] = damage.ID(), map[string]any{"type": maintapp.ReportEntityType, "id": reportOf(damage), "expected_revision": nil}
		payload["description"] = "Outro texto"
	})
	if refusal, err := x.process(t, d, reused); err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same operation was not refused", err, refusal.Code())
	}
	x.reportAccepted(t, damage, ack, nil)
	incident := damagePayload(x.vehicle.AssetID, nil)
	incident["report_type"], incident["incident_kind"], incident["location"], incident["blocks_use"] = "INCIDENT", "BREAKDOWN", nil, true
	breakdown := x.reportOp(t, d, incident, at.Add(time.Minute), nil)
	ack, err = x.process(t, d, breakdown)
	x.reportAccepted(t, breakdown, ack, err)
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT incident_kind='BREAKDOWN' AND location IS NULL AND blocks_use FROM org_incident_reports WHERE org_id=$1 AND id=$2`, org, reportOf(breakdown)).Scan(&exact); err != nil || !exact {
		t.Fatal("incident report columns", err)
	}
	// BR-CHECK-018: no report books a cost, a document, an allocation or a fueling.
	if after := x.costSnapshot(t); !sameCounts(before, after) {
		t.Fatal("a report changed cost or document rows", before, after)
	}
}

// B-BR-EMP-370 refusals: nothing is stored or revealed.
func TestIncidentReportIntakeRefusals(t *testing.T) {
	x := incidentFixture(t)
	d := x.syncFixtureState
	at := x.declaredNow(t)
	accepted := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, nil), at, nil)
	ack, err := x.process(t, d, accepted)
	x.reportAccepted(t, accepted, ack, err)
	t.Run("foreign-asset-not-found", func(t *testing.T) {
		op := x.reportOp(t, d, damagePayload(x.foreignAsset(t), nil), at, nil)
		ack, err := x.process(t, d, op)
		x.reportRefused(t, op, ack, err, orgapi.NotFound)
	})
	t.Run("unknown-asset-not-found", func(t *testing.T) {
		op := x.reportOp(t, d, damagePayload(onboardingID(t), nil), at, nil)
		ack, err := x.process(t, d, op)
		x.reportRefused(t, op, ack, err, orgapi.NotFound)
	})
	t.Run("unassigned-self-driver-forbidden", func(t *testing.T) {
		driver := x.driver(t)
		op := x.reportOp(t, driver, damagePayload(x.vehicle.AssetID, nil), x.declaredNow(t), nil)
		ack, err := x.process(t, driver, op)
		x.reportRefused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("assigned-self-driver-accepted", func(t *testing.T) {
		driver := x.driver(t)
		x.assign(t, driver.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
		op := x.reportOp(t, driver, damagePayload(x.vehicle.AssetID, nil), x.declaredNow(t), nil)
		ack, err := x.process(t, driver, op)
		x.reportAccepted(t, op, ack, err)
	})
	t.Run("payload-malformed", func(t *testing.T) {
		for name, edit := range map[string]func(raw, p map[string]any){
			"damage-with-kind":      func(_, p map[string]any) { p["incident_kind"] = "ACCIDENT" },
			"incident-without-kind": func(_, p map[string]any) { p["report_type"] = "INCIDENT" },
			"empty-kind":            func(_, p map[string]any) { p["report_type"], p["incident_kind"] = "INCIDENT", "" },
			"unknown-type":          func(_, p map[string]any) { p["report_type"] = "FIRE" },
			"blank-description":     func(_, p map[string]any) { p["description"] = "   " },
			"newline-description":   func(_, p map[string]any) { p["description"] = "linha\nnova" },
			"long-description":      func(_, p map[string]any) { p["description"] = string(bytes.Repeat([]byte("a"), 1001)) },
			"empty-location":        func(_, p map[string]any) { p["location"] = "" },
			"long-location":         func(_, p map[string]any) { p["location"] = string(bytes.Repeat([]byte("b"), 141)) },
			"blocks-not-boolean":    func(_, p map[string]any) { p["blocks_use"] = "no" },
			"extra-key":             func(_, p map[string]any) { p["amount_centavos"] = "100" },
			"health-key":            func(_, p map[string]any) { p["injuries"] = "none" },
			"missing-trip-key":      func(_, p map[string]any) { delete(p, "trip_id") },
			"asset-not-uuid":        func(_, p map[string]any) { p["asset_id"] = "asset" },
			"expected-revision": func(raw, _ map[string]any) {
				raw["entity"] = map[string]any{"type": maintapp.ReportEntityType, "id": onboardingID(t), "expected_revision": 1}
			},
		} {
			op := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, nil), at, edit)
			ack, err := x.process(t, d, op)
			if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
				t.Fatal("malformed report", name, err, ack.Code())
			}
			x.reportMalformed(t, op, ack, err)
		}
	})
	t.Run("identity-reused-malformed", func(t *testing.T) {
		op := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, nil), at, func(raw, _ map[string]any) {
			raw["entity"] = map[string]any{"type": maintapp.ReportEntityType, "id": reportOf(accepted), "expected_revision": nil}
		})
		ack, err := x.process(t, d, op)
		x.reportMalformed(t, op, ack, err)
	})
	t.Run("draft-asset-malformed", func(t *testing.T) {
		input := fleetCreateInput()
		input.UnitID = x.unit
		draft, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, input))
		if err != nil {
			t.Fatal(err)
		}
		op := x.reportOp(t, d, damagePayload(draft.AssetID, nil), at, nil)
		ack, err := x.process(t, d, op)
		x.reportMalformed(t, op, ack, err)
	})
	t.Run("attachment-declaration-unavailable", func(t *testing.T) {
		op := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, nil), at, func(raw, _ map[string]any) {
			raw["attachments"] = []any{map[string]any{"id": onboardingID(t), "sha256": string(bytes.Repeat([]byte("a"), 64)), "bytes": 10, "mime": "image/jpeg"}}
		})
		ack, err := x.process(t, d, op)
		x.unavailable(t, op, ack, err)
		x.reportNothing(t, op)
	})
}

// B-BR-EMP-371 / EDR-240: inside a trip the report is an INCIDENT or DAMAGE
// timeline fact; the trip rules refuse a foreign, another member's, another
// asset's or an outside-window trip and never wait on a trip forever.
func TestIncidentReportInsideATripIsATimelineFact(t *testing.T) {
	x := incidentFixture(t)
	d := x.syncFixtureState
	trip, started := x.startTrip(t, d)
	damage := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, trip), started.Add(30*time.Minute), nil)
	ack, err := x.process(t, d, damage)
	x.reportAccepted(t, damage, ack, err)
	incident := damagePayload(x.vehicle.AssetID, trip)
	incident["report_type"], incident["incident_kind"], incident["description"] = "INCIDENT", "ACCIDENT", "Colisao leve no patio sem feridos"
	accident := x.reportOp(t, d, incident, started.Add(time.Hour), nil)
	ack, err = x.process(t, d, accident)
	x.reportAccepted(t, accident, ack, err)
	note := x.eventOp(t, d, trip, "NOTE", "Seguindo viagem", started.Add(45*time.Minute), nil)
	noteAck, err := x.process(t, d, note)
	x.accepted(t, note, noteAck, err)
	t.Run("unknown-trip-pending", func(t *testing.T) {
		op := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, onboardingID(t)), started.Add(time.Minute), nil)
		ack, err := x.process(t, d, op)
		if err != nil || ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
			t.Fatal("expected RETRY/DEPENDENCY_PENDING", err, ack.Result(), ack.Code())
		}
		x.reportNothing(t, op)
	})
	t.Run("foreign-trip-pending", func(t *testing.T) {
		foreign := x.foreignTrip(t)
		op := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, foreign), started.Add(time.Minute), nil)
		ack, err := x.process(t, d, op)
		if err != nil || ack.Result() != protocol.Retry || ack.Code() != "DEPENDENCY_PENDING" {
			t.Fatal("expected RETRY/DEPENDENCY_PENDING", err, ack.Result(), ack.Code())
		}
		x.reportNothing(t, op)
		var n int
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_incident_reports WHERE trip_id=$1`, foreign).Scan(&n); err != nil || n != 0 {
			t.Fatal("a report reached another organization's trip", n, err)
		}
	})
	t.Run("before-start-malformed", func(t *testing.T) {
		op := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, trip), started.Add(-time.Nanosecond), nil)
		ack, err := x.process(t, d, op)
		x.reportMalformed(t, op, ack, err)
	})
	t.Run("another-asset-trip-malformed", func(t *testing.T) {
		input := fleetCreateInput()
		input.UnitID = x.unit
		created, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, input))
		if err != nil {
			t.Fatal(err)
		}
		active, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: 1}))
		if err != nil || active.State != fleet.Active {
			t.Fatal("second asset not active", err)
		}
		op := x.reportOp(t, d, damagePayload(created.AssetID, trip), started.Add(time.Minute), nil)
		ack, err := x.process(t, d, op)
		x.reportMalformed(t, op, ack, err)
	})
	t.Run("another-member-trip-forbidden", func(t *testing.T) {
		driver := x.driver(t)
		x.assign(t, driver.person.employee, x.vehicle.AssetID, fleet.AdditionalAssignment, nil, nil)
		op := x.reportOp(t, driver, damagePayload(x.vehicle.AssetID, trip), x.declaredNow(t), nil)
		ack, err := x.process(t, driver, op)
		x.reportRefused(t, op, ack, err, orgapi.Forbidden)
	})
	finished := started.Add(2 * time.Hour)
	x.finishTrip(t, d, trip, finished)
	atFinish := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, trip), finished, nil)
	ack, err = x.process(t, d, atFinish)
	x.reportAccepted(t, atFinish, ack, err)
	t.Run("after-finish-malformed", func(t *testing.T) {
		op := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, trip), finished.Add(time.Nanosecond), nil)
		ack, err := x.process(t, d, op)
		x.reportMalformed(t, op, ack, err)
	})
	entries := x.timeline(t, trip)
	want := []struct{ kind, id string }{{"START", trip}, {"DAMAGE", reportOf(damage)}, {"NOTE", eventOf(note)}, {"INCIDENT", reportOf(accident)}, {"DAMAGE", reportOf(atFinish)}, {"FINISH", trip}}
	if len(entries) != len(want) {
		t.Fatal("timeline length", len(entries))
	}
	for i, w := range want {
		e := entries[i]
		if e.FactType != w.kind || e.Item.ID != w.id || e.AuthorMembershipID != d.source.MembershipID || e.ReceivedAt.IsZero() || e.Declared.EntryZone != "America/Cuiaba" {
			t.Fatal("timeline item", i, e.FactType, e.Item.ID == w.id)
		}
		if (w.kind == "DAMAGE" || w.kind == "INCIDENT") && e.Text != nil {
			t.Fatal("a report's description reached the timeline")
		}
	}
	// The report is read from its owner: no logbook row was written for it.
	var logbook int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trip_events WHERE org_id=$1 AND trip_id=$2 AND event_type IN('INCIDENT','DAMAGE')`, x.scope.OrgID().String(), trip).Scan(&logbook); err != nil || logbook != 0 {
		t.Fatal("a report was copied into the logbook", logbook, err)
	}
	if x.stateOf(t, trip) != "COMPLETED" {
		t.Fatal("a report changed the trip state")
	}
}

// Twelve identical concurrent deliveries leave one report and one receipt;
// an injected store failure commits nothing and the same operation then
// commits.
func TestIncidentReportConcurrentDuplicatesAndRollback(t *testing.T) {
	x := incidentFixture(t)
	d := x.syncFixtureState
	op := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, nil), x.declaredNow(t), nil)
	var wg sync.WaitGroup
	start := make(chan struct{})
	acks := make([]protocol.Acknowledgement, 12)
	errs := make([]error, 12)
	for i := range acks {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			acks[i], errs[i] = x.trips.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.report)
		}(i)
	}
	close(start)
	wg.Wait()
	var first []byte
	for i := range acks {
		if errs[i] != nil {
			continue
		}
		if first == nil {
			first = acks[i].CanonicalBytes()
		} else if !bytes.Equal(first, acks[i].CanonicalBytes()) {
			t.Fatal("concurrent duplicates returned different ACKs")
		}
	}
	if first == nil {
		t.Fatal("no concurrent delivery succeeded", errs)
	}
	ack, err := x.process(t, d, op)
	x.reportAccepted(t, op, ack, err)
	failing := x.reportOp(t, d, damagePayload(x.vehicle.AssetID, nil), x.declaredNow(t), nil)
	meterAuthority := x.meters.Authority.(*orgpg.MeterAuthority)
	broken, err := maintpg.NewReportHandler((*orgpg.ReportAuthority)(meterAuthority), x.meters, x.assets, x.store, incidentFailingStore{x.reports})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.trips.Process(t.Context(), d.source, failing, protocol.DefaultLimits(), broken); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("injected store failure was not unavailable", err)
	}
	x.reportNothing(t, failing)
	ack, err = x.process(t, d, failing)
	x.reportAccepted(t, failing, ack, err)
}

// The report and its fact commit together or not at all.
type incidentFailingStore struct{ *maintpg.IncidentStore }

func (s incidentFailingStore) InsertReportTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, maintapp.ReportCommand, maintapp.ReportFacts) error {
	return errors.New("injected report failure")
}
