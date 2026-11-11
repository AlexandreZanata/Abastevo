//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditstore "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

type mediaPolicy struct {
	mu      sync.Mutex
	objects map[string]domain.Object
	members map[string]domain.Member
	revoked map[string]bool
	denied  bool
}

func (p *mediaPolicy) Resolve(_ context.Context, scope kernel.OrgScope, member domain.Member, id string, _ domain.Intent) (domain.Object, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	object, exists := p.objects[scope.OrgID().String()]
	if p.denied || !exists || p.members[object.OrgID] != member || object.DocumentID != id {
		return domain.Object{}, domain.ErrNotFound
	}
	return object, nil
}
func (p *mediaPolicy) CheckAccess(_ context.Context, _ kernel.OrgScope, _ domain.Member, _ domain.Object, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.denied || p.revoked[id] {
		return domain.ErrNotFound
	}
	return nil
}

// This narrow policy fixture supplies the future AUTH/DOC provider; document
// metadata persistence belongs to EMP-DOC-002, not this storage foundation.
type mediaFixture struct {
	db               *orgtest.Fixture
	config           adapters.StorageConfig
	store            *adapters.S3
	service          *application.Service
	policy           *mediaPolicy
	tokens           *adapters.Tokens
	serviceConfig    application.Config
	scopeA, scopeB   kernel.OrgScope
	memberA, memberB domain.Member
	objectA, objectB domain.Object
	raw              []byte
	now              time.Time
}

