//go:build integration

package adapters_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

// B-BR-EMP-227, BUC-EMP-116: one request stores one QUEUED job, one audit
// event and one reference-only queue entry; the operation ID replays it.
func TestExportRequestIsAuditedEnqueuedAndIdempotent(t *testing.T) {
	h := newHarness(t, false)
	c := h.command(t, h.a, 2)
	export := newID(t)
	v, replayed, err := h.store.Request(t.Context(), h.a, h.actor(h.requester), h.requester.member, export, c)
	if err != nil || replayed || v.ExportID != export || v.State != domain.Queued || v.Kind != "test.synthetic_rows" || string(v.Parameters) != `{"rows":2}` || v.ParametersDigest != c.ParametersDigest() ||
		v.Attempts != 0 || v.MaxAttempts != 3 || v.RequestedAt.IsZero() || !v.AsOf.Equal(v.RequestedAt) || v.Validate() != nil {
		t.Fatal("request", err, v.State)
	}
	var stored struct {
		account, member, hash, media string
		rows, bytes, ms, retention   int64
	}
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT requester_account_id::text,requester_membership_id::text,request_hash,media_type,row_limit,byte_limit,time_limit_ms,retention_seconds FROM org_export_jobs WHERE org_id=$1 AND id=$2`, h.a.OrgID().String(), export).
		Scan(&stored.account, &stored.member, &stored.hash, &stored.media, &stored.rows, &stored.bytes, &stored.ms, &stored.retention); err != nil {
		t.Fatal(err)
	}
	if stored.account != h.requester.account || stored.member != h.requester.member || stored.hash != c.Digest(h.a, app.Requester{AccountID: h.requester.account, MembershipID: h.requester.member}) ||
		stored.media != "text/csv" || stored.rows != 50 || stored.bytes != 4096 || stored.ms != 2000 || stored.retention != int64(domain.DefaultRetention/time.Second) {
		t.Fatal("stored request facts", stored)
	}
	var action, target, actor, hash string
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT action,target_id::text,actor_id::text,request_hash FROM org_audit_events WHERE org_id=$1 AND operation_id=$2`, h.a.OrgID().String(), c.OperationID()).Scan(&action, &target, &actor, &hash); err != nil {
		t.Fatal(err)
	}
	if action != "rpt.export.requested" || target != export || actor != h.requester.account || hash != stored.hash {
		t.Fatal("request audit event", action, target)
	}
	queued := h.queueRows(t, adapters.BuildKind)
	var payload map[string]any
	if len(queued) != 1 || json.Unmarshal([]byte(queued[0].payload), &payload) != nil || len(payload) != 3 || payload["version"] != float64(1) || payload["org_id"] != h.a.OrgID().String() || payload["export_id"] != export ||
		queued[0].dedupe != "org-export-build:"+h.a.OrgID().String()+":"+export || queued[0].status != "queued" {
		t.Fatal("queue entry is not the exact reference-only envelope", queued)
	}
	again, replayed, err := h.store.Request(t.Context(), h.a, h.actor(h.requester), h.requester.member, newID(t), c)
	if err != nil || !replayed || again.ExportID != export || again.ParametersDigest != v.ParametersDigest || !again.RequestedAt.Equal(v.RequestedAt) {
		t.Fatal("replay", err)
	}
	changed, err := app.NewCommand(h.a, h.formats, app.CommandInput{OperationID: c.OperationID(), Kind: "test.synthetic_rows", Parameters: []byte(`{"rows":3}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.store.Request(t.Context(), h.a, h.actor(h.requester), h.requester.member, newID(t), changed); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("changed command reused an operation", err)
	}
	if _, _, err := h.store.Request(t.Context(), h.a, h.actor(h.colleague), h.colleague.member, newID(t), c); !errors.Is(err, app.ErrOperationReused) {
		t.Fatal("another member reused an operation", err)
	}
	var jobs, events, entries int
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_export_jobs),(SELECT count(*) FROM org_audit_events),(SELECT count(*) FROM job_queue)`).Scan(&jobs, &events, &entries); err != nil || jobs != 1 || events != 1 || entries != 1 {
		t.Fatal("replay or refusal wrote again", jobs, events, entries)
	}
	// Concurrent identical requests converge on one job.
	dup := h.command(t, h.a, 1)
	var wg sync.WaitGroup
	results := make(chan bool, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, replayed, err := h.store.Request(t.Context(), h.a, h.actor(h.requester), h.requester.member, newID(t), dup)
			if err != nil {
				failures <- err
				return
			}
			results <- replayed
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	created := 0
	for replayed := range results {
		if !replayed {
			created++
		}
	}
	for err := range failures {
		t.Fatal("concurrent duplicate refused", err)
	}
	if created != 1 {
		t.Fatal("concurrent duplicates created", created)
	}
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_export_jobs WHERE operation_id=$1`, dup.OperationID()).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatal("concurrent duplicates stored", jobs)
	}
}

// B-BR-EMP-227: three open jobs per requester; concurrent requests cannot
// exceed the limit; a settled job frees a slot.
func TestOpenExportLimitIsEnforcedUnderConcurrency(t *testing.T) {
	h := newHarness(t, false)
	var wg sync.WaitGroup
	outcomes := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := h.store.Request(t.Context(), h.a, h.actor(h.requester), h.requester.member, newID(t), h.command(t, h.a, 1))
			outcomes <- err
		}()
	}
	wg.Wait()
	close(outcomes)
	accepted, full := 0, 0
	for err := range outcomes {
		switch {
		case err == nil:
			accepted++
		case errors.Is(err, app.ErrQueueFull):
			full++
		default:
			t.Fatal("unexpected refusal", err)
		}
	}
	if accepted != domain.OpenJobLimit || full != 3 {
		t.Fatal("open limit not enforced", accepted, full)
	}
	// Another member keeps an independent budget.
	h.request(t, h.a, h.colleague, 1)
	// A job settled by the worker frees one slot.
	var export string
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_export_jobs WHERE requester_membership_id=$1 LIMIT 1`, h.requester.member).Scan(&export); err != nil {
		t.Fatal(err)
	}
	h.authority.revoke(h.requester.member)
	if err := h.worker.Build(t.Context(), h.a, export); err != nil {
		t.Fatal(err)
	}
	h.authority.revoked = map[string]bool{}
	h.request(t, h.a, h.requester, 1)
	if _, _, err := h.store.Request(t.Context(), h.a, h.actor(h.requester), h.requester.member, newID(t), h.command(t, h.a, 1)); !errors.Is(err, app.ErrQueueFull) {
		t.Fatal("limit not restored after the new job", err)
	}
}

// B-BR-EMP-230, BR-PRIV-002: only the requester reads its jobs; another
// member and another organization get NOT_FOUND; lists page newest first.
func TestExportReadsAreRequesterBoundAndIsolated(t *testing.T) {
	h := newHarness(t, false)
	first := h.request(t, h.a, h.requester, 1)
	second := h.request(t, h.a, h.requester, 2)
	third := h.request(t, h.a, h.requester, 3)
	other := h.request(t, h.a, h.colleague, 1)
	foreign := h.request(t, h.b, h.foreign, 1)
	v, err := h.store.Read(t.Context(), h.a, h.actor(h.requester), h.requester.member, first.ExportID)
	if err != nil || v.ExportID != first.ExportID || v.Validate() != nil {
		t.Fatal("own read", err)
	}
	for name, call := range map[string]func() error{
		"colleague read": func() error {
			_, err := h.store.Read(t.Context(), h.a, h.actor(h.colleague), h.colleague.member, first.ExportID)
			return err
		},
		"colleague deliverable": func() error {
			_, err := h.store.Deliverable(t.Context(), h.a, h.actor(h.colleague), h.colleague.member, first.ExportID)
			return err
		},
		"foreign organization read": func() error {
			_, err := h.store.Read(t.Context(), h.b, h.actor(h.foreign), h.foreign.member, first.ExportID)
			return err
		},
		"requester in the other organization": func() error {
			_, err := h.store.Read(t.Context(), h.a, h.actor(h.requester), h.requester.member, foreign.ExportID)
			return err
		},
		"unknown": func() error {
			_, err := h.store.Read(t.Context(), h.a, h.actor(h.requester), h.requester.member, newID(t))
			return err
		},
	} {
		if err := call(); !errors.Is(err, app.ErrNotFound) {
			t.Fatal("job visible to a non-requester:", name, err)
		}
	}
	if _, err := h.store.Read(t.Context(), h.a, h.actor(h.colleague), h.requester.member, first.ExportID); err == nil {
		t.Fatal("borrowed membership read a job")
	}
	if _, err := h.store.Deliverable(t.Context(), h.a, h.actor(h.requester), h.requester.member, first.ExportID); !errors.Is(err, app.ErrNotReady) {
		t.Fatal("queued job deliverable", err)
	}
	page, err := h.store.List(t.Context(), h.a, h.actor(h.requester), h.requester.member, app.Filter{Limit: 2})
	if err != nil || len(page.Exports) != 2 || page.Exports[0].ExportID != third.ExportID || page.Exports[1].ExportID != second.ExportID || page.Next != app.KeyOf(page.Exports[1]) {
		t.Fatal("first page", err)
	}
	rest, err := h.store.List(t.Context(), h.a, h.actor(h.requester), h.requester.member, app.Filter{Limit: 2, After: page.Next})
	if err != nil || len(rest.Exports) != 1 || rest.Exports[0].ExportID != first.ExportID || !rest.Next.Zero() {
		t.Fatal("second page", err)
	}
	for _, view := range append(page.Exports, rest.Exports...) {
		if view.ExportID == other.ExportID || view.ExportID == foreign.ExportID {
			t.Fatal("list leaked another requester's job")
		}
	}
	none, err := h.store.List(t.Context(), h.a, h.actor(h.requester), h.requester.member, app.Filter{Limit: 5, State: domain.Ready})
	if err != nil || len(none.Exports) != 0 {
		t.Fatal("state filter", err)
	}
}

// BR-PRIV-001/002, EDR-156: forced RLS, the transition guard and the
// immutability triggers hold even for direct SQL by the runtime role.
func TestExportSchemaGuardsHoldForDirectSQL(t *testing.T) {
	h := newHarness(t, false)
	v := h.request(t, h.a, h.requester, 1)
	h.request(t, h.b, h.foreign, 1)
	org := h.a.OrgID().String()
	var visible int
	if err := h.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_export_jobs`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("rows visible without organization scope", visible, err)
	}
	if err := database.WithOrgTx(t.Context(), h.f.Runtime, h.b, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_export_jobs`).Scan(&visible)
	}); err != nil || visible != 1 {
		t.Fatal("other organization sees more than its own job", visible, err)
	}
	exec := func(sql string, args ...any) error {
		return database.WithOrgTx(t.Context(), h.f.Runtime, h.a, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), sql, args...); return err })
	}
	if _, err := h.f.Runtime.Exec(t.Context(), `INSERT INTO org_export_jobs(org_id,id,operation_id,requester_account_id,requester_membership_id,kind,parameters,parameters_digest,request_hash,media_type,row_limit,byte_limit,time_limit_ms,retention_seconds,max_attempts,state,attempts,requested_at,updated_at)
		VALUES($1,$2,$3,$4,$5,'test.synthetic_rows','{}',repeat('a',64),repeat('a',64),'text/csv',1,1,1000,86400,3,'QUEUED',0,now(),now())`, org, newID(t), newID(t), h.requester.account, h.requester.member); pgCode(err) != "42501" {
		t.Fatal("unscoped insert admitted", err)
	}
	insert := `INSERT INTO org_export_jobs(org_id,id,operation_id,requester_account_id,requester_membership_id,kind,parameters,parameters_digest,request_hash,media_type,row_limit,byte_limit,time_limit_ms,retention_seconds,max_attempts,state,attempts,requested_at,updated_at)
		VALUES($1,$2,$3,$4,$5,'test.synthetic_rows','{}',repeat('a',64),repeat('a',64),'text/csv',1,1,1000,86400,3,$6,0,now(),now())`
	if err := exec(insert, h.b.OrgID().String(), newID(t), newID(t), h.requester.account, h.requester.member, "QUEUED"); pgCode(err) != "42501" {
		t.Fatal("cross-organization insert admitted", err)
	}
	if err := exec(insert, org, newID(t), newID(t), h.requester.account, h.requester.member, "READY"); pgCode(err) == "" {
		t.Fatal("job inserted in another state")
	}
	if err := exec(insert, org, newID(t), newID(t), h.requester.account, h.foreign.member, "QUEUED"); pgCode(err) == "" {
		t.Fatal("job inserted for a non-member requester")
	}
	if err := exec(insert, org, newID(t), newID(t), h.requester.account, h.requester.member, "QUEUED"); pgCode(err) != "23503" {
		t.Fatal("job committed without its request audit event", err)
	}
	for name, sql := range map[string]string{
		"skip to ready":      `UPDATE org_export_jobs SET state='READY' WHERE id=$1`,
		"rewrite kind":       `UPDATE org_export_jobs SET kind='test.other_rows' WHERE id=$1`,
		"rewrite parameters": `UPDATE org_export_jobs SET parameters='{"rows":9}' WHERE id=$1`,
		"rewrite requester":  `UPDATE org_export_jobs SET requester_membership_id=requester_account_id WHERE id=$1`,
		"raise ceiling":      `UPDATE org_export_jobs SET row_limit=row_limit+1 WHERE id=$1`,
		"fail without code":  `UPDATE org_export_jobs SET state='FAILED' WHERE id=$1`,
		"claim twice":        `UPDATE org_export_jobs SET state='RUNNING',attempts=attempts+2,lease_token=gen_random_uuid(),lease_expires_at=clock_timestamp()+interval '1 minute' WHERE id=$1`,
		"object on queued":   `UPDATE org_export_jobs SET object_sha256=repeat('b',64),object_bytes=1,row_count=0,object_deleted_at=clock_timestamp() WHERE id=$1`,
	} {
		if err := exec(sql, v.ExportID); pgCode(err) == "" {
			t.Fatal("guard admitted:", name)
		}
	}
	for _, sql := range []string{`DELETE FROM org_export_jobs WHERE id=$1`, `TRUNCATE org_export_jobs`} {
		var err error
		if strings.HasPrefix(sql, "TRUNCATE") {
			err = exec(sql)
		} else {
			err = exec(sql, v.ExportID)
		}
		if pgCode(err) == "" {
			t.Fatal("export metadata removed", sql)
		}
	}
	state, _, _, _ := h.state(t, h.a, v.ExportID)
	if state != domain.Queued {
		t.Fatal("refused SQL changed the job")
	}
	// A settled job cannot move again, even after an accidental broad grant.
	h.authority.revoke(h.requester.member)
	if err := h.worker.Build(t.Context(), h.a, v.ExportID); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`UPDATE org_export_jobs SET state='QUEUED',failure_code=NULL,failed_at=NULL WHERE id=$1`, `UPDATE org_export_jobs SET failure_code='EXPORT_TOO_LARGE' WHERE id=$1`} {
		if err := exec(sql, v.ExportID); pgCode(err) == "" {
			t.Fatal("terminal job changed", sql)
		}
	}
}

