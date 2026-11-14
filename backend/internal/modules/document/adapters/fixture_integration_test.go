//go:build integration

package adapters_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	roles "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

// roleAuthority is the current role snapshot that CurrentAuthority loads in
// production. Coverage comes from the real catalog; SELF ("S") cells cover
// only documents the member authored. The composed HTTP suite in
// organization/adapters proves the real source through orgaccess Prepare.
type roleAuthority struct {
	mu      sync.Mutex
	members map[string]map[string]roleState
	checks  int
}
type roleState struct {
	account   string
	role      roles.Role
	suspended bool
}

func (a *roleAuthority) CheckDocumentTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, policy access.Policy, facts app.Facts) error {
	if err := database.ValidateOrgTx(ctx, tx, scope); err != nil {
		return access.ErrUnavailable
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.checks++
	state, exists := a.members[scope.OrgID().String()][actor.MembershipID]
	if !exists || state.account != actor.AccountID {
		return access.ErrNoMembership
	}
	if state.suspended {
		return access.ErrMembershipSuspended
	}
	rule, err := roles.SystemCatalog().Lookup(state.role, policy.Permission)
	if err != nil {
		return access.ErrUnavailable
	}
	if rule.Coverage() == roles.Denied || rule.Coverage() == roles.OwnRecords && facts.AuthoredBy != actor.MembershipID {
		return roles.DenyPermission(policy.Permission)
	}
	return nil
}

// CheckTargetTx applies the same snapshot to a target owner's permission and
// resolves the target only after a positive permission (EMP-DOC-003).
func (a *roleAuthority) CheckTargetTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, policy access.Policy, resolve func(context.Context) (app.Facts, error)) error {
	if err := database.ValidateOrgTx(ctx, tx, scope); err != nil {
		return access.ErrUnavailable
	}
	a.mu.Lock()
	state, exists := a.members[scope.OrgID().String()][actor.MembershipID]
	a.mu.Unlock()
	if !exists || state.account != actor.AccountID {
		return access.ErrNoMembership
	}
	if state.suspended {
		return access.ErrMembershipSuspended
	}
	rule, err := roles.SystemCatalog().Lookup(state.role, policy.Permission)
	if err != nil {
		return access.ErrUnavailable
	}
	if rule.Coverage() == roles.Denied {
		return roles.DenyPermission(policy.Permission)
	}
	facts, err := resolve(ctx)
	if err != nil {
		return err
	}
	if rule.Coverage() == roles.OwnRecords && facts.AuthoredBy != actor.MembershipID {
		return roles.DenyPermission(policy.Permission)
	}
	return nil
}
func (a *roleAuthority) set(scope kernel.OrgScope, actor app.Actor, change func(*roleState)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.members[scope.OrgID().String()][actor.MembershipID]
	change(&state)
	a.members[scope.OrgID().String()][actor.MembershipID] = state
}

type docFixture struct {
	db                        *orgtest.Fixture
	scopeA, scopeB            kernel.OrgScope
	authority                 *roleAuthority
	auditStore                *auditpg.Store
	store                     *adapters.Store
	driver, manager, approver app.Actor
	supervisor, foreign       app.Actor
}

func newID(t *testing.T) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newDocFixture(t *testing.T) *docFixture {
	t.Helper()
	f := &docFixture{db: orgtest.New(t)}
	role := pgx.Identifier{f.db.RuntimeRole}.Sanitize()
	if _, err := f.db.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+role+
		"; GRANT SELECT, INSERT ON org_audit_events, org_sensitive_access_events TO "+role+
		"; GRANT SELECT, INSERT, UPDATE ON org_documents TO "+role+
		// EMP-DOC-014: declarations and received uploads queue their safety jobs.
		"; GRANT SELECT, INSERT ON job_queue TO "+role); err != nil {
		t.Fatal(err)
	}
	f.scopeA, f.scopeB = f.db.Organizations[0].Scope(t), f.db.Organizations[1].Scope(t)
	actor := func() app.Actor { return app.Actor{AccountID: newID(t), MembershipID: newID(t), Channel: "web"} }
	f.driver, f.manager, f.approver, f.supervisor, f.foreign = actor(), actor(), actor(), actor(), actor()
	f.authority = &roleAuthority{members: map[string]map[string]roleState{
		f.scopeA.OrgID().String(): {
			f.driver.MembershipID:     {account: f.driver.AccountID, role: roles.Driver},
			f.manager.MembershipID:    {account: f.manager.AccountID, role: roles.FleetManager},
			f.approver.MembershipID:   {account: f.approver.AccountID, role: roles.Approver},
			f.supervisor.MembershipID: {account: f.supervisor.AccountID, role: roles.Supervisor},
		},
		f.scopeB.OrgID().String(): {f.foreign.MembershipID: {account: f.foreign.AccountID, role: roles.FleetManager}},
	}}
	catalog, err := audit.NewCatalog(append(adapters.AuditDefinitions(), mediapg.AuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	f.auditStore = auditpg.NewStore(f.db.Runtime, catalog)
	f.store = &adapters.Store{Pool: f.db.Runtime, Authority: f.authority, Audit: f.auditStore}
	return f
}

func syntheticJPEG(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../evidence/adapters/media/testdata/valid-4x4.jpg")
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("SYNTHETIC-DOCUMENT-ORIGINAL")
	out := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(marker) + 2)}, marker...)
	return append(out, raw[2:]...)
}

func declare(t *testing.T, raw []byte) app.Create {
	t.Helper()
	sum := sha256.Sum256(raw)
	return app.Create{OperationID: newID(t), DocumentID: newID(t), Original: app.Declaration{Kind: app.KindReceipt, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw)), MediaType: app.MediaTypeJPEG}}
}

// count reads one scoped aggregate through the ordinary runtime login.
func (f *docFixture) count(t *testing.T, scope kernel.OrgScope, query string, args ...any) int {
	t.Helper()
	var n int
	if err := database.WithOrgTx(t.Context(), f.db.Runtime, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), query, args...).Scan(&n)
	}); err != nil {
		t.Fatal(err)
	}
	return n
}
