//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
)

// present reports whether the exact private object exists in the disposable
// bucket, through an authenticated HEAD that transfers no bytes.
func (h *harness) present(t *testing.T, scope kernel.OrgScope, object domain.Object) bool {
	t.Helper()
	key, err := object.Key(scope)
	if err != nil {
		t.Fatal(err)
	}
	prefix, _ := object.Prefix(scope)
	head, err := storage.PresignHEAD(storage.PresignInput{Endpoint: h.storageConfig.Endpoint, Bucket: h.storageConfig.Bucket, Region: h.storageConfig.Region, Key: key, Namespace: prefix, TTL: time.Minute, Now: time.Now()}, h.storageConfig.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	found, err := storage.Exists(t.Context(), &http.Client{Timeout: 10 * time.Second}, head.URL)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func (h *harness) object(t *testing.T, scope kernel.OrgScope, export string) domain.Object {
	t.Helper()
	o := domain.Object{OrgID: scope.OrgID().String(), ExportID: export}
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT object_sha256,media_type,object_bytes,row_count FROM org_export_jobs WHERE org_id=$1 AND id=$2`, o.OrgID, export).Scan(&o.SHA256, &o.MediaType, &o.Bytes, &o.Rows); err != nil {
		t.Fatal(err)
	}
	return o
}

// expire moves one READY job's clock into the past as the migration owner
// with triggers suspended, simulating the passage of the retention window.
func (h *harness) expire(t *testing.T, scope kernel.OrgScope, export string) {
	t.Helper()
	tx, err := h.f.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `UPDATE org_export_jobs SET ready_at=now()-interval '8 days',expires_at=now()-interval '1 day',requested_at=now()-interval '8 days 1 minute',as_of=now()-interval '8 days 1 minute',updated_at=now()-interval '8 days' WHERE org_id=$1 AND id=$2 AND state='READY'`, scope.OrgID().String(), export); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `UPDATE job_queue SET not_before=clock_timestamp() WHERE kind=$1 AND dedupe_key=$2`, adapters.CleanupKind, adapters.Dedupe(adapters.CleanupKind, scope, export)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) runCleanup(t *testing.T) bool {
	t.Helper()
	d := &jobs.Dispatcher{Queue: h.queue, Handlers: map[string]jobs.Handler{adapters.CleanupKind: adapters.CleanupHandler{Worker: h.worker}}, WorkerID: "export-cleanup-test", Kind: adapters.CleanupKind, LeaseTTL: time.Minute, RetryDelay: time.Millisecond}
	found, err := d.RunOnce(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func accessRows(t *testing.T, h *harness, scope kernel.OrgScope) []string {
	t.Helper()
	rows, err := h.f.Admin.Query(t.Context(), `SELECT d.stage||'|'||d.membership_id::text||'|'||d.access_id::text||'|'||e.target_id::text||'|'||e.actor_id::text||'|'||d.operation_id::text FROM org_sensitive_access_events d JOIN org_audit_events e USING(org_id,operation_id) WHERE d.org_id=$1 AND d.category='EXPORT_FILE' ORDER BY e.recorded_at`, scope.OrgID().String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	return out
}

// BUC-EMP-116, B-BR-EMP-228/229/230 on PostGIS and the disposable bucket:
// request, dispatch, verified publication, audited access and content,
// expiry at read time and confirmed deletion.
func TestExportLifecycleOnRealStorage(t *testing.T) {
	h := newHarness(t, true)
	service := h.service()
	reqCtx, reqDecision := decisionFor(t, h.a, h.requester, app.ActionRequest)
	v, replayed, err := service.Request(reqCtx, h.a, h.actor(h.requester), reqDecision, app.CommandInput{OperationID: newID(t), Kind: "test.synthetic_rows", Parameters: []byte(`{"rows":3}`)})
	if err != nil || replayed || v.State != domain.Queued {
		t.Fatal("request", err)
	}
	if !h.runBuild(t) {
		t.Fatal("build job not dispatched")
	}
	state, _, attempts, _ := h.state(t, h.a, v.ExportID)
	if state != domain.Ready || attempts != 1 {
		t.Fatal("job not READY after one attempt", state)
	}
	object := h.object(t, h.a, v.ExportID)
	if object.Rows != 3 || object.MediaType != "text/csv" || !h.present(t, h.a, object) {
		t.Fatal("verified private object missing", object.Rows)
	}
	if h.runBuild(t) {
		t.Fatal("a completed build was dispatched again")
	}
	cleanup := h.queueRows(t, adapters.CleanupKind)
	readCtx, readDecision := decisionFor(t, h.a, h.requester, app.ActionRead)
	view, err := service.Read(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID)
	if err != nil || view.State != domain.Ready || view.Rows != 3 || view.SHA256 != object.SHA256 || view.ExpiresAt.Sub(view.ReadyAt) != domain.DefaultRetention {
		t.Fatal("ready view", err)
	}
	if len(cleanup) != 1 || cleanup[0].status != "queued" || cleanup[0].notBefore.Sub(view.ExpiresAt).Abs() > time.Millisecond {
		t.Fatal("cleanup not scheduled at expiry", cleanup)
	}
	capability, err := service.Access(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID)
	if err != nil || capability.Token == "" || capability.ExpiresAt.After(time.Now().Add(domain.CapabilityTTL)) {
		t.Fatal("access", err)
	}
	content, err := service.Open(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID, capability.Token)
	if err != nil || string(content.Bytes) != "row_id,value_centavos\n1,100\n2,200\n3,300\n" || content.SHA256 != object.SHA256 || content.Rows != 3 {
		t.Fatal("content", err)
	}
	records := accessRows(t, h, h.a)
	if len(records) != 2 || !strings.HasPrefix(records[0], "ISSUED|"+h.requester.member) || !strings.HasPrefix(records[1], "SERVED|"+h.requester.member) {
		t.Fatal("access records", records)
	}
	issued, served := strings.Split(records[0], "|"), strings.Split(records[1], "|")
	if issued[2] != issued[5] || served[2] != issued[5] || served[5] == issued[5] || issued[3] != v.ExportID || served[4] != h.requester.account {
		t.Fatal("served record is not bound to its member issuance", records)
	}
	// Nothing in the job, its audit or queue rows carries content, a key, a
	// URL or the capability.
	var dump string
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT concat_ws('|',(SELECT jsonb_agg(to_jsonb(x))::text FROM org_export_jobs x),(SELECT jsonb_agg(to_jsonb(x))::text FROM org_audit_events x),(SELECT jsonb_agg(to_jsonb(x))::text FROM org_sensitive_access_events x),(SELECT jsonb_agg(jsonb_build_array(payload,dedupe_key,last_error))::text FROM job_queue x))`).Scan(&dump); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"row_id", "1,100", "org/", "/export/", "http", "X-Amz", capability.Token, h.storageConfig.Bucket} {
		if strings.Contains(dump, forbidden) {
			t.Fatal("private content, key, URL or capability persisted", forbidden)
		}
	}
	// Another member and another organization cannot use the job or capability.
	colleagueCtx, colleagueDecision := decisionFor(t, h.a, h.colleague, app.ActionRead)
	if _, err := service.Access(colleagueCtx, h.a, h.actor(h.colleague), colleagueDecision, v.ExportID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("colleague obtained access", err)
	}
	if _, err := service.Open(colleagueCtx, h.a, h.actor(h.colleague), colleagueDecision, v.ExportID, capability.Token); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("colleague opened content with the requester's capability", err)
	}
	foreignCtx, foreignDecision := decisionFor(t, h.b, h.foreign, app.ActionRead)
	if _, err := service.Open(foreignCtx, h.b, h.actor(h.foreign), foreignDecision, v.ExportID, capability.Token); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("another organization opened content", err)
	}
	if len(accessRows(t, h, h.a)) != 2 || len(accessRows(t, h, h.b)) != 0 {
		t.Fatal("refused access was recorded")
	}
	// Expiry is enforced at read time before the cleanup runs.
	h.expire(t, h.a, v.ExportID)
	if _, err := service.Access(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID); !errors.Is(err, app.ErrExpired) {
		t.Fatal("expired job issued access", err)
	}
	if _, err := service.Open(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID, capability.Token); !errors.Is(err, app.ErrExpired) {
		t.Fatal("expired job served content", err)
	}
	if !h.runCleanup(t) {
		t.Fatal("cleanup not dispatched")
	}
	state, _, _, _ = h.state(t, h.a, v.ExportID)
	var deleted bool
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT object_deleted_at IS NOT NULL AND expired_at IS NOT NULL FROM org_export_jobs WHERE id=$1`, v.ExportID).Scan(&deleted); err != nil || state != domain.Expired || !deleted || h.present(t, h.a, object) {
		t.Fatal("expired object not deleted and recorded", state, deleted)
	}
	if expired, err := service.Read(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID); err != nil || expired.State != domain.Expired || expired.SHA256 != object.SHA256 {
		t.Fatal("expired metadata not kept", err)
	}
	if _, err := service.Access(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID); !errors.Is(err, app.ErrExpired) {
		t.Fatal("expired job downloadable", err)
	}
	if len(accessRows(t, h, h.a)) != 2 {
		t.Fatal("refused expired access was recorded")
	}
}

// failingObjects injects storage failure mid-publication around real S3.
type failingObjects struct {
	*adapters.S3
	config   adapters.StorageConfig
	mu       sync.Mutex
	mode     string
	attempts int
}

func (o *failingObjects) Put(ctx context.Context, scope kernel.OrgScope, object domain.Object, raw []byte) error {
	o.mu.Lock()
	mode := o.mode
	o.attempts++
	o.mu.Unlock()
	switch mode {
	case "partial":
		// A truncated body lands at the exact key before verification.
		key, _ := object.Key(scope)
		prefix, _ := object.Prefix(scope)
		put, err := storage.PresignServerPUT(storage.PresignInput{Endpoint: o.config.Endpoint, Bucket: o.config.Bucket, Region: o.config.Region, Key: key, Namespace: prefix, ContentType: object.MediaType, MaxBytes: object.Bytes, TTL: time.Minute, Now: time.Now()}, o.config.Credentials)
		if err != nil {
			return err
		}
		if _, err := storage.PutIfAbsent(ctx, &http.Client{Timeout: 10 * time.Second}, put.URL, object.MediaType, raw[:len(raw)/2]); err != nil {
			return err
		}
	case "outage":
		return app.ErrUnavailable
	}
	return o.S3.Put(ctx, scope, object, raw)
}

// B-BR-EMP-229, BUC-EMP-117: a partial or failed upload is never served; the
// corrupt object is removed and the retry publishes the complete file.
func TestIncompleteUploadIsNeverServed(t *testing.T) {
	h := newHarness(t, true)
	objects := &failingObjects{S3: h.s3, config: h.storageConfig, mode: "partial"}
	h.worker.Objects = objects
	service := h.service()
	v := h.request(t, h.a, h.requester, 4)
	if !h.runBuild(t) {
		t.Fatal("build not dispatched")
	}
	state, _, attempts, provisional := h.state(t, h.a, v.ExportID)
	if state != domain.Queued || attempts != 1 || !provisional {
		t.Fatal("partial upload did not leave a retryable provisional job", state, attempts)
	}
	object := h.object(t, h.a, v.ExportID)
	if h.present(t, h.a, object) {
		t.Fatal("corrupt object left at the key")
	}
	readCtx, readDecision := decisionFor(t, h.a, h.requester, app.ActionRead)
	if _, err := service.Access(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID); !errors.Is(err, app.ErrNotReady) {
		t.Fatal("incomplete job issued access", err)
	}
	for _, row := range h.queueRows(t, adapters.BuildKind) {
		for _, forbidden := range []string{"http", "org/", "X-Amz", h.storageConfig.Bucket} {
			if strings.Contains(row.lastError, forbidden) {
				t.Fatal("queue error leaks a storage reference", row.lastError)
			}
		}
	}
	objects.mode = "outage"
	if _, err := h.f.Admin.Exec(t.Context(), `UPDATE job_queue SET not_before=clock_timestamp() WHERE kind=$1`, adapters.BuildKind); err != nil {
		t.Fatal(err)
	}
	h.runBuild(t)
	if state, _, attempts, _ := h.state(t, h.a, v.ExportID); state != domain.Queued || attempts != 2 {
		t.Fatal("storage outage changed the job", state, attempts)
	}
	objects.mode = ""
	if _, err := h.f.Admin.Exec(t.Context(), `UPDATE job_queue SET not_before=clock_timestamp() WHERE kind=$1`, adapters.BuildKind); err != nil {
		t.Fatal(err)
	}
	h.runBuild(t)
	if state, _, attempts, _ := h.state(t, h.a, v.ExportID); state != domain.Ready || attempts != 3 {
		t.Fatal("healthy retry did not publish", state, attempts)
	}
	capability, err := service.Access(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := service.Open(readCtx, h.a, h.actor(h.requester), readDecision, v.ExportID, capability.Token)
	if err != nil || strings.Count(string(content.Bytes), "\n") != 5 {
		t.Fatal("published content is not the complete file", err)
	}
	// Exhausting the attempts settles FAILED and deletes the provisional object.
	other := h.request(t, h.a, h.requester, 2)
	objects.mode = "partial"
	for range domain.MaxAttempts {
		if _, err := h.f.Admin.Exec(t.Context(), `UPDATE job_queue SET not_before=clock_timestamp() WHERE kind=$1`, adapters.BuildKind); err != nil {
			t.Fatal(err)
		}
		h.runBuild(t)
	}
	state, failure, _, _ := h.state(t, h.a, other.ExportID)
	if state != domain.Failed || failure != string(domain.AttemptsExhausted) {
		t.Fatal("attempts not bounded", state, failure)
	}
	if !h.runCleanup(t) {
		t.Fatal("failed provisional object not scheduled for cleanup")
	}
	var deleted bool
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT object_deleted_at IS NOT NULL FROM org_export_jobs WHERE id=$1`, other.ExportID).Scan(&deleted); err != nil || !deleted || h.present(t, h.a, h.object(t, h.a, other.ExportID)) {
		t.Fatal("failed provisional object kept", deleted)
	}
}

