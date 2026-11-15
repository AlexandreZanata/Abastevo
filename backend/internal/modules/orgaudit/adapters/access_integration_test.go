//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

const accessMigration = "000202_org_sensitive_access_events.sql"

func accessID(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8300-%012d", n) }

func newAccessFixture(t *testing.T) (*orgtest.Fixture, *adapters.Store, adapters.AccessLog) {
	t.Helper()
	f := orgtest.New(t)
	grantAccessLog(t, f)
	catalog, err := domain.NewCatalog(domain.AccessDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	store := adapters.NewStore(f.Runtime, catalog)
	return f, store, adapters.AccessLog{Runner: store}
}

func grantAccessLog(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+role+
		"; GRANT SELECT, INSERT ON org_audit_events, org_sensitive_access_events TO "+role); err != nil {
		t.Fatal(err)
	}
}

// sensitive builds one record: the grant is the request for an issuance or a
// live read, and the issuance's grant for a served capability read.
func sensitive(category domain.AccessCategory, stage domain.AccessStage, request, grant int) domain.SensitiveAccess {
	item := domain.SensitiveAccess{Category: category, Stage: stage,
		AccountID: accessID(901), MembershipID: accessID(902), TargetID: accessID(903),
		AccessID: accessID(grant), RequestID: accessID(request), Binding: strings.Repeat("e", 64)}
	return item
}

func accessCounts(t *testing.T, f *orgtest.Fixture, org string) (events, details int) {
	t.Helper()
	if err := f.Admin.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM org_audit_events WHERE org_id=$1),
		(SELECT count(*) FROM org_sensitive_access_events WHERE org_id=$1)`, org).Scan(&events, &details); err != nil {
		t.Fatal(err)
	}
	return events, details
}

func pgCode(err error) string {
	var failure *pgconn.PgError
	if errors.As(err, &failure) {
		return failure.Code
	}
	return ""
}

// B-BR-EMP-207/209, BUC-EMP-107: every category records exactly one scoped
// row of references, with the actor, membership and grant.
func TestSensitiveAccessRecordsScopedReferencesForEveryCategory(t *testing.T) {
	f, _, log := newAccessFixture(t)
	a, b := f.Organizations[0], f.Organizations[1]
	scope := a.Scope(t)
	records := []domain.SensitiveAccess{
		sensitive(domain.DocumentContent, domain.AccessIssued, 1, 1),
		sensitive(domain.DocumentContent, domain.AccessServed, 2, 1),
		sensitive(domain.ExportFile, domain.AccessIssued, 3, 3),
		sensitive(domain.ExportFile, domain.AccessServed, 4, 3),
		sensitive(domain.LocationTrack, domain.AccessIssued, 5, 5),
		sensitive(domain.LocationTrack, domain.AccessServed, 6, 5),
		sensitive(domain.LocationLive, domain.AccessServed, 7, 7),
	}
	for _, item := range records {
		if err := log.RecordAccess(t.Context(), scope, item); err != nil {
			t.Fatalf("%s/%s: %v", item.Category, item.Stage, err)
		}
	}
	if events, details := accessCounts(t, f, a.ID); events != len(records) || details != len(records) {
		t.Fatalf("recorded %d events and %d details", events, details)
	}
	if events, details := accessCounts(t, f, b.ID); events != 0 || details != 0 {
		t.Fatal("another organization received records")
	}
	for _, item := range records {
		want, err := item.Event(scope)
		if err != nil {
			t.Fatal(err)
		}
		var got domain.Event
		var category, stage, member, grant string
		if err := f.Admin.QueryRow(t.Context(), `SELECT e.operation_id::text, e.actor_id::text, COALESCE(e.delegation_id::text,''),
			e.action, e.target_type, e.target_id::text, e.reason_code, e.request_hash,
			d.category, d.stage, d.membership_id::text, d.access_id::text
			FROM org_audit_events e JOIN org_sensitive_access_events d USING (org_id, operation_id)
			WHERE e.org_id=$1 AND e.operation_id=$2`, a.ID, item.RequestID).Scan(&got.OperationID, &got.ActorID, &got.DelegationID,
			&got.Action, &got.TargetType, &got.TargetID, &got.ReasonCode, &got.RequestHash, &category, &stage, &member, &grant); err != nil {
			t.Fatal(err)
		}
		if got != want || category != string(item.Category) || stage != string(item.Stage) || member != item.MembershipID || grant != item.AccessID {
			t.Fatalf("stored %+v %s/%s/%s/%s, want %+v", got, category, stage, member, grant, want)
		}
	}
	// The detail schema has only identifiers and closed codes: no column can
	// carry content, names, keys, URLs, tokens or coordinates.
	rows, err := f.Admin.Query(t.Context(), `SELECT column_name || ':' || data_type FROM information_schema.columns
		WHERE table_schema='public' AND table_name='org_sensitive_access_events' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	columns, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(columns, ",") != "org_id:uuid,operation_id:uuid,category:text,stage:text,membership_id:uuid,access_id:uuid" {
		t.Fatalf("detail columns changed: %v", columns)
	}
}

