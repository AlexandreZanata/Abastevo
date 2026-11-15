//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	roles "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditstore "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

// contentMember is the current session and role snapshot that the future
// AUTH/DOC authority resolves; the content permission comes from the real
// role catalog (BR-AUTH-011), never from reading the document record.
type contentMember struct {
	account string
	role    roles.Role
	expired bool
}

type contentAuthority struct {
	mu      sync.Mutex
	objects map[string]domain.Object
	members map[string]map[string]contentMember
	revoked map[string]bool
}

var errSyntheticSessionExpired = errors.New("SYNTHETIC-SESSION-EXPIRED")

func (p *contentAuthority) current(scope kernel.OrgScope, member domain.Member) error {
	state, exists := p.members[scope.OrgID().String()][member.MembershipID]
	if !exists || state.account != member.AccountID {
		return domain.ErrNotFound
	}
	if state.expired {
		return errSyntheticSessionExpired
	}
	rule, err := roles.SystemCatalog().Lookup(state.role, roles.Permission(audit.DocumentContent.Permission()))
	if err != nil {
		return err
	}
	if rule.Coverage() == roles.Denied {
		return domain.ErrForbidden
	}
	return nil
}
func (p *contentAuthority) Resolve(_ context.Context, scope kernel.OrgScope, member domain.Member, id string, _ domain.Intent) (domain.Object, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	object, exists := p.objects[scope.OrgID().String()]
	if !exists || object.DocumentID != id {
		return domain.Object{}, domain.ErrNotFound
	}
	if err := p.current(scope, member); err != nil {
		return domain.Object{}, err
	}
	return object, nil
}
func (p *contentAuthority) CheckAccess(_ context.Context, scope kernel.OrgScope, member domain.Member, _ domain.Object, grant string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.current(scope, member); err != nil {
		return err
	}
	if p.revoked[grant] {
		return domain.ErrNotFound
	}
	return nil
}
func (p *contentAuthority) set(org, membership string, change func(*contentMember)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	state := p.members[org][membership]
	change(&state)
	p.members[org][membership] = state
}

// memoryObjects replaces private S3 here: this suite proves the access log,
// and the S3 suites keep proving storage with explicit disposable buckets.
type memoryObjects struct {
	mu   sync.Mutex
	data map[string][]byte
	gets int
}

