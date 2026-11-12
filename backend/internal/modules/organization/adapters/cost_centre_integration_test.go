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
	cap "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

func costCentreStore(t *testing.T, fx legalFixtureState) *adapters.CostCentreStore {
	t.Helper()
	catalog, err := audit.NewCatalog(adapters.CostCentreAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &adapters.CostCentreStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
func costCentreCommand(t *testing.T, v app.CostCentreWriteInput) app.CostCentreWrite {
	t.Helper()
	if v.OperationID == "" {
		v.OperationID = onboardingID(t)
	}
	c, err := app.NewCostCentreWrite(v)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestCostCentreStorePreservesAliasesReplayAndArchivedIdentity(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	store := costCentreStore(t, fx)
	write := func(command app.CostCentreWrite) (app.CostCentreResult, bool, error) {
		t.Helper()
		v, err := command.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		id := ""
		if v.Kind == app.CreateCostCentre {
			id = onboardingID(t)
		}
		return store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, id, command)
	}
	create := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: "Synthetic centre", Code: " OLD "}})
	first, replay, err := write(create)
	if err != nil || replay || first.Revision != 1 {
		t.Fatal("cost-centre create", err)
	}
	if result, replay, err := write(create); err != nil || !replay || result != first {
		t.Fatal("cost-centre creation retry", err)
	}
	change := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.ChangeCostCentre, CentreID: first.CentreID, ExpectedRevision: 1, Profile: d.CostCentreProfile{Name: "Synthetic corrected", Code: "NEW"}})
	changed, _, err := write(change)
	if err != nil || changed.Revision != 2 {
		t.Fatal("code correction", err)
	}
	if result, replay, err := write(change); err != nil || !replay || result != changed {
		t.Fatal("code correction retry", err)
	}
	if result, replay, err := write(create); err != nil || !replay || result != first {
		t.Fatal("creation retry returned current metadata", err)
	}
	for _, code := range []string{"old", " NEW "} {
		value, err := store.ReadCostCentreCode(t.Context(), fx.scope, fx.actor, fx.member, code)
		if err != nil {
			t.Fatal(err)
		}
		snap, err := value.Snapshot(fx.scope)
		if err != nil || snap.ID != first.CentreID || snap.Code != "NEW" {
			t.Fatal("historical alias remapped", err)
		}
	}
	for _, code := range []string{"OLD", "new"} {
		_, _, err := write(costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: "Synthetic reused", Code: code}}))
		if !errors.Is(err, app.ErrCostCentreCodeReserved) {
			t.Fatal("historical/current code was reused", err)
		}
	}
	archive := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.ArchiveCostCentre, CentreID: first.CentreID, ExpectedRevision: 2})
	archived, _, err := write(archive)
	if err != nil || archived.State != d.CostCentreArchived || archived.Revision != 3 {
		t.Fatal("centre archive", err)
	}
	if _, _, err := write(costCentreCommand(t, app.CostCentreWriteInput{Kind: app.RestoreCostCentre, CentreID: first.CentreID, ExpectedRevision: 2})); !errors.Is(err, d.ErrConflict) {
		t.Fatal("stale centre restore admitted", err)
	}
	if _, _, err := write(costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: "Synthetic reuse archive", Code: "OLD"}})); !errors.Is(err, app.ErrCostCentreCodeReserved) {
		t.Fatal("archived historical code reused", err)
	}
	restored, _, err := write(costCentreCommand(t, app.CostCentreWriteInput{Kind: app.RestoreCostCentre, CentreID: first.CentreID, ExpectedRevision: 3}))
	if err != nil || restored.Revision != 4 || restored.CentreID != first.CentreID {
		t.Fatal("restore lost centre identity", err)
	}
	// Names are descriptive: distinct codes can share a name.
	second, _, err := write(costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: "Synthetic corrected", Code: "OTHER"}}))
	if err != nil || second.CentreID == first.CentreID {
		t.Fatal("name incorrectly unique", err)
	}
	page, err := store.ListCostCentres(t.Context(), fx.scope, fx.actor, fx.member, app.CostCentreFilter{Limit: 1})
	if err != nil || len(page.Centres) != 1 || page.NextID == "" {
		t.Fatal("centre keyset page", err)
	}
	next, err := store.ListCostCentres(t.Context(), fx.scope, fx.actor, fx.member, app.CostCentreFilter{Limit: 1, AfterID: page.NextID})
	if err != nil || len(next.Centres) != 1 || next.NextID != "" {
		t.Fatal("centre keyset continuation", err)
	}
	if _, err := store.FindCostCentre(t.Context(), scopeFor(t, onboardingID(t)), first.CentreID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("foreign centre exposed", err)
	}
	var payload string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT COALESCE(string_agg(to_jsonb(e)::text,''),'') FROM org_audit_events e`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, "Synthetic corrected") || strings.Contains(payload, "OLD") {
		t.Fatal("private centre content entered audit")
	}
}
func TestCostCentreStoreSerializesReservationsAndRollsBackAliases(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(fmt.Sprint(same), func(t *testing.T) {
			fx := costCentreSchemaFixture(t)
			store := costCentreStore(t, fx)
			const n = 12
			commands := make([]app.CostCentreWrite, n)
			ids := make([]string, n)
			operation := onboardingID(t)
			for i := range n {
				if !same {
					operation = onboardingID(t)
				}
				commands[i] = costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, OperationID: operation, Profile: d.CostCentreProfile{Name: "Synthetic race", Code: "RACE"}})
				ids[i] = onboardingID(t)
			}
			type outcome struct {
				value  app.CostCentreResult
				replay bool
				err    error
			}
			out := make(chan outcome, n)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range n {
				wg.Go(func() {
					<-start
					value, replay, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, ids[i], commands[i])
					out <- outcome{value, replay, err}
				})
			}
			close(start)
			wg.Wait()
			close(out)
			fresh, replays, duplicates := 0, 0, 0
			chosen := ""
			for result := range out {
				if result.err != nil {
					if !errors.Is(result.err, app.ErrCostCentreCodeReserved) || result.replay || result.value != (app.CostCentreResult{}) {
						t.Fatal("unexpected centre race refusal", result.err)
					}
					duplicates++
					continue
				}
				if chosen == "" {
					chosen = result.value.CentreID
				}
				if result.value.CentreID != chosen {
					t.Fatal("duplicate code selected distinct identities")
				}
				if result.replay {
					replays++
				} else {
					fresh++
				}
			}
			if fresh != 1 || same && (replays != 11 || duplicates != 0) || !same && (duplicates != 11 || replays != 0) {
				t.Fatal("wrong centre race outcomes")
			}
			if _, err := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_cost_receipt_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic receipt outage';END $$; CREATE CONSTRAINT TRIGGER synthetic_cost_receipt_failure AFTER INSERT ON org_cost_centre_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_cost_receipt_failure()`); err != nil {
				t.Fatal(err)
			}
			command := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.ChangeCostCentre, CentreID: chosen, ExpectedRevision: 1, Profile: d.CostCentreProfile{Name: "Synthetic failed correction", Code: "ROLLBACK"}})
			result, replay, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, "", command)
			if !errors.Is(err, app.ErrUnavailable) || replay || result != (app.CostCentreResult{}) {
				t.Fatal("failed correction returned provisional acknowledgement", err)
			}
			var count int
			if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_cost_centre_codes WHERE org_id=$1 AND code_key='rollback'`, fx.scope.OrgID().String()).Scan(&count); err != nil || count != 0 {
				t.Fatal("failed commit retained code reservation", err)
			}
			centre, err := store.FindCostCentre(t.Context(), fx.scope, chosen)
			if err != nil {
				t.Fatal(err)
			}
			snap, err := centre.Snapshot(fx.scope)
			if err != nil || snap.Revision != 1 || snap.Code != "RACE" {
				t.Fatal("failed correction rewrote centre", err)
			}
		})
	}
}

func TestCostCentreStoreReportsActualUsageAndChecksNewAliasConsumers(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	store := costCentreStore(t, fx)
	org := fx.scope.OrgID().String()
	if err := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	create := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: "Synthetic used centre", Code: "USED"}})
	centre, _, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), create)
	if err != nil {
		t.Fatal(err)
	}
	installSyntheticCostCentreSource(t, fx)
	if err := unitSchemaExec(t, fx, `INSERT INTO synthetic_cost_work(org_id,centre_id,id,kind,state) VALUES($1,$2,$3,'BUDGET','ACTIVE')`, org, centre.CentreID, onboardingID(t)); err != nil {
		t.Fatal(err)
	}
	command := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.ArchiveCostCentre, CentreID: centre.CentreID, ExpectedRevision: 1})
	result, replay, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, "", command)
	var used app.StructureUsageError
	if !errors.As(err, &used) || used.Usage() != (d.StructureUsage{Budgets: 1}) || replay || result != (app.CostCentreResult{}) {
		t.Fatal("actual scoped usage counts lost", err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_unknown_alias_consumer(org_id uuid,code_key text,FOREIGN KEY(org_id,code_key) REFERENCES org_cost_centre_codes(org_id,code_key))`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, "", command); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unknown alias consumer treated as empty", err)
	}
}

