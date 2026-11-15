//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// lifecycleStore uses the installed ordinary Fleet grants plus the exact new
// history privileges and the explicit runtime "none" open-work adapter.
func lifecycleStore(t *testing.T, fx legalFixtureState) *fleetpg.AssetStore {
	t.Helper()
	s := fleetStore(t, fx)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_asset_lifecycle_events TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	s.OpenWork = fleetpg.NoOpenWork{}
	return s
}

// lifecycleFixture activates the actual organization and returns its unit.
func lifecycleFixture(t *testing.T) (legalFixtureState, *fleetpg.AssetStore, string) {
	t.Helper()
	fx := legalFixture(t)
	s := lifecycleStore(t, fx)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal("actual organization activation", err)
	}
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	return fx, s, unit
}

func lifecycleCommand(t *testing.T, v fa.AssetLifecycleInput) fa.AssetLifecycle {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := fa.NewAssetLifecycle(v)
	if err != nil {
		t.Fatal("invalid fixture lifecycle command", err)
	}
	return c
}
func lifecycleWrite(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, c fa.AssetLifecycle) (fa.AssetResult, bool, error) {
	t.Helper()
	return s.TransitionAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, c)
}

// lifecycleDBNow is the server clock that bounds lifecycle instants.
func lifecycleDBNow(t *testing.T, fx legalFixtureState) time.Time {
	t.Helper()
	var now time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now.UTC()
}

// lifecycleInputFor returns a valid command for a known source revision.
func lifecycleInputFor(t *testing.T, fx legalFixtureState, command fleet.LifecycleCommand, asset string, revision int64) fa.AssetLifecycleInput {
	t.Helper()
	v := fa.AssetLifecycleInput{Command: command, AssetID: asset, ExpectedRevision: revision}
	at := lifecycleDBNow(t, fx).Add(-time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	switch command {
	case fleet.RecordAcquisition:
		v.At = at.Add(-24 * time.Hour)
	case fleet.Deactivate:
		v.Reason = "Synthetic seasonal idle period"
	case fleet.Dispose:
		v.Reason, v.DisposalKind, v.At = "Synthetic sale to a synthetic buyer", fleet.Sale, at
	case fleet.RevertDisposal:
		v.Reason = "Synthetic disposal recorded by mistake"
	}
	return v
}

// lifecycleAssetIn builds an actual asset in the requested state through the
// real commands only (create, activate, deactivate, dispose, archive).
func lifecycleAssetIn(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, unit string, state fleet.State) (string, int64) {
	t.Helper()
	input := fleetCreateInput()
	input.UnitID = unit
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	id, revision := created.AssetID, int64(1)
	steps := map[fleet.State][]fleet.LifecycleCommand{fleet.Inactive: {fleet.Deactivate}, fleet.Disposed: {fleet.Deactivate, fleet.Dispose}, fleet.Archived: {fleet.Dispose, fleet.Archive}}
	if state == fleet.Draft {
		return id, revision
	}
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: id, ExpectedRevision: 1})); err != nil {
		t.Fatal("actual implement activation", err)
	}
	revision++
	for _, command := range steps[state] {
		result, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, command, id, revision)))
		if err != nil {
			t.Fatal("lifecycle fixture step", command, err)
		}
		revision = result.Revision
	}
	return id, revision
}

type lifecycleCounts struct{ audit, history, receipts, revisions int64 }

