//go:build integration

package adapters_test

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"testing"
)

func settingsSchemaFixture(t *testing.T) legalFixtureState {
	t.Helper()
	fx := legalFixture(t)
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_settings_commands TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	return fx
}
func settingsSchemaWrite(t *testing.T, fx legalFixtureState, expression string, publish bool) error {
	t.Helper()
	op := onboardingID(t)
	return database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		if _, e := tx.Exec(t.Context(), `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,reason_code,request_hash) VALUES($1,$2,$3,'org.settings.changed','org.settings',$1,'configuration',repeat('a',64))`, fx.scope.OrgID().String(), op, fx.actor.AccountID); e != nil {
			return e
		}
		if _, e := tx.Exec(t.Context(), `INSERT INTO org_settings_versions(org_id,revision,settings_values,source,author_account_id,author_membership_id,reason,operation_id)
 SELECT h.org_id,h.revision+1,`+expression+`,'COMMAND',$2,$3,'Synthetic settings reason',$4 FROM org_settings_heads h JOIN org_settings_versions v ON(v.org_id,v.revision)=(h.org_id,h.revision) WHERE h.org_id=$1`, fx.scope.OrgID().String(), fx.actor.AccountID, fx.member, op); e != nil {
			return e
		}
		if publish {
			_, e := tx.Exec(t.Context(), `UPDATE org_settings_heads SET revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String())
			return e
		}
		return nil
	})
}
func TestSettingsSchemaOnboardingDefaultsImmutablePublicationAndRootZone(t *testing.T) {
	fx := settingsSchemaFixture(t)
	org := fx.scope.OrgID().String()
	var valid bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT v.source='ONBOARDING' AND v.revision=1 AND v.author_account_id=$2 AND v.author_membership_id=$3 AND v.settings_values=org_settings_defaults(o.time_zone,$3::uuid::text) AND(SELECT count(*) FROM org_settings_designations WHERE org_id=o.org_id)=3 FROM organizations o JOIN org_settings_heads h USING(org_id) JOIN org_settings_versions v ON(v.org_id,v.revision)=(h.org_id,h.revision) WHERE o.org_id=$1`, org, fx.actor.AccountID, fx.member).Scan(&valid); e != nil || !valid {
		t.Fatal("defaults not atomically initialized", e)
	}
	var leaked int
	if e := fx.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_settings_versions`).Scan(&leaked); e != nil || leaked != 0 {
		t.Fatal("unscoped settings", e)
	}
	teamSQLCode(t, settingsSchemaWrite(t, fx, `jsonb_set(v.settings_values,'{time_zone}','"GMT"')`, true), "P8102")
	teamSQLCode(t, settingsSchemaWrite(t, fx, `v.settings_values-'review_required'`, true), "P8102")
	teamSQLCode(t, settingsSchemaWrite(t, fx, `v.settings_values`, false), "P8104")
	if e := settingsSchemaWrite(t, fx, `jsonb_set(v.settings_values,'{time_zone}','"Pacific/Auckland"')`, true); e != nil {
		t.Fatal("settings publication", e)
	}
	var zone string
	var revision, versions int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT time_zone,revision,(SELECT count(*) FROM org_settings_versions WHERE org_id=$1) FROM organizations WHERE org_id=$1`, org).Scan(&zone, &revision, &versions); e != nil || zone != "Pacific/Auckland" || revision != 2 || versions != 2 {
		t.Fatal("head/default/history did not commit together", e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE organizations SET time_zone='UTC',revision=revision+1 WHERE org_id=$1`, org), "P8104")
	if _, e := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE ON org_settings_versions TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx, `UPDATE org_settings_versions SET reason='rewritten' WHERE org_id=$1`, org), "55000")
	teamSQLCode(t, unitSchemaExec(t, fx, `DELETE FROM org_settings_versions WHERE org_id=$1`, org), "55000")
}