// B-BR-EMP-210: one record per request; a replay or conflicting reuse is
// refused and concurrent duplicates store one row.
func TestSensitiveAccessReplayConflictAndConcurrentDuplicates(t *testing.T) {
	f, _, _ := newAccessFixture(t)
	a := f.Organizations[0]
	scope := a.Scope(t)
	log := adapters.AccessLog{Runner: adapters.NewStore(f.RuntimePool(t, 4), mustAccessCatalog(t))}
	grant := sensitive(domain.DocumentContent, domain.AccessIssued, 10, 10)
	if err := log.RecordAccess(t.Context(), scope, grant); err != nil {
		t.Fatal(err)
	}
	if err := log.RecordAccess(t.Context(), scope, grant); !errors.Is(err, domain.ErrReplayed) {
		t.Fatal("replayed request was not refused", err)
	}
	changed := grant
	changed.Binding = strings.Repeat("f", 64)
	if err := log.RecordAccess(t.Context(), scope, changed); !errors.Is(err, domain.ErrReused) {
		t.Fatal("conflicting request reuse was not refused", err)
	}
	// A second issuance cannot claim an existing grant under a new request.
	again := sensitive(domain.DocumentContent, domain.AccessIssued, 11, 10)
	if err := log.RecordAccess(t.Context(), scope, again); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("duplicate grant issuance accepted", err)
	}
	if events, details := accessCounts(t, f, a.ID); events != 1 || details != 1 {
		t.Fatalf("replay changed the log: %d/%d", events, details)
	}
	read := sensitive(domain.DocumentContent, domain.AccessServed, 12, 10)
	var recorded, replayed atomic.Int32
	start := make(chan struct{})
	results := make(chan error, 12)
	for range 12 {
		go func() {
			<-start
			err := log.RecordAccess(t.Context(), scope, read)
			switch {
			case err == nil:
				recorded.Add(1)
			case errors.Is(err, domain.ErrReplayed):
				replayed.Add(1)
				err = nil
			}
			results <- err
		}()
	}
	close(start)
	for range 12 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if recorded.Load() != 1 || replayed.Load() != 11 {
		t.Fatalf("concurrent duplicates recorded %d, replayed %d", recorded.Load(), replayed.Load())
	}
	if events, details := accessCounts(t, f, a.ID); events != 2 || details != 2 {
		t.Fatalf("concurrent duplicates stored %d/%d", events, details)
	}
}

