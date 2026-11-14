//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	mediaapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// jpegWith inserts one metadata segment after SOI of the synthetic original.
func jpegWith(raw []byte, marker byte, payload []byte) []byte {
	n := len(payload) + 2
	out := append([]byte{0xff, 0xd8, 0xff, marker, byte(n >> 8), byte(n)}, payload...)
	return append(out, raw[2:]...)
}

func (l *liveDocuments) objectOf(scope kernel.OrgScope, command app.Create) media.Object {
	return media.Object{OrgID: scope.OrgID().String(), DocumentID: command.DocumentID, SHA256: command.Original.SHA256, MIME: app.MediaTypeJPEG,
		Bytes: command.Original.Bytes, Revision: 2, Synthetic: true}
}

func (l *liveDocuments) input(t *testing.T, object media.Object) storage.PresignInput {
	t.Helper()
	return storage.PresignInput{Endpoint: l.config.Endpoint, Bucket: l.config.Bucket, Region: l.config.Region,
		Namespace: "org/" + object.OrgID + "/doc/" + object.DocumentID + "/", Key: "org/" + object.OrgID + "/doc/" + object.DocumentID + "/" + object.SHA256,
		ContentType: app.MediaTypeJPEG, MaxBytes: 6 << 20, TTL: 5 * time.Minute}
}