func lifecycleCount(t *testing.T, fx legalFixtureState, asset string) lifecycleCounts {
	t.Helper()
	var c lifecycleCounts
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2),(SELECT count(*) FROM org_asset_lifecycle_events WHERE org_id=$1 AND asset_id=$2),(SELECT count(*) FROM org_asset_commands WHERE org_id=$1 AND asset_id=$2),(SELECT count(*) FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2)`, fx.scope.OrgID().String(), asset).Scan(&c.audit, &c.history, &c.receipts, &c.revisions); err != nil {
		t.Fatal(err)
	}
	return c
}

var lifecycleRows = map[fleet.LifecycleCommand]map[fleet.State]fleet.State{
	fleet.RecordAcquisition: {fleet.Draft: fleet.Draft, fleet.Active: fleet.Active, fleet.Inactive: fleet.Inactive},
	fleet.Deactivate:        {fleet.Active: fleet.Inactive},
	fleet.Reactivate:        {fleet.Inactive: fleet.Active},
	fleet.Dispose:           {fleet.Active: fleet.Disposed, fleet.Inactive: fleet.Disposed},
	fleet.RevertDisposal:    {fleet.Disposed: fleet.Inactive},
	fleet.Archive:           {fleet.Disposed: fleet.Archived},
}

// AC1: every row succeeds from its source state and is CONFLICT from all others.
func TestFleetLifecycleEveryTransitionAndRefusalOnRealPostGIS(t *testing.T) {
	fx, s, unit := lifecycleFixture(t)
	for _, command := range fleet.LifecycleCommands() {
		for _, from := range []fleet.State{fleet.Draft, fleet.Active, fleet.Inactive, fleet.Disposed, fleet.Archived} {
			t.Run(string(command)+"/"+string(from), func(t *testing.T) {
				asset, revision := lifecycleAssetIn(t, fx, s, unit, from)
				before := lifecycleCount(t, fx, asset)
				c := lifecycleCommand(t, lifecycleInputFor(t, fx, command, asset, revision))
				result, replayed, err := lifecycleWrite(t, fx, s, c)
				view, readErr := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, asset)
				if readErr != nil {
					t.Fatal("asset unreadable after lifecycle command", readErr)
				}
				after := lifecycleCount(t, fx, asset)
				want, allowed := lifecycleRows[command][from]
				if !allowed {
					if !errors.Is(err, fa.ErrConflict) || view.State != from || view.Revision != revision || after != before {
						t.Fatal("refused transition changed state or recorded history", err)
					}
					return
				}
				if err != nil || replayed || result.State != want || result.Revision != revision+1 || view.State != want || view.Revision != revision+1 {
					t.Fatal("allowed transition failed", err, result)
				}
				history := int64(1)
				if command == fleet.RecordAcquisition {
					history = 0
				}
				if after.audit != before.audit+1 || after.receipts != before.receipts+1 || after.revisions != before.revisions+1 || after.history != before.history+history {
					t.Fatal("transition is not exactly one audit/receipt/revision/history effect", before, after)
				}
				var exact bool
				if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT snapshot->>'state' FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2 AND revision=$3)=$4
 AND(SELECT count(*) FROM org_audit_events a JOIN org_asset_commands c USING(org_id,operation_id) WHERE a.org_id=$1 AND a.target_id=$2 AND c.kind=$5 AND c.result_state=$4 AND c.result_revision=$3 AND a.action=$6)=1`, fx.scope.OrgID().String(), asset, revision+1, string(want), string(command), lifecycleAction(command)).Scan(&exact); err != nil || !exact {
					t.Fatal("snapshot, receipt and audit action disagree", err)
				}
				// The previous revision is now stale: CONFLICT even for an allowed command.
				if next := map[fleet.State]fleet.LifecycleCommand{fleet.Draft: fleet.RecordAcquisition, fleet.Active: fleet.RecordAcquisition, fleet.Inactive: fleet.RecordAcquisition, fleet.Disposed: fleet.RevertDisposal}[want]; next != "" {
					if _, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, next, asset, revision))); !errors.Is(err, fa.ErrConflict) {
						t.Fatal("stale expected revision accepted", err)
					}
				}
			})
		}
	}
}

func lifecycleAction(command fleet.LifecycleCommand) string {
	return map[fleet.LifecycleCommand]string{fleet.RecordAcquisition: "fleet.asset.acquisition_recorded", fleet.Deactivate: "fleet.asset.deactivated", fleet.Reactivate: "fleet.asset.reactivated", fleet.Dispose: "fleet.asset.disposed", fleet.RevertDisposal: "fleet.asset.disposal_reverted", fleet.Archive: "fleet.asset.archived"}[command]
}

type openWorkFixture struct {
	kinds []fa.OpenWorkKind
	err   error
	calls int
}

