//go:build integration

package adapters_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func schemaCode(t *testing.T, err error, want string) {
	t.Helper()
	var p *pgconn.PgError
	if !errors.As(err, &p) || p.Code != want {
		t.Fatalf("want %s, got %v", want, err)
	}
}
func TestOnboardingSchemaGuardsManagedBindingAndDefaultUnit(t *testing.T) {
	f := orgtest.New(t)
	var complete bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT count(*)=2 AND bool_and(relrowsecurity AND relforcerowsecurity) FROM pg_class WHERE oid IN (to_regclass('org_units'),to_regclass('org_onboarding_commands'))`).Scan(&complete); err != nil || !complete {
		t.Fatal("onboarding tables missing or not forced", err)
	}
	r := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON organizations,accounts,org_units TO "+r); err != nil {
		t.Fatal(err)
	}
	a := scopeFor(t, "7b0e2a54-0000-4000-8000-000000000301")
	root, err := domain.NewPending(a, domain.Profile{Name: "Synthetic company", TimeZone: "UTC", Kinds: []domain.Kind{domain.Other}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapters.NewStore(f.Runtime).Create(t.Context(), a, root); err != nil {
		t.Fatal(err)
	}
	account := "7b0e2a54-0000-4000-8000-000000000701"
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO accounts(id,alias,status,enterprise_managed_org_id) VALUES($1,'synthetic-managed','active',$2)`, account, a.OrgID().String())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, err = f.Admin.Exec(t.Context(), `UPDATE accounts SET enterprise_managed_org_id=NULL WHERE id=$1`, account)
	schemaCode(t, err, "55000")
	unit := "7b0e2a54-0000-4000-8000-000000000501"
	if err := database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_units(org_id,id,name,code,time_zone,state) VALUES($1,$2,'Main synthetic unit','MAIN','UTC','ACTIVE')`, a.OrgID().String(), unit)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err = database.WithOrgTx(t.Context(), f.Runtime, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_units SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1 AND id=$2`, a.OrgID().String(), unit)
		return err
	})
	schemaCode(t, err, "P7506")
	// A supplied pure-domain checklist cannot replace persisted terms/owner facts.
	next, err := root.Transition(a, 1, domain.Active, domain.Conditions{ActiveUnits: 1, ActiveOwners: 1, TermsRecorded: true})
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := adapters.NewStore(f.Runtime).Save(t.Context(), a, 1, next); changed || err == nil {
		t.Fatal("activation bypassed persisted setup", changed, err)
	}
	if _, err := adapters.NewStore(f.Runtime).Get(t.Context(), kernel.OrgScope{}); err != domain.ErrScope {
		t.Fatal(err)
	}
}

func TestOnboardingReceiptAndUnitIsolationAndTermsConstraints(t *testing.T) {
	f, _, service, actor, _ := onboardingFixture(t)
	a, _, err := service.Create(t.Context(), actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := service.Create(t.Context(), actor, onboardingCommand(t, onboardingID(t)))
	if err != nil {
		t.Fatal(err)
	}
	scope := scopeFor(t, a.OrgID.String())
	for _, table := range []string{"org_units", "org_onboarding_commands"} {
		var count int
		if err := f.Runtime.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil || count != 0 {
			t.Fatal("unscoped private setup exposed", table, err)
		}
		if err := database.WithOrgTx(t.Context(), f.Runtime, scope, func(tx pgx.Tx) error {
			return tx.QueryRow(t.Context(), "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()+" WHERE org_id=$1", b.OrgID.String()).Scan(&count)
		}); err != nil || count != 0 {
			t.Fatal("foreign private setup exposed", table, err)
		}
	}
	for _, terms := range []string{"NULL,terms_digest,terms_kind", "terms_version,NULL,terms_kind", "terms_version,terms_digest,NULL", "terms_version,repeat('0',64),terms_kind"} {
		operation := onboardingID(t)
		err := database.WithOrgTx(t.Context(), f.Runtime, scope, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), `INSERT INTO org_onboarding_commands(org_id,operation_id,kind,actor_id,request_hash,unit_id,employee_id,membership_id,assignment_id,terms_version,terms_digest,terms_kind,result_revision)
 SELECT org_id,$1,kind,actor_id,request_hash,unit_id,employee_id,membership_id,assignment_id,`+terms+`,result_revision FROM org_onboarding_commands WHERE org_id=$2 AND kind='CREATED'`, operation, a.OrgID.String())
			return err
		})
		schemaCode(t, err, "23514")
	}
	if _, err := f.Admin.Exec(t.Context(), "GRANT ALL ON org_onboarding_commands,org_units TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`UPDATE org_onboarding_commands SET result_revision=result_revision+1`, `DELETE FROM org_onboarding_commands`, `TRUNCATE org_units CASCADE`} {
		err := database.WithOrgTx(t.Context(), f.Runtime, scope, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), statement); return err })
		schemaCode(t, err, "55000")
	}
}
