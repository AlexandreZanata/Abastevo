package application_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
)

const orgID = "7b0e2a54-0000-4000-8000-000000000001"
const documentID = "7b0e2a54-0000-4000-8000-000000000002"
const accessID = "7b0e2a54-0000-4000-8000-000000000003"

type authority struct {
	object          domain.Object
	denied, revoked bool
	calls           int
	failure         error
	grantFailure    error
}

func (a *authority) Resolve(_ context.Context, scope kernel.OrgScope, _ domain.Member, id string, _ domain.Intent) (domain.Object, error) {
	a.calls++
	if a.failure != nil {
		return domain.Object{}, a.failure
	}
	if a.denied || id != a.object.DocumentID || scope.OrgID().String() != a.object.OrgID {
		return domain.Object{}, domain.ErrNotFound
	}
	return a.object, nil
}
func (a *authority) CheckAccess(context.Context, kernel.OrgScope, domain.Member, domain.Object, string) error {
	if a.grantFailure != nil {
		return a.grantFailure
	}
	if a.revoked {
		return domain.ErrNotFound
	}
	return nil
}

type objects struct {
	data       []byte
	puts, gets int
	failure    error
	afterGet   func()
}

func (o *objects) Put(_ context.Context, _ kernel.OrgScope, _ domain.Object, raw []byte) error {
	o.puts++
	if o.failure != nil {
		return o.failure
	}
	o.data = append([]byte(nil), raw...)
	return nil
}
func (o *objects) Get(context.Context, kernel.OrgScope, domain.Object) ([]byte, error) {
	o.gets++
	if o.afterGet != nil {
		o.afterGet()
	}
	return append([]byte(nil), o.data...), o.failure
}

type inspector struct {
	mu      sync.Mutex
	calls   int
	mime    string
	failure error
	after   func()
	edit    bool
}

func (i *inspector) Inspect(_ context.Context, raw []byte) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.calls++
	if i.edit {
		raw[0] ^= 1
	}
	if i.after != nil {
		i.after()
	}
	return i.mime, i.failure
}

type tokens struct {
	invalid      bool
	deadline     time.Time
	issueFailure error
}

func (t *tokens) Issue(_ kernel.OrgScope, _ domain.Member, _ domain.Object, _ string, now time.Time) (application.Access, error) {
	if t.issueFailure != nil {
		return application.Access{}, t.issueFailure
	}
	t.deadline = now.Add(5 * time.Minute)
	return application.Access{Token: "synthetic-token", ExpiresAt: t.deadline}, nil
}
func (t *tokens) Verify(_ kernel.OrgScope, _ domain.Member, _ domain.Object, _ string, now time.Time) (string, error) {
	if t.invalid || !now.Before(t.deadline) {
		return "", domain.ErrNotFound
	}
	return accessID, nil
}

type audit struct {
	calls   int
	failure error
	after   func()
}

func (a *audit) Record(context.Context, kernel.OrgScope, domain.Member, domain.Object, string, string, domain.Intent) error {
	a.calls++
	if a.after != nil {
		a.after()
	}
	return a.failure
}

type state struct {
	policy    *authority
	objects   *objects
	inspector *inspector
	tokens    *tokens
	audit     *audit
	now       time.Time
	config    application.Config
}

