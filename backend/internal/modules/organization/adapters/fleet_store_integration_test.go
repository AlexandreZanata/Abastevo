//go:build integration

package adapters_test

import (
	"errors"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync"
	"testing"
)

func fleetStore(t *testing.T, fx legalFixtureState) *fleetpg.AssetStore {
	t.Helper()
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON org_assets,org_asset_tanks,org_asset_tank_products,org_asset_expected TO "+role+";GRANT SELECT,INSERT ON org_asset_categories,org_asset_unit_allocations,org_asset_revisions,org_asset_commands TO "+role+";GRANT SELECT ON org_fuel_products_v1,org_sync_operations,org_sync_effects,org_team_links,org_teams TO "+role); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(fleetpg.AssetAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &fleetpg.AssetStore{Pool: fx.f.Runtime, Authority: &adapters.FleetAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog), NewID: jobs.NewUUIDv4}
}
func fleetActor(fx legalFixtureState) fa.FleetActor {
	return fa.FleetActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
}
func fleetCommand(t *testing.T, v fa.AssetWriteInput) fa.AssetWrite {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := fa.NewAssetWrite(v)
	if err != nil {
		t.Fatal("invalid fixture fleet command", err)
	}
	return c
}
func fleetCreateInput() fa.AssetWriteInput {
	return fa.AssetWriteInput{Kind: fa.CreateAsset, Profile: fleet.Profile{Kind: fleet.Implement, Name: "Synthetic private implement", SerialNumber: "SYNTHETIC-1", UsageBasis: fleet.Hours}, Category: fa.CategoryChoice{NewLabel: "Synthetic category"}}
}
func fleetWrite(t *testing.T, fx legalFixtureState, s *fleetpg.AssetStore, c fa.AssetWrite) (fa.AssetResult, bool, error) {
	t.Helper()
	v, err := c.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	if v.Kind == fa.CreateAsset {
		id = onboardingID(t)
	}
	return s.WriteAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, id, c)
}
func TestFleetStorePersistsExactProfileAndOriginalReceipts(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	input := fleetCreateInput()
	input.Profile.FleetNumber = "ab- 7"
	create := fleetCommand(t, input)
	r, replayed, err := fleetWrite(t, fx, s, create)
	if err != nil || replayed || r.State != fleet.Draft || r.Revision != 1 {
		t.Fatal("real fleet create", err)
	}
	view, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, r.AssetID)
	if err != nil || view.Profile.Name != input.Profile.Name || view.Profile.FleetNumber != input.Profile.FleetNumber || view.CategoryLabel != input.Category.NewLabel {
		t.Fatal("private fleet read", err)
	}
	again, replayed, err := fleetWrite(t, fx, s, create)
	if err != nil || !replayed || again != r {
		t.Fatal("original create acknowledgement changed", err)
	}
	change := fleetCreateInput()
	change.Kind = fa.ChangeAsset
	change.AssetID = r.AssetID
	change.ExpectedRevision = 1
	change.Profile.Name = "Synthetic changed"
	change.Category = fa.CategoryChoice{ExistingID: view.Profile.Category}
	updated, _, err := fleetWrite(t, fx, s, fleetCommand(t, change))
	if err != nil || updated.Revision != 2 {
		t.Fatal("real fleet CAS change", err)
	}
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, change)); !errors.Is(err, fa.ErrConflict) {
		t.Fatal("stale revision changed asset", err)
	}
	var complete bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2)=2 AND (SELECT count(*) FROM org_asset_commands WHERE org_id=$1 AND asset_id=$2)=2 AND (SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2)=2 AND(SELECT snapshot->>'name' FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2 AND revision=1)=$3`, fx.scope.OrgID().String(), r.AssetID, input.Profile.Name).Scan(&complete); err != nil || !complete {
		t.Fatal("revision/command/audit not atomic", err)
	}
}
func TestFleetStoreDiscardReplayAndExactUncappedCategory(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	input := fleetCreateInput()
	input.Category.NewLabel = strings.Repeat("Synthetic category ", 300)
	create := fleetCommand(t, input)
	r, _, err := fleetWrite(t, fx, s, create)
	if err != nil {
		t.Fatal("uncapped exact category", err)
	}
	second, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	var categories int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_asset_categories WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&categories); err != nil || categories != 1 {
		t.Fatal("exact category not reused", err)
	}
	discard := fleetCommand(t, fa.AssetWriteInput{Kind: fa.DiscardAsset, AssetID: r.AssetID, ExpectedRevision: 1})
	gone, _, err := fleetWrite(t, fx, s, discard)
	if err != nil || !gone.Discarded || gone.State != "" || gone.Revision != 2 {
		t.Fatal("unused draft discard", err)
	}
	if _, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, r.AssetID); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("discarded row readable", err)
	}
	again, replay, err := fleetWrite(t, fx, s, discard)
	if err != nil || !replay || again != gone {
		t.Fatal("discard replay needs live target", err)
	}
	original, replay, err := fleetWrite(t, fx, s, create)
	if err != nil || !replay || original != r {
		t.Fatal("create replay recreated discarded asset", err)
	}
	var preserved bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_assets WHERE org_id=$1 AND id=$2) AND NOT EXISTS(SELECT 1 FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2) AND(SELECT count(*) FROM org_asset_commands WHERE org_id=$1 AND asset_id=$2)=2 AND EXISTS(SELECT 1 FROM org_assets WHERE org_id=$1 AND id=$3)`, fx.scope.OrgID().String(), r.AssetID, second.AssetID).Scan(&preserved); err != nil || !preserved {
		t.Fatal("discard crossed history retention boundary", err)
	}
}
func TestFleetStoreTwelveWayIdentifierRace(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	start := make(chan struct{})
	results := make(chan error, 12)
	var wg sync.WaitGroup
	for range 12 {
		input := fleetCreateInput()
		input.Profile.Plate = "abc-1d23"
		c := fleetCommand(t, input)
		id := onboardingID(t)
		wg.Go(func() {
			<-start
			_, _, err := s.WriteAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, id, c)
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	won, denied := 0, 0
	for err := range results {
		if err == nil {
			won++
		} else if errors.Is(err, fa.ErrIdentifierTaken) {
			denied++
		} else {
			t.Fatal("unexpected race result", err)
		}
	}
	if won != 1 || denied != 11 {
		t.Fatal("identifier concurrency admitted duplicate effects")
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_assets WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_asset_commands WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_asset_categories WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='fleet.asset.created')=1`, fx.scope.OrgID().String()).Scan(&exact); err != nil || !exact {
		t.Fatal("duplicate effects survived rollback", err)
	}
}

func TestFleetStoreImplementsActivationAndClosesPoweredAdmission(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal("actual organization activation", err)
	}
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	input := fleetCreateInput()
	input.UnitID = unit
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	activated, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: 1}))
	if err != nil || activated.State != fleet.Active || activated.Revision != 2 {
		t.Fatal("implement activation without fabricated meter", err)
	}
	view, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID)
	if err != nil || view.Allocation == nil || view.Allocation.UnitID != unit || view.Allocation.StartedAt.IsZero() {
		t.Fatal("actual initial period missing", err)
	}
	change := fleetCreateInput()
	change.Kind = fa.ChangeAsset
	change.AssetID = created.AssetID
	change.ExpectedRevision = 2
	change.Profile.Kind = fleet.Vehicle
	change.Category = fa.CategoryChoice{ExistingID: view.Profile.Category}
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, change)); !errors.Is(err, fa.ErrUnavailable) {
		t.Fatal("active kind change fabricated meter availability", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_asset_unit_allocations WHERE org_id=$1 AND asset_id=$2)=1 AND(SELECT count(*) FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2)=2 AND(SELECT kind='IMPLEMENT' AND revision=2 FROM org_assets WHERE org_id=$1 AND id=$2)`, fx.scope.OrgID().String(), created.AssetID).Scan(&exact); err != nil || !exact {
		t.Fatal("failed powered transition changed private state", err)
	}
}