// Additive migration 000207: a failure leaves no schema or ledger state; the
// retry applies exactly once; the previous chain stays compatible.
func TestExportMigrationPopulatedUpgradeFailureAndRecovery(t *testing.T) {
	previous := fstest.MapFS{}
	entries, err := fs.ReadDir(dbmigrations.Files, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "schema_min_supported" && entry.Name() >= exportMigration {
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
	// Populated previous head: an audit event and a queued platform job.
	if _, err := f.Admin.Exec(ctx, `INSERT INTO org_audit_events(org_id,operation_id,actor_id,action,target_type,target_id,reason_code,request_hash) VALUES($1,$2,$3,'synthetic.previous.recorded','synthetic',$4,'',$5)`,
		a.ID, newID(t), newID(t), newID(t), strings.Repeat("c", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(ctx, `INSERT INTO job_queue(id,kind,payload,max_attempts,not_before) VALUES($1,'synthetic-previous','{"version":1}',3,now())`, newID(t)); err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var out string
		if err := f.Admin.QueryRow(ctx, `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY operation_id) FROM org_audit_events x),(SELECT jsonb_agg(jsonb_build_array(id,kind,status) ORDER BY id) FROM job_queue x))::text`).Scan(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := snapshot()
	raw, err := fs.ReadFile(dbmigrations.Files, exportMigration)
	if err != nil {
		t.Fatal(err)
	}
	current := fstest.MapFS{}
	for name, file := range previous {
		current[name] = file
	}
	current[exportMigration] = &fstest.MapFile{Data: raw}
	broken := fstest.MapFS{exportMigration: &fstest.MapFile{Data: append(append([]byte(nil), raw...), []byte("\nSELECT 1/0;\n")...)}}
	if _, err := migrate.Apply(ctx, f.AdminDSN, broken); err == nil {
		t.Fatal("broken export migration succeeded")
	}
	var clean bool
	if err := f.Admin.QueryRow(ctx, `SELECT to_regclass('org_export_jobs') IS NULL AND to_regprocedure('org_export_job_guard()') IS NULL
		AND NOT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, exportVersion).Scan(&clean); err != nil || !clean {
		t.Fatal("failed export migration left schema or ledger state", err)
	}
	if snapshot() != before {
		t.Fatal("failed migration changed populated rows")
	}
	applied, err := migrate.Apply(ctx, f.AdminDSN, current)
	if err != nil || len(applied) != 1 || applied[0] != exportVersion {
		t.Fatalf("export migration retry: %v %v", applied, err)
	}
	if err := migrate.Check(ctx, f.Admin, previous); err != nil {
		t.Fatal("previous chain became incompatible", err)
	}
	if err := migrate.Check(ctx, f.Admin, current); err != nil {
		t.Fatal(err)
	}
	if again, err := migrate.Apply(ctx, f.AdminDSN, current); err != nil || len(again) != 0 {
		t.Fatal("export migration repeated work", again, err)
	}
	if snapshot() != before {
		t.Fatal("upgrade changed populated rows")
	}
	floor, err := fs.ReadFile(dbmigrations.Files, "schema_min_supported")
	if err != nil || string(floor) != "000132\n" {
		t.Fatal("export migration changed the compatibility floor")
	}
	var forced, owned bool
	if err := f.Admin.QueryRow(ctx, `SELECT relforcerowsecurity, relrowsecurity FROM pg_class WHERE oid='org_export_jobs'::regclass`).Scan(&forced, &owned); err != nil || !forced || !owned {
		t.Fatal("forced RLS missing after upgrade", err)
	}
	// The upgraded schema accepts real requests through the ordinary login.
	grantExports(t, f)
	scope := a.Scope(t)
	p := newPerson(t, f, scope)
	format := &rowsFormat{limits: domain.DefaultLimits()}
	formats, _ := app.NewRegistry(format)
	catalog, _ := audit.NewCatalog(adapters.AuditDefinitions())
	store := &adapters.Store{Pool: f.Runtime, Authority: &testAuthority{revoked: map[string]bool{}}, Audit: auditpg.NewStore(f.Runtime, catalog)}
	c, err := app.NewCommand(scope, formats, app.CommandInput{OperationID: newID(t), Kind: "test.synthetic_rows", Parameters: []byte(`{"rows":1}`)})
	if err != nil {
		t.Fatal(err)
	}
	if v, _, err := store.Request(ctx, scope, app.Actor{AccountID: p.account, SessionID: p.member}, p.member, newID(t), c); err != nil || v.State != domain.Queued {
		t.Fatal("upgraded store refused a request", err)
	}
}