// B-BR-EMP-228, BUC-EMP-117: revoked rights and unbounded work end FAILED
// without any stored object; nothing is truncated.
func TestRightsLossAndCeilingsPublishNothing(t *testing.T) {
	h := newHarness(t, true)
	revoked := h.request(t, h.a, h.requester, 1)
	h.authority.revoke(h.requester.member)
	if !h.runBuild(t) {
		t.Fatal("build not dispatched")
	}
	if state, failure, _, object := h.state(t, h.a, revoked.ExportID); state != domain.Failed || failure != string(domain.RightsLost) || object || h.format.count() != 0 {
		t.Fatal("revoked requester produced", state, failure)
	}
	big := h.request(t, h.a, h.colleague, 51)
	if !h.runBuild(t) {
		t.Fatal("build not dispatched")
	}
	if state, failure, _, object := h.state(t, h.a, big.ExportID); state != domain.Failed || failure != string(domain.TooLarge) || object {
		t.Fatal("unbounded work published", state, failure)
	}
	if len(h.queueRows(t, adapters.CleanupKind)) != 0 {
		t.Fatal("cleanup scheduled although nothing was stored")
	}
	readCtx, readDecision := decisionFor(t, h.a, h.colleague, app.ActionRead)
	if _, err := h.service().Access(readCtx, h.a, h.actor(h.colleague), readDecision, big.ExportID); !errors.Is(err, app.ErrNotReady) {
		t.Fatal("failed job issued access", err)
	}
}