// plant writes bytes under a document key without any verification, as a
// crashed transfer, an older inspector or a substituted object would leave.
func (l *liveDocuments) plant(t *testing.T, object media.Object, raw []byte) {
	t.Helper()
	signed, err := storage.PresignServerPUT(l.input(t, object), l.config.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := storage.PutIfAbsent(t.Context(), &http.Client{Timeout: 20 * time.Second}, signed.URL, app.MediaTypeJPEG, raw); err != nil || !created {
		t.Fatal("planting a synthetic object failed", err)
	}
}

func (l *liveDocuments) present(t *testing.T, object media.Object) bool {
	t.Helper()
	head, err := storage.PresignHEAD(l.input(t, object), l.config.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	present, err := storage.Exists(t.Context(), &http.Client{Timeout: 20 * time.Second}, head.URL)
	if err != nil {
		t.Fatal(err)
	}
	return present
}

// received declares a document and records its verified bytes as RECEIVED
// without transferring them; the test then plants what storage holds.
func (l *liveDocuments) received(t *testing.T, raw []byte) app.Create {
	t.Helper()
	command, _ := l.declared(t, l.driver, raw)
	if _, _, err := l.store.MarkReceived(t.Context(), l.scopeA, l.driver, command.DocumentID, newID(t)); err != nil {
		t.Fatal(err)
	}
	return command
}

func (l *liveDocuments) row(t *testing.T, id string) (string, int64, string) {
	t.Helper()
	var state, reason string
	var revision int64
	if err := l.db.Admin.QueryRow(t.Context(), `SELECT storage_state,storage_revision,COALESCE(storage_reason,'') FROM org_documents WHERE id=$1`, id).Scan(&state, &revision, &reason); err != nil {
		t.Fatal(err)
	}
	return state, revision, reason
}

func (l *liveDocuments) events(t *testing.T, id, action string) int {
	t.Helper()
	return l.count(t, l.scopeA, `SELECT count(*) FROM org_audit_events WHERE target_id=$1 AND action=$2`, id, action)
}

func (l *liveDocuments) eventReason(t *testing.T, id, action string) (string, string) {
	t.Helper()
	var reason, actor string
	if err := database.WithOrgTx(t.Context(), l.db.Runtime, l.scopeA, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT reason_code,actor_id::text FROM org_audit_events WHERE target_id=$1 AND action=$2`, id, action).Scan(&reason, &actor)
	}); err != nil {
		t.Fatal(err)
	}
	return reason, actor
}

// served reports whether any member can obtain a content capability.
func (l *liveDocuments) served(t *testing.T, service app.Service, id string) bool {
	t.Helper()
	_, err := service.IssueContent(t.Context(), l.scopeA, l.manager, id)
	if err != nil && !errors.Is(err, media.ErrNotFound) {
		t.Fatal("content refusal is not NOT_FOUND", err)
	}
	return err == nil
}

// BUC-EMP-128 (ingress): integrity refusals stay UPLOADING with no object;
// refused content closes terminally with its reason and stores nothing; a
// clean original is RECEIVED, checked over the stored object and STORED.
func TestUploadSafetyClosesRefusedContentAndStoresOnlyCheckedOriginals(t *testing.T) {
	l := newLiveDocuments(t)
	clean := l.raw
	cases := []struct {
		name   string
		raw    []byte
		err    error
		state  string
		reason string
		action string
		code   string
	}{
		{"trailing-archive", append(bytes.Clone(clean), []byte("PK\x03\x04synthetic")...), media.ErrUnsafe, "QUARANTINED", "UNSAFE_CONTENT", "document.upload.quarantined", "content.unsafe"},
		{"embedded-script", jpegWith(clean, 0xfe, []byte("<script>synthetic()</script>")), media.ErrUnsafe, "QUARANTINED", "UNSAFE_CONTENT", "document.upload.quarantined", "content.unsafe"},
		{"png-as-jpeg", append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{1}, 64)...), media.ErrUnsupported, "FAILED", "UNSUPPORTED_CONTENT", "document.upload.failed", "content.unsupported"},
		{"html-as-jpeg", []byte("<html><script>synthetic()</script></html>"), media.ErrUnsupported, "FAILED", "UNSUPPORTED_CONTENT", "document.upload.failed", "content.unsupported"},
	}
	for _, c := range cases {
		command, created := l.declared(t, l.driver, c.raw)
		if _, err := l.service.VerifyUpload(l.scopeA, command.DocumentID, created.Upload.Token); err != nil {
			t.Fatal(err)
		}
		if _, err := l.service.Upload(t.Context(), l.scopeA, l.driver, command.DocumentID, bytes.NewReader(c.raw)); !errors.Is(err, c.err) {
			t.Fatal("refused content verdict", c.name, err)
		}
		state, revision, reason := l.row(t, command.DocumentID)
		if state != c.state || revision != 3 || reason != c.reason {
			t.Fatal("refused content state", c.name, state, revision, reason)
		}
		code, actor := l.eventReason(t, command.DocumentID, c.action)
		if l.events(t, command.DocumentID, c.action) != 1 || code != c.code || actor != l.driver.AccountID ||
			l.events(t, command.DocumentID, "document.upload.received")+l.events(t, command.DocumentID, "document.upload.stored") != 0 {
			t.Fatal("refusal not audited once with its reason", c.name, code)
		}
		if l.present(t, l.objectOf(l.scopeA, command)) {
			t.Fatal("refused bytes reached the bucket", c.name)
		}
		body := &countingReader{}
		if _, err := l.service.Upload(t.Context(), l.scopeA, l.driver, command.DocumentID, body); !errors.Is(err, app.ErrConflict) || body.reads != 0 {
			t.Fatal("retry after a terminal refusal", c.name, err)
		}
		if replay, err := l.service.Create(t.Context(), l.scopeA, l.driver, command); err != nil || replay.Upload != nil || string(replay.View.Statuses.Storage) != c.state {
			t.Fatal("declaration replay after a terminal refusal", c.name, err)
		}
		if l.served(t, l.service, command.DocumentID) {
			t.Fatal("refused document served", c.name)
		}
	}
	// Integrity refusals keep the session open and write no object.
	command, _ := l.declared(t, l.driver, clean)
	mismatch := bytes.Clone(clean)
	mismatch[len(mismatch)-3] ^= 0x55
	for name, body := range map[string][]byte{"digest": mismatch, "oversized": append(bytes.Clone(clean), 0xd9), "partial": clean[:len(clean)/2]} {
		if _, err := l.service.Upload(t.Context(), l.scopeA, l.driver, command.DocumentID, bytes.NewReader(body)); err == nil {
			t.Fatal("unverified bytes accepted", name)
		}
		if state, revision, reason := l.row(t, command.DocumentID); state != "UPLOADING" || revision != 1 || reason != "" || l.present(t, l.objectOf(l.scopeA, command)) {
			t.Fatal("integrity refusal changed the session or wrote bytes", name, state)
		}
	}
	stored, err := l.service.Upload(t.Context(), l.scopeA, l.driver, command.DocumentID, bytes.NewReader(clean))
	if err != nil || stored.Replayed || stored.View.Statuses.Storage != domain.Stored || stored.View.Revisions.Storage != 3 {
		t.Fatal("clean original not stored through the check", err)
	}
	if l.events(t, command.DocumentID, "document.upload.received") != 1 || l.events(t, command.DocumentID, "document.upload.stored") != 1 || !l.present(t, l.objectOf(l.scopeA, command)) {
		t.Fatal("clean original events or object")
	}
	if _, actor := l.eventReason(t, command.DocumentID, "document.upload.stored"); actor != l.driver.AccountID {
		t.Fatal("stored settlement not attributed to the creating account")
	}
	access, err := l.service.IssueContent(t.Context(), l.scopeA, l.manager, command.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if content, err := l.service.ReadContent(t.Context(), l.scopeA, l.manager, command.DocumentID, access.Token); err != nil || !bytes.Equal(content.Bytes, clean) {
		t.Fatal("checked original not served exactly", err)
	}
}

// BUC-EMP-128 (stored-object check): what storage holds under RECEIVED is
// checked again; partial, substituted, oversized and inadmissible objects
// fail and are removed, unsafe ones are quarantined and kept, none is served,
// and another organization's job or key reaches nothing.
func TestSafetyCheckOverStoredObjectsKeepsUnverifiedBytesInaccessible(t *testing.T) {
	l := newLiveDocuments(t)
	clean := l.raw
	unsafe := jpegWith(clean, 0xe1, []byte("synthetic %PDF-1.7 payload"))
	pdf := append([]byte("%PDF-1.7 synthetic "), bytes.Repeat([]byte{0x20}, 64)...)
	substituted := bytes.Clone(clean)
	substituted[len(substituted)-3] ^= 0x55
	handler := adapters.SafetyHandler{Safety: l.checker}
	run := func(command app.Create, final bool) error {
		t.Helper()
		payload := []byte(`{"version":1,"org_id":"` + l.scopeA.OrgID().String() + `","document_id":"` + command.DocumentID + `"}`)
		attempts := int32(1)
		if final {
			attempts = adapters.SafetyAttempts
		}
		return handler.Handle(t.Context(), jobs.Job{Kind: adapters.SafetyKind, Payload: payload, DedupeKey: adapters.SafetyDedupe(adapters.CheckPurpose, l.scopeA, command.DocumentID), Attempts: attempts, MaxAttempts: adapters.SafetyAttempts})
	}
	for _, c := range []struct {
		name     string
		declared []byte
		stored   []byte
		state    string
		reason   string
		kept     bool
	}{
		{"partial", clean, clean[:len(clean)/2], "FAILED", "INTEGRITY_MISMATCH", false},
		{"substituted", clean, substituted, "FAILED", "INTEGRITY_MISMATCH", false},
		{"oversized", clean, append(bytes.Clone(clean), 0xd9, 0xd9), "FAILED", "INTEGRITY_MISMATCH", false},
		{"inadmissible", pdf, pdf, "FAILED", "UNSUPPORTED_CONTENT", false},
		{"unsafe", unsafe, unsafe, "QUARANTINED", "UNSAFE_CONTENT", true},
		{"clean", clean, clean, "STORED", "", true},
	} {
		command := l.received(t, c.declared)
		object := l.objectOf(l.scopeA, command)
		l.plant(t, object, c.stored)
		if l.served(t, l.service, command.DocumentID) {
			t.Fatal("received bytes served before their check", c.name)
		}
		if err := run(command, false); err != nil {
			t.Fatal("check job", c.name, err)
		}
		state, revision, reason := l.row(t, command.DocumentID)
		if state != c.state || revision != 3 || reason != c.reason || l.present(t, object) != c.kept {
			t.Fatal("stored-object verdict", c.name, state, revision, reason)
		}
		if l.served(t, l.service, command.DocumentID) != (c.state == "STORED") {
			t.Fatal("only a checked original may be served", c.name)
		}
		// A repeated job converges on the settled outcome and writes nothing.
		before := l.count(t, l.scopeA, `SELECT count(*) FROM org_audit_events WHERE target_id=$1`, command.DocumentID)
		if err := run(command, true); err != nil || l.count(t, l.scopeA, `SELECT count(*) FROM org_audit_events WHERE target_id=$1`, command.DocumentID) != before {
			t.Fatal("repeated job wrote again", c.name, err)
		}
		if again, _, _ := l.row(t, command.DocumentID); again != c.state {
			t.Fatal("repeated job changed the outcome", c.name)
		}
	}
	// A missing object is an incomplete check until the final attempt.
	missing := l.received(t, clean)
	if err := run(missing, false); err == nil {
		t.Fatal("missing object acknowledged")
	}
	if state, revision, _ := l.row(t, missing.DocumentID); state != "RECEIVED" || revision != 2 || l.events(t, missing.DocumentID, "document.upload.failed") != 0 {
		t.Fatal("incomplete check wrote an outcome", state)
	}
	if err := run(missing, true); err != nil {
		t.Fatal(err)
	}
	if state, _, reason := l.row(t, missing.DocumentID); state != "FAILED" || reason != "CHECK_ATTEMPTS_EXHAUSTED" {
		t.Fatal("final attempt did not settle exhausted", state, reason)
	}
	if code, _ := l.eventReason(t, missing.DocumentID, "document.upload.failed"); code != "check.exhausted" {
		t.Fatal("exhaustion reason code", code)
	}
	// Cross-organization: organization B's job and key reach nothing of A.
	target := l.received(t, clean)
	objectA := l.objectOf(l.scopeA, target)
	l.plant(t, objectA, clean)
	foreign := l.objectOf(l.scopeB, target)
	l.plant(t, foreign, substituted)
	payload := []byte(`{"version":1,"org_id":"` + l.scopeB.OrgID().String() + `","document_id":"` + target.DocumentID + `"}`)
	if err := handler.Handle(t.Context(), jobs.Job{Kind: adapters.SafetyKind, Payload: payload, DedupeKey: adapters.SafetyDedupe(adapters.CheckPurpose, l.scopeB, target.DocumentID), Attempts: 10, MaxAttempts: 10}); err != nil {
		t.Fatal("foreign job did not converge to nothing", err)
	}
	if _, err := l.checker.Converge(t.Context(), l.scopeB, target.DocumentID, true); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("organization B resolved A's document", err)
	}
	if state, revision, _ := l.row(t, target.DocumentID); state != "RECEIVED" || revision != 2 || !l.present(t, objectA) {
		t.Fatal("foreign job touched A's document or object", state)
	}
	if _, err := l.s3.Get(t.Context(), l.scopeB, objectA); !errors.Is(err, media.ErrInvalid) {
		t.Fatal("organization B's scope addressed A's key", err)
	}
	if err := l.s3.Delete(t.Context(), l.scopeB, objectA); !errors.Is(err, media.ErrInvalid) || !l.present(t, objectA) {
		t.Fatal("organization B's scope removed A's key", err)
	}
	// A's own check reads A's key only: B's substituted copy changes nothing.
	if err := run(target, false); err != nil {
		t.Fatal(err)
	}
	if state, _, _ := l.row(t, target.DocumentID); state != "STORED" || !l.present(t, foreign) {
		t.Fatal("A's check read or removed another organization's key", state)
	}
}

// BUC-EMP-129: concurrent checks of one received document commit one
// outcome and one event, whatever the verdict.
func TestConcurrentSafetyChecksCommitOneOutcome(t *testing.T) {
	l := newLiveDocuments(t)
	substituted := bytes.Clone(l.raw)
	substituted[len(substituted)-3] ^= 0x55
	unsafe := jpegWith(l.raw, 0xfe, []byte("<iframe src=synthetic>"))
	for _, c := range []struct {
		declared, stored []byte
		state            domain.State
		action           string
		kept             bool
	}{
		{l.raw, l.raw, domain.Stored, "document.upload.stored", true},
		{unsafe, unsafe, domain.Quarantined, "document.upload.quarantined", true},
		{l.raw, substituted, domain.StorageFailed, "document.upload.failed", false},
	} {
		command := l.received(t, c.declared)
		object := l.objectOf(l.scopeA, command)
		l.plant(t, object, c.stored)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var mu sync.Mutex
		var failures []error
		for i := range 8 {
			wg.Go(func() {
				<-start
				outcome, err := l.checker.Converge(t.Context(), l.scopeA, command.DocumentID, i%2 == 0)
				if err != nil || outcome.State != c.state {
					mu.Lock()
					failures = append(failures, err)
					mu.Unlock()
				}
			})
		}
		close(start)
		wg.Wait()
		if len(failures) != 0 {
			t.Fatal("concurrent checks disagreed", c.state, failures)
		}
		if state, revision, _ := l.row(t, command.DocumentID); domain.State(state) != c.state || revision != 3 || l.events(t, command.DocumentID, c.action) != 1 ||
			l.count(t, l.scopeA, `SELECT count(*) FROM org_audit_events WHERE target_id=$1`, command.DocumentID) != 3 || l.present(t, object) != c.kept {
			t.Fatal("concurrent checks committed more than one outcome", c.state, state, revision)
		}
	}
}

// faultyObjects injects storage failures in front of the real bucket.
type faultyObjects struct {
	inner                    *mediapg.S3
	mu                       sync.Mutex
	getFaults, deleteFaults  int // remaining faults; negative fails forever
	getCalls, deleteAttempts int
}

func (f *faultyObjects) fault(remaining *int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if *remaining == 0 {
		return false
	}
	if *remaining > 0 {
		*remaining--
	}
	return true
}
func (f *faultyObjects) Get(ctx context.Context, scope kernel.OrgScope, object media.Object) ([]byte, error) {
	f.mu.Lock()
	f.getCalls++
	f.mu.Unlock()
	if f.fault(&f.getFaults) {
		return nil, media.ErrUnavailable
	}
	return f.inner.Get(ctx, scope, object)
}
func (f *faultyObjects) Delete(ctx context.Context, scope kernel.OrgScope, object media.Object) error {
	f.mu.Lock()
	f.deleteAttempts++
	f.mu.Unlock()
	if f.fault(&f.deleteFaults) {
		return media.ErrUnavailable
	}
	return f.inner.Delete(ctx, scope, object)
}

// restart composes a fresh process over the same database and bucket.
func (l *liveDocuments) restart(t *testing.T, objects app.SafetyObjects) (app.Service, app.Checker) {
	t.Helper()
	pool := l.db.RuntimePool(t, 4)
	catalog, err := audit.NewCatalog(append(adapters.AuditDefinitions(), mediapg.AuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	auditStore := auditpg.NewStore(pool, catalog)
	store := &adapters.Store{Pool: pool, Authority: l.authority, Audit: auditStore}
	s3, err := mediapg.NewS3(l.config)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte(strings.Repeat("synthetic-", 5))
	contentTokens, err := mediapg.NewTokens("https://api.example.invalid", secret)
	if err != nil {
		t.Fatal(err)
	}
	uploads, err := adapters.NewUploadTokens("https://api.example.invalid", secret)
	if err != nil {
		t.Fatal(err)
	}
	originals, err := mediaapp.NewService(mediaapp.Config{Enabled: true, Authority: adapters.MediaAuthority{Pool: pool, Authority: l.authority}, Objects: s3,
		Inspector: mediapg.JPEGInspector{}, Tokens: contentTokens, Audit: mediapg.AccessAudit{Runner: auditStore}, Now: l.tick, NewID: jobs.NewUUIDv4})
	if err != nil {
		t.Fatal(err)
	}
	if objects == nil {
		objects = s3
	}
	checker := app.Checker{Repository: store, Objects: objects, Inspector: mediapg.JPEGInspector{}, NewID: jobs.NewUUIDv4}
	return app.Service{Enabled: true, Repository: store, Media: originals, Safety: checker, Tokens: uploads, Now: l.tick, NewID: jobs.NewUUIDv4}, checker
}

func (l *liveDocuments) dispatcher(t *testing.T, safety app.Safety) *jobs.Dispatcher {
	t.Helper()
	role := pgx.Identifier{l.db.RuntimeRole}.Sanitize()
	if _, err := l.db.Admin.Exec(t.Context(), "GRANT UPDATE ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
	return &jobs.Dispatcher{Queue: jobs.NewQueue(l.db.RuntimePool(t, 2)), Handlers: map[string]jobs.Handler{adapters.SafetyKind: adapters.SafetyHandler{Safety: safety}},
		WorkerID: "doc014-safety-test", Kind: adapters.SafetyKind, LeaseTTL: time.Minute, RetryDelay: time.Minute}
}

// due makes exactly one queued job claimable now and returns its state.
func (l *liveDocuments) due(t *testing.T, dedupe string) {
	t.Helper()
	if tag, err := l.db.Admin.Exec(t.Context(), `UPDATE job_queue SET not_before=now()-interval '1 second' WHERE dedupe_key=$1 AND status='queued'`, dedupe); err != nil || tag.RowsAffected() != 1 {
		t.Fatal("queued safety job absent", dedupe, err)
	}
}

func (l *liveDocuments) job(t *testing.T, dedupe string) (string, int32, string) {
	t.Helper()
	var status, lastError string
	var attempts int32
	if err := l.db.Admin.QueryRow(t.Context(), `SELECT status,attempts,last_error FROM job_queue WHERE dedupe_key=$1`, dedupe).Scan(&status, &attempts, &lastError); err != nil {
		t.Fatal(err)
	}
	return status, attempts, lastError
}

// BUC-EMP-129: a storage failure mid-check leaves RECEIVED with nothing
// written; a freshly started process over the same database and bucket
// serves nothing until the queued job, claimed by the real dispatcher,
// stores it; exhaustion and failed removals stay truthful and retriable.
func TestStorageFailureMidCheckAndRestartServeOnlyVerifiedBytes(t *testing.T) {
	l := newLiveDocuments(t)
	broken := &faultyObjects{inner: l.s3, getFaults: -1}
	interrupted := l.service
	interrupted.Safety = app.Checker{Repository: l.store, Objects: broken, Inspector: mediapg.JPEGInspector{}, NewID: jobs.NewUUIDv4}
	command, _ := l.declared(t, l.driver, l.raw)
	pending, err := interrupted.Upload(t.Context(), l.scopeA, l.driver, command.DocumentID, bytes.NewReader(l.raw))
	if err != nil || pending.View.Statuses.Storage != domain.Received || pending.View.Revisions.Storage != 2 || broken.getCalls != 1 {
		t.Fatal("interrupted check must answer the received document", err)
	}
	object := l.objectOf(l.scopeA, command)
	if !l.present(t, object) || l.events(t, command.DocumentID, "document.upload.stored") != 0 || l.events(t, command.DocumentID, "document.upload.received") != 1 {
		t.Fatal("interrupted check wrote an outcome or lost the verified bytes")
	}
	// Restart: nothing in memory survives; the received bytes stay unserved.
	restarted, checker := l.restart(t, nil)
	if l.served(t, restarted, command.DocumentID) {
		t.Fatal("unverified bytes served after a restart")
	}
	if _, err := restarted.ReadContent(t.Context(), l.scopeA, l.manager, command.DocumentID, "v1.synthetic-forged"); !errors.Is(err, media.ErrNotFound) {
		t.Fatal("received bytes read after a restart", err)
	}
	// The queued check, claimed through the real queue, converges it.
	check := adapters.SafetyDedupe(adapters.CheckPurpose, l.scopeA, command.DocumentID)
	failing := l.dispatcher(t, app.Checker{Repository: l.store, Objects: broken, Inspector: mediapg.JPEGInspector{}, NewID: jobs.NewUUIDv4})
	l.due(t, check)
	if found, err := failing.RunOnce(t.Context()); err != nil || !found {
		t.Fatal("dispatcher did not claim the check", err)
	}
	if status, attempts, lastError := l.job(t, check); status != "queued" || attempts != 1 || lastError != app.ErrRetry.Error() {
		t.Fatal("storage failure did not leave a retriable job", status, attempts, lastError)
	}
	if state, revision, _ := l.row(t, command.DocumentID); state != "RECEIVED" || revision != 2 {
		t.Fatal("storage failure mid-check changed the document", state)
	}
	worker := l.dispatcher(t, checker)
	l.due(t, check)
	if found, err := worker.RunOnce(t.Context()); err != nil || !found {
		t.Fatal("dispatcher did not claim the retried check", err)
	}
	if status, attempts, _ := l.job(t, check); status != "done" || attempts != 2 {
		t.Fatal("retried check not completed", status, attempts)
	}
	if state, revision, _ := l.row(t, command.DocumentID); state != "STORED" || revision != 3 || l.events(t, command.DocumentID, "document.upload.stored") != 1 {
		t.Fatal("job did not store the verified original", state)
	}
	access, err := restarted.IssueContent(t.Context(), l.scopeA, l.manager, command.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if content, err := restarted.ReadContent(t.Context(), l.scopeA, l.manager, command.DocumentID, access.Token); err != nil || !bytes.Equal(content.Bytes, l.raw) {
		t.Fatal("restarted process did not serve the checked original", err)
	}
	// An upload retry while RECEIVED drives the check without a transfer.
	retried := l.received(t, l.raw)
	l.plant(t, l.objectOf(l.scopeA, retried), l.raw)
	body := &countingReader{}
	if again, err := restarted.Upload(t.Context(), l.scopeA, l.driver, retried.DocumentID, body); err != nil || !again.Replayed || again.View.Statuses.Storage != domain.Stored || body.reads != 0 {
		t.Fatal("upload retry did not converge the received document", err)
	}
	// Exhaustion settles FAILED and removes the object.
	exhausted := l.received(t, l.raw)
	exhaustedObject := l.objectOf(l.scopeA, exhausted)
	l.plant(t, exhaustedObject, l.raw)
	if outcome, err := (app.Checker{Repository: l.store, Objects: &faultyObjects{inner: l.s3, getFaults: -1}, Inspector: mediapg.JPEGInspector{}, NewID: jobs.NewUUIDv4}).Converge(t.Context(), l.scopeA, exhausted.DocumentID, true); err != nil || outcome.Reason != app.CheckExhausted {
		t.Fatal("final attempt", outcome, err)
	}
	if state, _, reason := l.row(t, exhausted.DocumentID); state != "FAILED" || reason != "CHECK_ATTEMPTS_EXHAUSTED" || l.present(t, exhaustedObject) {
		t.Fatal("exhausted check state or object", state, reason)
	}
	// A failed removal keeps the truthful FAILED state and is retried.
	substituted := bytes.Clone(l.raw)
	substituted[len(substituted)-3] ^= 0x55
	removal := l.received(t, l.raw)
	removalObject := l.objectOf(l.scopeA, removal)
	l.plant(t, removalObject, substituted)
	stuck := &faultyObjects{inner: l.s3, deleteFaults: 1}
	sticky := app.Checker{Repository: l.store, Objects: stuck, Inspector: mediapg.JPEGInspector{}, NewID: jobs.NewUUIDv4}
	if outcome, err := sticky.Converge(t.Context(), l.scopeA, removal.DocumentID, false); !errors.Is(err, app.ErrRetry) || outcome.State != domain.StorageFailed {
		t.Fatal("failed removal", outcome, err)
	}
	if state, _, reason := l.row(t, removal.DocumentID); state != "FAILED" || reason != "INTEGRITY_MISMATCH" || !l.present(t, removalObject) {
		t.Fatal("failed removal state", state, reason)
	}
	if outcome, err := sticky.Converge(t.Context(), l.scopeA, removal.DocumentID, false); err != nil || outcome.State != domain.StorageFailed || l.present(t, removalObject) ||
		l.events(t, removal.DocumentID, "document.upload.failed") != 1 || stuck.deleteAttempts != 2 {
		t.Fatal("retried removal", outcome, err)
	}
}

func (l *liveDocuments) backdate(t *testing.T, id string, age time.Duration) {
	t.Helper()
	tx, err := l.db.Admin.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	// The guard keeps created_at immutable; only this fixture ages a row.
	if _, err := tx.Exec(t.Context(), `SET LOCAL session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `UPDATE org_documents SET created_at=clock_timestamp()-make_interval(secs=>$2) WHERE id=$1`, id, age.Seconds()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// B-BR-EMP-254: a session closes only after its window and grace, by the
// job queued at declaration, and any object under its key is removed.
func TestAbandonedUploadSessionsCloseOnlyAfterTheirWindow(t *testing.T) {
	l := newLiveDocuments(t)
	command, created := l.declared(t, l.driver, l.raw)
	expiry := adapters.SafetyDedupe(adapters.ExpiryPurpose, l.scopeA, command.DocumentID)
	var due, declared time.Time
	if err := l.db.Admin.QueryRow(t.Context(), `SELECT j.not_before,d.created_at FROM job_queue j JOIN org_documents d ON d.id=$2 WHERE j.dedupe_key=$1 AND j.kind='org-document-safety' AND j.max_attempts=10
		AND j.payload=jsonb_build_object('version',1,'org_id',d.org_id::text,'document_id',d.id::text)`, expiry, command.DocumentID).Scan(&due, &declared); err != nil {
		t.Fatal("declaration did not queue its reference-only closure job", err)
	}
	if drift := due.Sub(declared.Add(app.UploadWindow + app.UploadGrace)); drift < -time.Minute || drift > time.Minute {
		t.Fatal("closure job not due at the session close plus grace", drift)
	}
	if created.Upload == nil || created.Upload.ExpiresAt.After(declared.Add(app.UploadWindow)) {
		t.Fatal("declaration capability")
	}
	if _, err := l.checker.Converge(t.Context(), l.scopeA, command.DocumentID, true); !errors.Is(err, app.ErrNotDue) {
		t.Fatal("open session closed", err)
	}
	abandon := `UPDATE org_documents SET storage_state='FAILED',storage_revision=2,storage_reason='UPLOAD_ABANDONED' WHERE org_id=$1 AND id=$2`
	if err := l.scoped(t, l.scopeA, abandon, l.scopeA.OrgID().String(), command.DocumentID); sqlState(err) != "23514" {
		t.Fatal("database closed an open session", err)
	}
	// One minute before the grace ends the session is closed to new
	// capabilities but still not abandonable, also directly in the database.
	l.backdate(t, command.DocumentID, app.UploadWindow+app.UploadGrace-time.Minute)
	if replay, err := l.service.Create(t.Context(), l.scopeA, l.driver, command); err != nil || !replay.Replayed || replay.Upload != nil {
		t.Fatal("capability minted after the session close", err)
	}
	if _, err := l.checker.Converge(t.Context(), l.scopeA, command.DocumentID, true); !errors.Is(err, app.ErrNotDue) {
		t.Fatal("session abandoned within its grace", err)
	}
	if err := l.scoped(t, l.scopeA, abandon, l.scopeA.OrgID().String(), command.DocumentID); sqlState(err) != "23514" {
		t.Fatal("database abandoned a session within its grace", err)
	}
	// A crashed transfer left an object; the closure removes it.
	object := l.objectOf(l.scopeA, command)
	l.plant(t, object, l.raw)
	l.backdate(t, command.DocumentID, app.UploadWindow+app.UploadGrace+time.Minute)
	worker := l.dispatcher(t, l.checker)
	l.due(t, expiry)
	if found, err := worker.RunOnce(t.Context()); err != nil || !found {
		t.Fatal("closure job not claimed", err)
	}
	if status, _, _ := l.job(t, expiry); status != "done" {
		t.Fatal("closure job not completed", status)
	}
	state, revision, reason := l.row(t, command.DocumentID)
	code, actor := l.eventReason(t, command.DocumentID, "document.upload.failed")
	if state != "FAILED" || revision != 2 || reason != "UPLOAD_ABANDONED" || code != "upload.abandoned" || actor != l.driver.AccountID || l.present(t, object) {
		t.Fatal("abandoned session not closed with its object removed", state, revision, reason, code)
	}
	if again, err := l.checker.Converge(t.Context(), l.scopeA, command.DocumentID, true); err != nil || again.Reason != app.UploadAbandoned || l.events(t, command.DocumentID, "document.upload.failed") != 1 {
		t.Fatal("repeated closure", err)
	}
	if _, err := l.service.Upload(t.Context(), l.scopeA, l.driver, command.DocumentID, bytes.NewReader(l.raw)); !errors.Is(err, app.ErrConflict) {
		t.Fatal("bytes accepted after the session closed", err)
	}
}

// B-BR-EMP-252/253: the database fixes one closed reason per transition,
// equals the Go reason table and keeps forced row-level security.
func TestSafetySchemaGuardsReasonsAndScope(t *testing.T) {
	f := newDocFixture(t)
	org := f.scopeA.OrgID().String()
	ids := make([]string, 3)
	for i := range ids {
		command := declare(t, syntheticJPEG(t))
		if _, _, err := f.store.Create(t.Context(), f.scopeA, f.driver, command); err != nil {
			t.Fatal(err)
		}
		ids[i] = command.DocumentID
	}
	// The SQL reason table equals the Go reason table for every combination.
	states := []string{"UPLOADING", "RECEIVED", "STORED", "QUARANTINED", "FAILED", "REPLACED", "DELETED"}
	reasons := append(app.Reasons(), "UNKNOWN")
	for _, from := range states {
		for _, to := range states {
			for _, reason := range reasons {
				var allowed bool
				if err := f.db.Admin.QueryRow(t.Context(), `SELECT org_document_reason_allowed($1,$2,$3)`, from, to, string(reason)).Scan(&allowed); err != nil {
					t.Fatal(err)
				}
				start, end, ok := reason.Transition()
				if allowed != (ok && string(start) == from && string(end) == to) {
					t.Fatal("database reason table differs from the Go table", from, to, reason)
				}
			}
		}
	}
	// step probes one statement and always rolls back; commit keeps it.
	step := func(id, set string) error {
		return f.scoped(t, f.scopeA, `UPDATE org_documents SET `+set+` WHERE org_id=$1 AND id=$2`, org, id)
	}
	commit := func(id, set string) error {
		return database.WithOrgTx(t.Context(), f.db.Runtime, f.scopeA, func(tx pgx.Tx) error {
			_, err := tx.Exec(t.Context(), `UPDATE org_documents SET `+set+` WHERE org_id=$1 AND id=$2`, org, id)
			return err
		})
	}
	for name, probe := range map[string]string{
		"failed-without-reason":  `storage_state='FAILED',storage_revision=2`,
		"failed-unsafe":          `storage_state='FAILED',storage_revision=2,storage_reason='UNSAFE_CONTENT'`,
		"reason-without-step":    `storage_reason='INTEGRITY_MISMATCH'`,
		"received-with-reason":   `storage_state='RECEIVED',storage_revision=2,storage_reason='INTEGRITY_MISMATCH'`,
		"unknown-reason":         `storage_state='FAILED',storage_revision=2,storage_reason='LOST'`,
		"abandoned-while-open":   `storage_state='FAILED',storage_revision=2,storage_reason='UPLOAD_ABANDONED'`,
		"lowercase-known-reason": `storage_state='FAILED',storage_revision=2,storage_reason='upload_abandoned'`,
	} {
		if err := step(ids[0], probe); sqlState(err) != "23514" {
			t.Fatal("database admitted", name, err)
		}
	}
	if err := commit(ids[0], `storage_state='RECEIVED',storage_revision=2`); err != nil {
		t.Fatal(err)
	}
	for name, probe := range map[string]string{
		"quarantined-without-reason": `storage_state='QUARANTINED',storage_revision=3`,
		"quarantined-integrity":      `storage_state='QUARANTINED',storage_revision=3,storage_reason='INTEGRITY_MISMATCH'`,
		"failed-unsafe":              `storage_state='FAILED',storage_revision=3,storage_reason='UNSAFE_CONTENT'`,
		"failed-abandoned":           `storage_state='FAILED',storage_revision=3,storage_reason='UPLOAD_ABANDONED'`,
		"stored-with-reason":         `storage_state='STORED',storage_revision=3,storage_reason='UNSAFE_CONTENT'`,
	} {
		if err := step(ids[0], probe); sqlState(err) != "23514" {
			t.Fatal("database admitted", name, err)
		}
	}
	if err := commit(ids[0], `storage_state='QUARANTINED',storage_revision=3,storage_reason='UNSAFE_CONTENT'`); err != nil {
		t.Fatal("quarantine with its reason refused", err)
	}
	for name, probe := range map[string]string{
		"change": `storage_reason='INTEGRITY_MISMATCH'`,
		"clear":  `storage_reason=NULL`,
	} {
		if err := step(ids[0], probe); sqlState(err) != "23514" {
			t.Fatal("database changed a fixed reason", name, err)
		}
	}
	if err := commit(ids[0], `storage_state='DELETED',storage_revision=4`); err != nil {
		t.Fatal("quarantined document could not reach DELETED keeping its reason", err)
	}
	if err := commit(ids[1], `storage_state='RECEIVED',storage_revision=2`); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"UNSUPPORTED_CONTENT", "INTEGRITY_MISMATCH", "CHECK_ATTEMPTS_EXHAUSTED"} {
		err := database.WithOrgTx(t.Context(), f.db.Runtime, f.scopeA, func(tx pgx.Tx) error {
			if _, err := tx.Exec(t.Context(), `UPDATE org_documents SET storage_state='FAILED',storage_revision=3,storage_reason=$3 WHERE org_id=$1 AND id=$2`, org, ids[1], reason); err != nil {
				return err
			}
			return errors.New("rollback")
		})
		if err == nil || err.Error() != "rollback" {
			t.Fatal("failure reason refused", reason, err)
		}
	}
	insert := `INSERT INTO org_documents(org_id,id,kind,data_mode,operation_id,created_by,original_sha256,original_bytes,original_type,storage_state,storage_revision,extraction_state,extraction_revision,structure_state,structure_revision,conference_state,conference_revision,reconciliation_state,fiscal_state,fiscal_revision,storage_reason)
		VALUES($1,$2,'RECEIPT','SYNTHETIC',$3,$4,repeat('a',64),1,'image/jpeg','UPLOADING',1,'NONE',1,'NOT_APPLICABLE',1,'UNCONFERRED',1,'UNALLOCATED','NOT_CONSULTED',1,'UPLOAD_ABANDONED')`
	if err := f.scoped(t, f.scopeA, insert, org, newID(t), newID(t), f.driver.MembershipID); sqlState(err) != "23514" {
		t.Fatal("document created with a reason", err)
	}
	// Forced RLS: without the organization setting nothing is visible or
	// settled; organization B's scope cannot load or settle A's document.
	if tag, err := f.db.Runtime.Exec(t.Context(), `UPDATE org_documents SET storage_state='STORED',storage_revision=3 WHERE id=$1`, ids[1]); err != nil || tag.RowsAffected() != 0 {
		t.Fatal("unscoped session settled a document", err)
	}
	var visible int
	if err := f.db.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_documents WHERE storage_reason IS NOT NULL`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("unscoped session saw reasons", visible, err)
	}
	if _, err := f.store.Subject(t.Context(), f.scopeB, ids[1]); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("organization B loaded A's subject", err)
	}
	if outcome, err := f.store.Settle(t.Context(), f.scopeB, ids[1], domain.Received, 2, "", newID(t)); !errors.Is(err, app.ErrNotFound) || outcome.State != "" {
		t.Fatal("organization B settled A's document", outcome, err)
	}
	subject, err := f.store.Subject(t.Context(), f.scopeA, ids[1])
	if err != nil || subject.State != domain.Received || subject.Revision != 2 || subject.Abandoned || subject.Object.DocumentID != ids[1] || subject.Object.Ready ||
		subject.Object.OrgID != org || subject.Object.MIME != app.MediaTypeJPEG || subject.Object.Validate(f.scopeA, ids[1]) != nil {
		t.Fatal("subject projection", err)
	}
	if _, err := f.store.Settle(t.Context(), f.scopeA, ids[2], domain.Uploading, 1, app.UploadAbandoned, newID(t)); err != nil {
		t.Fatal(err)
	}
	if state := f.count(t, f.scopeA, `SELECT count(*) FROM org_documents WHERE id=$1 AND storage_state='UPLOADING'`, ids[2]); state != 1 {
		t.Fatal("store abandoned an open session")
	}
	var forced bool
	if err := f.db.Admin.QueryRow(t.Context(), `SELECT relforcerowsecurity FROM pg_class WHERE relname='org_documents'`).Scan(&forced); err != nil || !forced {
		t.Fatal("forced row-level security lost", err)
	}
}