func fixture(t *testing.T) (*application.Service, *state, kernel.OrgScope, domain.Member) {
	t.Helper()
	id, _ := kernel.ParseOrgID(orgID)
	scope, _ := kernel.NewOrgScope(id)
	raw := []byte("synthetic")
	sum := sha256.Sum256(raw)
	s := &state{policy: &authority{object: domain.Object{OrgID: orgID, DocumentID: documentID, SHA256: hex.EncodeToString(sum[:]), MIME: "image/jpeg", Bytes: int64(len(raw)), Revision: 1, Synthetic: true, Ready: true}},
		objects: &objects{data: raw}, inspector: &inspector{mime: "image/jpeg"}, tokens: &tokens{}, audit: &audit{}, now: time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)}
	s.config = application.Config{Enabled: true, Authority: s.policy, Objects: s.objects, Inspector: s.inspector, Tokens: s.tokens, Audit: s.audit, Now: func() time.Time { return s.now }, NewID: func() (string, error) { return accessID, nil }}
	service, err := application.NewService(s.config)
	if err != nil {
		t.Fatal(err)
	}
	return service, s, scope, domain.Member{AccountID: orgID, MembershipID: documentID}
}
func TestBoundedUploadUsesOneInspectedOriginal(t *testing.T) {
	service, s, scope, member := fixture(t)
	if err := service.Upload(t.Context(), scope, member, documentID, bytes.NewReader(s.objects.data)); err != nil {
		t.Fatal(err)
	}
	if s.objects.puts != 1 || s.inspector.calls != 1 {
		t.Fatal("upload bypassed inspection")
	}
	for _, body := range []string{"short", "substitut", "synthetic-extra"} {
		if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader(body)); err == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
	if s.objects.puts != 1 || s.inspector.calls != 1 {
		t.Fatal("invalid bytes reached inspector or storage")
	}
	s.inspector.mime = "application/pdf"
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatal(err)
	}
	s.inspector.mime = "image/jpeg"
	s.inspector.failure = errors.New("private scanner output")
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
	s.policy.denied = true
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
}
func TestAccessIsAuditedAndRevocationRechecked(t *testing.T) {
	service, s, scope, member := fixture(t)
	access, err := service.Issue(t.Context(), scope, member, documentID)
	if err != nil {
		t.Fatal(err)
	}
	if s.audit.calls != 1 || access.Token == "" {
		t.Fatal("unlogged grant")
	}
	for range 2 {
		content, err := service.Open(t.Context(), scope, member, documentID, access.Token)
		if err != nil || string(content.Bytes) != "synthetic" || content.MIME != "image/jpeg" {
			t.Fatal("private read failed", err)
		}
	}
	if s.audit.calls != 3 {
		t.Fatal("repeated content access was not audited")
	}
	s.policy.revoked = true
	if content, err := service.Open(t.Context(), scope, member, documentID, access.Token); !errors.Is(err, domain.ErrNotFound) || len(content.Bytes) != 0 {
		t.Fatal("revoked capability delivered bytes", err)
	}
	if s.objects.gets != 2 {
		t.Fatal("revocation reached storage")
	}
	s.policy.revoked = false
	s.objects.afterGet = func() { s.policy.denied = true }
	if content, err := service.Open(t.Context(), scope, member, documentID, access.Token); !errors.Is(err, domain.ErrNotFound) || len(content.Bytes) != 0 {
		t.Fatal("offboarding during download delivered bytes", err)
	}
}
func TestNoContentOnAuditExpiryIntegrityOrStorageFailure(t *testing.T) {
	for _, kind := range []string{"audit", "expiry", "integrity", "storage", "unverified", "token"} {
		t.Run(kind, func(t *testing.T) {
			service, s, scope, member := fixture(t)
			access, err := service.Issue(t.Context(), scope, member, documentID)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "audit":
				s.audit.failure = errors.New("private database output")
			case "expiry":
				s.audit.after = func() { s.now = s.now.Add(5 * time.Minute) }
			case "integrity":
				s.objects.data = []byte("substitut")
			case "storage":
				s.objects.failure = errors.New("https://private.invalid/?token=secret")
			case "unverified":
				s.policy.object.Ready = false
			case "token":
				s.tokens.invalid = true
			}
			content, err := service.Open(t.Context(), scope, member, documentID, access.Token)
			if err == nil || len(content.Bytes) != 0 || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "secret") {
				t.Fatal("unsafe content/error response")
			}
		})
	}
	service, s, scope, member := fixture(t)
	s.audit.failure = errors.New("secret")
	if access, err := service.Issue(t.Context(), scope, member, documentID); err == nil || access.Token != "" {
		t.Fatal("audit failure issued grant")
	}
	s.audit.failure = nil
	s.objects.failure = io.ErrUnexpectedEOF
	if err := service.Upload(t.Context(), scope, member, documentID, strings.NewReader("synthetic")); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal(err)
	}
}
