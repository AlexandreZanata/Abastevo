//go:build integration

package adapters_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
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
	roles "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	mediaapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type liveDocuments struct {
	*docFixture
	s3      *mediapg.S3
	config  mediapg.StorageConfig
	service app.Service
	raw     []byte
	clock   sync.Mutex
	now     time.Time
}

func (l *liveDocuments) tick() time.Time {
	l.clock.Lock()
	defer l.clock.Unlock()
	return l.now
}
func (l *liveDocuments) advance(d time.Duration) {
	l.clock.Lock()
	defer l.clock.Unlock()
	l.now = l.now.Add(d)
}

func newLiveDocuments(t *testing.T) *liveDocuments {
	t.Helper()
	required := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("explicit disposable %s required", name)
		}
		return value
	}
	l := &liveDocuments{docFixture: newDocFixture(t), raw: syntheticJPEG(t), now: time.Now().UTC()}
	pool := l.db.RuntimePool(t, 4)
	catalog, err := audit.NewCatalog(append(adapters.AuditDefinitions(), mediapg.AuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	l.auditStore = auditpg.NewStore(pool, catalog)
	l.store.Pool, l.store.Audit = pool, l.auditStore
	l.config = mediapg.StorageConfig{Endpoint: required("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: required("ANPFUEL_TEST_STORAGE_BUCKET"),
		Region: required("ANPFUEL_TEST_STORAGE_REGION"), Credentials: storage.Credentials{AccessKeyID: required("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: required("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")}, AllowLoopback: true}
	if l.s3, err = mediapg.NewS3(l.config); err != nil {
		t.Fatal("invalid disposable storage configuration")
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
	originals, err := mediaapp.NewService(mediaapp.Config{Enabled: true, Authority: adapters.MediaAuthority{Pool: pool, Authority: l.authority}, Objects: l.s3,
		Inspector: mediapg.JPEGInspector{}, Tokens: contentTokens, Audit: mediapg.AccessAudit{Runner: l.auditStore}, Now: l.tick, NewID: jobs.NewUUIDv4})
	if err != nil {
		t.Fatal(err)
	}
	l.service = app.Service{Enabled: true, Repository: l.store, Media: originals, Tokens: uploads, Now: l.tick, NewID: jobs.NewUUIDv4}
	return l
}

func (l *liveDocuments) storage(t *testing.T, scope kernel.OrgScope, id string) (string, int64) {
	t.Helper()
	var state string
	var revision int64
	if err := l.db.Admin.QueryRow(t.Context(), `SELECT storage_state,storage_revision FROM org_documents WHERE org_id=$1 AND id=$2`, scope.OrgID().String(), id).Scan(&state, &revision); err != nil {
		t.Fatal(err)
	}
	return state, revision
}

func (l *liveDocuments) declared(t *testing.T, actor app.Actor, raw []byte) (app.Create, app.Created) {
	t.Helper()
	command := declare(t, raw)
	created, err := l.service.Create(t.Context(), l.scopeA, actor, command)
	if err != nil || created.Upload == nil {
		t.Fatal("declaration", err)
	}
	return command, created
}

type failingReader struct {
	data []byte
	sent bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, errors.New("synthetic interrupted transfer")
	}
	r.sent = true
	return copy(p, r.data), nil
}

func TestVerifiedUploadStoresOnlyTheDeclaredOriginalAndKeepsRefusalsPending(t *testing.T) {
	l := newLiveDocuments(t)
	command, created := l.declared(t, l.driver, l.raw)
	actor, err := l.service.VerifyUpload(l.scopeA, command.DocumentID, created.Upload.Token)
	if err != nil || actor != l.driver {
		t.Fatal("issued upload capability does not bind the creator", err)
	}
	for _, wrong := range []struct {
		scope kernel.OrgScope
		id    string
	}{{l.scopeB, command.DocumentID}, {l.scopeA, newID(t)}} {
		if _, err := l.service.VerifyUpload(wrong.scope, wrong.id, created.Upload.Token); !errors.Is(err, app.ErrNotFound) {
			t.Fatal("upload capability crossed organization or document", err)
		}
	}
	mismatch := bytes.Clone(l.raw)
	mismatch[len(mismatch)-3] ^= 0x55
	stored := `SELECT count(*) FROM org_audit_events WHERE action='document.upload.stored'`
	for name, body := range map[string]io.Reader{
		"digest-mismatch": bytes.NewReader(mismatch),
		"oversized":       bytes.NewReader(append(bytes.Clone(l.raw), 0xd9)),
		"truncated":       bytes.NewReader(l.raw[:len(l.raw)-1]),
		"interrupted":     &failingReader{data: l.raw[:len(l.raw)/2]},
		"empty":           bytes.NewReader(nil),
	} {
		if _, err := l.service.Upload(t.Context(), l.scopeA, l.driver, command.DocumentID, body); err == nil {
			t.Fatal("unverified bytes accepted", name)
		}
		if state, revision := l.storage(t, l.scopeA, command.DocumentID); state != "UPLOADING" || revision != 1 {
			t.Fatal("unverified bytes moved storage past UPLOADING", name, state)
		}
	}
	if _, err := l.service.Upload(t.Context(), l.scopeA, l.manager, command.DocumentID, bytes.NewReader(l.raw)); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("another member uploaded into the creator's session", err)
	}
	if _, err := l.service.Upload(t.Context(), l.scopeB, l.foreign, command.DocumentID, bytes.NewReader(l.raw)); err == nil {
		t.Fatal("foreign organization uploaded into the session")
	}
	// Matching digest and size but content that is not a JPEG: the declared
	// type is decided by inspection, so the document stays pending.
	polyglot := append([]byte("%PDF-1.7 synthetic /JavaScript "), bytes.Repeat([]byte{0x20}, 64)...)
	wrongType, wrongCreated := l.declared(t, l.driver, polyglot)
	if _, err := l.service.Upload(t.Context(), l.scopeA, l.driver, wrongType.DocumentID, bytes.NewReader(polyglot)); !errors.Is(err, media.ErrUnsupported) {
		t.Fatal("wrong content type accepted", err)
	}
	if state, _ := l.storage(t, l.scopeA, wrongType.DocumentID); state != "UPLOADING" || wrongCreated.Upload == nil {
		t.Fatal("wrong-type bytes left pending state")
	}
	if l.count(t, l.scopeA, stored) != 0 {
		t.Fatal("refused transfer recorded a stored event")
	}
	done, err := l.service.Upload(t.Context(), l.scopeA, l.driver, command.DocumentID, bytes.NewReader(l.raw))
	if err != nil || done.Replayed || done.View.Statuses.Storage != domain.Stored || done.View.Revisions.Storage != 3 {
		t.Fatal("verified upload not stored", err)
	}
	object := media.Object{OrgID: l.scopeA.OrgID().String(), DocumentID: command.DocumentID, SHA256: command.Original.SHA256, MIME: app.MediaTypeJPEG, Bytes: command.Original.Bytes, Revision: 3, Synthetic: true}
	key, err := object.Key(l.scopeA)
	if err != nil || !strings.HasPrefix(key, "org/"+l.scopeA.OrgID().String()+"/doc/"+command.DocumentID+"/") {
		t.Fatal("storage key escaped the organization prefix", err)
	}
	got, err := l.s3.Get(t.Context(), l.scopeA, object)
	if err != nil || !bytes.Equal(got, l.raw) {
		t.Fatal("stored original differs from the verified snapshot", err)
	}
	foreign := object
	foreign.OrgID = l.scopeB.OrgID().String()
	if _, err := l.s3.Get(t.Context(), l.scopeB, foreign); err == nil {
		t.Fatal("original visible under another organization prefix")
	}
	anonymous, err := storage.PresignGET(storage.PresignInput{Endpoint: l.config.Endpoint, Bucket: l.config.Bucket, Region: l.config.Region, Namespace: "org/", Key: key, TTL: time.Minute}, storage.Credentials{AccessKeyID: "anonymous-synthetic", SecretAccessKey: "anonymous-synthetic-secret"})
	if err == nil {
		if response, err := http.Get(anonymous.URL); err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				t.Fatal("private original readable without the storage credential")
			}
		}
	}
	replayBody := &countingReader{}
	again, err := l.service.Upload(t.Context(), l.scopeA, l.driver, command.DocumentID, replayBody)
	if err != nil || !again.Replayed || replayBody.reads != 0 || l.count(t, l.scopeA, stored) != 1 {
		t.Fatal("retried upload created another record or read bytes", err)
	}
	if replay, err := l.service.Create(t.Context(), l.scopeA, l.driver, command); err != nil || !replay.Replayed || replay.Upload != nil || replay.View.Statuses.Storage != domain.Stored {
		t.Fatal("declaration replay after STORED", err)
	}
	if l.count(t, l.scopeA, `SELECT count(*) FROM org_documents`) != 2 || l.count(t, l.scopeA, `SELECT count(*) FROM org_audit_events WHERE action='document.upload.created'`) != 2 {
		t.Fatal("upload retries created other records")
	}
}

type countingReader struct{ reads int }

func (r *countingReader) Read([]byte) (int, error) { r.reads++; return 0, io.EOF }

func (l *liveDocuments) storedDocument(t *testing.T, actor app.Actor) app.Create {
	t.Helper()
	command, created := l.declared(t, actor, l.raw)
	if _, err := l.service.VerifyUpload(l.scopeA, command.DocumentID, created.Upload.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := l.service.Upload(t.Context(), l.scopeA, actor, command.DocumentID, bytes.NewReader(l.raw)); err != nil {
		t.Fatal(err)
	}
	return command
}

func (l *liveDocuments) access(t *testing.T, stage string) int {
	t.Helper()
	return l.count(t, l.scopeA, `SELECT count(*) FROM org_sensitive_access_events WHERE category='DOCUMENT_CONTENT' AND stage=$1`, stage)
}

func TestAuditedContentIsMemberBoundShortLivedAndFailsClosed(t *testing.T) {
	l := newLiveDocuments(t)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	log.SetOutput(&logs)
	t.Cleanup(func() { slog.SetDefault(previous); log.SetOutput(os.Stderr) })

	command := l.storedDocument(t, l.driver)
	pending, _ := l.declared(t, l.driver, l.raw)
	var secrets []string
	var failures []string
	record := func(err error) {
		if err != nil {
			failures = append(failures, err.Error())
		}
	}
	// FLEET_MANAGER covers the whole organization; the DRIVER only its own.
	for _, actor := range []app.Actor{l.manager, l.driver} {
		access, err := l.service.IssueContent(t.Context(), l.scopeA, actor, command.DocumentID)
		if err != nil || access.Token == "" || access.ExpiresAt.Sub(l.tick()) > 5*time.Minute {
			t.Fatal("permitted member denied a short capability", err)
		}
		secrets = append(secrets, access.Token)
		content, err := l.service.ReadContent(t.Context(), l.scopeA, actor, command.DocumentID, access.Token)
		if err != nil || !bytes.Equal(content.Bytes, l.raw) || content.MIME != app.MediaTypeJPEG {
			t.Fatal("permitted member could not read the exact original", err)
		}
	}
	if l.access(t, "ISSUED") != 2 || l.access(t, "SERVED") != 2 {
		t.Fatal("permitted reads not recorded exactly once each")
	}
	other := l.storedDocument(t, l.manager)
	issued, served := l.access(t, "ISSUED"), l.access(t, "SERVED")
	// A DRIVER (S) on another member's document and a SUPERVISOR (no content
	// permission) are FORBIDDEN without any record.
	for _, refused := range []struct {
		actor app.Actor
		id    string
	}{{l.driver, other.DocumentID}, {l.supervisor, command.DocumentID}} {
		_, err := l.service.IssueContent(t.Context(), l.scopeA, refused.actor, refused.id)
		record(err)
		if !errors.Is(err, media.ErrForbidden) {
			t.Fatal("member without covering content permission issued a capability", err)
		}
	}
	// Foreign organization, non-member and pending documents are NOT_FOUND.
	for _, refused := range []struct {
		scope kernel.OrgScope
		actor app.Actor
		id    string
	}{{l.scopeB, l.foreign, command.DocumentID}, {l.scopeA, l.foreign, command.DocumentID}, {l.scopeA, l.manager, pending.DocumentID}, {l.scopeA, l.manager, newID(t)}} {
		_, err := l.service.IssueContent(t.Context(), refused.scope, refused.actor, refused.id)
		record(err)
		if !errors.Is(err, media.ErrNotFound) {
			t.Fatal("foreign, non-member or pending document issued a capability", err)
		}
	}
	managerAccess, err := l.service.IssueContent(t.Context(), l.scopeA, l.manager, command.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	secrets = append(secrets, managerAccess.Token)
	issued++
	// Another member's, another organization's or another document's use of
	// the capability serves nothing.
	for _, misuse := range []struct {
		scope kernel.OrgScope
		actor app.Actor
		id    string
	}{{l.scopeA, l.driver, command.DocumentID}, {l.scopeA, l.approver, command.DocumentID}, {l.scopeB, l.foreign, command.DocumentID}, {l.scopeA, l.manager, other.DocumentID}} {
		_, err := l.service.ReadContent(t.Context(), misuse.scope, misuse.actor, misuse.id, managerAccess.Token)
		record(err)
		if err == nil {
			t.Fatal("capability served outside its member and document binding")
		}
	}
	// The five-minute deadline is exact; the expired capability serves nothing.
	l.advance(5 * time.Minute)
	_, err = l.service.ReadContent(t.Context(), l.scopeA, l.manager, command.DocumentID, managerAccess.Token)
	record(err)
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatal("expired capability served content", err)
	}
	if l.access(t, "ISSUED") != issued || l.access(t, "SERVED") != served {
		t.Fatal("refused or expired access wrote a record")
	}
	// Unavailable audit: no capability is minted and no byte is served.
	fresh, err := l.service.IssueContent(t.Context(), l.scopeA, l.manager, command.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	secrets = append(secrets, fresh.Token)
	issued++
	role := pgx.Identifier{l.db.RuntimeRole}.Sanitize()
	if _, err := l.db.Admin.Exec(t.Context(), "REVOKE INSERT ON org_sensitive_access_events FROM "+role); err != nil {
		t.Fatal(err)
	}
	minted, err := l.service.IssueContent(t.Context(), l.scopeA, l.manager, command.DocumentID)
	record(err)
	if !errors.Is(err, media.ErrUnavailable) || minted.Token != "" {
		t.Fatal("capability minted without its access record", err)
	}
	content, err := l.service.ReadContent(t.Context(), l.scopeA, l.manager, command.DocumentID, fresh.Token)
	record(err)
	if !errors.Is(err, media.ErrUnavailable) || content.Bytes != nil {
		t.Fatal("content served without its access record", err)
	}
	if _, err := l.db.Admin.Exec(t.Context(), "GRANT INSERT ON org_sensitive_access_events TO "+role); err != nil {
		t.Fatal(err)
	}
	if content, err := l.service.ReadContent(t.Context(), l.scopeA, l.manager, command.DocumentID, fresh.Token); err != nil || !bytes.Equal(content.Bytes, l.raw) {
		t.Fatal("restored audit did not restore access", err)
	}
	served++
	// A role change after issuance revokes the open capability's use.
	late, err := l.service.IssueContent(t.Context(), l.scopeA, l.manager, command.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	secrets = append(secrets, late.Token)
	issued++
	l.authority.set(l.scopeA, l.manager, func(s *roleState) { s.role = roles.Supervisor })
	_, err = l.service.ReadContent(t.Context(), l.scopeA, l.manager, command.DocumentID, late.Token)
	record(err)
	if !errors.Is(err, media.ErrForbidden) {
		t.Fatal("capability outlived the content permission", err)
	}
	if l.access(t, "ISSUED") != issued || l.access(t, "SERVED") != served {
		t.Fatal("audit failure or revocation changed the access log", l.access(t, "ISSUED"), l.access(t, "SERVED"))
	}
	// BR-PRIV-008: no log line, error, audit or document row carries content,
	// its encoding, the digest, the storage key or any capability.
	var rows string
	if err := l.db.Admin.QueryRow(t.Context(), `SELECT concat_ws('|',(SELECT string_agg(to_jsonb(e)::text,'|') FROM org_audit_events e),
		(SELECT string_agg(to_jsonb(s)::text,'|') FROM org_sensitive_access_events s))`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(l.raw)
	object := media.Object{OrgID: l.scopeA.OrgID().String(), DocumentID: command.DocumentID, SHA256: hex.EncodeToString(sum[:]), MIME: app.MediaTypeJPEG, Bytes: int64(len(l.raw)), Revision: 3, Synthetic: true}
	key, _ := object.Key(l.scopeA)
	forbidden := append([]string{"SYNTHETIC-DOCUMENT-ORIGINAL", base64.StdEncoding.EncodeToString(l.raw)[:40], hex.EncodeToString(sum[:]), key, "/doc/", "X-Amz", "image/jpeg"}, secrets...)
	for _, surface := range []string{logs.String(), strings.Join(failures, "\n"), rows} {
		for _, value := range forbidden {
			if value != "" && strings.Contains(surface, value) {
				t.Fatalf("private content or capability reached telemetry, errors or audit: %.24q", value)
			}
		}
	}
}
