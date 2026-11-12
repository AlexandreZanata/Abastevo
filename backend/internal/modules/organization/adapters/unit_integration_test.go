//go:build integration

package adapters_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/jackc/pgx/v5"
)

func operatingUnitFixture(t *testing.T) (legalFixtureState, *adapters.UnitStore) {
	t.Helper()
	fx := legalFixture(t)
	return fx, operatingUnitStore(t, fx)
}
func operatingUnitStore(t *testing.T, fx legalFixtureState) *adapters.UnitStore {
	t.Helper()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_unit_commands TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(adapters.UnitAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &adapters.UnitStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
func operatingUnitCommand(t *testing.T, v app.UnitWriteInput) app.UnitWrite {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	cmd, err := app.NewUnitWrite(v)
	if err != nil {
		t.Fatal(err)
	}
	return cmd
}
func TestOperatingUnitStoreReplaysCASArchivesRestoresAndScopes(t *testing.T) {
	fx, store := operatingUnitFixture(t)
	input := app.UnitWriteInput{Kind: app.CreateUnit, OperationID: onboardingID(t), Profile: d.UnitProfile{Name: " Synthetic North ", Code: " N01 "}}
	create := operatingUnitCommand(t, input)
	write := func(command app.UnitWrite) (app.UnitResult, bool, error) {
		t.Helper()
		id := ""
		v, err := command.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		if v.Kind == app.CreateUnit {
			id = onboardingID(t)
		}
		return store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, id, command)
	}
	first, replay, err := write(create)
	if err != nil || replay || first.State != d.UnitActive || first.Revision != 1 {
		t.Fatal("unit create", err)
	}
	repeated, replay, err := write(create)
	if err != nil || !replay || repeated != first {
		t.Fatal("unit replay changed acknowledgement", err)
	}
	unit, err := store.ReadUnit(t.Context(), fx.scope, fx.actor, fx.member, first.UnitID)
	if err != nil {
		t.Fatal(err)
	}
	root, err := adapters.NewStore(fx.f.Runtime).Get(t.Context(), fx.scope)
	if err != nil {
		t.Fatal(err)
	}
	rootSnapshot, err := root.Snapshot(fx.scope)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := unit.Snapshot(fx.scope)
	if err != nil || snap.TimeZone != rootSnapshot.Profile.TimeZone || snap.Name != "Synthetic North" || snap.Code != "N01" {
		t.Fatal("stored unit defaults", err)
	}
	input.Profile.Name = "Changed intent"
	if _, _, err := write(operatingUnitCommand(t, input)); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("changed operation accepted", err)
	}
	input.OperationID = onboardingID(t)
	input.Profile.Name = "synthetic north"
	if _, _, err := write(operatingUnitCommand(t, input)); !errors.Is(err, app.ErrUnitDuplicate) {
		t.Fatal("duplicate active name accepted", err)
	}
	archive := operatingUnitCommand(t, app.UnitWriteInput{Kind: app.ArchiveUnit, UnitID: first.UnitID, ExpectedRevision: 1})
	archived, replay, err := write(archive)
	if err != nil || replay || archived.State != d.UnitArchived || archived.Revision != 2 {
		t.Fatal("unit archive", err)
	}
	if got, replay, err := write(archive); err != nil || !replay || got != archived {
		t.Fatal("archive exact retry", err)
	}
	if _, _, err := write(operatingUnitCommand(t, app.UnitWriteInput{Kind: app.RestoreUnit, UnitID: first.UnitID, ExpectedRevision: 1})); !errors.Is(err, d.ErrConflict) {
		t.Fatal("stale restore accepted", err)
	}
	input.OperationID = onboardingID(t)
	other, _, err := write(operatingUnitCommand(t, input))
	if err != nil || other.UnitID == first.UnitID {
		t.Fatal("archived name incorrectly reserved", err)
	}
	if _, _, err := write(operatingUnitCommand(t, app.UnitWriteInput{Kind: app.RestoreUnit, UnitID: first.UnitID, ExpectedRevision: 2})); !errors.Is(err, app.ErrUnitDuplicate) {
		t.Fatal("colliding restore admitted", err)
	}
	change := operatingUnitCommand(t, app.UnitWriteInput{Kind: app.ChangeUnit, UnitID: first.UnitID, ExpectedRevision: 2, Profile: d.UnitProfile{Name: "Synthetic corrected", Code: "C01", TimeZone: "America/Cuiaba"}})
	if changed, _, err := write(change); err != nil || changed.State != d.UnitArchived || changed.Revision != 3 {
		t.Fatal("archived metadata correction", err)
	}
	if restored, _, err := write(operatingUnitCommand(t, app.UnitWriteInput{Kind: app.RestoreUnit, UnitID: first.UnitID, ExpectedRevision: 3})); err != nil || restored.State != d.UnitActive || restored.Revision != 4 {
		t.Fatal("corrected restoration", err)
	}
	page, err := store.ListUnits(t.Context(), fx.scope, fx.actor, fx.member, app.UnitFilter{Limit: 1})
	if err != nil || len(page.Units) != 1 || page.NextID == "" {
		t.Fatal("unit keyset first page", err)
	}
	next, err := store.ListUnits(t.Context(), fx.scope, fx.actor, fx.member, app.UnitFilter{Limit: 2, AfterID: page.NextID})
	if err != nil || len(next.Units) != 2 || next.NextID != "" {
		t.Fatal("unit keyset second page", err)
	}
	if _, err := store.FindUnit(t.Context(), scopeFor(t, onboardingID(t)), first.UnitID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("foreign unit exposed", err)
	}
	var payload string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(string_agg(to_jsonb(e)::text,''),'') FROM org_audit_events e`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, "Synthetic North") || strings.Contains(payload, "Synthetic corrected") {
		t.Fatal("unit names entered audit")
	}
}
func TestOperatingUnitStoreFailureAndConcurrentReplay(t *testing.T) {
	fx, store := operatingUnitFixture(t)
	c := operatingUnitCommand(t, app.UnitWriteInput{Kind: app.CreateUnit, Profile: d.UnitProfile{Name: "Synthetic race", Code: "RACE"}})
	const n = 12
	results := make(chan app.UnitResult, n)
	errs := make(chan error, n)
	replays := make(chan bool, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	ids := make([]string, n)
	for i := range ids {
		ids[i] = onboardingID(t)
	}
	for _, id := range ids {
		wg.Go(func() {
			<-start
			value, replay, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, id, c)
			results <- value
			replays <- replay
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	close(replays)
	for err := range errs {
		if err != nil {
			t.Fatal("racing unit repeat", err)
		}
	}
	chosen := ""
	for result := range results {
		if chosen == "" {
			chosen = result.UnitID
		}
		if result.UnitID != chosen {
			t.Fatal("unit retry selected different identity")
		}
	}
	repeated := 0
	for replay := range replays {
		if replay {
			repeated++
		}
	}
	if repeated != n-1 {
		t.Fatal("unit retry count")
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_unit_receipt_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic receipt failure';END $$;
 CREATE CONSTRAINT TRIGGER synthetic_unit_receipt_failure AFTER INSERT ON org_unit_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_unit_receipt_failure()`); err != nil {
		t.Fatal(err)
	}
	failed := operatingUnitCommand(t, app.UnitWriteInput{Kind: app.CreateUnit, Profile: d.UnitProfile{Name: "Synthetic rollback", Code: "ROLLBACK"}})
	id := onboardingID(t)
	result, replay, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, id, failed)
	if !errors.Is(err, app.ErrUnavailable) || replay || result != (app.UnitResult{}) {
		t.Fatal("failed commit returned unit acknowledgement", err)
	}
	if _, err := store.FindUnit(t.Context(), fx.scope, id); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("failed commit left unit", err)
	}
}

