//go:build integration

package adapters_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func settingsStore(t *testing.T, fx legalFixtureState) *adapters.SettingsStore {
	t.Helper()
	catalog, e := audit.NewCatalog(adapters.SettingsAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return &adapters.SettingsStore{Pool: fx.f.Runtime, Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
func settingsCommand(t *testing.T, rev int64, values d.SettingsValues, op string) app.SettingsWrite {
	t.Helper()
	if op == "" {
		op = onboardingID(t)
	}
	c, e := app.NewSettingsWrite(app.SettingsWriteInput{OperationID: op, ExpectedRevision: rev, Values: values, Reason: "Synthetic configuration update"})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func settingsRead(t *testing.T, fx legalFixtureState, s *adapters.SettingsStore, rev int64) d.SettingsSnapshot {
	t.Helper()
	result, e := s.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, rev)
	if e != nil {
		t.Fatal(e)
	}
	v, e := result.Settings.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestSettingsStorePreservesHistoryReplayExactValuesAndIntake(t *testing.T) {
	fx := settingsSchemaFixture(t)
	s := settingsStore(t, fx)
	before := settingsRead(t, fx, s, 0)
	v := before.Values
	v.TimeZone = "Pacific/Auckland"
	v.ConsumptionDisplay = d.ConsumptionLPer100KM
	v.ReviewRequired.Trips = false
	v.Policies.IntakeWindowDays = 0
	v.Policies.ClockSkewLimitSeconds = 9223372036854775807
	c := settingsCommand(t, 1, v, "")
	first, replay, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, c)
	if e != nil || replay || first.Revision != 2 || first.RootRevision != 2 || first.EffectiveAt.Before(before.EffectiveAt) {
		t.Fatal("change", e)
	}
	got := settingsRead(t, fx, s, 0)
	if got.Values != v || got.Author.MembershipID != fx.member {
		t.Fatal("typed values/author changed")
	}
	if settingsRead(t, fx, s, 1) != before {
		t.Fatal("history rewritten")
	}
	v.TimeZone = "UTC"
	if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 2, v, "")); e != nil {
		t.Fatal(e)
	}
	repeated, replay, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, c)
	if e != nil || !replay || repeated != first {
		t.Fatal("replay changed original root/time acknowledgement", e)
	}
	input, _ := c.Snapshot()
	input.Values.ClosurePeriodDays = 31
	changed, _ := app.NewSettingsWrite(input)
	if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, changed); !errors.Is(e, app.ErrOperationReused) {
		t.Fatal("changed intent reused", e)
	}
	if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 1, v, "")); !errors.Is(e, d.ErrConflict) {
		t.Fatal("stale write", e)
	}
	page, e := s.ListSettings(t.Context(), fx.scope, fx.actor, fx.member, app.SettingsFilter{Limit: 2})
	if e != nil || len(page.Versions) != 2 || page.NextRevision != 2 {
		t.Fatal("history first page", e)
	}
	page, e = s.ListSettings(t.Context(), fx.scope, fx.actor, fx.member, app.SettingsFilter{Limit: 2, BeforeRevision: page.NextRevision})
	if e != nil || len(page.Versions) != 1 || page.NextRevision != 0 {
		t.Fatal("history next page", e)
	}
	if e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		intake, e := s.CaptureSettingsTx(t.Context(), tx, fx.scope)
		if e != nil {
			return e
		}
		snap, e := intake.Settings.Snapshot(fx.scope)
		if e != nil {
			return e
		}
		if snap.Revision != 3 || intake.ReceivedAt.Before(snap.EffectiveAt) {
			t.Fatal("intake used device/historical revision")
		}
		historical, e := s.SettingsRevisionTx(t.Context(), tx, fx.scope, 1)
		if e != nil {
			return e
		}
		old, _ := historical.Snapshot(fx.scope)
		if old != before {
			t.Fatal("historical lookup changed")
		}
		return nil
	}); e != nil {
		t.Fatal("trusted intake", e)
	}
	other, _, e := fx.onboarding.Create(t.Context(), fx.actor, onboardingCommand(t, onboardingID(t)))
	if e != nil {
		t.Fatal(e)
	}
	if _, e := s.ReadSettings(t.Context(), scopeFor(t, other.OrgID.String()), fx.actor, fx.member, 1); e == nil {
		t.Fatal("foreign membership accepted")
	}
	var payload string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT coalesce(string_agg(to_jsonb(e)::text,''),'') FROM org_audit_events e WHERE action='org.settings.changed'`).Scan(&payload); e != nil {
		t.Fatal(e)
	}
	if len(payload) == 0 {
		t.Fatal("no audit")
	}
}
func TestSettingsConcurrentCASReplayAndServerReceiptSerialization(t *testing.T) {
	fx := settingsSchemaFixture(t)
	s := settingsStore(t, fx)
	v := settingsRead(t, fx, s, 0).Values
	const n = 12
	commands := make([]app.SettingsWrite, n)
	for i := range commands {
		value := v
		value.ClosurePeriodDays = 31 + i
		commands[i] = settingsCommand(t, 1, value, "")
	}
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for _, c := range commands {
		wg.Go(func() { _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, c); errs <- e })
	}
	wg.Wait()
	close(errs)
	successes := 0
	for e := range errs {
		if e == nil {
			successes++
		} else if !errors.Is(e, d.ErrConflict) {
			t.Fatal("CAS failure", e)
		}
	}
	if successes != 1 {
		t.Fatal("lost-update admission", successes)
	}
	c := settingsCommand(t, 2, v, "")
	results := make(chan app.SettingsResult, n)
	errs = make(chan error, n)
	for range n {
		wg.Go(func() {
			r, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, c)
			results <- r
			errs <- e
		})
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent retry", e)
		}
	}
	var original app.SettingsResult
	for r := range results {
		if original.Revision == 0 {
			original = r
		}
		if r != original {
			t.Fatal("replayed acknowledgement differs")
		}
	}
	// An owning intake holds the same root lock until its business mutation commits.
	locked, release, complete := make(chan app.SettingsIntake, 1), make(chan struct{}), make(chan error, 1)
	go func() {
		complete <- database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			v, e := s.CaptureSettingsTx(t.Context(), tx, fx.scope)
			if e != nil {
				return e
			}
			locked <- v
			<-release
			return nil
		})
	}()
	var captured app.SettingsIntake
	select {
	case captured = <-locked:
	case e := <-complete:
		t.Fatal(e)
	case <-time.After(10 * time.Second):
		t.Fatal("intake did not capture")
	}
	c = settingsCommand(t, 3, v, "")
	writing := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(writing)
		_, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, c)
		done <- e
	}()
	<-writing
	select {
	case e := <-done:
		t.Fatal("settings bypassed intake lock", e)
	case <-time.After(80 * time.Millisecond):
	}
	close(release)
	if e := <-complete; e != nil {
		t.Fatal(e)
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	old, _ := captured.Settings.Snapshot(fx.scope)
	current := settingsRead(t, fx, s, 0)
	if old.Revision != 3 || current.Revision != 4 || captured.ReceivedAt.After(current.EffectiveAt) {
		t.Fatal("effective server receipt boundary")
	}
}
func TestSettingsLateFailureRollsBackAuditReceiptHeadAndRoot(t *testing.T) {
	fx := settingsSchemaFixture(t)
	s := settingsStore(t, fx)
	v := settingsRead(t, fx, s, 0).Values
	v.TimeZone = "Pacific/Auckland"
	c := settingsCommand(t, 1, v, "")
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_settings_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic late settings outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_settings_failure AFTER INSERT ON org_settings_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_settings_failure()`); e != nil {
		t.Fatal(e)
	}
	r, replay, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, c)
	if !errors.Is(e, app.ErrUnavailable) || r != (app.SettingsResult{}) || replay {
		t.Fatal("late commit exposed provisional acknowledgement", e)
	}
	var intact bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT h.revision=1 AND o.revision=1 AND o.time_zone='America/Cuiaba' AND(SELECT count(*) FROM org_settings_versions WHERE org_id=o.org_id)=1 AND NOT EXISTS(SELECT 1 FROM org_settings_commands WHERE org_id=o.org_id) AND NOT EXISTS(SELECT 1 FROM org_audit_events WHERE org_id=o.org_id AND action='org.settings.changed') FROM org_settings_heads h JOIN organizations o USING(org_id) WHERE o.org_id=$1`, fx.scope.OrgID().String()).Scan(&intact); e != nil || !intact {
		t.Fatal("partial settings persisted", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_settings_failure ON org_settings_commands`); e != nil {
		t.Fatal(e)
	}
	if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, c); e != nil {
		t.Fatal("recovery", e)
	}
}