func (o *openWorkFixture) OpenAssetWorkTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, asset string) ([]fa.OpenWorkKind, error) {
	o.calls++
	if tx == nil || database.ValidateOrgDBTX(ctx, tx, scope) != nil || asset == "" {
		return nil, errors.New("open work port lost the command transaction")
	}
	return append([]fa.OpenWorkKind(nil), o.kinds...), o.err
}

// AC2 with a test-only open-work adapter; production wires NoOpenWork.
func TestFleetLifecycleOpenWorkRefusesRemovalFromServiceNamingKinds(t *testing.T) {
	fx, s, unit := lifecycleFixture(t)
	for _, test := range []struct {
		command fleet.LifecycleCommand
		from    fleet.State
	}{{fleet.Deactivate, fleet.Active}, {fleet.Dispose, fleet.Active}, {fleet.Dispose, fleet.Inactive}, {fleet.Archive, fleet.Disposed}} {
		asset, revision := lifecycleAssetIn(t, fx, s, unit, test.from)
		open := &openWorkFixture{kinds: []fa.OpenWorkKind{fa.OpenTrip, fa.OpenWorkOrder}}
		blocked := *s
		blocked.OpenWork = open
		before := lifecycleCount(t, fx, asset)
		_, _, err := lifecycleWrite(t, fx, &blocked, lifecycleCommand(t, lifecycleInputFor(t, fx, test.command, asset, revision)))
		var refusal *fa.OpenOperationsError
		if !errors.As(err, &refusal) || len(refusal.Kinds()) != 2 || refusal.Kinds()[0] != fa.OpenTrip || refusal.Kinds()[1] != fa.OpenWorkOrder || open.calls != 1 {
			t.Fatal("open work did not refuse removal from service", test.command, err)
		}
		if lifecycleCount(t, fx, asset) != before {
			t.Fatal("open work refusal left an effect")
		}
		open.kinds = nil
		if result, _, err := lifecycleWrite(t, fx, &blocked, lifecycleCommand(t, lifecycleInputFor(t, fx, test.command, asset, revision))); err != nil || result.Revision != revision+1 {
			t.Fatal("removal from service refused after work ended", err)
		}
	}
	// Commands that do not remove from service never ask the port.
	asset, revision := lifecycleAssetIn(t, fx, s, unit, fleet.Disposed)
	open := &openWorkFixture{kinds: []fa.OpenWorkKind{fa.OpenTrip}}
	ignored := *s
	ignored.OpenWork = open
	for _, command := range []fleet.LifecycleCommand{fleet.RevertDisposal, fleet.Reactivate, fleet.RecordAcquisition} {
		result, _, err := lifecycleWrite(t, fx, &ignored, lifecycleCommand(t, lifecycleInputFor(t, fx, command, asset, revision)))
		if err != nil {
			t.Fatal(command, err)
		}
		revision = result.Revision
	}
	if open.calls != 0 {
		t.Fatal("open work consulted outside BR-FLEET-010")
	}
	// The port is mandatory and its failure is an outage, never a decision.
	missing := *s
	missing.OpenWork = nil
	asset, revision = lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	if _, _, err := lifecycleWrite(t, fx, &missing, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Deactivate, asset, revision))); !errors.Is(err, fa.ErrUnavailable) {
		t.Fatal("missing open-work port admitted removal", err)
	}
	failing := *s
	failing.OpenWork = &openWorkFixture{err: errors.New("Synthetic private trip source detail")}
	if _, _, err := lifecycleWrite(t, fx, &failing, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Deactivate, asset, revision))); !errors.Is(err, fa.ErrUnavailable) {
		t.Fatal("failed open-work source admitted removal", err)
	}
	if view, _ := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, asset); view.State != fleet.Active || view.Revision != revision {
		t.Fatal("unavailable open-work source changed the asset")
	}
}

