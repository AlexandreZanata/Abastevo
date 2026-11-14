//go:build integration

package adapters_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const exportMigration = "000207_org_export_jobs.sql"

// exportVersion is derived from the file name so a renumbering at
// integration changes only exportMigration.
var exportVersion = exportMigration[:6]

func newID(t *testing.T) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func pgCode(err error) string {
	var failure *pgconn.PgError
	if errors.As(err, &failure) {
		return failure.Code
	}
	return ""
}

// grantExports provisions only the disposable ordinary runtime login with the
// privileges recorded for the runtime role (proposed README text).
func grantExports(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_export_jobs TO "+role+
		"; GRANT SELECT,INSERT ON org_audit_events,org_sensitive_access_events TO "+role+
		"; GRANT SELECT,INSERT,UPDATE ON job_queue TO "+role+
		"; GRANT SELECT ON org_memberships TO "+role); err != nil {
		t.Fatal(err)
	}
}

// person is a synthetic ACTIVE member created by the migration owner. The
// fixture organizations are bare roots without an onboarding snapshot, so the
// owner provisions the identity rows with triggers suspended; the export
// guard itself still checks the ACTIVE membership. The company pipeline on
// onboarded organizations is proven in organization/adapters.
type person struct{ account, member string }

func newPerson(t *testing.T, f *orgtest.Fixture, scope kernel.OrgScope) person {
	t.Helper()
	p := person{account: newID(t), member: newID(t)}
	employee := newID(t)
	org := scope.OrgID().String()
	ctx := t.Context()
	tx, err := f.Admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`SELECT set_config('anpfuel.org_id',$1,true)`, []any{org}},
		{`SET LOCAL session_replication_role = replica`, nil},
		{`INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, []any{p.account, "synexport" + p.account[:8]}},
		{`INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, []any{org, employee}},
		{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state,started_at) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','ACTIVE',clock_timestamp())`, []any{org, p.member, employee, p.account}},
	} {
		if _, err := tx.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal("synthetic member fixture", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return p
}

// testAuthority emulates the export permission port on the real database:
// the actor must hold the stated ACTIVE membership, and a requester whose
// membership is marked revoked has lost its rights. A data permission marked
// withheld is refused to every member (EMP-AUTH-011); each data check is
// counted. The concrete company pipeline is proven in organization/adapters.
type testAuthority struct {
	mu       sync.Mutex
	revoked  map[string]bool
	withheld map[permissions.Permission]bool
	checks   []permissions.Permission
	outage   bool
}

func (a *testAuthority) withhold(read permissions.Permission, withheld bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.withheld == nil {
		a.withheld = map[permissions.Permission]bool{}
	}
	a.withheld[read] = withheld
}

func (a *testAuthority) dataChecks() []permissions.Permission {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]permissions.Permission(nil), a.checks...)
}