func TestSettingsDesignationReductionPreservesOtherSlotsNoGrantAndRollback(t *testing.T) {
	fx := settingsSchemaFixture(t)
	s := settingsStore(t, fx)
	account := onboardingID(t)
	if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,'synthetic-settings-designate','active')`, account); e != nil {
		t.Fatal(e)
	}
	_, member := teamDriver(t, fx, account)
	v := settingsRead(t, fx, s, 0).Values
	v.Responsibilities.ReviewFallback = d.ResponsibilityPair{PrimaryID: member, SubstituteID: fx.member}
	if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 1, v, "")); e != nil {
		t.Fatal(e)
	}
	var roleCount int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND membership_id=$2 AND role<>'DRIVER'`, fx.scope.OrgID().String(), member).Scan(&roleCount); e != nil || roleCount != 0 {
		t.Fatal("designation granted authority", e)
	}
	end := `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`
	if e := unitSchemaExec(t, fx, end, fx.scope.OrgID().String(), member); e != nil {
		t.Fatal(e)
	}
	cleared := settingsRead(t, fx, s, 0)
	if cleared.Revision != 3 || cleared.Source != d.SettingsMembershipReduction || cleared.SourceMembershipID != member || cleared.SourceMembershipRevision != 3 || cleared.Values.Responsibilities.ReviewFallback.PrimaryID != "" || cleared.Values.Responsibilities.ReviewFallback.SubstituteID != fx.member || cleared.Values.Responsibilities.Billing.PrimaryID != fx.member {
		t.Fatal("cleanup erased unaffected pair/source")
	}
	var pending int
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_settings_notices WHERE org_id=$1 AND delivery_state='PENDING_NOTIFICATION_CORE'`, fx.scope.OrgID().String()).Scan(&pending); e != nil || pending != 1 {
		t.Fatal("durable notice missing", e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE org_memberships SET state='ACTIVE',suspension_origin='',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), member); e != nil {
		t.Fatal(e)
	}
	if settingsRead(t, fx, s, 0) != cleared {
		t.Fatal("reactivation restored designation")
	}
	if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 3, v, "")); e != nil {
		t.Fatal("explicit redesignation", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_notice_outage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='P8104',MESSAGE='synthetic notice outage';END $$;CREATE CONSTRAINT TRIGGER synthetic_notice_outage AFTER INSERT ON org_settings_notices DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_notice_outage()`); e != nil {
		t.Fatal(e)
	}
	// Safety reduction must also run when normal configuration writes are blocked.
	org := fx.scope.OrgID().String()
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, org); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='SUSPENDED',suspension_reason='SECURITY',revision=revision+1 WHERE org_id=$1`, org); e != nil {
		t.Fatal(e)
	}
	teamSQLCode(t, unitSchemaExec(t, fx, end, org, member), "P8104")
	var intact bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT m.state='ACTIVE' AND m.revision=4 AND h.revision=4 AND EXISTS(SELECT 1 FROM org_role_assignments WHERE org_id=m.org_id AND membership_id=m.id AND ended_at IS NULL) FROM org_memberships m JOIN org_settings_heads h USING(org_id) WHERE m.org_id=$1 AND m.id=$2`, org, member).Scan(&intact); e != nil || !intact {
		t.Fatal("cleanup failure partially revoked", e)
	}
	if _, e := fx.f.Admin.Exec(t.Context(), `DROP TRIGGER synthetic_notice_outage ON org_settings_notices`); e != nil {
		t.Fatal(e)
	}
	if e := unitSchemaExec(t, fx, end, org, member); e != nil {
		t.Fatal("cleanup recovery", e)
	}
	if settingsRead(t, fx, s, 0).Revision != 5 {
		t.Fatal("recovery settings revision")
	}
}