// AC3: archive frees the normalized plate and fleet number for a new asset.
func TestFleetLifecycleArchiveFreesIdentifiersAndKeepsTheArchivedRecord(t *testing.T) {
	fx, s, unit := lifecycleFixture(t)
	input := fleetCreateInput()
	input.UnitID, input.Profile.Plate, input.Profile.FleetNumber = unit, "ABC1D23", "FR-07"
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: 1})); err != nil {
		t.Fatal(err)
	}
	disposed, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Dispose, created.AssetID, 2)))
	if err != nil {
		t.Fatal(err)
	}
	again := fleetCreateInput()
	again.Profile.Plate, again.Profile.FleetNumber = "abc-1d23", "fr 07"
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, again)); !errors.Is(err, fa.ErrIdentifierTaken) {
		t.Fatal("disposed asset released its identifiers before archive", err)
	}
	if _, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Archive, created.AssetID, disposed.Revision))); err != nil {
		t.Fatal(err)
	}
	reused, _, err := fleetWrite(t, fx, s, fleetCommand(t, again))
	if err != nil {
		t.Fatal("archived identifiers were not freed for a new asset", err)
	}
	archived, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID)
	if err != nil || archived.State != fleet.Archived || archived.Profile.Plate != "ABC1D23" || archived.Profile.FleetNumber != "FR-07" || archived.Disposal == nil || archived.Disposal.Kind != fleet.Sale {
		t.Fatal("archived record was renamed, lost or lost its disposal", err)
	}
	page, err := s.ListAssets(t.Context(), fx.scope, fleetActor(fx), fx.member, fa.AssetFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range page.Assets {
		if v.Ref.ID().Value() == created.AssetID {
			t.Fatal("archived asset in default list")
		}
	}
	page, err = s.ListAssets(t.Context(), fx.scope, fleetActor(fx), fx.member, fa.AssetFilter{Limit: 100, State: fleet.Archived})
	if err != nil || len(page.Assets) != 1 || page.Assets[0].Ref.ID().Value() != created.AssetID {
		t.Fatal("archived asset unavailable through its filter", err)
	}
	// A second archive of the same plate is fine; a second live one is not.
	second := fleetCreateInput()
	second.Profile.Plate = "ABC1D23"
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, second)); !errors.Is(err, fa.ErrIdentifierTaken) || reused.AssetID == "" {
		t.Fatal("reused live identifier admitted a duplicate", err)
	}
}