func realMedia(t *testing.T) *mediaFixture {
	t.Helper()
	required := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("explicit disposable %s required", name)
		}
		return value
	}
	f := &mediaFixture{db: orgtest.New(t), raw: originalJPEG(t), now: time.Now().UTC()}
	f.config = adapters.StorageConfig{Endpoint: required("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: required("ANPFUEL_TEST_STORAGE_BUCKET"),
		Region: required("ANPFUEL_TEST_STORAGE_REGION"), Credentials: storage.Credentials{AccessKeyID: required("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: required("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")}, AllowLoopback: true}
	var err error
	f.store, err = adapters.NewS3(f.config)
	if err != nil {
		t.Fatal("invalid disposable storage config")
	}
	f.tokens, err = adapters.NewTokens("https://api.example.invalid", []byte(strings.Repeat("synthetic-", 5)))
	if err != nil {
		t.Fatal(err)
	}
	f.scopeA = f.db.Organizations[0].Scope(t)
	f.scopeB = f.db.Organizations[1].Scope(t)
	f.memberA = domain.Member{AccountID: orgID, MembershipID: documentID}
	f.memberB = domain.Member{AccountID: otherID, MembershipID: accessID}
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(f.raw)
	f.objectA = domain.Object{OrgID: f.scopeA.OrgID().String(), DocumentID: id, SHA256: hex.EncodeToString(sum[:]), MIME: "image/jpeg", Bytes: int64(len(f.raw)), Revision: 1, Synthetic: true, Ready: true}
	f.objectB = f.objectA
	f.objectB.OrgID = f.scopeB.OrgID().String()
	f.policy = &mediaPolicy{objects: map[string]domain.Object{f.objectA.OrgID: f.objectA, f.objectB.OrgID: f.objectB},
		members: map[string]domain.Member{f.objectA.OrgID: f.memberA, f.objectB.OrgID: f.memberB}, revoked: map[string]bool{}}
	if _, err := f.db.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_audit_events TO "+pgx.Identifier{f.db.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(adapters.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	f.serviceConfig = application.Config{Enabled: true, Authority: f.policy, Objects: f.store, Inspector: adapters.JPEGInspector{}, Tokens: f.tokens,
		Audit: adapters.AccessAudit{Runner: auditstore.NewStore(f.db.Runtime, catalog)}, Now: func() time.Time { return f.now }, NewID: jobs.NewUUIDv4}
	f.service, err = application.NewService(f.serviceConfig)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func TestPrivateS3OriginalAndMemberBoundAuditedAccess(t *testing.T) {
	f := realMedia(t)
	ctx := t.Context()
	for _, item := range []struct {
		scope  kernel.OrgScope
		member domain.Member
		object domain.Object
	}{{f.scopeA, f.memberA, f.objectA}, {f.scopeB, f.memberB, f.objectB}} {
		if err := f.service.Upload(ctx, item.scope, item.member, item.object.DocumentID, bytes.NewReader(f.raw)); err != nil {
			t.Fatal("private upload failed", err)
		}
		key, err := item.object.Key(item.scope)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.Get(f.config.Endpoint + "/" + f.config.Bucket + "/" + key)
		if err != nil {
			t.Fatal("anonymous probe failed")
		}
		response.Body.Close()
		if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusUnauthorized {
			t.Fatal("private object anonymously readable")
		}
	}
	access, err := f.service.Issue(ctx, f.scopeA, f.memberA, f.objectA.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		content, err := f.service.Open(ctx, f.scopeA, f.memberA, f.objectA.DocumentID, access.Token)
		if err != nil || !bytes.Equal(content.Bytes, f.raw) {
			t.Fatal("original snapshot not preserved", err)
		}
	}
	var events int
	if err := f.db.Admin.QueryRow(ctx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1`, f.objectA.OrgID).Scan(&events); err != nil || events != 3 {
		t.Fatal("missing issuance/read audit", events, err)
	}
	for _, item := range []struct {
		scope  kernel.OrgScope
		member domain.Member
	}{{f.scopeB, f.memberB}, {f.scopeA, f.memberB}} {
		if content, err := f.service.Open(ctx, item.scope, item.member, f.objectA.DocumentID, access.Token); !errors.Is(err, domain.ErrNotFound) || len(content.Bytes) != 0 {
			t.Fatal("foreign capability delivered bytes", err)
		}
	}
	grantID, err := f.tokens.Verify(f.scopeA, f.memberA, f.objectA, access.Token, f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.policy.revoked[grantID] = true
	if content, err := f.service.Open(ctx, f.scopeA, f.memberA, f.objectA.DocumentID, access.Token); !errors.Is(err, domain.ErrNotFound) || len(content.Bytes) != 0 {
		t.Fatal("revoked grant delivered bytes", err)
	}
	delete(f.policy.revoked, grantID)
	f.now = f.now.Add(5 * time.Minute)
	if content, err := f.service.Open(ctx, f.scopeA, f.memberA, f.objectA.DocumentID, access.Token); !errors.Is(err, domain.ErrNotFound) || len(content.Bytes) != 0 {
		t.Fatal("expired grant delivered bytes", err)
	}
	var ledger string
	if err := f.db.Admin.QueryRow(ctx, `SELECT COALESCE(json_agg(e)::text,'[]') FROM org_audit_events e`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ledger, access.Token) || strings.Contains(ledger, "SYNTHETIC-PRIVATE-ORIGINAL") || strings.Contains(ledger, "X-Amz-") {
		t.Fatal("content or transport credential in audit")
	}
	var unscoped int
	if err := f.db.Runtime.QueryRow(ctx, `SELECT count(*) FROM org_audit_events`).Scan(&unscoped); err != nil || unscoped != 0 {
		t.Fatal("unscoped audit leak", err)
	}
}

type countTransport struct {
	base              http.RoundTripper
	created, existing atomic.Int64
}

func (c *countTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := c.base.RoundTrip(request)
	if err == nil && request.Method == http.MethodPut {
		if response.StatusCode == 200 {
			c.created.Add(1)
		}
		if response.StatusCode == 412 {
			c.existing.Add(1)
		}
	}
	return response, err
}
func TestConcurrentPrivatePUTNeverOverwritesAndDetectsSubstitution(t *testing.T) {
	f := realMedia(t)
	ctx := t.Context()
	transport := &countTransport{base: http.DefaultTransport}
	config := f.config
	config.Client = &http.Client{Transport: transport}
	store, err := adapters.NewS3(config)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 8)
	for range 8 {
		go func() { <-start; results <- store.Put(ctx, f.scopeA, f.objectA, f.raw) }()
	}
	close(start)
	for range 8 {
		if err := <-results; err != nil {
			t.Fatal("concurrent conditional write failed", err)
		}
	}
	if transport.created.Load() != 1 || transport.existing.Load() != 7 {
		t.Fatalf("conditional outcomes created=%d existing=%d", transport.created.Load(), transport.existing.Load())
	}
	key, _ := f.objectA.Key(f.scopeA)
	input := storage.PresignInput{Endpoint: f.config.Endpoint, Bucket: f.config.Bucket, Region: f.config.Region, Key: key, Namespace: "org/" + f.objectA.OrgID + "/",
		ContentType: f.objectA.MIME, MaxBytes: f.objectA.Bytes}
	// A compromised-provider fixture changes bytes behind the fixed key.
	bad := bytes.Clone(f.raw)
	bad[len(bad)/2] ^= 1
	signed, err := storage.PresignServerPUT(input, f.config.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Put(ctx, nil, signed.URL, f.objectA.MIME, bad); err != nil {
		t.Fatal("substitution fixture failed")
	}
	if err := store.Put(ctx, f.scopeA, f.objectA, f.raw); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal("substituted stored object accepted", err)
	}
	if content, err := store.Get(ctx, f.scopeA, f.objectA); !errors.Is(err, domain.ErrIntegrity) || content != nil {
		t.Fatal("substituted read delivered bytes", err)
	}
	signed, err = storage.PresignGET(input, f.config.Credentials)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := storage.Get(ctx, nil, signed.URL, f.objectA.Bytes)
	if err != nil || !bytes.Equal(actual, bad) {
		t.Fatal("retry overwrote existing object")
	}
}
func TestRealAuditFailureAndIdentityChangeReleaseNoContent(t *testing.T) {
	f := realMedia(t)
	ctx := t.Context()
	if err := f.service.Upload(ctx, f.scopeA, f.memberA, f.objectA.DocumentID, bytes.NewReader(f.raw)); err != nil {
		t.Fatal(err)
	}
	access, err := f.service.Issue(ctx, f.scopeA, f.memberA, f.objectA.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Admin.Exec(ctx, "REVOKE INSERT ON org_audit_events FROM "+pgx.Identifier{f.db.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if content, err := f.service.Open(ctx, f.scopeA, f.memberA, f.objectA.DocumentID, access.Token); !errors.Is(err, domain.ErrUnavailable) || len(content.Bytes) != 0 {
		t.Fatal("audit failure released bytes", err)
	}
	if grant, err := f.service.Issue(ctx, f.scopeA, f.memberA, f.objectA.DocumentID); !errors.Is(err, domain.ErrUnavailable) || grant.Token != "" {
		t.Fatal("audit failure issued token", err)
	}
	if _, err := f.db.Admin.Exec(ctx, "GRANT INSERT ON org_audit_events TO "+pgx.Identifier{f.db.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	changed := f.objectA
	changed.Revision++
	f.policy.objects[changed.OrgID] = changed
	if content, err := f.service.Open(ctx, f.scopeA, f.memberA, f.objectA.DocumentID, access.Token); !errors.Is(err, domain.ErrNotFound) || len(content.Bytes) != 0 {
		t.Fatal("old revision grant delivered content", err)
	}
	changed.Ready = false
	f.policy.objects[changed.OrgID] = changed
	if grant, err := f.service.Issue(ctx, f.scopeA, f.memberA, f.objectA.DocumentID); !errors.Is(err, domain.ErrNotFound) || grant.Token != "" {
		t.Fatal("unverified object became readable", err)
	}
}

func TestFiveMiBPrivateJPEGUsesServerPolicyAndPreservesBytes(t *testing.T) {
	f := realMedia(t)
	base := f.raw
	remaining := (5 << 20) - len(base)
	raw := make([]byte, 0, 5<<20)
	raw = append(raw, base[:2]...)
	for remaining > 0 {
		size := min(remaining, 65537)
		if tail := remaining - size; tail > 0 && tail < 4 {
			size -= 4 - tail
		}
		length := size - 2
		raw = append(raw, 0xff, 0xe2, byte(length>>8), byte(length))
		raw = append(raw, make([]byte, size-4)...)
		remaining -= size
	}
	raw = append(raw, base[2:]...)
	if len(raw) != 5<<20 {
		t.Fatal("invalid boundary fixture")
	}
	object := f.objectA
	sum := sha256.Sum256(raw)
	object.SHA256 = hex.EncodeToString(sum[:])
	object.Bytes = int64(len(raw))
	object.Ready = false
	f.policy.objects[object.OrgID] = object
	if err := f.service.Upload(t.Context(), f.scopeA, f.memberA, object.DocumentID, bytes.NewReader(raw)); err != nil {
		t.Fatal("bounded five-MiB original refused", err)
	}
	// Only the fixture's owning document workflow marks the inspected row ready.
	object.Ready = true
	f.policy.objects[object.OrgID] = object
	access, err := f.service.Issue(t.Context(), f.scopeA, f.memberA, object.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := f.service.Open(t.Context(), f.scopeA, f.memberA, object.DocumentID, access.Token)
	if err != nil || !bytes.Equal(content.Bytes, raw) {
		t.Fatal("boundary original changed", err)
	}
}
