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

func costCentreSchemaFixture(t *testing.T) legalFixtureState {
	t.Helper()
	fx := legalFixture(t)
	grantCostCentreTables(t, fx)
	return fx
}
func grantCostCentreTables(t *testing.T, fx legalFixtureState) {
	t.Helper()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON org_cost_centres TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()+"; GRANT SELECT,INSERT ON org_cost_centre_codes,org_cost_centre_commands TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
}
func costCentreSchemaCode(t *testing.T, err error, want string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != want {
		t.Fatalf("cost-centre SQL refusal mismatch, want %s", want)
	}
	if strings.Contains(pe.Detail, "Synthetic") || strings.Contains(pe.Detail, "PRIVATE") {
		t.Fatal("cost-centre SQL detail leaked private content")
	}
}
func TestCostCentreSchemaRetainsHistoricalCodeMeaningAndIsolation(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	org := fx.scope.OrgID().String()
	id := onboardingID(t)
	insert := `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,$3,$4,'ACTIVE')`
	if err := unitSchemaExec(t, fx, insert, org, id, "Synthetic centre", "OLD"); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, `UPDATE org_cost_centres SET code='NEW',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, id); err != nil {
		t.Fatal(err)
	}
	costCentreSchemaCode(t, unitSchemaExec(t, fx, insert, org, onboardingID(t), "Synthetic other", "old"), "P7801")
	if err := unitSchemaExec(t, fx, `UPDATE org_cost_centres SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, id); err != nil {
		t.Fatal(err)
	}
	costCentreSchemaCode(t, unitSchemaExec(t, fx, insert, org, onboardingID(t), "Synthetic reused archive", "new"), "P7801")
	if err := unitSchemaExec(t, fx, `UPDATE org_cost_centres SET code='old',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, id); err != nil {
		t.Fatal("own historical reservation cannot be selected", err)
	}
	var count int
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_cost_centre_codes WHERE org_id=$1 AND centre_id=$2`, org, id).Scan(&count)
	}); err != nil || count != 2 {
		t.Fatal("historical reservation lost or duplicated", err)
	}
	costCentreSchemaCode(t, unitSchemaExec(t, fx, `DELETE FROM org_cost_centres WHERE org_id=$1 AND id=$2`, org, id), "55000")
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_cost_centre_codes`).Scan(&count); err != nil || count != 0 {
		t.Fatal("unscoped code reservation exposed", err)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if err := unitSchemaExec(t, other, insert, other.scope.OrgID().String(), onboardingID(t), "Synthetic independent", "OLD"); err != nil {
		t.Fatal("foreign code prevented independent declaration", err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_unknown_centre_consumer(org_id uuid,centre_id uuid,FOREIGN KEY(org_id,centre_id) REFERENCES org_cost_centres(org_id,id))`); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, `UPDATE org_cost_centres SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, id); err != nil {
		t.Fatal(err)
	}
	costCentreSchemaCode(t, unitSchemaExec(t, fx, `UPDATE org_cost_centres SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, id), "P7804")
}
func TestCostCentreSchemaSerializesDuplicateReservations(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	const n = 12
	ids := make([]string, n)
	for i := range ids {
		ids[i] = onboardingID(t)
	}
	results := make(chan error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() {
			<-start
			results <- unitSchemaExec(t, fx, `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic race','RACE','ACTIVE')`, fx.scope.OrgID().String(), id)
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
			costCentreSchemaCode(t, err, "P7801")
			denied++
		}
	}
	if success != 1 || denied != 11 {
		t.Fatal("duplicate reservations created multiple centres")
	}
}