// AC4 + AC5: earlier records stay readable; revert keeps disposal history.
func TestFleetLifecycleRevertKeepsDisposalHistoryAndRecordsStayReadable(t *testing.T) {
	fx, s, unit := lifecycleFixture(t)
	meters := meterStore(t, fx)
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "275000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	input := fleetCreateInput()
	input.UnitID = unit
	input.Profile.Kind, input.Profile.UsageBasis, input.Profile.Tanks = fleet.Vehicle, fleet.Distance, []fleet.Tank{tank}
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	now := lifecycleDBNow(t, fx)
	installed, _, err := meterInstall(t, fx, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, now.Add(-5*time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision})); err != nil {
		t.Fatal("powered activation", err)
	}
	revision := installed.AssetRevision + 1
	acquisition := lifecycleInputFor(t, fx, fleet.RecordAcquisition, created.AssetID, revision)
	acquisition.At = now.Add(-30 * 24 * time.Hour).Add(7 * time.Nanosecond)
	acquired, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, acquisition))
	if err != nil {
		t.Fatal(err)
	}
	// Disposal cannot precede acquisition nor follow the server clock.
	early := lifecycleInputFor(t, fx, fleet.Dispose, created.AssetID, acquired.Revision)
	early.At = acquisition.At.Add(-time.Nanosecond)
	future := lifecycleInputFor(t, fx, fleet.Dispose, created.AssetID, acquired.Revision)
	future.At = lifecycleDBNow(t, fx).Add(time.Hour)
	futureAcquisition := lifecycleInputFor(t, fx, fleet.RecordAcquisition, created.AssetID, acquired.Revision)
	futureAcquisition.At = lifecycleDBNow(t, fx).Add(time.Hour)
	for name, v := range map[string]fa.AssetLifecycleInput{"before acquisition": early, "future disposal": future, "future acquisition": futureAcquisition} {
		if _, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, v)); !errors.Is(err, fleet.ErrInvalid) {
			t.Fatal("invalid lifecycle instant accepted", name, err)
		}
	}
	dispose := lifecycleInputFor(t, fx, fleet.Dispose, created.AssetID, acquired.Revision)
	dispose.DisposalKind, dispose.At = fleet.ReturnToHolder, acquisition.At
	disposed, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, dispose))
	if err != nil {
		t.Fatal("disposal at the acquisition instant", err)
	}
	// Disposed: asset, meters, latest observed reading and history stay readable.
	view, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID)
	if err != nil || view.State != fleet.Disposed || view.Disposal == nil || view.Disposal.Kind != fleet.ReturnToHolder || !view.Disposal.At.Equal(dispose.At) || view.Disposal.Reason != dispose.Reason || view.AcquiredAt == nil || !view.AcquiredAt.Equal(acquisition.At) || len(view.Profile.Tanks) != 1 || view.Allocation == nil {
		t.Fatal("disposed asset lost its current lifecycle data or earlier records", err)
	}
	latest, err := meters.ReadLatestMeter(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID, installed.MeterID)
	if err != nil || latest.Latest == nil {
		t.Fatal("disposed asset meter evidence unreadable", err)
	}
	if page, err := meters.ListAssetMeters(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID, fa.MeterFilter{Limit: 10}); err != nil || len(page.Meters) != 1 {
		t.Fatal("disposed asset meter list unreadable", err)
	}
	reverted, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.RevertDisposal, created.AssetID, disposed.Revision)))
	if err != nil || reverted.State != fleet.Inactive {
		t.Fatal("revert disposal did not return INACTIVE", err)
	}
	view, err = s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID)
	if err != nil || view.State != fleet.Inactive || view.Disposal != nil || view.AcquiredAt == nil {
		t.Fatal("revert kept a current disposal", err)
	}
	history, err := s.ListAssetLifecycle(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID, fa.LifecycleFilter{Limit: 100})
	if err != nil || len(history.Entries) != 2 {
		t.Fatal("lifecycle history unreadable", err)
	}
	first, second := history.Entries[0], history.Entries[1]
	if first.Command != fleet.Dispose || first.From != fleet.Active || first.To != fleet.Disposed || first.DisposalKind != fleet.ReturnToHolder || first.DisposedAt == nil || !first.DisposedAt.Equal(dispose.At) || first.Reason != dispose.Reason || first.Revision != disposed.Revision {
		t.Fatal("revert erased the original disposal from history", first)
	}
	if second.Command != fleet.RevertDisposal || second.From != fleet.Disposed || second.To != fleet.Inactive || second.Reason == "" || second.DisposedAt != nil || second.Revision != reverted.Revision {
		t.Fatal("revert history row changed", second)
	}
	// Disposed again with a different kind: both disposals remain in history.
	redispose := lifecycleInputFor(t, fx, fleet.Dispose, created.AssetID, reverted.Revision)
	redispose.DisposalKind = fleet.WriteOff
	if _, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, redispose)); err != nil {
		t.Fatal("asset cannot be disposed again after revert", err)
	}
	page, err := s.ListAssetLifecycle(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID, fa.LifecycleFilter{Limit: 2})
	if err != nil || len(page.Entries) != 2 || page.NextRevision != second.Revision {
		t.Fatal("bounded history keyset", err)
	}
	rest, err := s.ListAssetLifecycle(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID, fa.LifecycleFilter{Limit: 2, AfterRevision: page.NextRevision})
	if err != nil || len(rest.Entries) != 1 || rest.Entries[0].DisposalKind != fleet.WriteOff || rest.NextRevision != 0 {
		t.Fatal("history continuation", err)
	}
	var immutable bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2)=$3 AND(SELECT count(*) FROM org_meter_readings r JOIN org_meters m ON(m.org_id,m.id)=(r.org_id,r.meter_id) WHERE m.org_id=$1 AND m.asset_id=$2)=1
 AND(SELECT snapshot->'disposal'->>'kind' FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2 AND revision=$4)='RETURN_TO_HOLDER' AND(SELECT snapshot->'disposal' IS NULL FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2 AND revision=$5)`, fx.scope.OrgID().String(), created.AssetID, reverted.Revision+1, disposed.Revision, reverted.Revision).Scan(&immutable); err != nil || !immutable {
		t.Fatal("revision snapshots or readings changed", err)
	}
	// The history is append-only for the runtime and even for the owner.
	exec := func(sql string) error {
		return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql); return err })
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_asset_lifecycle_events TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`UPDATE org_asset_lifecycle_events SET reason='rewritten'`, `DELETE FROM org_asset_lifecycle_events`, `TRUNCATE org_asset_lifecycle_events`} {
		if code := lifecycleSQLCode(exec(sql)); code != "55000" {
			t.Fatal("lifecycle history rewritable", sql, code)
		}
	}
}

func lifecycleSQLCode(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	if err == nil {
		return ""
	}
	return "other"
}

// Replay returns the first result; reuse with other intent is refused.
func TestFleetLifecycleReplayReturnsTheFirstResultAfterLaterChanges(t *testing.T) {
	fx, s, unit := lifecycleFixture(t)
	asset, revision := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	deactivate := lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Deactivate, asset, revision))
	first, replayed, err := lifecycleWrite(t, fx, s, deactivate)
	if err != nil || replayed {
		t.Fatal(err)
	}
	if again, replayed, err := lifecycleWrite(t, fx, s, deactivate); err != nil || !replayed || again != first {
		t.Fatal("identical retry did not return the first result", err)
	}
	disposed, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Dispose, asset, first.Revision)))
	if err != nil {
		t.Fatal(err)
	}
	if late, replayed, err := lifecycleWrite(t, fx, s, deactivate); err != nil || !replayed || late != first || late.State != fleet.Inactive {
		t.Fatal("replay after a later transition was re-evaluated", err)
	}
	v, _ := deactivate.Snapshot()
	changed := v
	changed.Reason = "Synthetic different intent"
	if _, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, changed)); !errors.Is(err, fa.ErrOperationReused) {
		t.Fatal("changed intent reused the operation", err)
	}
	other := lifecycleInputFor(t, fx, fleet.Archive, asset, disposed.Revision)
	other.OperationID = v.OperationID
	if _, _, err := lifecycleWrite(t, fx, s, lifecycleCommand(t, other)); !errors.Is(err, fa.ErrOperationReused) {
		t.Fatal("another lifecycle kind reused the operation", err)
	}
	createInput := fleetCreateInput()
	createInput.OperationID = v.OperationID
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, createInput)); !errors.Is(err, fa.ErrOperationReused) {
		t.Fatal("register command reused a lifecycle operation", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_asset_lifecycle_events WHERE org_id=$1 AND operation_id=$2)=1 AND(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)=1 AND(SELECT state FROM org_assets WHERE org_id=$1 AND id=$3)='DISPOSED'`, fx.scope.OrgID().String(), v.OperationID, asset).Scan(&exact); err != nil || !exact {
		t.Fatal("replay duplicated an effect", err)
	}
}