func TestSettingsDesignationVersusMembershipEndRaceCannotLeaveUnavailableContact(t *testing.T) {
	fx := settingsSchemaFixture(t)
	s := settingsStore(t, fx)
	org := fx.scope.OrgID().String()
	for range 12 {
		account := onboardingID(t)
		if _, e := fx.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1::uuid,'synthetic-settings-race-'||$1::text,'active')`, account); e != nil {
			t.Fatal(e)
		}
		_, member := teamDriver(t, fx, account)
		before := settingsRead(t, fx, s, 0)
		values := before.Values
		values.Responsibilities.ReviewFallback = d.ResponsibilityPair{PrimaryID: member}
		command := settingsCommand(t, before.Revision, values, "")
		start := make(chan struct{})
		writer, end := make(chan error, 1), make(chan error, 1)
		go func() {
			<-start
			_, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, command)
			writer <- e
		}()
		go func() {
			<-start
			end <- unitSchemaExec(t, fx, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, member)
		}()
		close(start)
		if e := <-end; e != nil {
			t.Fatal("concurrent offboarding", e)
		}
		e := <-writer
		if e != nil && !errors.Is(e, d.ErrInvalid) {
			t.Fatal("unexpected raced configuration refusal", e)
		}
		after := settingsRead(t, fx, s, 0)
		if after.Values.Responsibilities.ReviewFallback.PrimaryID != "" {
			t.Fatal("unavailable designation committed")
		}
		if e == nil && after.Revision != before.Revision+2 || e != nil && after.Revision != before.Revision {
			t.Fatal("source reduction revision lost")
		}
	}
}

func TestSettingsOwningIntakePinsServerRevisionWithoutRewritingBaseMeasures(t *testing.T) {
	fx := settingsSchemaFixture(t)
	s := settingsStore(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, e := fx.f.Admin.Exec(t.Context(), `CREATE TABLE synthetic_settings_intake(org_id uuid NOT NULL,id uuid NOT NULL,settings_revision bigint NOT NULL,received_at timestamptz NOT NULL,declared_at timestamptz NOT NULL,root_zone text NOT NULL,base_distance_m bigint NOT NULL,base_volume_ml bigint NOT NULL,PRIMARY KEY(org_id,id),FOREIGN KEY(org_id,settings_revision) REFERENCES org_settings_versions(org_id,revision));ALTER TABLE synthetic_settings_intake ENABLE ROW LEVEL SECURITY;ALTER TABLE synthetic_settings_intake FORCE ROW LEVEL SECURITY;CREATE POLICY synthetic_settings_intake_scope ON synthetic_settings_intake USING(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid) WITH CHECK(org_id=NULLIF(current_setting('anpfuel.org_id',true),'')::uuid);GRANT SELECT,INSERT ON synthetic_settings_intake TO `+role); e != nil {
		t.Fatal(e)
	}
	declared := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	record := func() string {
		t.Helper()
		id := onboardingID(t)
		e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			intake, e := s.CaptureSettingsTx(t.Context(), tx, fx.scope)
			if e != nil {
				return e
			}
			v, e := intake.Settings.Snapshot(fx.scope)
			if e != nil {
				return e
			}
			_, e = tx.Exec(t.Context(), `INSERT INTO synthetic_settings_intake(org_id,id,settings_revision,received_at,declared_at,root_zone,base_distance_m,base_volume_ml) VALUES($1,$2,$3,$4,$5,$6,8000,1000)`, fx.scope.OrgID().String(), id, v.Revision, intake.ReceivedAt, declared, v.Values.TimeZone)
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		return id
	}
	first := record()
	old := settingsRead(t, fx, s, 0)
	values := old.Values
	values.ConsumptionDisplay = d.ConsumptionLPer100KM
	values.TimeZone = "Pacific/Auckland"
	if _, _, e := s.ChangeSettings(t.Context(), fx.scope, fx.actor, fx.member, settingsCommand(t, 1, values, "")); e != nil {
		t.Fatal(e)
	}
	second := record()
	var valid bool
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT a.settings_revision=1 AND a.root_zone='America/Cuiaba' AND b.settings_revision=2 AND b.root_zone='Pacific/Auckland' AND a.base_distance_m=b.base_distance_m AND a.base_distance_m=8000 AND a.base_volume_ml=b.base_volume_ml AND a.base_volume_ml=1000 AND a.declared_at=b.declared_at AND b.declared_at<v.effective_at AND b.received_at>=v.effective_at FROM synthetic_settings_intake a JOIN synthetic_settings_intake b ON b.org_id=a.org_id AND b.id=$3 JOIN org_settings_versions v ON(v.org_id,v.revision)=(b.org_id,b.settings_revision) WHERE a.org_id=$1 AND a.id=$2`, fx.scope.OrgID().String(), first, second).Scan(&valid); e != nil || !valid {
		t.Fatal("device time selected policy or display changed stored measures", e)
	}
	if settingsRead(t, fx, s, 1) != old {
		t.Fatal("owning intake modified retained settings")
	}
	if _, e := s.CaptureSettingsTx(t.Context(), nil, fx.scope); e == nil {
		t.Fatal("missing transaction accepted")
	}
	tx, e := fx.f.Admin.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(t.Context())
	if _, e := s.CaptureSettingsTx(t.Context(), tx, fx.scope); e == nil {
		t.Fatal("privileged transaction accepted")
	}
}