// B-BR-EMP-228: concurrent workers claim one job once and publish one file.
func TestConcurrentWorkersProcessOnce(t *testing.T) {
	h := newHarness(t, true)
	v := h.request(t, h.a, h.requester, 2)
	h.format.started, h.format.release = make(chan struct{}, 8), make(chan struct{})
	workers := 6
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool := h.f.RuntimePool(t, 2)
			w := h.worker
			w.Store = &adapters.Store{Pool: pool, Authority: h.authority, Audit: h.store.Audit}
			errs <- w.Build(t.Context(), h.a, v.ExportID)
		}()
	}
	<-h.format.started
	time.Sleep(300 * time.Millisecond)
	close(h.format.release)
	wg.Wait()
	close(errs)
	busy := 0
	for err := range errs {
		if errors.Is(err, app.ErrBusy) {
			busy++
		} else if err != nil {
			t.Fatal("unexpected worker failure", err)
		}
	}
	if h.format.count() != 1 || busy != workers-1 {
		t.Fatal("concurrent claims processed more than once", h.format.count(), busy)
	}
	if state, _, attempts, _ := h.state(t, h.a, v.ExportID); state != domain.Ready || attempts != 1 {
		t.Fatal("job not published once", state, attempts)
	}
	// The first job's queue entry settles without new work.
	if !h.runBuild(t) || h.format.count() != 1 {
		t.Fatal("settled queue entry produced again")
	}
	// Two real dispatchers draining one queue job: exactly one finds it.
	second := h.request(t, h.a, h.requester, 1)
	h.format.started, h.format.release = nil, nil
	found := make(chan bool, 2)
	for i := range 2 {
		go func(i int) {
			d := &jobs.Dispatcher{Queue: jobs.NewQueue(h.f.RuntimePool(t, 2)), Handlers: map[string]jobs.Handler{adapters.BuildKind: adapters.BuildHandler{Worker: h.worker}}, WorkerID: "export-race-" + string(rune('a'+i)), Kind: adapters.BuildKind, LeaseTTL: time.Minute}
			ok, err := d.RunOnce(context.Background())
			if err != nil {
				t.Error(err)
			}
			found <- ok
		}(i)
	}
	if a, b := <-found, <-found; a == b {
		t.Fatal("queue job claimed by both or neither dispatcher", a, b)
	}
	if state, _, _, _ := h.state(t, h.a, second.ExportID); state != domain.Ready {
		t.Fatal("dispatched job not published", state)
	}
}

