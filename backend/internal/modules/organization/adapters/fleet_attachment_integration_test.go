//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// attachmentStore is the reservation store plus the exact EMP-FLEET-011 grants.
func attachmentStore(t *testing.T, fx legalFixtureState) *fleetpg.AssetStore {
	t.Helper()
	s := reservationStore(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_asset_implement_attachments TO "+role+";GRANT SELECT,INSERT ON org_asset_attachment_commands TO "+role); err != nil {
		t.Fatal(err)
	}
	return s
}

// poweredAsset registers an ACTIVE VEHICLE or MACHINE with one tank and an
// installed odometer through the actual asset and meter stores.
func poweredAsset(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, ms *fleetpg.MeterStore, unit string, kind fleet.Kind) string {
	t.Helper()
	input := fleetCreateInput()
	input.Profile.Kind, input.Profile.UsageBasis, input.UnitID = kind, fleet.Distance, unit
	input.Profile.SerialNumber = "SYNTHETIC-POWERED-" + onboardingID(t)[:8]
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	input.Profile.Tanks = []fleet.Tank{tank}
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	installed, _, err := meterInstall(t, fx, ms, meterInstallCommand(t, meterInstallInput(t, created.AssetID, lifecycleDBNow(t, fx).Add(-time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision})); err != nil {
		t.Fatal("powered asset activation", err)
	}
	return created.AssetID
}

func attach(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, v fa.AttachmentCreateInput) (fa.AttachmentResult, bool, error) {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := fa.NewAttachmentCreate(v)
	if err != nil {
		t.Fatal("invalid fixture attachment", err)
	}
	return s.AttachImplement(t.Context(), fx.scope, fleetActor(fx), fx.member, onboardingID(t), c)
}

func detach(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, v fa.AttachmentDetachInput) error {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := fa.NewAttachmentDetach(v)
	if err != nil {
		t.Fatal("invalid fixture detach", err)
	}
	_, _, err = s.DetachImplement(t.Context(), fx.scope, fleetActor(fx), fx.member, c)
	return err
}

func onOrgTx[T any](t *testing.T, fx legalFixtureState, call func(syncapp.DBTX) (T, error)) T {
	t.Helper()
	var out T
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		var err error
		out, err = call(syncapp.Borrow(tx))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// tripSource answers one trip in progress on one asset: a test stand-in for
// the EMP-TRIP-002 source that will replace NoActiveUse.
type tripSource struct{ asset, trip string }

func (s tripSource) ActiveUseDBTX(_ context.Context, _ syncapp.DBTX, _ kernel.OrgScope, asset string, at time.Time) (fa.UseAnswer, error) {
	if asset == s.asset {
		return fa.UseAnswer{Sourced: true, Uses: []fleet.UseFact{{ID: s.trip, From: at.Add(-time.Hour)}}}, nil
	}
	return fa.UseAnswer{Sourced: true}, nil
}

// AC1, AC2 (port), AC3, kinds, references and detach rules (B-BR-EMP-325..328).
func TestFleetImplementAttachmentsOverlapPortAndAvailability(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := attachmentStore(t, fx)
	ms := meterStore(t, fx)
	org := fx.scope.OrgID().String()
	harrow, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	sprayer, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	tractor := poweredAsset(t, fx, s, ms, unit, fleet.Machine)
	truck := poweredAsset(t, fx, s, ms, unit, fleet.Vehicle)
	input := fa.AttachmentCreateInput{OperationID: onboardingID(t), ImplementID: harrow, PoweredAssetID: tractor}
	first, replayed, err := attach(t, fx, s, input)
	if err != nil || replayed || first.AssetID != harrow {
		t.Fatal("attach", err)
	}
	if again, replayed, err := attach(t, fx, s, input); err != nil || !replayed || again != first {
		t.Fatal("attach replay", err)
	}
	other := input
	other.PoweredAssetID = truck
	if _, _, err := attach(t, fx, s, other); !errors.Is(err, fa.ErrOperationReused) {
		t.Fatal("same operation, other intent", err)
	}
	// AC1: an overlapping attachment of the same implement is refused whole.
	audits := func() int {
		return assignmentCount(t, fx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1`, org)
	}
	before := audits()
	if _, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: harrow, PoweredAssetID: truck}); !errors.Is(err, fa.ErrAttachmentOverlap) {
		t.Fatal("AC1 overlapping attachment", err)
	}
	if before != audits() || assignmentCount(t, fx, `SELECT count(*) FROM org_asset_implement_attachments WHERE org_id=$1`, org) != 1 {
		t.Fatal("refused attachment left rows or audit")
	}
	// A powered asset may carry several implements at once.
	if _, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: sprayer, PoweredAssetID: tractor}); err != nil {
		t.Fatal("second implement on the tractor", err)
	}
	now := exactNow(t, fx)
	// AC2 through the port: the implements attached to the powered asset.
	listed := onOrgTx(t, fx, func(db syncapp.DBTX) ([]string, error) {
		return s.AttachedImplementsDBTX(t.Context(), db, fx.scope, tractor, now)
	})
	want := []string{harrow, sprayer}
	slices.Sort(want)
	if !slices.Equal(listed, want) {
		t.Fatal("AC2 implements attached at the instant", listed)
	}
	if powered := onOrgTx(t, fx, func(db syncapp.DBTX) (string, error) {
		p, ok, err := s.PoweredAssetDBTX(t.Context(), db, fx.scope, harrow, now)
		if !ok {
			return "", err
		}
		return p, err
	}); powered != tractor {
		t.Fatal("powered asset of the harrow", powered)
	}
	// Detach later (planned) and attach elsewhere from that instant.
	switchAt := now.Add(time.Hour)
	if err := detach(t, fx, s, fa.AttachmentDetachInput{AssetID: tractor, AttachmentID: first.AttachmentID, End: &switchAt, Reason: "Synthetic implement swap"}); err != nil {
		t.Fatal("planned detach through the powered asset", err)
	}
	if _, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: harrow, PoweredAssetID: truck, Start: ptrTime(switchAt.Add(-time.Minute))}); !errors.Is(err, fa.ErrAttachmentOverlap) {
		t.Fatal("overlap with the planned end", err)
	}
	moved, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: harrow, PoweredAssetID: truck, Start: &switchAt})
	if err != nil {
		t.Fatal("attachment from the switch instant", err)
	}
	later := onOrgTx(t, fx, func(db syncapp.DBTX) ([]string, error) {
		return s.AttachedImplementsDBTX(t.Context(), db, fx.scope, truck, switchAt)
	})
	if !slices.Equal(later, []string{harrow}) {
		t.Fatal("implements of the truck after the switch", later)
	}
	if before := onOrgTx(t, fx, func(db syncapp.DBTX) ([]string, error) {
		return s.AttachedImplementsDBTX(t.Context(), db, fx.scope, truck, switchAt.Add(-time.Microsecond))
	}); len(before) != 0 {
		t.Fatal("half-open attachment period", before)
	}
	// Detach before the attach instant, in the past, or after the planned end.
	for name, end := range map[string]time.Time{"before the attach instant": switchAt.Add(-time.Minute), "in the past": now.Add(-time.Minute)} {
		if err := detach(t, fx, s, fa.AttachmentDetachInput{AssetID: harrow, AttachmentID: moved.AttachmentID, End: &end}); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("detach", name, err)
		}
	}
	if err := detach(t, fx, s, fa.AttachmentDetachInput{AssetID: harrow, AttachmentID: moved.AttachmentID}); err != nil {
		t.Fatal("detach a scheduled attachment", err)
	}
	if err := detach(t, fx, s, fa.AttachmentDetachInput{AssetID: harrow, AttachmentID: moved.AttachmentID}); !errors.Is(err, fleet.ErrAttachmentEnded) {
		t.Fatal("detach an ended attachment", err)
	}
	if err := detach(t, fx, s, fa.AttachmentDetachInput{AssetID: sprayer, AttachmentID: first.AttachmentID}); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("detach through an unrelated asset", err)
	}
	// Kinds: attaching to an implement or attaching a powered asset refused.
	if _, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: sprayer, PoweredAssetID: harrow}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("attaching to an implement", err)
	}
	if _, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: truck, PoweredAssetID: tractor}); !errors.Is(err, fleet.ErrInvalid) {
		t.Fatal("a vehicle attached as an implement", err)
	}
	for _, state := range []fleet.State{fleet.Draft, fleet.Disposed} {
		id, _ := lifecycleAssetIn(t, fx, s, unit, state)
		if _, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: id, PoweredAssetID: tractor}); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("implement out of service", state, err)
		}
	}
	// Another organization's assets are not found.
	foreignFx := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := foreignFx.onboarding.Repository.ActivateOrganization(t.Context(), foreignFx.scope, foreignFx.actor, foreignFx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var foreignUnit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, foreignFx.scope.OrgID().String()).Scan(&foreignUnit); err != nil {
		t.Fatal(err)
	}
	foreignImplement, _ := lifecycleAssetIn(t, foreignFx, s, foreignUnit, fleet.Active)
	if _, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: sprayer, PoweredAssetID: foreignImplement}); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("foreign powered asset", err)
	}
	if _, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: foreignImplement, PoweredAssetID: tractor}); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("foreign implement", err)
	}
	// BR-FLEET-002: an attachment fixes the kind of both assets.
	allocationWant(t, "implement kind after an attachment", allocationExec(t, fx, `UPDATE org_assets SET kind='MACHINE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, sprayer), "P7903")
	// AC3: through the active-use port, the attached implement is IN_USE while
	// its powered asset has a trip in progress; without a source it says so.
	read := func(asset string) fa.AvailabilityView {
		t.Helper()
		v, err := s.ReadAvailability(t.Context(), fx.scope, fleetActor(fx), fx.member, asset)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	if v := read(sprayer); v.Assessment.Availability != fleet.Available || len(v.MissingSources) != 2 {
		t.Fatal("no active-use source", v)
	}
	trip := onboardingID(t)
	withTrips := *s
	withTrips.ActiveUse = tripSource{asset: tractor, trip: trip}
	v, err := withTrips.ReadAvailability(t.Context(), fx.scope, fleetActor(fx), fx.member, sprayer)
	if err != nil || v.Assessment.Availability != fleet.InUse || len(v.Assessment.Causes) != 1 || v.Assessment.Causes[0].ID != trip || v.Assessment.Causes[0].Via != tractor || len(v.MissingSources) != 1 || v.MissingSources[0] != fa.MaintenanceHoldSource {
		t.Fatal("AC3 implement in use through its powered asset", v, err)
	}
	// The harrow stays attached to the tractor until the planned switch.
	if v, _ := withTrips.ReadAvailability(t.Context(), fx.scope, fleetActor(fx), fx.member, harrow); v.Assessment.Availability != fleet.InUse || v.Assessment.Causes[0].Via != tractor {
		t.Fatal("implement attached until its planned end", v)
	}
	loose, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	if v, _ := withTrips.ReadAvailability(t.Context(), fx.scope, fleetActor(fx), fx.member, loose); v.Assessment.Availability != fleet.Available {
		t.Fatal("an unattached implement follows no powered asset", v)
	}
	if v, _ := withTrips.ReadAvailability(t.Context(), fx.scope, fleetActor(fx), fx.member, tractor); v.Assessment.Availability != fleet.InUse || v.Assessment.Causes[0].Via != "" {
		t.Fatal("powered asset's own use", v)
	}
	page, err := s.ListAttachments(t.Context(), fx.scope, fleetActor(fx), fx.member, tractor, fa.PeriodFilter{Limit: 10})
	if err != nil || len(page.Attachments) != 2 {
		t.Fatal("tractor attachments", err)
	}
	if n := assignmentCount(t, fx, `SELECT count(*) FROM org_asset_attachment_commands WHERE org_id=$1`, org); n != 5 {
		t.Fatal("one receipt per committed command", n)
	}
}

// Two concurrent attachments of one implement, both orders with barriers:
// the first queued commits, the second is refused (AC1 under concurrency).
func TestFleetImplementAttachmentConcurrentRace(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := attachmentStore(t, fx)
	ms := meterStore(t, fx)
	org := fx.scope.OrgID().String()
	tractor := poweredAsset(t, fx, s, ms, unit, fleet.Machine)
	truck := poweredAsset(t, fx, s, ms, unit, fleet.Vehicle)
	for _, order := range [][2]string{{tractor, truck}, {truck, tractor}} {
		implement, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		holder, err := fx.f.Runtime.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// A failed wait releases the held lock and stops the waiters.
		t.Cleanup(func() { _ = holder.Rollback(context.Background()); cancel() })
		if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, org); err != nil {
			t.Fatal(err)
		}
		if _, err := holder.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, org); err != nil {
			t.Fatal(err)
		}
		waitBlocked := func(n int) {
			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				var blocked int
				if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND pg_blocking_pids(pid)<>'{}'`, fx.f.RuntimeRole).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
				if blocked >= n {
					return
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					t.Fatal("no lock wait observed")
				}
			}
		}
		results := [2]chan error{make(chan error, 1), make(chan error, 1)}
		for i, powered := range order {
			c, err := fa.NewAttachmentCreate(fa.AttachmentCreateInput{OperationID: onboardingID(t), ImplementID: implement, PoweredAssetID: powered})
			if err != nil {
				t.Fatal(err)
			}
			id := onboardingID(t)
			go func() {
				_, _, err := s.AttachImplement(ctx, fx.scope, fleetActor(fx), fx.member, id, c)
				results[i] <- err
			}()
			waitBlocked(i + 1)
		}
		if err := holder.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		first, second := <-results[0], <-results[1]
		cancel()
		if first != nil || !errors.Is(second, fa.ErrAttachmentOverlap) {
			t.Fatal("concurrent attachments", first, second)
		}
		var powered string
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT powered_asset_id::text FROM org_asset_implement_attachments WHERE implement_id=$1`, implement).Scan(&powered); err != nil || powered != order[0] {
			t.Fatal("the first queued attachment did not win", powered, err)
		}
	}
}

// Migration 222 guards on the ordinary runtime role: forced RLS, kinds,
// retroactive starts, the exclusion, immutability and append-only history.
func TestFleetImplementAttachmentSchemaGuards(t *testing.T) {
	fx, _, unit := lifecycleFixture(t)
	s := attachmentStore(t, fx)
	ms := meterStore(t, fx)
	org := fx.scope.OrgID().String()
	implement, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	tractor := poweredAsset(t, fx, s, ms, unit, fleet.Machine)
	created, _, err := attach(t, fx, s, fa.AttachmentCreateInput{ImplementID: implement, PoweredAssetID: tractor})
	if err != nil {
		t.Fatal(err)
	}
	var operation, actor string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT operation_id::text,attached_by::text FROM org_asset_implement_attachments WHERE id=$1`, created.AttachmentID).Scan(&operation, &actor); err != nil {
		t.Fatal(err)
	}
	insert := func(implement, powered string, from time.Time) error {
		return allocationExec(t, fx, `INSERT INTO org_asset_implement_attachments(org_id,id,implement_id,powered_asset_id,valid_from,operation_id,attached_by) VALUES($1,$2,$3,$4,$5,$6,$7)`, org, onboardingID(t), implement, powered, from, operation, actor)
	}
	now := exactNow(t, fx)
	allocationWant(t, "retroactive attachment", insert(implement, tractor, now.Add(-time.Hour)), "23514")
	allocationWant(t, "powered asset as implement", insert(tractor, implement, now.Add(time.Hour)), "23514")
	if code, constraint := allocationCode(insert(implement, tractor, now.Add(time.Hour))); code != "23P01" || constraint != "org_asset_implement_attachments_no_overlap" {
		t.Fatal("overlap reached storage", code, constraint)
	}
	allocationWant(t, "same asset twice", insert(implement, implement, now.Add(time.Hour)), "23514")
	allocationWant(t, "powered asset rewritten", allocationExec(t, fx, `UPDATE org_asset_implement_attachments SET powered_asset_id=$2 WHERE id=$1`, created.AttachmentID, implement), "55000")
	allocationWant(t, "detached without its command", allocationExec(t, fx, `UPDATE org_asset_implement_attachments SET valid_to=clock_timestamp()+interval '1 hour' WHERE id=$1`, created.AttachmentID), "23514")
	allocationWant(t, "end moved before the start", allocationExec(t, fx, `UPDATE org_asset_implement_attachments SET valid_to=valid_from-interval '1 second',detach_operation_id=operation_id,detached_by=attached_by WHERE id=$1`, created.AttachmentID), "55000")
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT DELETE ON org_asset_implement_attachments TO "+role+";GRANT UPDATE ON org_asset_attachment_commands TO "+role); err != nil {
		t.Fatal(err)
	}
	allocationWant(t, "attachment delete", allocationExec(t, fx, `DELETE FROM org_asset_implement_attachments WHERE org_id=$1`, org), "55000")
	allocationWant(t, "receipt update", allocationExec(t, fx, `UPDATE org_asset_attachment_commands SET kind='DETACH' WHERE org_id=$1`, org), "55000")
	if err := pgx.BeginTxFunc(t.Context(), fx.f.Runtime, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, table := range []string{"org_asset_implement_attachments", "org_asset_attachment_commands"} {
			var n int
			if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
				t.Fatal(table, "visible without organization setting", n, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	allocationWant(t, "insert into another organization", allocationExec(t, other, `INSERT INTO org_asset_implement_attachments(org_id,id,implement_id,powered_asset_id,valid_from,operation_id,attached_by) VALUES($1,$2,$3,$4,clock_timestamp()+interval '2 hours',$5,$6)`, org, onboardingID(t), implement, tractor, operation, actor), "42501")
}