func mustAccessCatalog(t *testing.T) domain.Catalog {
	t.Helper()
	catalog, err := domain.NewCatalog(domain.AccessDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

// A refused detail write rolls back the generic event too: an access is
// recorded completely or not at all, and the caller serves nothing.
func TestFailedDetailWriteLeavesNoAuditEvent(t *testing.T) {
	f, _, log := newAccessFixture(t)
	a := f.Organizations[0]
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "REVOKE INSERT ON org_sensitive_access_events FROM "+role); err != nil {
		t.Fatal(err)
	}
	grant := sensitive(domain.DocumentContent, domain.AccessIssued, 20, 20)
	if err := log.RecordAccess(t.Context(), a.Scope(t), grant); !errors.Is(err, domain.ErrUnavailable) || strings.Contains(err.Error(), "permission") {
		t.Fatal("failed detail write did not fail closed", err)
	}
	if events, details := accessCounts(t, f, a.ID); events != 0 || details != 0 {
		t.Fatalf("failed detail write left %d events and %d details", events, details)
	}
	if _, err := f.Admin.Exec(t.Context(), "GRANT INSERT ON org_sensitive_access_events TO "+role); err != nil {
		t.Fatal(err)
	}
	if err := log.RecordAccess(t.Context(), a.Scope(t), grant); err != nil {
		t.Fatal("recovered log refused the retry", err)
	}
	if events, details := accessCounts(t, f, a.ID); events != 1 || details != 1 {
		t.Fatal("retry after recovery not recorded once")
	}
}

// BUC-EMP-107: two-organization RLS, append-only rows and database guards that
// keep a record consistent with its event, its grant and its scope.
func TestSensitiveAccessIsolationAppendOnlyAndDatabaseGuards(t *testing.T) {
	f, _, log := newAccessFixture(t)
	a, b := f.Organizations[0], f.Organizations[1]
	ctx := t.Context()
	// The same request and grant identifiers are independent per organization.
	for _, org := range []orgtest.Organization{a, b} {
		for _, item := range []domain.SensitiveAccess{
			sensitive(domain.DocumentContent, domain.AccessIssued, 30, 30),
			sensitive(domain.DocumentContent, domain.AccessServed, 31, 30),
		} {
			if err := log.RecordAccess(ctx, org.Scope(t), item); err != nil {
				t.Fatal(err)
			}
		}
	}
	var visible int
	if err := database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM org_sensitive_access_events`).Scan(&visible)
	}); err != nil || visible != 2 {
		t.Fatalf("scoped read saw %d rows: %v", visible, err)
	}
	var unscoped int
	if err := f.Runtime.QueryRow(ctx, `SELECT count(*) FROM org_sensitive_access_events`).Scan(&unscoped); err != nil || unscoped != 0 {
		t.Fatalf("unscoped runtime read saw %d rows: %v", unscoped, err)
	}
	type statement struct {
		query string
		args  []any
	}
	inScope := func(statements ...statement) error {
		return database.WithOrgTx(ctx, f.Runtime, a.Scope(t), func(tx pgx.Tx) error {
			for _, item := range statements {
				if _, err := tx.Exec(ctx, item.query, item.args...); err != nil {
					return err
				}
			}
			return nil
		})
	}
	event := func(op int, action, targetType string, target int, reason string) statement {
		return statement{`INSERT INTO org_audit_events (org_id,operation_id,actor_id,action,target_type,target_id,reason_code,request_hash)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, []any{a.ID, accessID(op), accessID(901), action, targetType, accessID(target), reason, strings.Repeat("a", 64)}}
	}
	detail := func(org string, op int, category, stage string, member, grant int) statement {
		return statement{`INSERT INTO org_sensitive_access_events (org_id,operation_id,category,stage,membership_id,access_id)
			VALUES ($1,$2,$3,$4,$5,$6)`, []any{org, accessID(op), category, stage, accessID(member), accessID(grant)}}
	}
	const member = domain.AccessReason
	for name, probe := range map[string]struct {
		statements []statement
		code       string
	}{
		"foreign organization":      {[]statement{detail(b.ID, 31, "DOCUMENT_CONTENT", "SERVED", 902, 30)}, "42501"},
		"detail without event":      {[]statement{detail(a.ID, 40, "DOCUMENT_CONTENT", "ISSUED", 902, 40)}, "23503"},
		"event without detail":      {[]statement{event(41, "document.access.issued", "document", 903, member)}, "23514"},
		"category/action":           {[]statement{event(42, "document.access.issued", "document", 903, member), detail(a.ID, 42, "EXPORT_FILE", "ISSUED", 902, 42)}, "23514"},
		"stage/action":              {[]statement{event(43, "document.access.issued", "document", 903, member), detail(a.ID, 43, "DOCUMENT_CONTENT", "SERVED", 902, 30)}, "23514"},
		"operator reason":           {[]statement{event(44, "document.access.issued", "document", 903, "support.session"), detail(a.ID, 44, "DOCUMENT_CONTENT", "ISSUED", 902, 44)}, "23514"},
		"target type":               {[]statement{event(54, "document.access.issued", "export", 903, member), detail(a.ID, 54, "DOCUMENT_CONTENT", "ISSUED", 902, 54)}, "23514"},
		"served without grant":      {[]statement{event(45, "document.content.accessed", "document", 903, member), detail(a.ID, 45, "DOCUMENT_CONTENT", "SERVED", 902, 46)}, "23503"},
		"grant of another member":   {[]statement{event(47, "document.content.accessed", "document", 903, member), detail(a.ID, 47, "DOCUMENT_CONTENT", "SERVED", 904, 30)}, "23503"},
		"grant of another target":   {[]statement{event(53, "document.content.accessed", "document", 905, member), detail(a.ID, 53, "DOCUMENT_CONTENT", "SERVED", 902, 30)}, "23503"},
		"grant of another category": {[]statement{event(48, "export.content.accessed", "export", 903, member), detail(a.ID, 48, "EXPORT_FILE", "SERVED", 902, 30)}, "23503"},
		"served as own grant":       {[]statement{event(49, "document.content.accessed", "document", 903, member), detail(a.ID, 49, "DOCUMENT_CONTENT", "SERVED", 902, 49)}, "23514"},
		"live with a grant":         {[]statement{event(50, "location.live.accessed", "trip", 903, member), detail(a.ID, 50, "LOCATION_LIVE", "SERVED", 902, 30)}, "23514"},
		"live issued":               {[]statement{event(51, "location.live.accessed", "trip", 903, member), detail(a.ID, 51, "LOCATION_LIVE", "ISSUED", 902, 51)}, "23514"},
		"unknown category":          {[]statement{event(52, "document.access.issued", "document", 903, member), detail(a.ID, 52, "SUPPORT_SESSION", "ISSUED", 902, 52)}, "23514"},
	} {
		if err := inScope(probe.statements...); pgCode(err) != probe.code {
			t.Fatalf("%s returned %v, want %s", name, err, probe.code)
		}
	}
	// Even an accidental broad grant cannot mutate or truncate the records.
	if _, err := f.Admin.Exec(ctx, "GRANT UPDATE, DELETE, TRUNCATE ON org_sensitive_access_events TO "+pgx.Identifier{f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`UPDATE org_sensitive_access_events SET membership_id=access_id`, `DELETE FROM org_sensitive_access_events`, `TRUNCATE org_sensitive_access_events`} {
		if err := inScope(statement{query: query}); pgCode(err) != "55000" {
			t.Fatalf("%s returned %v", query, err)
		}
	}
	for _, org := range []orgtest.Organization{a, b} {
		if events, details := accessCounts(t, f, org.ID); events != 2 || details != 2 {
			t.Fatalf("guards changed %s: %d/%d", org.Key, events, details)
		}
	}
}

