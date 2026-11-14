//go:build integration

package adapters_test

import (
	"errors"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"sync"
	"testing"
)

func fleetSQL(t *testing.T, fx legalFixtureState, sql string, args ...any) error {
	t.Helper()
	return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql, args...); return err })
}
func fleetSQLCode(t *testing.T, err error, code string) {
	t.Helper()
	var p *pgconn.PgError
	if !errors.As(err, &p) || p.Code != code {
		t.Fatalf("expected safe SQLSTATE %s, got %v", code, err)
	}
}
func TestFleetSchemaRLSExactKeysAndForeignCategories(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	input := fleetCreateInput()
	input.Profile.Plate = "abc-1d23"
	input.Profile.FleetNumber = "fleet-1"
	r, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, r.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	otherFX := fx
	otherFX.scope = scopeFor(t, other.OrgID.String())
	otherFX.member = other.MembershipID
	if _, err := fx.flags.Set(t.Context(), otherFX.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, otherFX, s, fleetCommand(t, input)); err != nil {
		t.Fatal("same keys in another organization rejected", err)
	}
	if _, err := s.ReadAsset(t.Context(), otherFX.scope, fleetActor(fx), otherFX.member, r.AssetID); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("foreign asset disclosed", err)
	}
	input.Category = fa.CategoryChoice{ExistingID: view.Profile.Category}
	input.Profile.Plate = ""
	input.Profile.FleetNumber = ""
	if _, _, err := fleetWrite(t, otherFX, s, fleetCommand(t, input)); !errors.Is(err, fa.ErrNotFound) {
		t.Fatal("foreign category accepted", err)
	}
	var n int
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_assets`).Scan(&n); err != nil || n != 0 {
		t.Fatal("unscoped runtime observed assets", err)
	}
	tag, err := fx.f.Runtime.Exec(t.Context(), `UPDATE org_assets SET name='Synthetic forbidden',revision=revision+1 WHERE id=$1`, r.AssetID)
	if err != nil || tag.RowsAffected() != 0 {
		t.Fatal("unscoped write escaped RLS", err)
	}
	fleetSQLCode(t, fleetSQL(t, otherFX, `INSERT INTO org_asset_categories(org_id,id,label) VALUES($1,$2,'Synthetic foreign')`, fx.scope.OrgID().String(), onboardingID(t)), "42501")
	fleetSQLCode(t, fleetSQL(t, fx, `UPDATE org_assets SET plate_key='BAD',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), r.AssetID), "23514")
}
func TestFleetSchemaUnknownConsumersAndImmutableHistory(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	r, _, err := fleetWrite(t, fx, s, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE ON org_asset_commands,org_asset_revisions TO "+role+";GRANT UPDATE ON org_fuel_products_v1 TO "+role); err != nil {
		t.Fatal(err)
	}
	org := fx.scope.OrgID().String()
	for _, sql := range []string{`UPDATE org_asset_revisions SET snapshot='{}' WHERE org_id=$1`, `DELETE FROM org_asset_revisions WHERE org_id=$1`, `DELETE FROM org_asset_commands WHERE org_id=$1`, `UPDATE org_fuel_products_v1 SET unit='M3' WHERE code='ETHANOL' AND $1::text<>''`} {
		fleetSQLCode(t, fleetSQL(t, fx, sql, org), "55000")
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_unregistered_asset_source(org_id uuid,asset_id uuid,FOREIGN KEY(org_id,asset_id) REFERENCES org_assets(org_id,id))`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.DiscardAsset, AssetID: r.AssetID, ExpectedRevision: 1})); !errors.Is(err, fa.ErrUnavailable) {
		t.Fatal("unknown source guessed unused", err)
	}
	change := fleetCreateInput()
	change.Kind = fa.ChangeAsset
	change.AssetID = r.AssetID
	change.ExpectedRevision = 1
	change.Profile.Kind = fleet.Vehicle
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, change)); !errors.Is(err, fa.ErrUnavailable) {
		t.Fatal("kind changed without actual source", err)
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_asset_commands WHERE org_id=$1)=1 AND(SELECT revision=1 AND kind='IMPLEMENT' FROM org_assets WHERE org_id=$1 AND id=$2)`, org, r.AssetID).Scan(&exact); err != nil || !exact {
		t.Fatal("source unavailable changed state", err)
	}
}
func TestFleetStoreCommitFailureDoesNotPublishAcknowledgement(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_fleet_commit_failure() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION USING ERRCODE='23514',MESSAGE='synthetic publication failure';END$$;CREATE CONSTRAINT TRIGGER synthetic_fleet_commit_failure AFTER INSERT ON org_asset_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_fleet_commit_failure()`); err != nil {
		t.Fatal(err)
	}
	r, replay, err := fleetWrite(t, fx, s, fleetCommand(t, fleetCreateInput()))
	if err == nil || replay || r.AssetID != "" || r.Revision != 0 {
		t.Fatal("failed commit exposed acknowledgement")
	}
	var clean bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT NOT EXISTS(SELECT 1 FROM org_assets WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_asset_categories WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_asset_revisions WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_asset_commands WHERE org_id=$1) AND NOT EXISTS(SELECT 1 FROM org_audit_events WHERE org_id=$1 AND action LIKE 'fleet.asset.%')`, fx.scope.OrgID().String()).Scan(&clean); err != nil || !clean {
		t.Fatal("partial private effects committed", err)
	}
}
func TestFleetStoreConcurrentOriginalReplayAndCAS(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	c := fleetCommand(t, fleetCreateInput())
	start := make(chan struct{})
	results := make(chan fa.AssetResult, 12)
	errs := make(chan error, 12)
	var wg sync.WaitGroup
	for range 12 {
		id := onboardingID(t)
		wg.Go(func() {
			<-start
			r, _, err := s.WriteAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, id, c)
			results <- r
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var original fa.AssetResult
	for r := range results {
		if original.AssetID == "" {
			original = r
		}
		if r != original {
			t.Fatal("replay returned another original")
		}
	}
	errs = make(chan error, 12)
	start = make(chan struct{})
	for i := range 12 {
		input := fleetCreateInput()
		input.Kind = fa.ChangeAsset
		input.AssetID = original.AssetID
		input.ExpectedRevision = 1
		input.Profile.Name = "Synthetic version " + string(rune('A'+i))
		cmd := fleetCommand(t, input)
		wg.Go(func() {
			<-start
			_, _, err := s.WriteAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, "", cmd)
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	won, denied := 0, 0
	for err := range errs {
		if err == nil {
			won++
		} else if errors.Is(err, fa.ErrConflict) {
			denied++
		} else {
			t.Fatal(err)
		}
	}
	if won != 1 || denied != 11 {
		t.Fatal("CAS did not serialize original revision")
	}
}