func (o *memoryObjects) Put(_ context.Context, scope kernel.OrgScope, object domain.Object, raw []byte) error {
	key, err := object.Key(scope)
	if err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.data[key] = bytes.Clone(raw)
	return nil
}
func (o *memoryObjects) Get(_ context.Context, scope kernel.OrgScope, object domain.Object) ([]byte, error) {
	key, err := object.Key(scope)
	if err != nil {
		return nil, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.gets++
	raw, exists := o.data[key]
	if !exists {
		return nil, domain.ErrUnavailable
	}
	return bytes.Clone(raw), nil
}

type loggedMedia struct {
	db                   *orgtest.Fixture
	service              *application.Service
	policy               *contentAuthority
	objects              *memoryObjects
	scopeA, scopeB       kernel.OrgScope
	manager, supervisor  domain.Member
	colleague, foreign   domain.Member
	documentA, documentB domain.Object
	raw                  []byte
	now                  time.Time
	clock                sync.Mutex
}

func newLoggedMedia(t *testing.T) *loggedMedia {
	t.Helper()
	f := &loggedMedia{db: orgtest.New(t), raw: originalJPEG(t), now: time.Now().UTC(), objects: &memoryObjects{data: map[string][]byte{}}}
	role := pgx.Identifier{f.db.RuntimeRole}.Sanitize()
	if _, err := f.db.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+role+
		"; GRANT SELECT, INSERT ON org_audit_events, org_sensitive_access_events TO "+role); err != nil {
		t.Fatal(err)
	}
	f.scopeA, f.scopeB = f.db.Organizations[0].Scope(t), f.db.Organizations[1].Scope(t)
	id := func() string {
		value, err := jobs.NewUUIDv4()
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	f.manager = domain.Member{AccountID: id(), MembershipID: id()}
	f.supervisor = domain.Member{AccountID: id(), MembershipID: id()}
	f.colleague = domain.Member{AccountID: id(), MembershipID: id()}
	f.foreign = domain.Member{AccountID: id(), MembershipID: id()}
	sum := sha256.Sum256(f.raw)
	f.documentA = domain.Object{OrgID: f.scopeA.OrgID().String(), DocumentID: id(), SHA256: hex.EncodeToString(sum[:]), MIME: "image/jpeg",
		Bytes: int64(len(f.raw)), Revision: 1, Synthetic: true, Ready: true}
	f.documentB = f.documentA
	f.documentB.OrgID, f.documentB.DocumentID = f.scopeB.OrgID().String(), id()
	f.policy = &contentAuthority{
		objects: map[string]domain.Object{f.documentA.OrgID: f.documentA, f.documentB.OrgID: f.documentB},
		members: map[string]map[string]contentMember{
			f.documentA.OrgID: {f.manager.MembershipID: {account: f.manager.AccountID, role: roles.FleetManager},
				f.supervisor.MembershipID: {account: f.supervisor.AccountID, role: roles.Supervisor},
				f.colleague.MembershipID:  {account: f.colleague.AccountID, role: roles.Finance}},
			f.documentB.OrgID: {f.foreign.MembershipID: {account: f.foreign.AccountID, role: roles.FleetManager}},
		},
		revoked: map[string]bool{},
	}
	tokens, err := adapters.NewTokens("https://api.example.invalid", []byte(strings.Repeat("synthetic-", 5)))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(adapters.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = application.NewService(application.Config{Enabled: true, Authority: f.policy, Objects: f.objects,
		Inspector: adapters.JPEGInspector{}, Tokens: tokens, Audit: adapters.AccessAudit{Runner: auditstore.NewStore(f.db.Runtime, catalog)},
		Now: f.clockNow, NewID: jobs.NewUUIDv4})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		scope  kernel.OrgScope
		member domain.Member
		object domain.Object
	}{{f.scopeA, f.manager, f.documentA}, {f.scopeB, f.foreign, f.documentB}} {
		if err := f.service.Upload(t.Context(), item.scope, item.member, item.object.DocumentID, bytes.NewReader(f.raw)); err != nil {
			t.Fatal(err)
		}
	}
	return f
}
func (f *loggedMedia) clockNow() time.Time {
	f.clock.Lock()
	defer f.clock.Unlock()
	return f.now
}
func (f *loggedMedia) advance(by time.Duration) {
	f.clock.Lock()
	defer f.clock.Unlock()
	f.now = f.now.Add(by)
}

type accessRow struct {
	action, actor, target, targetType, reason, hash string
	category, stage, membership, grant, request     string
}

func (f *loggedMedia) rows(t *testing.T, org string) []accessRow {
	t.Helper()
	rows, err := f.db.Admin.Query(t.Context(), `SELECT e.action, e.actor_id::text, e.target_id::text, e.target_type, e.reason_code, e.request_hash,
		d.category, d.stage, d.membership_id::text, d.access_id::text, e.operation_id::text
		FROM org_audit_events e LEFT JOIN org_sensitive_access_events d USING (org_id, operation_id)
		WHERE e.org_id=$1 ORDER BY e.recorded_at, e.operation_id`, org)
	if err != nil {
		t.Fatal(err)
	}
	result, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (accessRow, error) {
		var r accessRow
		var category, stage, membership, grant *string
		err := row.Scan(&r.action, &r.actor, &r.target, &r.targetType, &r.reason, &r.hash, &category, &stage, &membership, &grant, &r.request)
		if category == nil || stage == nil || membership == nil || grant == nil {
			t.Fatal("sensitive audit event without its scoped detail")
		}
		r.category, r.stage, r.membership, r.grant = *category, *stage, *membership, *grant
		return r, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func (f *loggedMedia) counts(t *testing.T) (int, int) {
	t.Helper()
	return len(f.rows(t, f.documentA.OrgID)), len(f.rows(t, f.documentB.OrgID))
}

var coordinateShape = regexp.MustCompile(`-?[0-9]{1,3}\.[0-9]{3,}`)

// BUC-EMP-106: an authorized issue and read each write exactly one scoped
// record with the actor, and the stored rows hold no content-shaped value.
func TestDocumentContentReadsAreRecordedThroughTheSensitiveAccessLog(t *testing.T) {
	f := newLoggedMedia(t)
	ctx := t.Context()
	access, err := f.service.Issue(ctx, f.scopeA, f.manager, f.documentA.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	issued := f.rows(t, f.documentA.OrgID)
	if len(issued) != 1 {
		t.Fatalf("issuance wrote %d records", len(issued))
	}
	grant := issued[0]
	if grant.action != "document.access.issued" || grant.category != "DOCUMENT_CONTENT" || grant.stage != "ISSUED" ||
		grant.actor != f.manager.AccountID || grant.membership != f.manager.MembershipID || grant.target != f.documentA.DocumentID ||
		grant.targetType != "document" || grant.reason != audit.AccessReason || grant.grant != grant.request {
		t.Fatalf("issuance record %+v", grant)
	}
	content, err := f.service.Open(ctx, f.scopeA, f.manager, f.documentA.DocumentID, access.Token)
	if err != nil || !bytes.Equal(content.Bytes, f.raw) {
		t.Fatal("authorized read failed", err)
	}
	rows := f.rows(t, f.documentA.OrgID)
	if len(rows) != 2 {
		t.Fatalf("one read wrote %d records", len(rows)-1)
	}
	read := rows[1]
	if read.action != "document.content.accessed" || read.category != "DOCUMENT_CONTENT" || read.stage != "SERVED" ||
		read.actor != f.manager.AccountID || read.membership != f.manager.MembershipID || read.target != f.documentA.DocumentID ||
		read.grant != grant.request || read.request == grant.request || read.reason != audit.AccessReason {
		t.Fatalf("read record %+v", read)
	}
	if _, b := f.counts(t); b != 0 {
		t.Fatal("another organization received a record")
	}
	key, err := f.documentA.Key(f.scopeA)
	if err != nil {
		t.Fatal(err)
	}
	var ledger string
	if err := f.db.Admin.QueryRow(ctx, `SELECT COALESCE(json_agg(json_build_array(e.org_id, e.operation_id, e.actor_id, e.delegation_id,
		e.action, e.target_type, e.target_id, e.reason_code, e.request_hash, d.category, d.stage, d.membership_id, d.access_id))::text, '[]')
		FROM org_audit_events e LEFT JOIN org_sensitive_access_events d USING (org_id, operation_id)`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{access.Token, "SYNTHETIC-PRIVATE-ORIGINAL", key, "/doc/", f.documentA.SHA256, f.documentA.MIME,
		"document.jpg", "http", "X-Amz", "api.example.invalid", "capability"} {
		if strings.Contains(ledger, leaked) {
			t.Fatalf("access log holds %q", leaked)
		}
	}
	if coordinateShape.MatchString(ledger) {
		t.Fatal("access log holds a coordinate-shaped value")
	}
}

// BUC-EMP-106 negatives: nothing is served and no record is written for a
// foreign item or organization, a missing content permission, an expired
// session, or an expired, revoked or foreign capability.
func TestDeniedDocumentReadsServeNothingAndLeaveNoRecord(t *testing.T) {
	f := newLoggedMedia(t)
	ctx := t.Context()
	assertNoRecords := func(why string, wantA int) {
		t.Helper()
		if a, b := f.counts(t); a != wantA || b != 0 {
			t.Fatalf("%s left records A=%d B=%d", why, a, b)
		}
	}
	for name, attempt := range map[string]struct {
		scope  kernel.OrgScope
		member domain.Member
		id     string
	}{
		"foreign document":     {f.scopeA, f.manager, f.documentB.DocumentID},
		"foreign organization": {f.scopeB, f.manager, f.documentB.DocumentID},
		"foreign member":       {f.scopeA, f.foreign, f.documentA.DocumentID},
	} {
		if access, err := f.service.Issue(ctx, attempt.scope, attempt.member, attempt.id); !errors.Is(err, domain.ErrNotFound) || access.Token != "" {
			t.Fatalf("%s was not NOT_FOUND: %v", name, err)
		}
		assertNoRecords(name, 0)
	}
	if access, err := f.service.Issue(ctx, f.scopeA, f.supervisor, f.documentA.DocumentID); !errors.Is(err, domain.ErrForbidden) || access.Token != "" {
		t.Fatal("member without doc.document.view_content was not forbidden", err)
	}
	assertNoRecords("missing permission", 0)
	f.policy.set(f.documentA.OrgID, f.manager.MembershipID, func(m *contentMember) { m.expired = true })
	if access, err := f.service.Issue(ctx, f.scopeA, f.manager, f.documentA.DocumentID); err == nil || access.Token != "" || strings.Contains(err.Error(), "SYNTHETIC") {
		t.Fatal("expired session received a capability", err)
	}
	assertNoRecords("expired session", 0)
	f.policy.set(f.documentA.OrgID, f.manager.MembershipID, func(m *contentMember) { m.expired = false })
	access, err := f.service.Issue(ctx, f.scopeA, f.manager, f.documentA.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	grant := f.rows(t, f.documentA.OrgID)[0].request
	denied := func(name string, want error, member domain.Member, scope kernel.OrgScope, id string) {
		t.Helper()
		content, err := f.service.Open(ctx, scope, member, id, access.Token)
		if err == nil || (want != nil && !errors.Is(err, want)) || len(content.Bytes) != 0 {
			t.Fatalf("%s served content: %v", name, err)
		}
		assertNoRecords(name, 1)
	}
	f.policy.set(f.documentA.OrgID, f.manager.MembershipID, func(m *contentMember) { m.role = roles.Supervisor })
	denied("role change", domain.ErrForbidden, f.manager, f.scopeA, f.documentA.DocumentID)
	f.policy.set(f.documentA.OrgID, f.manager.MembershipID, func(m *contentMember) { m.role = roles.FleetManager; m.expired = true })
	denied("expired session at read", nil, f.manager, f.scopeA, f.documentA.DocumentID)
	f.policy.set(f.documentA.OrgID, f.manager.MembershipID, func(m *contentMember) { m.expired = false })
	denied("capability of another member", domain.ErrNotFound, f.colleague, f.scopeA, f.documentA.DocumentID)
	denied("capability in another organization", domain.ErrNotFound, f.foreign, f.scopeB, f.documentB.DocumentID)
	f.policy.mu.Lock()
	f.policy.revoked[grant] = true
	f.policy.mu.Unlock()
	denied("revoked capability", domain.ErrNotFound, f.manager, f.scopeA, f.documentA.DocumentID)
	f.policy.mu.Lock()
	delete(f.policy.revoked, grant)
	f.policy.mu.Unlock()
	f.advance(5 * time.Minute)
	denied("expired capability", domain.ErrNotFound, f.manager, f.scopeA, f.documentA.DocumentID)
	f.objects.mu.Lock()
	gets := f.objects.gets
	f.objects.mu.Unlock()
	if gets != 0 {
		t.Fatalf("denied reads reached private storage %d times", gets)
	}
}

// B-BR-EMP-208: a refused record write mints no capability and releases no
// bytes, and leaves neither the generic event nor its detail behind.
func TestRefusedAccessRecordServesNothing(t *testing.T) {
	f := newLoggedMedia(t)
	ctx := t.Context()
	access, err := f.service.Issue(ctx, f.scopeA, f.manager, f.documentA.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{f.db.RuntimeRole}.Sanitize()
	if _, err := f.db.Admin.Exec(ctx, "REVOKE INSERT ON org_sensitive_access_events FROM "+role); err != nil {
		t.Fatal(err)
	}
	if grant, err := f.service.Issue(ctx, f.scopeA, f.manager, f.documentA.DocumentID); !errors.Is(err, domain.ErrUnavailable) || grant.Token != "" {
		t.Fatal("refused record issued a capability", err)
	}
	if content, err := f.service.Open(ctx, f.scopeA, f.manager, f.documentA.DocumentID, access.Token); !errors.Is(err, domain.ErrUnavailable) || len(content.Bytes) != 0 {
		t.Fatal("refused record released content", err)
	}
	var events int
	if err := f.db.Admin.QueryRow(ctx, `SELECT count(*) FROM org_audit_events WHERE org_id=$1`, f.documentA.OrgID).Scan(&events); err != nil || events != 1 {
		t.Fatalf("refused record left %d events: %v", events, err)
	}
	if _, err := f.db.Admin.Exec(ctx, "GRANT INSERT ON org_sensitive_access_events TO "+role); err != nil {
		t.Fatal(err)
	}
	if content, err := f.service.Open(ctx, f.scopeA, f.manager, f.documentA.DocumentID, access.Token); err != nil || !bytes.Equal(content.Bytes, f.raw) {
		t.Fatal("restored log refused an authorized read", err)
	}
	if a, _ := f.counts(t); a != 2 {
		t.Fatalf("restored read wrote %d records in total", a)
	}
}
