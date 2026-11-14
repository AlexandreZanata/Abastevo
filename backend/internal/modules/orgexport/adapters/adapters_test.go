package adapters_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func uid(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

func scopeOf(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(uid(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func objectOf(t *testing.T, s kernel.OrgScope, export string, body string) domain.Object {
	t.Helper()
	sink, err := domain.NewSink(domain.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Row([]byte(body)); err != nil {
		t.Fatal(err)
	}
	file, err := sink.Result()
	if err != nil {
		t.Fatal(err)
	}
	object, err := domain.ObjectFor(s, export, "text/csv", file)
	if err != nil {
		t.Fatal(err)
	}
	return object
}

// EDR-157, B-BR-EMP-230: a capability is bound to origin, organization,
// requester and the exact file, and expires within five minutes.
func TestCapabilityIsRequesterAndFileBoundAndShort(t *testing.T) {
	secret := []byte(strings.Repeat("synthetic-", 4))
	tokens, err := adapters.NewTokens("https://api.example.invalid", secret)
	if err != nil {
		t.Fatal(err)
	}
	s := scopeOf(t, 1)
	requester := app.Requester{AccountID: uid(2), MembershipID: uid(4)}
	object := objectOf(t, s, uid(50), "1,2\n")
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	capability, err := tokens.Issue(s, requester, object, uid(60), now)
	if err != nil || capability.Token == "" || !capability.ExpiresAt.Equal(now.Add(5*time.Minute)) {
		t.Fatal("issue", err)
	}
	for _, forbidden := range []string{"org/", "/export/", object.SHA256, uid(50), uid(2), "http"} {
		if strings.Contains(capability.Token, forbidden) {
			t.Fatal("capability carries a readable private reference", forbidden)
		}
	}
	if got, err := tokens.Verify(s, requester, object, capability.Token, now.Add(299*time.Second)); err != nil || got != uid(60) {
		t.Fatal("valid capability refused", err)
	}
	other := object
	other.Rows++
	changed := map[string]func() (string, error){
		"expired": func() (string, error) {
			return tokens.Verify(s, requester, object, capability.Token, now.Add(5*time.Minute))
		},
		"future issuance": func() (string, error) {
			return tokens.Verify(s, requester, object, capability.Token, now.Add(-time.Second))
		},
		"other member": func() (string, error) {
			return tokens.Verify(s, app.Requester{AccountID: uid(2), MembershipID: uid(5)}, object, capability.Token, now)
		},
		"other account": func() (string, error) {
			return tokens.Verify(s, app.Requester{AccountID: uid(3), MembershipID: uid(4)}, object, capability.Token, now)
		},
		"other organization": func() (string, error) {
			return tokens.Verify(scopeOf(t, 7), requester, objectOf(t, scopeOf(t, 7), uid(50), "1,2\n"), capability.Token, now)
		},
		"other file": func() (string, error) {
			return tokens.Verify(s, requester, objectOf(t, s, uid(50), "1,3\n"), capability.Token, now)
		},
		"other job": func() (string, error) {
			return tokens.Verify(s, requester, objectOf(t, s, uid(51), "1,2\n"), capability.Token, now)
		},
		"changed facts":  func() (string, error) { return tokens.Verify(s, requester, other, capability.Token, now) },
		"tampered":       func() (string, error) { return tokens.Verify(s, requester, object, capability.Token+"x", now) },
		"empty":          func() (string, error) { return tokens.Verify(s, requester, object, "", now) },
		"oversized":      func() (string, error) { return tokens.Verify(s, requester, object, strings.Repeat("a", 2049), now) },
		"invalid member": func() (string, error) { return tokens.Verify(s, app.Requester{}, object, capability.Token, now) },
	}
	for name, verify := range changed {
		if got, err := verify(); err == nil || got != "" {
			t.Fatal("capability accepted:", name)
		}
	}
	otherOrigin, _ := adapters.NewTokens("https://other.example.invalid", secret)
	if _, err := otherOrigin.Verify(s, requester, object, capability.Token, now); err == nil {
		t.Fatal("capability accepted at another origin")
	}
	for _, bad := range []struct {
		origin string
		secret []byte
	}{{"http://api.example.invalid", secret}, {"https://API.example.invalid", secret}, {"https://api.example.invalid/path", secret}, {"https://api.example.invalid", secret[:31]}, {"", secret}} {
		if _, err := adapters.NewTokens(bad.origin, bad.secret); err == nil {
			t.Fatal("invalid token configuration accepted", bad.origin)
		}
	}
	if _, err := tokens.Issue(s, requester, object, "bad", now); err == nil {
		t.Fatal("capability for an invalid grant")
	}
	if _, err := tokens.Issue(s, requester, object, uid(60), time.Time{}); err == nil {
		t.Fatal("capability without a clock")
	}
	if fmt.Sprint(*tokens) != "orgexport: token handle redacted" || fmt.Sprintf("%#v", tokens) == "" || strings.Contains(fmt.Sprintf("%v", *tokens), "synthetic") {
		t.Fatal("token handle formatting leaks its secret")
	}
}

// BR-PRIV-003: queue payloads are reference-only, versioned and exact.
func TestQueuePayloadIsReferenceOnly(t *testing.T) {
	s := scopeOf(t, 1)
	if adapters.Dedupe(adapters.BuildKind, s, uid(50)) != "org-export-build:"+uid(1)+":"+uid(50) || adapters.Dedupe(adapters.CleanupKind, s, uid(50)) != "org-export-cleanup:"+uid(1)+":"+uid(50) ||
		adapters.Dedupe("other", s, uid(50)) != "" || adapters.Dedupe(adapters.BuildKind, s, "bad") != "" {
		t.Fatal("dedupe key drifted")
	}
	scope, export, err := adapters.ParsePayload([]byte(`{"version":1,"org_id":"` + uid(1) + `","export_id":"` + uid(50) + `"}`))
	if err != nil || scope != s || export != uid(50) {
		t.Fatal("payload", err)
	}
	for _, raw := range []string{`{}`, `{"version":2,"org_id":"` + uid(1) + `","export_id":"` + uid(50) + `"}`, `{"version":1,"org_id":"` + uid(1) + `"}`,
		`{"version":1,"org_id":"` + uid(1) + `","export_id":"` + uid(50) + `","rows":1}`, `{"version":1,"org_id":"bad","export_id":"` + uid(50) + `"}`, `[]`} {
		if _, _, err := adapters.ParsePayload([]byte(raw)); err == nil {
			t.Fatal("payload accepted", raw)
		}
	}
	if (adapters.BuildHandler{}).Kind() != "org-export-build" || (adapters.CleanupHandler{}).Kind() != "org-export-cleanup" || (adapters.BuildHandler{}).Version() != 1 || (adapters.CleanupHandler{}).Version() != 1 {
		t.Fatal("handler identity drifted")
	}
	if err := (adapters.BuildHandler{}).Handle(context.Background(), jobs.Job{Kind: adapters.BuildKind, Payload: []byte(`{}`)}); err == nil || !strings.Contains(err.Error(), "payload") {
		t.Fatal("malformed build payload not parked", err)
	}
	if err := (adapters.CleanupHandler{}).Handle(context.Background(), jobs.Job{Kind: adapters.BuildKind, Payload: []byte(`{"version":1,"org_id":"` + uid(1) + `","export_id":"` + uid(50) + `"}`)}); err == nil {
		t.Fatal("cleanup handled another kind")
	}
}

func TestAuditVocabularyAndStorageConfiguration(t *testing.T) {
	defs := adapters.AuditDefinitions()
	catalog, err := audit.NewCatalog(defs)
	if err != nil || len(defs) != 3 {
		t.Fatal("export audit vocabulary", err)
	}
	if defs[0].Action != "rpt.export.requested" || defs[0].TargetType != "export" || defs[0].RequireReason || len(defs[0].Reasons) != 0 {
		t.Fatal("request event definition drifted", defs[0])
	}
	if catalog.Validate(audit.Event{OperationID: uid(1), ActorID: uid(2), Action: "export.access.issued", TargetType: "export", TargetID: uid(3), ReasonCode: audit.AccessReason, RequestHash: strings.Repeat("a", 64)}) != nil {
		t.Fatal("EXPORT_FILE issuance not registered")
	}
	for _, bad := range []adapters.StorageConfig{{}, {Endpoint: "http://storage.example.invalid", Bucket: "validation-private", Region: "us-east-1", Credentials: storage.Credentials{AccessKeyID: "a", SecretAccessKey: "b"}},
		{Endpoint: "http://127.0.0.1:9000", Bucket: "validation-private", Region: "us-east-1", Credentials: storage.Credentials{AccessKeyID: "a", SecretAccessKey: "b"}},
		{Endpoint: "https://storage.example.invalid/path", Bucket: "validation-private", Region: "us-east-1", Credentials: storage.Credentials{AccessKeyID: "a", SecretAccessKey: "b"}},
		{Endpoint: "https://storage.example.invalid", Bucket: "validation-private", Region: "us-east-1"}} {
		if _, err := adapters.NewS3(bad); err == nil {
			t.Fatal("unsafe storage configuration accepted")
		}
	}
	good := adapters.StorageConfig{Endpoint: "https://storage.example.invalid", Bucket: "validation-private", Region: "us-east-1", Credentials: storage.Credentials{AccessKeyID: "synthetic-access", SecretAccessKey: "synthetic-private"}}
	store, err := adapters.NewS3(good)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{fmt.Sprint(good), fmt.Sprintf("%#v", good), fmt.Sprint(*store), fmt.Sprintf("%#v", *store)} {
		if strings.Contains(text, "synthetic") || strings.Contains(text, "storage.example") {
			t.Fatal("storage configuration formatting leaks", text)
		}
	}
	s := scopeOf(t, 1)
	object := objectOf(t, s, uid(50), "1\n")
	none := adapters.Unconfigured{}
	if err := none.Put(context.Background(), s, object, []byte("1\n")); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unconfigured storage stored", err)
	}
	if _, err := none.Get(context.Background(), s, object); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unconfigured storage served", err)
	}
	if err := none.Delete(context.Background(), s, object); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("unconfigured storage deleted", err)
	}
	var missing *adapters.S3
	if err := missing.Put(context.Background(), s, object, []byte("1\n")); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil storage stored", err)
	}
	if err := store.Put(context.Background(), scopeOf(t, 7), object, []byte("1\n")); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("object of another organization accepted", err)
	}
	if err := store.Put(context.Background(), s, object, []byte("2\n")); !errors.Is(err, domain.ErrIntegrity) {
		t.Fatal("bytes that do not match the object accepted", err)
	}
}

func TestStoreRefusesWithoutSourcesOrScope(t *testing.T) {
	s := scopeOf(t, 1)
	var store *adapters.Store
	actor := app.Actor{AccountID: uid(2), SessionID: uid(3)}
	if _, err := store.Read(context.Background(), s, actor, uid(4), uid(50)); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil store read", err)
	}
	empty := &adapters.Store{}
	if _, err := empty.Deliverable(context.Background(), s, actor, uid(4), uid(50)); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("store without sources served", err)
	}
	if _, err := empty.Claim(context.Background(), s, uid(50), uid(60), time.Minute); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("store without pool claimed", err)
	}
	if _, err := empty.Read(context.Background(), kernel.OrgScope{}, actor, uid(4), uid(50)); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("invalid scope", err)
	}
	if _, err := empty.Read(context.Background(), s, app.Actor{}, uid(4), uid(50)); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("invalid actor", err)
	}
}