type accessReadPolicy struct{ orgID string }

func (policy accessReadPolicy) AuthorizeAuditRead(_ context.Context, scope kernel.OrgScope, _ application.ReadAccess) error {
	if scope.OrgID().String() != policy.orgID {
		return domain.ErrDenied
	}
	return nil
}

// BR-AUD-006: the existing audit read service lists access records only after
// its authorizer confirms audit.event.read in that organization.
func TestAuditReaderListsAccessRecordsOnlyWithScopedAuthority(t *testing.T) {
	f, store, log := newAccessFixture(t)
	a, b := f.Organizations[0], f.Organizations[1]
	for _, org := range []orgtest.Organization{a, b} {
		if err := log.RecordAccess(t.Context(), org.Scope(t), sensitive(domain.LocationLive, domain.AccessServed, 60, 60)); err != nil {
			t.Fatal(err)
		}
	}
	if err := log.RecordAccess(t.Context(), a.Scope(t), sensitive(domain.LocationTrack, domain.AccessIssued, 61, 61)); err != nil {
		t.Fatal(err)
	}
	service := application.Service{Reader: store, Authorizer: accessReadPolicy{orgID: a.ID}}
	reader := application.ReadAccess{ActorID: accessID(990)}
	records, err := service.List(t.Context(), a.Scope(t), reader, domain.Filter{})
	if err != nil || len(records) != 2 {
		t.Fatalf("authorized reader listed %d records: %v", len(records), err)
	}
	for _, record := range records {
		if record.Event.ActorID != accessID(901) || record.Event.ReasonCode != domain.AccessReason || record.At.IsZero() {
			t.Fatalf("listed record lost its references: %+v", record)
		}
	}
	if records, err := service.List(t.Context(), b.Scope(t), reader, domain.Filter{}); !errors.Is(err, domain.ErrDenied) || records != nil {
		t.Fatal("reader without authority in the other organization listed records", err)
	}
	if records, err := (application.Service{Reader: store}).List(t.Context(), a.Scope(t), reader, domain.Filter{}); !errors.Is(err, domain.ErrDenied) || records != nil {
		t.Fatal("missing authorizer listed records", err)
	}
}