// Concurrent deactivate and dispose on one revision leave exactly one winner.
func TestFleetLifecycleConcurrentDeactivateAndDisposeLeaveOneWinner(t *testing.T) {
	fx, s, unit := lifecycleFixture(t)
	asset, revision := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	concurrent := *s
	concurrent.Pool = fx.f.RuntimePool(t, 3)
	commands := []fa.AssetLifecycle{lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Deactivate, asset, revision)), lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Dispose, asset, revision))}
	block, err := fx.f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// Always release the barrier connection, even when an assertion fails.
	defer func() { _ = block.Rollback(context.Background()) }()
	if _, err := block.Exec(t.Context(), `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, fx.scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	results := make([]error, 2)
	states := make([]fleet.State, 2)
	var wg sync.WaitGroup
	for i, c := range commands {
		wg.Go(func() {
			r, _, err := concurrent.TransitionAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, c)
			results[i], states[i] = err, r.State
		})
	}
	// Barrier: both real transactions are observed waiting on the root lock.
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM pg_stat_activity WHERE usename=$1 AND wait_event_type='Lock'`, fx.f.RuntimeRole).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("both lifecycle commands never reached the root lock")
		}
		timer := time.NewTimer(5 * time.Millisecond)
		<-timer.C
	}
	if err := block.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	won, lost := 0, 0
	winner := fleet.State("")
	for i, err := range results {
		switch {
		case err == nil:
			won++
			winner = states[i]
		case errors.Is(err, fa.ErrConflict):
			lost++
		default:
			t.Fatal("unexpected concurrent lifecycle result", err)
		}
	}
	if won != 1 || lost != 1 {
		t.Fatal("concurrent transitions did not leave exactly one winner", won, lost)
	}
	view, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, asset)
	if err != nil || view.State != winner || view.Revision != revision+1 {
		t.Fatal("loser changed the asset", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_asset_lifecycle_events WHERE org_id=$1 AND asset_id=$2)=1 AND(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2 AND action IN('fleet.asset.deactivated','fleet.asset.disposed'))=1`, fx.scope.OrgID().String(), asset).Scan(&exact); err != nil || !exact {
		t.Fatal("loser left history or audit", err)
	}
}

// The database itself refuses fabricated lifecycle states and history.
func TestFleetLifecycleSchemaRefusesFieldEditsAndFabricatedHistory(t *testing.T) {
	fx, s, unit := lifecycleFixture(t)
	active, activeRevision := lifecycleAssetIn(t, fx, s, unit, fleet.Active)
	archived, _ := lifecycleAssetIn(t, fx, s, unit, fleet.Archived)
	exec := func(sql string, args ...any) error {
		return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql, args...); return err })
	}
	org := fx.scope.OrgID().String()
	at := lifecycleDBNow(t, fx).Add(-time.Hour)
	for name, test := range map[string]struct {
		sql  string
		args []any
		code string
	}{
		"state field edit":           {`UPDATE org_assets SET state='INACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, active}, "23514"},
		"disposed without data":      {`UPDATE org_assets SET state='DISPOSED',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, active}, "23514"},
		"skip to archive":            {`UPDATE org_assets SET state='ARCHIVED',revision=revision+1,disposal_kind='SALE',disposed_at=$3,disposal_reason='R' WHERE org_id=$1 AND id=$2`, []any{org, active, at}, "P7903"},
		"future disposal":            {`UPDATE org_assets SET state='DISPOSED',revision=revision+1,disposal_kind='SALE',disposed_at=clock_timestamp()+interval '1 hour',disposal_reason='R' WHERE org_id=$1 AND id=$2`, []any{org, active}, "23514"},
		"empty disposal reason":      {`UPDATE org_assets SET state='DISPOSED',revision=revision+1,disposal_kind='SALE',disposed_at=$3,disposal_reason='' WHERE org_id=$1 AND id=$2`, []any{org, active, at}, "23514"},
		"unknown disposal kind":      {`UPDATE org_assets SET state='DISPOSED',revision=revision+1,disposal_kind='THEFT',disposed_at=$3,disposal_reason='R' WHERE org_id=$1 AND id=$2`, []any{org, active, at}, "23514"},
		"archived terminal":          {`UPDATE org_assets SET revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, archived}, "P7903"},
		"archived name rewrite":      {`UPDATE org_assets SET name='Synthetic renamed',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, archived}, "P7903"},
		"future acquisition":         {`UPDATE org_assets SET acquired_at=clock_timestamp()+interval '1 day',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, active}, "23514"},
		"insert with lifecycle data": {`INSERT INTO org_assets(org_id,id,kind,category_id,name,serial_number,usage_basis,state,revision,acquired_at) SELECT org_id,$2::uuid,kind,category_id,name,'S-2',usage_basis,'DRAFT',1,clock_timestamp() FROM org_assets WHERE org_id=$1 AND id=$3`, []any{org, onboardingID(t), active}, "23514"},
		"fabricated history":         {`INSERT INTO org_asset_lifecycle_events(org_id,asset_id,revision,operation_id,actor_id,command,from_state,to_state,reason) VALUES($1,$2,$3,$4,$5,'DEACTIVATE','ACTIVE','INACTIVE','R')`, []any{org, active, activeRevision + 1, onboardingID(t), fx.actor.AccountID}, "23514"},
	} {
		if code := lifecycleSQLCode(exec(test.sql, test.args...)); code != test.code {
			t.Fatal("schema accepted fabricated lifecycle data", name, code)
		}
	}
	if view, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, active); err != nil || view.State != fleet.Active || view.Revision != activeRevision {
		t.Fatal("refused SQL changed the asset", err)
	}
}