// This disposable source proves the admission/count port; it does not implement
// production employees, assets, budgeting or operations.
func installSyntheticCostCentreSource(t *testing.T, fx legalFixtureState) {
	t.Helper()
	if _, err := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_cost_work(org_id uuid NOT NULL,centre_id uuid NOT NULL,id uuid NOT NULL,kind text NOT NULL CHECK(kind IN('ASSET','EMPLOYEE','BUDGET','OPERATION')),state text NOT NULL CHECK(state IN('ACTIVE','HISTORY')),PRIMARY KEY(org_id,id),FOREIGN KEY(org_id,centre_id) REFERENCES org_cost_centres(org_id,id));
 ALTER TABLE synthetic_cost_work ENABLE ROW LEVEL SECURITY;ALTER TABLE synthetic_cost_work FORCE ROW LEVEL SECURITY;
 CREATE POLICY synthetic_cost_scope ON synthetic_cost_work USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);
 CREATE FUNCTION synthetic_cost_admission() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='ACTIVE' THEN PERFORM org_cost_centre_admit(NEW.org_id,NEW.centre_id);ELSE PERFORM 1 FROM organizations WHERE org_id=NEW.org_id FOR UPDATE;END IF;RETURN NEW;END $$;
 CREATE TRIGGER synthetic_cost_admission BEFORE INSERT OR UPDATE ON synthetic_cost_work FOR EACH ROW EXECUTE FUNCTION synthetic_cost_admission();
 CREATE OR REPLACE FUNCTION org_cost_centre_current_usage(selected_org uuid,selected_centre uuid) RETURNS jsonb LANGUAGE plpgsql SET search_path=pg_catalog AS $$ BEGIN
 IF selected_org IS DISTINCT FROM NULLIF(current_setting('anpfuel.org_id',true),'')::uuid THEN RAISE EXCEPTION USING ERRCODE='42501',MESSAGE='organization scope denied';END IF;
 PERFORM 1 FROM public.organizations WHERE org_id=selected_org FOR UPDATE;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid IN('public.org_cost_centres'::regclass,'public.org_cost_centre_codes'::regclass) AND NOT((c.conrelid='public.org_cost_centre_codes'::regclass AND c.conname='org_cost_centre_codes_centre_fkey') OR(c.conrelid='public.org_cost_centre_commands'::regclass AND c.conname='org_cost_centre_commands_centre_fkey') OR(c.conrelid='public.org_cost_centres'::regclass AND c.conname='org_cost_centre_current_code_fkey') OR c.conrelid='public.synthetic_cost_work'::regclass)) THEN RAISE EXCEPTION USING ERRCODE='P7804',MESSAGE='cost-centre usage source unavailable';END IF;
 RETURN(SELECT jsonb_build_object('assets',count(*) FILTER(WHERE kind='ASSET'),'employees',count(*) FILTER(WHERE kind='EMPLOYEE'),'budgets',count(*) FILTER(WHERE kind='BUDGET'),'operations',count(*) FILTER(WHERE kind='OPERATION')) FROM public.synthetic_cost_work WHERE org_id=selected_org AND centre_id=selected_centre AND state='ACTIVE');END $$;`); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON synthetic_cost_work TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
}
func TestCostCentreUsageAndArchiveAdmissionRace(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	org := fx.scope.OrgID().String()
	id := onboardingID(t)
	if err := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic used centre','USED','ACTIVE')`, org, id); err != nil {
		t.Fatal(err)
	}
	installSyntheticCostCentreSource(t, fx)
	insert := `INSERT INTO synthetic_cost_work(org_id,centre_id,id,kind,state) VALUES($1,$2,$3,$4,'ACTIVE')`
	archive := `UPDATE org_cost_centres SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`
	for _, kind := range []string{"ASSET", "EMPLOYEE", "BUDGET", "OPERATION"} {
		work := onboardingID(t)
		if err := unitSchemaExec(t, fx, insert, org, id, work, kind); err != nil {
			t.Fatal(err)
		}
		costCentreSchemaCode(t, unitSchemaExec(t, fx, archive, org, id), "P7803")
		if err := unitSchemaExec(t, fx, `UPDATE synthetic_cost_work SET state='HISTORY' WHERE org_id=$1 AND id=$2`, org, work); err != nil {
			t.Fatal(err)
		}
	}
	if err := unitSchemaExec(t, fx, archive, org, id); err != nil {
		t.Fatal("historical reference prevented archive", err)
	}
	costCentreSchemaCode(t, unitSchemaExec(t, fx, insert, org, id, onboardingID(t), "ASSET"), "23503")
	for range 12 {
		if err := unitSchemaExec(t, fx, `UPDATE org_cost_centres SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, id); err != nil {
			t.Fatal(err)
		}
		begin := make(chan struct{})
		results := make(chan error, 2)
		work := onboardingID(t)
		var wg sync.WaitGroup
		wg.Go(func() { <-begin; results <- unitSchemaExec(t, fx, archive, org, id) })
		wg.Go(func() { <-begin; results <- unitSchemaExec(t, fx, insert, org, id, work, "ASSET") })
		close(begin)
		wg.Wait()
		close(results)
		success, refused := 0, 0
		for err := range results {
			if err == nil {
				success++
				continue
			}
			var pe *pgconn.PgError
			if !errors.As(err, &pe) || (pe.Code != "23503" && pe.Code != "P7803") {
				t.Fatal("unexpected cost-centre race failure", err)
			}
			refused++
		}
		if success != 1 || refused != 1 {
			t.Fatal("archive and active assignment escaped serialization")
		}
		var state string
		var count int
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT c.state,(SELECT count(*) FROM synthetic_cost_work w WHERE(w.org_id,w.centre_id)=(c.org_id,c.id) AND w.state='ACTIVE') FROM org_cost_centres c WHERE org_id=$1 AND id=$2`, org, id).Scan(&state, &count); err != nil {
			t.Fatal(err)
		}
		if state == "ARCHIVED" && count != 0 || state == "ACTIVE" && count != 1 {
			t.Fatal("archived centre has active work")
		}
		if state == "ACTIVE" {
			if err := unitSchemaExec(t, fx, `UPDATE synthetic_cost_work SET state='HISTORY' WHERE org_id=$1 AND id=$2`, org, work); err != nil {
				t.Fatal(err)
			}
			if err := unitSchemaExec(t, fx, archive, org, id); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestCostCentreReservationsCannotBeReassignedOrInvented(t *testing.T) {
	fx := costCentreSchemaFixture(t)
	org := fx.scope.OrgID().String()
	id := onboardingID(t)
	if err := unitSchemaExec(t, fx, `INSERT INTO org_cost_centres(org_id,id,name,code,state) VALUES($1,$2,'Synthetic stable centre','PRIVATE-CODE','ACTIVE')`, org, id); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE ON org_cost_centre_codes TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	costCentreSchemaCode(t, unitSchemaExec(t, fx, `UPDATE org_cost_centre_codes SET code='OTHER' WHERE org_id=$1`, org), "55000")
	costCentreSchemaCode(t, unitSchemaExec(t, fx, `DELETE FROM org_cost_centre_codes WHERE org_id=$1`, org), "55000")
	costCentreSchemaCode(t, unitSchemaExec(t, fx, `INSERT INTO org_cost_centre_codes(org_id,code,centre_id) VALUES($1,'INVENTED',$2)`, org, id), "23503")
}