// Additive migration 000202: failure leaves no schema or ledger state, a retry
// applies once, previous rows and the compatibility floor are unchanged.
func TestSensitiveAccessMigrationUpgradeFailureRecoveryAndPreviousCompatibility(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= accessMigration {
			continue
		}
		raw, err := fs.ReadFile(dbmigrations.Files, entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		previous[entry.Name()] = &fstest.MapFile{Data: raw}
	}
	f := orgtest.NewWithMigrations(t, previous)
	ctx := t.Context()
	a := f.Organizations[0]
	// A pre-upgrade EMP-PLAT-006 issuance event has no detail and stays valid.
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_audit_events (org_id,operation_id,actor_id,action,target_type,target_id,reason_code,request_hash)
		VALUES ($1,$2,$3,'document.access.issued','document',$4,'member.authorized',$5)`,
		a.ID, accessID(70), accessID(901), accessID(903), strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	raw, err := fs.ReadFile(dbmigrations.Files, accessMigration)
	if err != nil {
		t.Fatal(err)
	}
	current := fstest.MapFS{}
	for name, file := range previous {
		current[name] = file
	}
	current[accessMigration] = &fstest.MapFile{Data: raw}
	broken := fstest.MapFS{accessMigration: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken access migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_sensitive_access_events') IS NULL
		AND to_regprocedure('org_sensitive_access_guard()') IS NULL
		AND NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid='org_audit_events'::regclass AND tgname LIKE 'org_sensitive_access%')
		AND NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version='000202')`).Scan(&clean); err != nil || !clean {
		t.Fatal("failed access migration left schema or ledger state", err)
	}
	applied, err := migrate.Apply(ctx, f.AdminDSN, current)
	if err != nil || len(applied) != 1 || applied[0] != "000202" {
		t.Fatalf("access migration retry failed: %v, %v", applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatalf("previous chain became incompatible: %v", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if applied, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(applied) != 0 {
		t.Fatalf("access migration repeated work: %v, %v", applied, err)
	}
	var kept int
	if err := f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2`, a.ID, accessID(70)).Scan(&kept); err != nil || kept != 1 {
		t.Fatal("upgrade changed previous audit events", err)
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("access migration changed compatibility floor")
	}
	grantAccessLog(t, f)
	log := adapters.AccessLog{Runner: adapters.NewStore(f.Runtime, mustAccessCatalog(t))}
	// A grant recorded before the upgrade has no member-bound detail, so it
	// cannot be redeemed through the new log; a fresh grant works.
	if err := log.RecordAccess(ctx, a.Scope(t), sensitive(domain.DocumentContent, domain.AccessServed, 71, 70)); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("pre-upgrade grant redeemed without its member binding", err)
	}
	for _, item := range []domain.SensitiveAccess{
		sensitive(domain.DocumentContent, domain.AccessIssued, 72, 72),
		sensitive(domain.DocumentContent, domain.AccessServed, 73, 72),
	} {
		if err := log.RecordAccess(ctx, a.Scope(t), item); err != nil {
			t.Fatal("upgraded log refused a fresh access", err)
		}
	}
}