func TestOperatingUnitEstablishmentLinkSurvivesWithdrawalWithoutAcceptingNewRevokedOrForeignLinks(t *testing.T) {
	fx, store := operatingUnitFixture(t)
	company, _, err := fx.write(t, legalCommand(t, onboardingID(t), false, 0))
	if err != nil {
		t.Fatal(err)
	}
	create := operatingUnitCommand(t, app.UnitWriteInput{Kind: app.CreateUnit, Profile: d.UnitProfile{Name: "Synthetic linked", Code: "LINKED", EstablishmentID: company.EstablishmentID}})
	linked, _, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), create)
	if err != nil {
		t.Fatal("scoped establishment link refused", err)
	}
	withdrawal, err := app.WithdrawLegalCommand(company.EntityID, onboardingID(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	retained, _, err := fx.write(t, withdrawal)
	if err != nil || retained.State != "REVOKED" {
		t.Fatal("unit reference did not retain legal history", err)
	}
	input, _ := create.Snapshot()
	input.OperationID = onboardingID(t)
	input.Profile.Name = "Synthetic new revoked link"
	input.Profile.Code = "REVOKED"
	if result, replay, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), operatingUnitCommand(t, input)); !errors.Is(err, app.ErrNotFound) || replay || result != (app.UnitResult{}) {
		t.Fatal("new revoked establishment link admitted", err)
	}
	change := operatingUnitCommand(t, app.UnitWriteInput{Kind: app.ChangeUnit, UnitID: linked.UnitID, ExpectedRevision: 1, Profile: d.UnitProfile{Name: "Synthetic retained reference", Code: "LINKED", EstablishmentID: company.EstablishmentID}})
	if result, _, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", change); err != nil || result.Revision != 2 {
		t.Fatal("retained reference blocked unrelated metadata correction", err)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	foreign, _, err := other.write(t, legalCommand(t, onboardingID(t), false, 0))
	if err != nil {
		t.Fatal(err)
	}
	input.OperationID = onboardingID(t)
	input.Profile.EstablishmentID = foreign.EstablishmentID
	if _, _, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), operatingUnitCommand(t, input)); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("foreign establishment link admitted", err)
	}
}
func TestOperatingUnitStoreUsesActualCurrentCountsAndFailsClosedForUnknownConsumer(t *testing.T) {
	fx, store := operatingUnitFixture(t)
	org := fx.scope.OrgID().String()
	if err := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	command := operatingUnitCommand(t, app.UnitWriteInput{Kind: app.CreateUnit, Profile: d.UnitProfile{Name: "Synthetic used unit", Code: "USED"}})
	unit, _, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), command)
	if err != nil {
		t.Fatal(err)
	}
	installSyntheticUnitUsageSource(t, fx)
	if err := unitSchemaExec(t, fx, `INSERT INTO synthetic_unit_work(org_id,unit_id,id,kind,state) VALUES($1,$2,$3,'EMPLOYEE','ACTIVE')`, org, unit.UnitID, onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	archive := operatingUnitCommand(t, app.UnitWriteInput{Kind: app.ArchiveUnit, UnitID: unit.UnitID, ExpectedRevision: 1})
	result, replay, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", archive)
	var inUse app.UnitUsageError
	if !errors.As(err, &inUse) || inUse.Usage() != (d.UnitUsage{Employees: 1}) || replay || result != (app.UnitResult{}) {
		t.Fatal("current bounded category counts lost", err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_unknown_unit_consumer(org_id uuid,unit_id uuid,FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id))`); err != nil {
		t.Fatal(err)
	}
	result, replay, err = store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", archive)
	if !errors.Is(err, app.ErrUnavailable) || replay || result != (app.UnitResult{}) {
		t.Fatal("unknown source treated as empty", err)
	}
	var receipts int
	v, _ := archive.Snapshot()
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2`, org, v.OperationID).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatal("refused unit archive retained an audit reservation", err)
	}
}