// B-BR-EMP-230, BR-AUD: an audit outage issues no capability and serves no
// byte; recovery restores access with exactly one new record.
func TestAuditWriteFailureServesNothing(t *testing.T) {
	h := newHarness(t, true)
	v := h.request(t, h.a, h.requester, 1)
	h.runBuild(t)
	service := h.service()
	ctx, d := decisionFor(t, h.a, h.requester, app.ActionRead)
	capability, err := service.Access(ctx, h.a, h.actor(h.requester), d, v.ExportID)
	if err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{h.f.RuntimeRole}.Sanitize()
	if _, err := h.f.Admin.Exec(t.Context(), "REVOKE INSERT ON org_sensitive_access_events FROM "+role); err != nil {
		t.Fatal(err)
	}
	if again, err := service.Access(ctx, h.a, h.actor(h.requester), d, v.ExportID); err == nil || again.Token != "" {
		t.Fatal("capability issued without its record")
	}
	if content, err := service.Open(ctx, h.a, h.actor(h.requester), d, v.ExportID, capability.Token); err == nil || content.Bytes != nil {
		t.Fatal("content served without its record")
	}
	var events int
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_audit_events WHERE action IN ('export.access.issued','export.content.accessed')`).Scan(&events); err != nil || events != 1 {
		t.Fatal("failed record left a generic audit event", events)
	}
	if _, err := h.f.Admin.Exec(t.Context(), "GRANT INSERT ON org_sensitive_access_events TO "+role); err != nil {
		t.Fatal(err)
	}
	if content, err := service.Open(ctx, h.a, h.actor(h.requester), d, v.ExportID, capability.Token); err != nil || len(content.Bytes) == 0 {
		t.Fatal("recovery did not restore content", err)
	}
	if len(accessRows(t, h, h.a)) != 2 {
		t.Fatal("recovery recorded more than one served access")
	}
}
