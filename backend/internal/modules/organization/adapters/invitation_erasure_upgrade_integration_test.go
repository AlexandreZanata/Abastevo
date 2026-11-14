//go:build integration

package adapters_test

import (
	"context"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// invitationFixtureOn builds the ordinary invitation fixture on a supplied
// (possibly previous-schema) database, exactly as invitationFixture does.
func invitationFixtureOn(t *testing.T, f *orgtest.Fixture) invitationFixtureState {
	t.Helper()
	_, _, onboarding, actor, flags := onboardingOnFixture(t, f)
	fx := legalFixtureFor(t, f, onboarding, actor, flags)
	grantEmployeeTables(t, fx)
	if _, _, e := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); e != nil {
		t.Fatal("activate invitation fixture", e)
	}
	catalog, e := audit.NewCatalog(memberpg.InvitationAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	settings := &orgpg.SettingsStore{Pool: fx.f.Runtime}
	store := &memberpg.InvitationStore{Pool: fx.f.Runtime, Authority: &orgpg.InvitationAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Settings: settings}, Memberships: fx.store.Memberships, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
	return invitationFixtureState{fx, store}
}

// B-BR-EMP-170/172, BUC-EMP-89: populated schema199 with a person whose account
// was deleted before the correction; failed 000200 changes nothing, the ordinary
// recovery unlinks only that stale acceptance, invents no notice or revision,
// keeps the previous image compatible and the upgraded erasure keeps working.
func TestInvitationErasurePopulatedUpgradeFailureRecoveryUnlinksDeletedAccounts(t *testing.T) {
	const name = "000200_invitation_erasure_corrections.sql"
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(migrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() != "schema_min_supported" && (entry.Name() >= name || !strings.HasSuffix(entry.Name(), ".sql")) {
			continue
		}
		raw, err := fs.ReadFile(migrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	fx := invitationFixtureOn(t, f)
	s := redemptionStore(t, fx)
	deleted := redemptionActor(t, fx)
	later := redemptionActor(t, fx)
	live := redemptionActor(t, fx)
	stale := invitationAccept(t, fx, s, deleted, "AUDITOR")
	active := invitationAccept(t, fx, s, later, "DRIVER")
	kept := invitationAccept(t, fx, s, live, "AUDITOR")
	if _, e := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA public TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); e != nil {
		t.Fatal(e)
	}
	// Schema199 erasure tombstones the membership but leaves the acceptance link.
	if e := invitationDeleteAccount(t, f, deleted.AccountID); e != nil {
		t.Fatal("schema199 account deletion", e)
	}
	invitationCount(t, fx, `SELECT count(*) FROM org_invitation_acceptances WHERE invitation_id=$1 AND account_id=$2 AND account_alias<>''`, 1, stale.ID, deleted.AccountID)
	history := func() string {
		t.Helper()
		var out string
		if e := f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(x)-'account_id'-'account_alias' ORDER BY org_id,invitation_id) FROM org_invitation_acceptances x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,invitation_id,revision) FROM org_invitation_acceptance_notices x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_invitation_person_commands x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_invitations x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,invitation_id,revision) FROM org_invitation_notices x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_memberships x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,id) FROM org_role_assignments x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,operation_id) FROM org_audit_events x),
 (SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM accounts x))::text`).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	links := func() map[string]string {
		t.Helper()
		rows, e := f.Admin.Query(t.Context(), `SELECT invitation_id::text,COALESCE(account_id::text,'')||'/'||account_alias FROM org_invitation_acceptances`)
		if e != nil {
			t.Fatal(e)
		}
		out := map[string]string{}
		for rows.Next() {
			var id, link string
			if e := rows.Scan(&id, &link); e != nil {
				t.Fatal(e)
			}
			out[id] = link
		}
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		return out
	}
	migrator := f.RuntimeRole + "_erasure_migrator"
	role := pgx.Identifier{migrator}.Sanitize()
	// An ordinary migrator owns exactly what 000200 alters or replaces plus the
	// erasure definer contract (org_memberships and its account-exit helper).
	if _, e := f.Admin.Exec(t.Context(), "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;GRANT USAGE,CREATE ON SCHEMA public TO "+role+";GRANT SELECT,INSERT ON schema_migrations TO "+role+
		";ALTER TABLE org_invitation_acceptances OWNER TO "+role+";ALTER FUNCTION org_invitation_acceptance_guard() OWNER TO "+role+";ALTER FUNCTION org_invitation_acceptance_project() OWNER TO "+role+";ALTER FUNCTION org_invitation_finish_reservation() OWNER TO "+role); e != nil {
		t.Fatal(e)
	}
	invitationEraserOwns(t, f, migrator)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := f.Admin.Exec(ctx, "REASSIGN OWNED BY "+role+" TO CURRENT_USER;DROP OWNED BY "+role+";DROP ROLE "+role); e != nil {
			t.Error(e)
		}
	})
	functions := func() string {
		t.Helper()
		var out string
		if e := f.Admin.QueryRow(t.Context(), `SELECT string_agg(pg_get_functiondef(oid),E'\n' ORDER BY proname) FROM pg_proc WHERE oid=ANY(ARRAY['org_invitation_acceptance_guard()'::regprocedure,'org_invitation_acceptance_project()'::regprocedure,'org_membership_account_erasure()'::regprocedure,'org_invitation_finish_reservation()'::regprocedure])`).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	before, beforeLinks, beforeFunctions := history(), links(), functions()
	location, err := url.Parse(f.AdminDSN)
	if err != nil {
		t.Fatal(err)
	}
	parameters := location.Query()
	parameters.Set("options", "-c role="+migrator)
	location.RawQuery = strings.ReplaceAll(parameters.Encode(), "+", "%20")
	raw, err := fs.ReadFile(migrations.Files, name)
	if err != nil {
		t.Fatal(err)
	}
	broken := fstest.MapFS{name: &fstest.MapFile{Data: append(append([]byte{}, raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(t.Context(), location.String(), broken); err == nil || !strings.Contains(err.Error(), "SQLSTATE 22012") {
		t.Fatal("injected erasure migration failure not reached", err)
	}
	var clean bool
	if err := f.Admin.QueryRow(t.Context(), `SELECT(SELECT attnotnull FROM pg_attribute WHERE attrelid='org_invitation_acceptances'::regclass AND attname='account_id')
 AND EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='org_invitation_acceptances'::regclass AND conname='org_invitation_acceptances_account_alias_check')
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='org_invitation_acceptances'::regclass AND conname='org_invitation_acceptances_account_link_check')
 AND(SELECT relforcerowsecurity FROM pg_class WHERE oid='org_invitation_acceptances'::regclass)
 AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000200')`).Scan(&clean); err != nil || !clean {
		t.Fatal("failed erasure migration changed schema199 structure", err)
	}
	if history() != before || functions() != beforeFunctions || len(links()) != len(beforeLinks) {
		t.Fatal("failed erasure migration changed history, links or guards")
	}
	for id, link := range links() {
		if beforeLinks[id] != link {
			t.Fatal("failed erasure migration changed an acceptance link")
		}
	}
	current := fstest.MapFS{}
	for key, data := range previous {
		current[key] = data
	}
	current[name] = &fstest.MapFile{Data: raw}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 1 || applied[0] != "000200" {
		t.Fatal("ordinary erasure migration recovery", err)
	}
	if history() != before {
		t.Fatal("erasure migration rewrote admission state, notices, receipts, memberships or audit")
	}
	after := links()
	for id, link := range beforeLinks {
		want := link
		if id == stale.ID {
			want = "/"
		}
		if after[id] != want {
			t.Fatal("erasure migration unlinked the wrong acceptance or kept a deleted alias", id == stale.ID)
		}
	}
	if applied, err := migrate.Apply(t.Context(), location.String(), current); err != nil || len(applied) != 0 {
		t.Fatal("erasure migration no-op replay", err)
	}
	if err := migrate.Check(t.Context(), f.Admin, previous); err != nil {
		t.Fatal("previous embedded image incompatible", err)
	}
	if err := f.Admin.QueryRow(t.Context(), `SELECT NOT rolsuper AND NOT rolbypassrls AND(SELECT relrowsecurity AND relforcerowsecurity AND relowner=r.oid FROM pg_class WHERE oid='org_invitation_acceptances'::regclass)
 AND(SELECT convalidated FROM pg_constraint WHERE conrelid='org_invitation_acceptances'::regclass AND conname='org_invitation_acceptances_account_link_check')
 AND NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='org_invitation_acceptances'::regclass AND conname='org_invitation_acceptances_account_alias_check')
 FROM pg_roles r WHERE r.rolname=$1`, migrator).Scan(&clean); err != nil || !clean {
		t.Fatal("upgraded acceptance storage is not the ordinary forced-RLS migrator's", err)
	}
	// Live links still refuse a half tombstone at the constraint level too.
	if _, e := f.Admin.Exec(t.Context(), `ALTER TABLE org_invitation_acceptances DISABLE TRIGGER org_invitation_acceptance_guard`); e != nil {
		t.Fatal(e)
	}
	for _, sql := range []string{`UPDATE org_invitation_acceptances SET account_alias='' WHERE invitation_id=$1`, `UPDATE org_invitation_acceptances SET account_id=NULL WHERE invitation_id=$1`} {
		if _, e := f.Admin.Exec(t.Context(), sql, kept.ID); e == nil {
			t.Fatal("half account unlink stored")
		} else {
			invitationSQLCode(t, e, "23514")
		}
	}
	if _, e := f.Admin.Exec(t.Context(), `ALTER TABLE org_invitation_acceptances ENABLE TRIGGER org_invitation_acceptance_guard`); e != nil {
		t.Fatal(e)
	}
	// The upgraded erasure path (ordinary migrator as definer) unlinks a later
	// deletion; manager reads project the tombstone for both people.
	if e := invitationDeleteAccount(t, f, later.AccountID); e != nil {
		t.Fatal("upgraded account deletion", e)
	}
	manager := invitationActor(fx)
	for _, id := range []string{stale.ID, active.ID} {
		read, _, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, id, "READ", 0, "")
		if e != nil || read.Alias != "" || !read.AccountErased {
			t.Fatal("upgraded acceptance read discloses an erased alias", e)
		}
	}
	read, _, e := s.ManageAdmission(t.Context(), fx.scope, manager, fx.member, kept.ID, "READ", 0, "")
	if e != nil || read.Alias == "" || read.AccountErased || read.State != d.AdmissionWaiting {
		t.Fatal("live acceptance changed by upgrade", e)
	}
}