func TestOperatingUnitStoreSerializesDistinctDuplicateAndRevisionRaces(t *testing.T) {
	for _, field := range []string{"name", "code"} {
		t.Run(field, func(t *testing.T) {
			fx, store := operatingUnitFixture(t)
			const n = 12
			commands := make([]app.UnitWrite, n)
			ids := make([]string, n)
			for i := range n {
				profile := d.UnitProfile{Name: fmt.Sprintf("Synthetic distinct %d", i), Code: fmt.Sprintf("C%d", i)}
				if field == "name" {
					profile.Name = "Synthetic racing name"
				} else {
					profile.Code = "RACING"
				}
				commands[i] = operatingUnitCommand(t, app.UnitWriteInput{Kind: app.CreateUnit, Profile: profile})
				ids[i] = onboardingID(t)
			}
			type outcome struct {
				value  app.UnitResult
				replay bool
				err    error
			}
			results := make(chan outcome, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range n {
				wg.Go(func() {
					<-start
					value, replay, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, ids[i], commands[i])
					results <- outcome{value, replay, err}
				})
			}
			close(start)
			wg.Wait()
			close(results)
			success, duplicates := 0, 0
			var created app.UnitResult
			for result := range results {
				if result.replay {
					t.Fatal("distinct unit intent became replay")
				}
				if result.err == nil {
					success++
					created = result.value
					continue
				}
				var duplicate app.UnitDuplicateError
				if !errors.As(result.err, &duplicate) || duplicate.Field() != field || result.value != (app.UnitResult{}) {
					t.Fatal("unit duplicate race lost safe field refusal", result.err)
				}
				duplicates++
			}
			if success != 1 || duplicates != n-1 {
				t.Fatal("unit duplicate race produced extra content")
			}
			var receipts, audits int
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_unit_commands WHERE org_id=$1),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='org.unit.created')`, fx.scope.OrgID().String()).Scan(&receipts, &audits); err != nil || receipts != 1 || audits != 1 {
				t.Fatal("duplicate unit race left orphan receipts/audit", err)
			}
			// Competing expected-revision changes produce one new revision and one conflict.
			changes := make([]app.UnitWrite, 2)
			for i := range 2 {
				changes[i] = operatingUnitCommand(t, app.UnitWriteInput{Kind: app.ChangeUnit, UnitID: created.UnitID, ExpectedRevision: 1, Profile: d.UnitProfile{Name: fmt.Sprintf("Synthetic revision %d", i), Code: fmt.Sprintf("REV%d", i)}})
			}
			errorsOut := make(chan error, 2)
			begin := make(chan struct{})
			for i := range 2 {
				wg.Go(func() {
					<-begin
					_, _, err := store.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", changes[i])
					errorsOut <- err
				})
			}
			close(begin)
			wg.Wait()
			close(errorsOut)
			success, conflicts := 0, 0
			for err := range errorsOut {
				if err == nil {
					success++
				} else if errors.Is(err, d.ErrConflict) {
					conflicts++
				} else {
					t.Fatal("unexpected unit revision race", err)
				}
			}
			if success != 1 || conflicts != 1 {
				t.Fatal("stale unit revision escaped serialization")
			}
		})
	}
}
