//go:build integration

package adapters_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func unitSchemaExec(t *testing.T, fx legalFixtureState, query string, args ...any) error {
	t.Helper()
	return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), query, args...); return err })
}
func unitSchemaCode(t *testing.T, err error, want string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != want {
		t.Fatalf("unit SQL refusal mismatch, want %s", want)
	}
	if strings.Contains(pe.Detail, "Synthetic") || strings.Contains(pe.Detail, "private") {
		t.Fatal("SQL refusal leaked private unit data")
	}
}
func TestOperatingUnitSchemaProtectsArchiveUniquenessAndReferences(t *testing.T) {
	fx := legalFixture(t)
	org := fx.scope.OrgID().String()
	second := onboardingID(t)
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT DELETE ON org_units TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	var first string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, org).Scan(&first); err != nil {
		t.Fatal(err)
	}
	unitSchemaCode(t, unitSchemaExec(t, fx, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, first), "P7506")
	insert := `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,$3,$4,'America/Cuiaba','ACTIVE')`
	if err := unitSchemaExec(t, fx, insert, org, second, "Synthetic North", "N01"); err != nil {
		t.Fatal(err)
	}
	unitSchemaCode(t, unitSchemaExec(t, fx, insert, org, onboardingID(t), "synthetic north", "N02"), "P7701")
	unitSchemaCode(t, unitSchemaExec(t, fx, insert, org, onboardingID(t), "Synthetic South", "n01"), "P7702")
	unitSchemaCode(t, unitSchemaExec(t, fx, `UPDATE org_units SET time_zone='GMT',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, second), "23514")
	unitSchemaCode(t, unitSchemaExec(t, fx, `UPDATE org_units SET establishment_id=$3,revision=revision+1 WHERE org_id=$1 AND id=$2`, org, second, onboardingID(t)), "23503")
	if err := unitSchemaExec(t, fx, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, second); err != nil {
		t.Fatal("known historical references incorrectly count as work", err)
	}
	if err := unitSchemaExec(t, fx, insert, org, onboardingID(t), "Synthetic North", "N01"); err != nil {
		t.Fatal("archived name/code reserved", err)
	}
	unitSchemaCode(t, unitSchemaExec(t, fx, `UPDATE org_units SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, second), "P7701")
	if err := unitSchemaExec(t, fx, `UPDATE org_units SET name='Synthetic corrected',code='C01',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, second); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, `UPDATE org_units SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, second); err != nil {
		t.Fatal(err)
	}
	unitSchemaCode(t, unitSchemaExec(t, fx, `DELETE FROM org_units WHERE org_id=$1 AND id=$2`, org, second), "55000")
	// A new source without an implemented usage policy must not be treated as empty.
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_unknown_unit_source(org_id uuid,unit_id uuid,FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id))`); err != nil {
		t.Fatal(err)
	}
	unitSchemaCode(t, unitSchemaExec(t, fx, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, second), "P7704")
}
func TestOperatingUnitSchemaSerializesLastUnitRace(t *testing.T) {
	fx := legalFixture(t)
	org := fx.scope.OrgID().String()
	second := onboardingID(t)
	if err := unitSchemaExec(t, fx, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic other','OTHER','UTC','ACTIVE')`, org, second); err != nil {
		t.Fatal(err)
	}
	var first string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1 AND id<>$2`, org, second).Scan(&first); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{first, second} {
		wg.Go(func() {
			<-start
			results <- unitSchemaExec(t, fx, `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, id)
		})
	}
	close(start)
	wg.Wait()
	close(results)
	success, denied := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			unitSchemaCode(t, err, "P7506")
			denied++
		}
	}
	if success != 1 || denied != 1 {
		t.Fatal("racing archive removed last active unit")
	}
	var active int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_units WHERE org_id=$1 AND state='ACTIVE'`, org).Scan(&active); err != nil || active != 1 {
		t.Fatal("invalid committed active count", err)
	}
}

// This disposable source demonstrates the owner migration/admission contract;
// it is not a production asset/employee/budget/operation implementation.
func installSyntheticUnitUsageSource(t *testing.T, fx legalFixtureState) {
	t.Helper()
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_unit_work(
 org_id uuid NOT NULL,unit_id uuid NOT NULL,id uuid NOT NULL,kind text NOT NULL CHECK(kind IN('ASSET','EMPLOYEE','BUDGET','OPERATION')),
 state text NOT NULL CHECK(state IN('ACTIVE','HISTORY')),PRIMARY KEY(org_id,id),FOREIGN KEY(org_id,unit_id) REFERENCES org_units(org_id,id));
 ALTER TABLE synthetic_unit_work ENABLE ROW LEVEL SECURITY; ALTER TABLE synthetic_unit_work FORCE ROW LEVEL SECURITY;
 CREATE POLICY synthetic_unit_work_scope ON synthetic_unit_work USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
 CREATE FUNCTION synthetic_unit_work_admission() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.state='ACTIVE' THEN PERFORM org_unit_admit(NEW.org_id,NEW.unit_id);
 ELSE PERFORM 1 FROM organizations WHERE org_id=NEW.org_id FOR UPDATE; END IF; RETURN NEW; END $$;
 CREATE TRIGGER synthetic_unit_work_admission BEFORE INSERT OR UPDATE ON synthetic_unit_work FOR EACH ROW EXECUTE FUNCTION synthetic_unit_work_admission();
 CREATE OR REPLACE FUNCTION org_unit_current_usage(selected_org uuid,selected_unit uuid) RETURNS jsonb LANGUAGE plpgsql SET search_path=pg_catalog AS $$
 BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org FOR UPDATE;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid='public.org_units'::regclass
 AND NOT ((c.conrelid='public.org_onboarding_commands'::regclass AND c.conname='org_onboarding_commands_org_id_unit_id_fkey')
 OR (c.conrelid='public.org_unit_commands'::regclass AND c.conname='org_unit_commands_unit_fkey')
 OR (c.conrelid='public.org_employee_profiles'::regclass AND c.conname='org_employee_profile_unit_fkey')
 OR (c.conrelid='public.org_teams'::regclass AND c.conname='org_team_unit_fkey')
 OR (c.conrelid='public.org_assets'::regclass AND c.conname='org_asset_unit_fkey')
 OR (c.conrelid='public.org_asset_unit_allocations'::regclass AND c.conname='org_asset_allocation_unit_fkey')
 OR (c.conrelid='public.org_meter_held_readings'::regclass AND c.conname='org_meter_held_readings_unit_fkey')
 OR c.conrelid='public.synthetic_unit_work'::regclass)) THEN RAISE EXCEPTION USING ERRCODE='P7704',MESSAGE='unit usage source unavailable';END IF;
 RETURN (SELECT jsonb_build_object('assets',count(*) FILTER(WHERE kind='ASSET')+(SELECT count(*) FROM public.org_asset_unit_allocations p JOIN public.org_assets a ON(a.org_id,a.id)=(p.org_id,p.asset_id) WHERE p.org_id=selected_org AND p.unit_id=selected_unit AND p.ended_at IS NULL AND a.state='ACTIVE'),'employees',count(*) FILTER(WHERE kind='EMPLOYEE')+(SELECT count(*) FROM (
  SELECT p.employee_id FROM public.org_employee_profiles p JOIN public.org_employee_identities e ON(e.org_id,e.id)=(p.org_id,p.employee_id) WHERE p.org_id=selected_org AND p.home_unit_id=selected_unit AND e.state='ACTIVE'
  UNION SELECT l.employee_id FROM public.org_team_links l JOIN public.org_teams t ON(t.org_id,t.id)=(l.org_id,l.team_id) JOIN public.org_employee_identities e ON(e.org_id,e.id)=(l.org_id,l.employee_id) WHERE l.org_id=selected_org AND t.unit_id=selected_unit AND l.ended_at IS NULL AND e.state='ACTIVE') actual_employees),
 'budgets',count(*) FILTER(WHERE kind='BUDGET'),'operations',count(*) FILTER(WHERE kind='OPERATION')) FROM public.synthetic_unit_work WHERE org_id=selected_org AND unit_id=selected_unit AND state='ACTIVE');
 END $$;`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON synthetic_unit_work TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
}
func TestOperatingUnitSourceUsageAndAssignmentArchiveRace(t *testing.T) {
	fx := legalFixture(t)
	org := fx.scope.OrgID().String()
	second := onboardingID(t)
	if err := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Synthetic North','NORTH','UTC','ACTIVE')`, org, second); err != nil {
		t.Fatal(err)
	}
	installSyntheticUnitUsageSource(t, fx)
	insert := `INSERT INTO synthetic_unit_work(org_id,unit_id,id,kind,state) VALUES($1,$2,$3,$4,'ACTIVE')`
	archive := `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`
	for _, kind := range []string{"ASSET", "EMPLOYEE", "BUDGET", "OPERATION"} {
		id := onboardingID(t)
		if err := unitSchemaExec(t, fx, insert, org, second, id, kind); err != nil {
			t.Fatal(err)
		}
		unitSchemaCode(t, unitSchemaExec(t, fx, archive, org, second), "P7703")
		if err := unitSchemaExec(t, fx, `UPDATE synthetic_unit_work SET state='HISTORY' WHERE org_id=$1 AND id=$2`, org, id); err != nil {
			t.Fatal(err)
		}
	}
	// Historical rows remain referenced yet do not block archival.
	if err := unitSchemaExec(t, fx, archive, org, second); err != nil {
		t.Fatal("history prevented archive", err)
	}
	unitSchemaCode(t, unitSchemaExec(t, fx, insert, org, second, onboardingID(t), "ASSET"), "23503")
	restore := `UPDATE org_units SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`
	for range 12 {
		if err := unitSchemaExec(t, fx, restore, org, second); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		id := onboardingID(t)
		var wg sync.WaitGroup
		wg.Go(func() { <-start; results <- unitSchemaExec(t, fx, archive, org, second) })
		wg.Go(func() { <-start; results <- unitSchemaExec(t, fx, insert, org, second, id, "ASSET") })
		close(start)
		wg.Wait()
		close(results)
		success, refused := 0, 0
		for err := range results {
			if err == nil {
				success++
				continue
			}
			var pe *pgconn.PgError
			if !errors.As(err, &pe) || (pe.Code != "23503" && pe.Code != "P7703") {
				t.Fatal("unexpected racing unit failure", err)
			}
			refused++
		}
		if success != 1 || refused != 1 {
			t.Fatal("archive and new assignment both succeeded or both failed")
		}
		var state string
		var current int64
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT u.state,(SELECT count(*) FROM synthetic_unit_work w WHERE (w.org_id,w.unit_id)=(u.org_id,u.id) AND w.state='ACTIVE') FROM org_units u WHERE org_id=$1 AND id=$2`, org, second).Scan(&state, &current); err != nil {
			t.Fatal(err)
		}
		if state == "ARCHIVED" && current != 0 || state == "ACTIVE" && current != 1 {
			t.Fatal("committed archived unit retained active work")
		}
		if state == "ACTIVE" {
			if err := unitSchemaExec(t, fx, `UPDATE synthetic_unit_work SET state='HISTORY' WHERE org_id=$1 AND id=$2`, org, id); err != nil {
				t.Fatal(err)
			}
			if err := unitSchemaExec(t, fx, archive, org, second); err != nil {
				t.Fatal(err)
			}
		}
	}
}