func TestCostCentreStoreRechecksProofAndCurrentCapability(t *testing.T) {
	for _, test := range []string{"revoked cookie", "revoked family", "stale authentication", "disabled flag"} {
		t.Run(test, func(t *testing.T) {
			fx := costCentreSchemaFixture(t)
			store := costCentreStore(t, fx)
			command := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: "Synthetic before", Code: "BEFORE"}})
			centre, _, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), command)
			if err != nil {
				t.Fatal(err)
			}
			switch test {
			case "revoked cookie":
				_, err = fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET revoked_at=clock_timestamp() WHERE account_id=$1`, fx.actor.AccountID)
			case "revoked family":
				_, err = fx.f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE account_id=$1`, fx.actor.AccountID)
			case "stale authentication":
				_, err = fx.f.Admin.Exec(t.Context(), `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '17 minutes',created_at=clock_timestamp()-interval '16 minutes',expires_at=clock_timestamp()+interval '7 hours' WHERE account_id=$1`, fx.actor.AccountID)
			case "disabled flag":
				_, err = fx.flags.Set(t.Context(), fx.scope, cap.Actor{AccountID: fx.actor.AccountID}, cap.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1})
			}
			if err != nil {
				t.Fatal(err)
			}
			next := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: "Synthetic after", Code: "AFTER"}})
			result, replay, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), next)
			if err == nil || replay || result != (app.CostCentreResult{}) {
				t.Fatal("stale proof/flag admitted new centre")
			}
			if test == "stale authentication" || test == "disabled flag" {
				if _, err := store.ReadCostCentre(t.Context(), fx.scope, fx.actor, fx.member, centre.CentreID); err != nil {
					t.Fatal("existing authorized read lost", err)
				}
			}
		})
	}
}