func (a *testAuthority) CheckDataTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, member string, read permissions.Permission) error {
	if actor.Validate() != nil || !app.ExportableRead(read) {
		return app.ErrUnavailable
	}
	ok, err := a.active(ctx, tx, scope, actor.AccountID, member)
	if err != nil {
		return app.ErrUnavailable
	}
	if !ok {
		return access.ErrNoMembership
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.checks = append(a.checks, read)
	if a.withheld[read] {
		return permissions.DenyPermission(read)
	}
	return nil
}

func (a *testAuthority) revoke(member string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revoked[member] = true
}

func (a *testAuthority) active(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, account, member string) (bool, error) {
	if err := database.ValidateOrgTx(ctx, tx, scope); err != nil {
		return false, app.ErrUnavailable
	}
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_memberships WHERE org_id=$1 AND id=$2 AND account_id=$3 AND state='ACTIVE')`, scope.OrgID().String(), member, account).Scan(&ok)
	return ok, err
}

func (a *testAuthority) CheckExportTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.Actor, member string, action app.Action) error {
	if actor.Validate() != nil || app.Policy(action).Validate() != nil {
		return app.ErrUnauthenticated
	}
	ok, err := a.active(ctx, tx, scope, actor.AccountID, member)
	if err != nil {
		return app.ErrUnavailable
	}
	if !ok {
		return access.ErrNoMembership
	}
	return nil
}

func (a *testAuthority) CheckRequesterTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, r app.Requester, kind string, read permissions.Permission) error {
	a.mu.Lock()
	revoked, outage, withheld := a.revoked[r.MembershipID], a.outage, a.withheld[read]
	a.checks = append(a.checks, read)
	a.mu.Unlock()
	if outage {
		return app.ErrUnavailable
	}
	if !domain.ValidKind(kind) || !app.ExportableRead(read) {
		return app.ErrUnavailable
	}
	if withheld {
		return app.ErrRightsLost
	}
	ok, err := a.active(ctx, tx, scope, r.AccountID, r.MembershipID)
	if err != nil {
		return app.ErrUnavailable
	}
	if !ok || revoked {
		return app.ErrRightsLost
	}
	return nil
}

// rowsFormat is the synthetic, test-only kind. It writes a header and the
// requested number of rows and can be held to prove single processing.
type rowsFormat struct {
	limits  domain.Limits
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (*rowsFormat) Kind() string                           { return "test.synthetic_rows" }
func (*rowsFormat) ReadPermission() permissions.Permission { return "exp.expense.read" }
func (*rowsFormat) MediaType() string                      { return "text/csv" }
func (f *rowsFormat) Limits() domain.Limits                { return f.limits }
func (*rowsFormat) Canonical(raw []byte) ([]byte, error)   { return raw, nil }
func (f *rowsFormat) Produce(ctx context.Context, in app.ProduceInput, sink *domain.Sink) error {
	f.mu.Lock()
	f.calls++
	started, release := f.started, f.release
	f.mu.Unlock()
	if started != nil {
		started <- struct{}{}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	var params struct {
		Rows int `json:"rows"`
	}
	if err := json.Unmarshal(in.Parameters, &params); err != nil {
		return err
	}
	if _, err := sink.Write([]byte("row_id,value_centavos\n")); err != nil {
		return err
	}
	for i := 1; i <= params.Rows; i++ {
		if err := sink.Row([]byte(fmt.Sprintf("%d,%d\n", i, i*100))); err != nil {
			return err
		}
	}
	return nil
}

func (f *rowsFormat) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type harness struct {
	f             *orgtest.Fixture
	a, b          kernel.OrgScope
	authority     *testAuthority
	format        *rowsFormat
	formats       app.Registry
	store         *adapters.Store
	objects       app.Objects
	s3            *adapters.S3
	tokens        *adapters.Tokens
	recorder      auditpg.AccessLog
	queue         *jobs.Queue
	worker        app.Worker
	storageConfig adapters.StorageConfig
	requester     person
	colleague     person
	foreign       person
}

func newHarness(t *testing.T, realStorage bool) *harness {
	t.Helper()
	f := orgtest.New(t)
	grantExports(t, f)
	h := &harness{f: f, a: f.Organizations[0].Scope(t), b: f.Organizations[1].Scope(t), authority: &testAuthority{revoked: map[string]bool{}},
		format: &rowsFormat{limits: domain.Limits{Rows: 50, Bytes: 4096, Duration: 2 * time.Second}}}
	var err error
	h.formats, err = app.NewRegistry(h.format)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(adapters.AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	auditStore := auditpg.NewStore(f.Runtime, catalog)
	h.recorder = auditpg.AccessLog{Runner: auditStore}
	h.store = &adapters.Store{Pool: f.Runtime, Authority: h.authority, Audit: auditStore}
	h.tokens, err = adapters.NewTokens("https://api.example.invalid", []byte(strings.Repeat("synthetic-", 4)))
	if err != nil {
		t.Fatal(err)
	}
	h.objects = adapters.Unconfigured{}
	if realStorage {
		h.storageConfig = storageConfig(t)
		h.s3, err = adapters.NewS3(h.storageConfig)
		if err != nil {
			t.Fatal("disposable storage configuration refused", err)
		}
		h.objects = h.s3
	}
	h.queue = jobs.NewQueue(f.Runtime)
	h.worker = app.Worker{Store: h.store, Formats: h.formats, Objects: h.objects, NewID: jobs.NewUUIDv4, LeaseTTL: app.DefaultLeaseTTL}
	h.requester, h.colleague, h.foreign = newPerson(t, f, h.a), newPerson(t, f, h.a), newPerson(t, f, h.b)
	return h
}

func storageConfig(t *testing.T) adapters.StorageConfig {
	t.Helper()
	required := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("explicit disposable %s required", name)
		}
		return value
	}
	return adapters.StorageConfig{Endpoint: required("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: required("ANPFUEL_TEST_STORAGE_BUCKET"), Region: required("ANPFUEL_TEST_STORAGE_REGION"),
		Credentials: storage.Credentials{AccessKeyID: required("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: required("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")}, AllowLoopback: true}
}

func (h *harness) actor(p person) app.Actor {
	return app.Actor{AccountID: p.account, SessionID: p.member}
}

// command builds a validated request of the synthetic kind.
func (h *harness) command(t *testing.T, scope kernel.OrgScope, rows int) app.Command {
	t.Helper()
	c, err := app.NewCommand(scope, h.formats, app.CommandInput{OperationID: newID(t), Kind: "test.synthetic_rows", Parameters: []byte(fmt.Sprintf(`{"rows":%d}`, rows))})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (h *harness) request(t *testing.T, scope kernel.OrgScope, p person, rows int) app.View {
	t.Helper()
	v, replayed, err := h.store.Request(t.Context(), scope, h.actor(p), p.member, newID(t), h.command(t, scope, rows))
	if err != nil || replayed || v.State != domain.Queued {
		t.Fatal("request", err)
	}
	return v
}

func (h *harness) state(t *testing.T, scope kernel.OrgScope, export string) (domain.State, string, int, bool) {
	t.Helper()
	var state, failure string
	var attempts int
	var object bool
	if err := h.f.Admin.QueryRow(t.Context(), `SELECT state,COALESCE(failure_code,''),attempts,object_sha256 IS NOT NULL FROM org_export_jobs WHERE org_id=$1 AND id=$2`, scope.OrgID().String(), export).Scan(&state, &failure, &attempts, &object); err != nil {
		t.Fatal(err)
	}
	return domain.State(state), failure, attempts, object
}

// decisionFor completes the real orgaccess pipeline for a FINANCE member.
func decisionFor(t *testing.T, scope kernel.OrgScope, p person, action app.Action) (context.Context, access.Decision) {
	t.Helper()
	now := time.Now().UTC()
	role, err := permissions.RehydrateAssignment(scope, permissions.AssignmentSnapshot{OrgID: scope.OrgID(), ID: newID(t), MembershipID: p.member, AssignedBy: p.member, Role: permissions.Finance, Scope: permissions.OrgScope, TargetID: scope.OrgID().String(), ValidFrom: now.Add(-time.Hour), Reason: "Synthetic finance fixture", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	a := access.Authorization{Source: sourceOf{access.Authority{OrgID: scope.OrgID(), AccountID: p.account, MembershipID: p.member, EmployeeID: newID(t), MembershipState: members.MembershipActive, EmployeeState: members.EmployeeActive, Model: members.ExistingAccount, OrganizationState: organization.Active, Assignments: []permissions.Assignment{role}}}, Capabilities: capabilityOK{}, Records: recordsOK{}, Now: func() time.Time { return now }}
	ctx, finish := access.BeginRequest(t.Context())
	t.Cleanup(finish)
	prepared, err := a.Prepare(ctx, scope, access.Actor{AccountID: p.account, Channel: "web"}, app.Policy(action), rootOf{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := a.Complete(ctx, prepared, allow{})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, d
}

type sourceOf struct{ value access.Authority }

func (s sourceOf) LoadAuthority(context.Context, kernel.OrgScope, string) (access.Authority, error) {
	return s.value, nil
}

type rootOf struct{}

func (rootOf) Resolve(_ context.Context, s kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	return permissions.ResourceFacts{OrgID: s.OrgID(), ID: s.OrgID().String()}, nil
}

type allow struct{}

func (allow) Confirm(context.Context, access.Subject, access.Policy) error { return nil }

type capabilityOK struct{}

func (capabilityOK) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy) error {
	return nil
}

type recordsOK struct{}

func (recordsOK) Check(context.Context, kernel.OrgScope, access.Subject, access.Policy, permissions.ResourceFacts, []permissions.Rule) error {
	return nil
}

func (h *harness) service() app.Service {
	return app.Service{Enabled: true, Repository: h.store, Formats: h.formats, Objects: h.objects, Tokens: h.tokens, Recorder: h.recorder, NewID: jobs.NewUUIDv4, Now: time.Now}
}

// runBuild drains exactly the build kind through the real Dispatcher.
func (h *harness) runBuild(t *testing.T) bool {
	t.Helper()
	d := &jobs.Dispatcher{Queue: h.queue, Handlers: map[string]jobs.Handler{adapters.BuildKind: adapters.BuildHandler{Worker: h.worker}}, WorkerID: "export-test", Kind: adapters.BuildKind, LeaseTTL: time.Minute, RetryDelay: time.Millisecond}
	found, err := d.RunOnce(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func (h *harness) queueRows(t *testing.T, kind string) []struct {
	payload, dedupe, status, lastError string
	attempts                           int
	notBefore                          time.Time
} {
	t.Helper()
	rows, err := h.f.Admin.Query(t.Context(), `SELECT payload::text,COALESCE(dedupe_key,''),status,last_error,attempts,not_before FROM job_queue WHERE kind=$1 ORDER BY created_at`, kind)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []struct {
		payload, dedupe, status, lastError string
		attempts                           int
		notBefore                          time.Time
	}
	for rows.Next() {
		var r struct {
			payload, dedupe, status, lastError string
			attempts                           int
			notBefore                          time.Time
		}
		if err := rows.Scan(&r.payload, &r.dedupe, &r.status, &r.lastError, &r.attempts, &r.notBefore); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}
