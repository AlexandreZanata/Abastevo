//go:build integration

package adapters_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	trippg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/adapters"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/jackc/pgx/v5"
)

// attachmentGrants adds the EMP-FLEET-011 Fleet principal grants to the trip
// fixture (attach/detach and the availability read of an implement).
func (x tripState) attachmentGrants(t *testing.T) {
	t.Helper()
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_asset_implement_attachments TO "+role+";GRANT SELECT,INSERT ON org_asset_attachment_commands TO "+role); err != nil {
		t.Fatal(err)
	}
}

// EMP-FLEET-011 AC4 through the signed intake (EDR-219): an IMPLEMENT is
// never the main asset of a trip. The start is a final MALFORMED with nothing
// stored, even for an assigned driver with a valid reading of its own meter.
func TestTripIntakeRefusesAnImplementAsTheMainAsset(t *testing.T) {
	x := tripFixture(t)
	// The FLEET-007 availability facts of an implement read its attachment.
	x.attachmentGrants(t)
	d := x.syncFixtureState
	// The self-intake fixture asset is an ACTIVE IMPLEMENT with an observed odometer.
	var kind string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT kind FROM org_assets WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), x.installed.AssetID).Scan(&kind); err != nil || kind != string(fleet.Implement) {
		t.Fatal("fixture implement", kind, err)
	}
	x.assign(t, d.person.employee, x.installed.AssetID, fleet.PrimaryAssignment, nil, nil)
	implementReading := func(t *testing.T) map[string]any {
		return map[string]any{"meter_id": x.installed.MeterID, "segment_id": x.installed.SegmentID, "reading_id": onboardingID(t), "kind": "ODOMETER", "value": "1000"}
	}
	declared := x.declaredNow(t)
	for name, edit := range map[string]func(_, p map[string]any){
		"with-reading": func(_, p map[string]any) { p["asset_id"], p["reading"] = x.installed.AssetID, implementReading(t) },
		"missing-reading": func(_, p map[string]any) {
			p["asset_id"], p["reading"], p["missing_reading_reason"] = x.installed.AssetID, nil, "Implemento sem painel"
		},
	} {
		t.Run(name, func(t *testing.T) {
			op := x.startOp(t, d, declared, edit)
			ack, err := x.process(t, d, op)
			x.malformed(t, op, ack, err)
			x.replays(t, d, op, ack)
			x.malformed(t, op, ack, nil)
		})
	}
	var trips int
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_trips WHERE org_id=$1 AND asset_id=$2`, x.scope.OrgID().String(), x.installed.AssetID).Scan(&trips); err != nil || trips != 0 {
		t.Fatal("an implement became the main asset of a trip", trips, err)
	}
	// The same member's start of the vehicle is accepted.
	op := x.startOp(t, d, declared, nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
}

// EMP-FLEET-011 AC2 and AC3 with real signed trips (EDR-219): the implements
// attached to the powered asset at the trip's start instant are listed through
// AttachedImplementsDBTX, and an attached implement reads IN_USE through the
// powered asset's trip while it is in progress and AVAILABLE after its finish.
func TestTripStartListsAttachedImplementsAndCarriesThemInUse(t *testing.T) {
	x := tripFixture(t)
	x.attachmentGrants(t)
	d := x.syncFixtureState
	fx := x.legalFixtureState
	implement := func() string {
		id, _ := lifecycleAssetIn(t, fx, x.assets, x.unit, fleet.Active)
		return id
	}
	harrow, sprayer, dropped, later, loose := implement(), implement(), implement(), implement(), implement()
	for _, id := range []string{harrow, sprayer, dropped} {
		if _, _, err := attach(t, fx, x.assets, fa.AttachmentCreateInput{ImplementID: id, PoweredAssetID: x.vehicle.AssetID}); err != nil {
			t.Fatal("attach", err)
		}
	}
	page, err := x.assets.ListAttachments(t.Context(), x.scope, fleetActor(fx), x.member, dropped, fa.PeriodFilter{Limit: 10})
	if err != nil || len(page.Attachments) != 1 {
		t.Fatal("dropped implement attachment", err)
	}
	if err := detach(t, fx, x.assets, fa.AttachmentDetachInput{AssetID: dropped, AttachmentID: page.Attachments[0].ID, Reason: "Synthetic detach before the trip"}); err != nil {
		t.Fatal("detach before the trip", err)
	}
	waitDBClock(t, fx, exactNow(t, fx).Add(time.Millisecond))
	started := x.declaredNow(t)
	// An implement attached only after the start instant is not listed.
	afterStart := started.Truncate(time.Microsecond).Add(time.Hour)
	if _, _, err := attach(t, fx, x.assets, fa.AttachmentCreateInput{ImplementID: later, PoweredAssetID: x.vehicle.AssetID, Start: &afterStart}); err != nil {
		t.Fatal("planned attachment after the start", err)
	}
	op := x.startOp(t, d, started, nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 1, 1)
	trip := tripOf(op)
	listing := trippg.StartImplements{Trips: x.store, Attachments: x.assets}
	list := func(id string) ([]string, error) {
		t.Helper()
		var ids []string
		err := pgx.BeginFunc(t.Context(), x.f.Runtime, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, x.scope.OrgID().String()); err != nil {
				return err
			}
			var err error
			ids, err = listing.ImplementsAtStartDBTX(t.Context(), syncapp.Borrow(tx), x.scope, id)
			return err
		})
		return ids, err
	}
	read := func(asset string) fa.AvailabilityView {
		t.Helper()
		v, err := x.assets.ReadAvailability(t.Context(), x.scope, fleetActor(fx), x.member, asset)
		if err != nil {
			t.Fatal("availability read", err)
		}
		return v
	}
	onlyMaintenanceMissing := func(v fa.AvailabilityView) bool {
		return len(v.MissingSources) == 1 && v.MissingSources[0] == fa.MaintenanceHoldSource
	}
	want := []string{harrow, sprayer}
	slices.Sort(want)
	// AC3: the attached implements follow the vehicle's trip in progress.
	for _, id := range want {
		v := read(id)
		if v.Assessment.Availability != fleet.InUse || len(v.Assessment.Causes) != 1 || v.Assessment.Causes[0].ID != trip || v.Assessment.Causes[0].Via != x.vehicle.AssetID || !onlyMaintenanceMissing(v) {
			t.Fatal("AC3 attached implement not in use through the powered asset's trip", id, v)
		}
	}
	if v := read(x.vehicle.AssetID); v.Assessment.Availability != fleet.InUse || v.Assessment.Causes[0].ID != trip || v.Assessment.Causes[0].Via != "" {
		t.Fatal("the vehicle's own trip", v)
	}
	for _, id := range []string{dropped, later, loose} {
		if v := read(id); v.Assessment.Availability != fleet.Available || !onlyMaintenanceMissing(v) {
			t.Fatal("an implement not attached now follows no trip", id, v)
		}
	}
	if ids, err := list(trip); err != nil || !slices.Equal(ids, want) {
		t.Error("AC2 implements attached at the trip start", ids, err)
	}
	if ids, err := list(onboardingID(t)); !errors.Is(err, tripapp.ErrNotFound) || ids != nil {
		t.Error("implements of a trip that is not stored here", ids, err)
	}
	end := x.finishOp(t, d, trip, x.declaredNow(t), nil)
	endAck, err := x.process(t, d, end)
	x.accepted(t, end, endAck, err, 2, 1)
	for _, id := range want {
		if v := read(id); v.Assessment.Availability != fleet.Available || !onlyMaintenanceMissing(v) {
			t.Fatal("AC3 implement still in use after the trip finished", id, v)
		}
	}
	// Read-time derivation: the start listing is unchanged after the finish.
	if ids, err := list(trip); err != nil || !slices.Equal(ids, want) {
		t.Error("start listing changed after the finish", ids, err)
	}
}