func TestCostCentreDeferredReservationFailureReturnsNoAcknowledgement(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	store := costCentreStore(t, fx)
	// Deliberately remove capture inside this disposable schema so the deferred
	// current-code ownership FK, rather than a callback error, rejects commit.
	if _, err := fx.f.Admin.Exec(t.Context(), `ALTER TABLE org_cost_centres DISABLE TRIGGER org_cost_centre_code_capture`); err != nil {
		t.Fatal(err)
	}
	command := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: "Synthetic missing reservation", Code: "MISSING"}})
	result, replay, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), command)
	if !errors.Is(err, app.ErrUnavailable) || replay || result != (app.CostCentreResult{}) {
		t.Fatal("deferred reservation failure was not an unavailable transaction", err)
	}
	var rows int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_cost_centres WHERE org_id=$1)+(SELECT count(*) FROM org_cost_centre_codes WHERE org_id=$1)+(SELECT count(*) FROM org_cost_centre_commands WHERE org_id=$1)`, fx.scope.OrgID().String()).Scan(&rows); err != nil || rows != 0 {
		t.Fatal("failed ownership FK left cost-centre data", err)
	}
}

func TestCostCentreConcurrentCorrectionsCannotReassignOneHistoricalCode(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	store := costCentreStore(t, fx)
	created := make([]app.CostCentreResult, 2)
	for i := range 2 {
		command := costCentreCommand(t, app.CostCentreWriteInput{Kind: app.CreateCostCentre, Profile: d.CostCentreProfile{Name: fmt.Sprintf("Synthetic correction %d", i), Code: fmt.Sprintf("OLD%d", i)}})
		value, _, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), command)
		if err != nil {
			t.Fatal(err)
		}
		created[i] = value
	}
	commands := make([]app.CostCentreWrite, 2)
	for i := range 2 {
		commands[i] = costCentreCommand(t, app.CostCentreWriteInput{Kind: app.ChangeCostCentre, CentreID: created[i].CentreID, ExpectedRevision: 1, Profile: d.CostCentreProfile{Name: fmt.Sprintf("Synthetic corrected %d", i), Code: "SHARED"}})
	}
	type result struct {
		value app.CostCentreResult
		err   error
	}
	out := make(chan result, 2)
	begin := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			<-begin
			value, _, err := store.WriteCostCentre(t.Context(), fx.scope, fx.actor, fx.member, "", commands[i])
			out <- result{value, err}
		})
	}
	close(begin)
	wg.Wait()
	close(out)
	success, denied := 0, 0
	winner := ""
	for value := range out {
		if value.err == nil {
			success++
			winner = value.value.CentreID
		} else if errors.Is(value.err, app.ErrCostCentreCodeReserved) {
			denied++
		} else {
			t.Fatal("unexpected alias correction race", value.err)
		}
	}
	if success != 1 || denied != 1 {
		t.Fatal("two corrections acquired the same code")
	}
	for i := range 2 {
		value, err := store.ReadCostCentreCode(t.Context(), fx.scope, fx.actor, fx.member, fmt.Sprintf("OLD%d", i))
		if err != nil {
			t.Fatal(err)
		}
		snapshot, err := value.Snapshot(fx.scope)
		if err != nil || snapshot.ID != created[i].CentreID {
			t.Fatal("correction race remapped an old export code", err)
		}
	}
	value, err := store.ReadCostCentreCode(t.Context(), fx.scope, fx.actor, fx.member, "shared")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := value.Snapshot(fx.scope)
	if err != nil || snapshot.ID != winner {
		t.Fatal("shared code did not resolve its committed winner", err)
	}
}
