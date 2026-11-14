//go:build integration

package adapters_test

import (
	"errors"
	"strings"
	"sync"
	"testing"

	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	reviewpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/adapters"
	reviewapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// periodGrant provisions only the disposable ordinary runtime role: the
// transition insert and the trigger-maintained head (EDR-148).
func periodGrant(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	role := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_finance_period_transitions TO "+role+";GRANT SELECT,INSERT,UPDATE ON org_finance_periods TO "+role); err != nil {
		t.Fatal(err)
	}
}

func periodStoreFor(t *testing.T, fx legalFixtureState) *reviewpg.PeriodStore {
	t.Helper()
	catalog, err := audit.NewCatalog(reviewpg.PeriodAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return &reviewpg.PeriodStore{Pool: fx.f.Runtime, Authority: &adapters.PeriodAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}

type periodFixtureState struct {
	deviceFixtureState
	periods *reviewpg.PeriodStore
}

// periodFixture is an ACTIVE organization with the creator as OWNER.
func periodFixture(t *testing.T) periodFixtureState {
	t.Helper()
	fx := devicesFixture(t)
	periodGrant(t, fx.f)
	return periodFixtureState{fx, periodStoreFor(t, fx.legalFixtureState)}
}

func periodOwner(fx legalFixtureState) reviewapp.PeriodActor {
	return reviewapp.PeriodActor{AccountID: fx.actor.AccountID, SessionID: fx.actor.SessionID}
}

func periodPersonActor(p devicePerson) reviewapp.PeriodActor {
	return reviewapp.PeriodActor{AccountID: p.web.AccountID, SessionID: p.web.SessionID}
}

func periodCmd(t *testing.T, action period.Action, month string, expected int64, reason string) reviewapp.PeriodCommand {
	t.Helper()
	c, err := reviewapp.NewPeriodCommand(reviewapp.PeriodCommandInput{Action: action, OperationID: onboardingID(t), Month: month, ExpectedRevision: expected, Reason: reason})
	if err != nil {
		t.Fatal("invalid fixture period command", err)
	}
	return c
}

func periodWrite(t *testing.T, fx legalFixtureState, s *reviewpg.PeriodStore, actor reviewapp.PeriodActor, member string, c reviewapp.PeriodCommand) (reviewapp.PeriodAck, bool, error) {
	t.Helper()
	return s.TransitionPeriod(t.Context(), fx.scope, actor, member, onboardingID(t), c)
}

// periodRows counts this organization's transitions, heads and period audit events.
func periodRows(t *testing.T, fx legalFixtureState) (int, int, int) {
	t.Helper()
	var transitions, heads, events int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_finance_period_transitions WHERE org_id=$1),(SELECT count(*) FROM org_finance_periods WHERE org_id=$1),(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_type='finance_period')`, fx.scope.OrgID().String()).Scan(&transitions, &heads, &events); err != nil {
		t.Fatal(err)
	}
	return transitions, heads, events
}

func periodPrivateSnapshot(t *testing.T, fx legalFixtureState) string {
	t.Helper()
	var out string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,period_month,revision) FROM org_finance_period_transitions x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY org_id,period_month) FROM org_finance_periods x),(SELECT count(*) FROM org_audit_events))::text`).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// BUC-EMP-108, B-BR-EMP-212: lock, reopen with reason and re-lock append three
// immutable transitions with one audit event each; the read keeps all history.
func TestPeriodStoreLockReopenRelockAppendsHistoryWithOneAuditEach(t *testing.T) {
	fx := periodFixture(t)
	s, owner := fx.periods, periodOwner(fx.legalFixtureState)
	open, err := s.ReadPeriod(t.Context(), fx.scope, owner, fx.member, mustMonth(t, "2026-10"))
	if err != nil || open.State != period.Open || open.Revision != 1 || open.PeriodID != "" || len(open.History) != 0 || open.Start != "2026-10-01" || open.End != "2026-11-01" {
		t.Fatal("untouched month is not OPEN at the implicit revision 1", err)
	}
	locked, replayed, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, periodCmd(t, period.ActionClose, "2026-10", 1, ""))
	if err != nil || replayed || locked.Kind != period.Lock || locked.State != period.Locked || locked.Revision != 2 || locked.Month != "2026-10" || locked.PeriodID == "" {
		t.Fatal("close", err)
	}
	reason := "  Synthetic VAL-19 volume correction  "
	reopened, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, periodCmd(t, period.ActionReopen, "2026-10", 2, reason))
	if err != nil || reopened.Kind != period.Reopen || reopened.State != period.Reopened || reopened.Revision != 3 || reopened.PeriodID != locked.PeriodID {
		t.Fatal("reopen", err)
	}
	relocked, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, periodCmd(t, period.ActionClose, "2026-10", 3, ""))
	if err != nil || relocked.Kind != period.Relock || relocked.State != period.Locked || relocked.Revision != 4 || relocked.PeriodID != locked.PeriodID {
		t.Fatal("re-lock", err)
	}
	if !locked.RecordedAt.Before(reopened.RecordedAt) || !reopened.RecordedAt.Before(relocked.RecordedAt) {
		t.Fatal("server receipt times are not ordered")
	}
	view, err := s.ReadPeriod(t.Context(), fx.scope, owner, fx.member, mustMonth(t, "2026-10"))
	if err != nil || view.State != period.Locked || view.Revision != 4 || view.PeriodID != locked.PeriodID || len(view.History) != 3 {
		t.Fatal("history read", err)
	}
	for i, want := range []struct {
		kind      period.Kind
		from, to  period.State
		authority string
		reason    string
	}{{period.Lock, period.Open, period.Locked, period.ClosePermission, ""}, {period.Reopen, period.Locked, period.Reopened, period.ReopenPermission, reason}, {period.Relock, period.Reopened, period.Locked, period.ClosePermission, ""}} {
		h := view.History[i]
		if h.Revision != int64(i+2) || h.Kind != want.kind || h.From != want.from || h.To != want.to || h.Authority != want.authority || h.Reason != want.reason || h.ActorAccountID != fx.actor.AccountID || h.ActorMembershipID != fx.member || h.OperationID == "" || h.RecordedAt.IsZero() {
			t.Fatalf("history entry %d lost a fact", i)
		}
	}
	transitions, heads, events := periodRows(t, fx.legalFixtureState)
	if transitions != 3 || heads != 1 || events != 3 {
		t.Fatal("not exactly one transition and one audit event per write", transitions, heads, events)
	}
	var matched bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT bool_and(e.action=CASE x.kind WHEN 'LOCK' THEN 'fin.period.locked' WHEN 'REOPEN' THEN 'fin.period.reopened' ELSE 'fin.period.relocked' END AND e.actor_id=x.actor_account_id AND e.target_id=p.id AND e.request_hash=x.request_hash) AND count(*)=3
	 FROM org_finance_period_transitions x JOIN org_finance_periods p USING(org_id,period_month) JOIN org_audit_events e ON(e.org_id,e.operation_id)=(x.org_id,x.operation_id) WHERE x.org_id=$1`, fx.scope.OrgID().String()).Scan(&matched); err != nil || !matched {
		t.Fatal("audit events do not reference their transitions", err)
	}
	page, err := s.ListPeriods(t.Context(), fx.scope, owner, fx.member, reviewapp.PeriodFilter{Limit: 10})
	if err != nil || len(page.Periods) != 1 || page.Periods[0].Month != "2026-10" || page.Periods[0].Revision != 4 || len(page.Periods[0].History) != 0 || page.NextMonth != "" {
		t.Fatal("list", err)
	}
}

func mustMonth(t *testing.T, raw string) period.Month {
	t.Helper()
	m, err := period.ParseMonth(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Negative cases of B-BR-EMP-212: stale revision, double close/lock, reopen of
// a non-locked period; every refusal changes nothing.
func TestPeriodStoreRefusalsChangeNothing(t *testing.T) {
	fx := periodFixture(t)
	s, owner := fx.periods, periodOwner(fx.legalFixtureState)
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, periodCmd(t, period.ActionReopen, "2026-10", 1, "Synthetic reason")); !errors.Is(err, period.ErrTransition) {
		t.Fatal("reopened a period that was never locked", err)
	}
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, periodCmd(t, period.ActionClose, "2026-10", 2, "")); !errors.Is(err, period.ErrConflict) {
		t.Fatal("future expected revision accepted", err)
	}
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); err != nil {
		t.Fatal(err)
	}
	before := periodPrivateSnapshot(t, fx.legalFixtureState)
	for name, c := range map[string]struct {
		cmd  reviewapp.PeriodCommand
		want error
	}{
		"stale close":            {periodCmd(t, period.ActionClose, "2026-10", 1, ""), period.ErrConflict},
		"double close":           {periodCmd(t, period.ActionClose, "2026-10", 2, ""), period.ErrTransition},
		"stale reopen":           {periodCmd(t, period.ActionReopen, "2026-10", 1, "Synthetic reason"), period.ErrConflict},
		"reopen future revision": {periodCmd(t, period.ActionReopen, "2026-10", 3, "Synthetic reason"), period.ErrConflict},
	} {
		if _, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, c.cmd); !errors.Is(err, c.want) {
			t.Fatal(name, err)
		}
	}
	if periodPrivateSnapshot(t, fx.legalFixtureState) != before {
		t.Fatal("a refused period command changed history, head or audit")
	}
}

// B-BR-EMP-213: replay returns the original result once; any reuse of the
// operation ID by another command, account or module is refused.
func TestPeriodStoreReplayAndOperationReuse(t *testing.T) {
	fx := periodFixture(t)
	s, owner := fx.periods, periodOwner(fx.legalFixtureState)
	close := periodCmd(t, period.ActionClose, "2026-10", 1, "")
	first, replayed, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, close)
	if err != nil || replayed {
		t.Fatal(err)
	}
	reason := "Synthetic reopening for replay"
	reopenInput := reviewapp.PeriodCommandInput{Action: period.ActionReopen, OperationID: onboardingID(t), Month: "2026-10", ExpectedRevision: 2, Reason: reason}
	reopen, err := reviewapp.NewPeriodCommand(reopenInput)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, reopen)
	if err != nil {
		t.Fatal(err)
	}
	before := periodPrivateSnapshot(t, fx.legalFixtureState)
	// The original close is replayed with its original result although the
	// period has moved on: same account, same canonical command.
	again, replayed, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, close)
	if err != nil || !replayed || again.Revision != first.Revision || again.State != period.Locked || again.Kind != period.Lock || !again.RecordedAt.Equal(first.RecordedAt) || again.PeriodID != first.PeriodID {
		t.Fatal("original close acknowledgement changed on replay", err)
	}
	if again, replayed, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, reopen); err != nil || !replayed || again.Revision != second.Revision {
		t.Fatal("reopen replay", err)
	}
	for name, input := range map[string]reviewapp.PeriodCommandInput{
		"other reason":   {Action: period.ActionReopen, OperationID: reopenInput.OperationID, Month: "2026-10", ExpectedRevision: 2, Reason: reason + "."},
		"other month":    {Action: period.ActionReopen, OperationID: reopenInput.OperationID, Month: "2026-09", ExpectedRevision: 2, Reason: reason},
		"other revision": {Action: period.ActionReopen, OperationID: reopenInput.OperationID, Month: "2026-10", ExpectedRevision: 4, Reason: reason},
		"other action":   {Action: period.ActionClose, OperationID: reopenInput.OperationID, Month: "2026-10", ExpectedRevision: 3},
	} {
		c, err := reviewapp.NewPeriodCommand(input)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, c); !errors.Is(err, reviewapp.ErrOperationReused) {
			t.Fatal("operation reuse accepted:", name, err)
		}
	}
	// Another FINANCE account replaying the owner's operation ID is a reuse.
	finance := deviceNewPerson(t, fx.deviceFixtureState)
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: finance.member}, "FINANCE", "ORG", "ACTIVE")
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, periodPersonActor(finance), finance.member, reopen); !errors.Is(err, reviewapp.ErrOperationReused) {
		t.Fatal("another account replayed the owner's command", err)
	}
	// An operation ID already recorded by another audited module is refused.
	var foreign string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT operation_id::text FROM org_audit_events WHERE org_id=$1 AND target_type<>'finance_period' LIMIT 1`, fx.scope.OrgID().String()).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	reused, err := reviewapp.NewPeriodCommand(reviewapp.PeriodCommandInput{Action: period.ActionClose, OperationID: foreign, Month: "2026-11", ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, reused); !errors.Is(err, reviewapp.ErrOperationReused) {
		t.Fatal("foreign operation ID accepted", err)
	}
	if periodPrivateSnapshot(t, fx.legalFixtureState) != before {
		t.Fatal("replay or reuse wrote a second transition or audit event")
	}
}

// B-BR-EMP-213: authority is the existing company pipeline at receipt, on the
// whole organization. Each refusal writes nothing.
func TestPeriodStoreAuthorizationAtReceipt(t *testing.T) {
	fx := periodFixture(t)
	s := fx.periods
	org := fx.scope.OrgID().String()
	driver := deviceNewPerson(t, fx.deviceFixtureState) // DRIVER/SELF from the fixture.
	finance := deviceNewPerson(t, fx.deviceFixtureState)
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: finance.member}, "FINANCE", "ORG", "ACTIVE")
	admin := deviceNewPerson(t, fx.deviceFixtureState)
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: admin.member}, "ADMIN", "ORG", "ACTIVE")
	before := periodPrivateSnapshot(t, fx.legalFixtureState)
	for name, p := range map[string]devicePerson{"driver": driver, "admin": admin} {
		if _, _, err := periodWrite(t, fx.legalFixtureState, s, periodPersonActor(p), p.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); !errors.Is(err, permissions.ErrPermissionDenied) {
			t.Fatal(name+" closed a period", err)
		}
		if _, err := s.ReadPeriod(t.Context(), fx.scope, periodPersonActor(p), p.member, mustMonth(t, "2026-10")); !errors.Is(err, permissions.ErrPermissionDenied) {
			t.Fatal(name+" read a period", err)
		}
	}
	// A member's identity cannot be borrowed by another account.
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, periodPersonActor(driver), finance.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); !errors.Is(err, access.ErrNoMembership) {
		t.Fatal("borrowed membership accepted", err)
	}
	if periodPrivateSnapshot(t, fx.legalFixtureState) != before {
		t.Fatal("refused authority changed period state")
	}
	ack, _, err := periodWrite(t, fx.legalFixtureState, s, periodPersonActor(finance), finance.member, periodCmd(t, period.ActionClose, "2026-10", 1, ""))
	if err != nil || ack.Revision != 2 {
		t.Fatal("FINANCE could not close", err)
	}
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, periodPersonActor(finance), finance.member, periodCmd(t, period.ActionReopen, "2026-10", 2, "Synthetic finance reopening")); err != nil {
		t.Fatal("FINANCE could not reopen", err)
	}
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	// A FINANCE grant that does not cover the whole organization (SELF here;
	// unit scopes are not persistable before EMP-AUTH-006) cannot close it.
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND membership_id=$2 AND ended_at IS NULL`, org, finance.member); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'FINANCE','SELF',$3,clock_timestamp(),$4,'Synthetic self finance')`, org, onboardingID(t), finance.member, fx.member); err != nil {
		t.Fatal(err)
	}
	before = periodPrivateSnapshot(t, fx.legalFixtureState)
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, periodPersonActor(finance), finance.member, periodCmd(t, period.ActionClose, "2026-10", 3, "")); !errors.Is(err, permissions.ErrPermissionDenied) {
		t.Fatal("non-organization grant closed an organization-wide period", err)
	}
	// Another organization's member is opaque not-found.
	if _, _, err := s.TransitionPeriod(t.Context(), other.scope, periodPersonActor(driver), driver.member, onboardingID(t), periodCmd(t, period.ActionClose, "2026-10", 1, "")); !errors.Is(err, access.ErrNoMembership) {
		t.Fatal("foreign member reached another organization", err)
	}
	// Suspended organization keeps reads and refuses writes.
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='SUSPENDED',suspension_reason='SECURITY',revision=revision+1 WHERE org_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	owner := periodOwner(fx.legalFixtureState)
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, owner, fx.member, periodCmd(t, period.ActionClose, "2026-10", 3, "")); !errors.Is(err, access.ErrOrgSuspended) {
		t.Fatal("suspended organization closed a period", err)
	}
	if v, err := s.ReadPeriod(t.Context(), fx.scope, owner, fx.member, mustMonth(t, "2026-10")); err != nil || v.Revision != 3 {
		t.Fatal("suspended organization lost period reads", err)
	}
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE organizations SET state='ACTIVE',suspension_reason='',revision=revision+1 WHERE org_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	// Ended membership is terminal.
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, admin.member); err != nil {
		t.Fatal(err)
	}
	if _, _, err := periodWrite(t, fx.legalFixtureState, s, periodPersonActor(admin), admin.member, periodCmd(t, period.ActionClose, "2026-10", 3, "")); !errors.Is(err, access.ErrNoMembership) {
		t.Fatal("ended membership reached periods", err)
	}
	if periodPrivateSnapshot(t, fx.legalFixtureState) != before {
		t.Fatal("refused authority changed period state")
	}
	// A pending-setup organization cannot close periods.
	pending := legalFixture(t)
	periodGrant(t, pending.f)
	if _, _, err := periodWrite(t, pending, periodStoreFor(t, pending), periodOwner(pending), pending.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); !errors.Is(err, access.ErrSetupIncomplete) {
		t.Fatal("pending setup closed a period", err)
	}
}

// Two organizations keep independent periods with identical keys.
func TestPeriodStoreTwoOrganizationIsolation(t *testing.T) {
	fx := periodFixture(t)
	other := legalFixtureFor(t, fx.f, fx.onboarding, fx.actor, fx.flags)
	if _, _, err := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	otherStore := periodStoreFor(t, other)
	if _, _, err := periodWrite(t, other, otherStore, periodOwner(other), other.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); err != nil {
		t.Fatal(err)
	}
	mine, err := fx.periods.ReadPeriod(t.Context(), fx.scope, periodOwner(fx.legalFixtureState), fx.member, mustMonth(t, "2026-10"))
	if err != nil || mine.State != period.Open || mine.Revision != 1 {
		t.Fatal("another organization's lock leaked", err)
	}
	if _, _, err := periodWrite(t, fx.legalFixtureState, fx.periods, periodOwner(fx.legalFixtureState), fx.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); err != nil {
		t.Fatal("identical key in another organization refused", err)
	}
	page, err := fx.periods.ListPeriods(t.Context(), fx.scope, periodOwner(fx.legalFixtureState), fx.member, reviewapp.PeriodFilter{Limit: 100})
	if err != nil || len(page.Periods) != 1 {
		t.Fatal("list crossed organizations", err)
	}
	var ids int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(DISTINCT id) FROM org_finance_periods WHERE period_month='2026-10-01'`).Scan(&ids); err != nil || ids != 2 {
		t.Fatal("identical months share an identity", err)
	}
}

// Eight concurrent closes of the same period admit exactly one.
func TestPeriodStoreConcurrentClosesAdmitOne(t *testing.T) {
	fx := periodFixture(t)
	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := range n {
		c := periodCmd(t, period.ActionClose, "2026-10", 1, "")
		id := onboardingID(t)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, errs[i] = fx.periods.TransitionPeriod(t.Context(), fx.scope, periodOwner(fx.legalFixtureState), fx.member, id, c)
		}()
	}
	close(start)
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.Is(err, period.ErrTransition) || errors.Is(err, period.ErrConflict):
		default:
			t.Fatal("unexpected concurrent refusal", err)
		}
	}
	transitions, heads, events := periodRows(t, fx.legalFixtureState)
	if won != 1 || transitions != 1 || heads != 1 || events != 1 {
		t.Fatal("concurrent closes admitted more or less than one", won, transitions, heads, events)
	}
}

// The schema keeps history append-only, enforces the transition chain, the
// reopening reason and the organization scope even for direct SQL (RLS).
func TestPeriodSchemaKeepsHistoryAppendOnlyAndScoped(t *testing.T) {
	fx := periodFixture(t)
	org := fx.scope.OrgID().String()
	if _, _, err := periodWrite(t, fx.legalFixtureState, fx.periods, periodOwner(fx.legalFixtureState), fx.member, periodCmd(t, period.ActionClose, "2026-10", 1, "")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := periodWrite(t, fx.legalFixtureState, fx.periods, periodOwner(fx.legalFixtureState), fx.member, periodCmd(t, period.ActionReopen, "2026-10", 2, "Synthetic reason")); err != nil {
		t.Fatal(err)
	}
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_finance_period_transitions,org_finance_periods TO "+role); err != nil {
		t.Fatal(err)
	}
	before := periodPrivateSnapshot(t, fx.legalFixtureState)
	code := func(err error, want string) {
		t.Helper()
		var pe *pgconn.PgError
		if !errors.As(err, &pe) || pe.Code != want {
			t.Fatalf("period SQL refusal mismatch, want %s: %v", want, err)
		}
		if strings.Contains(pe.Message+pe.Detail, "Synthetic") {
			t.Fatal("SQL refusal leaked a private reason")
		}
	}
	// "Approval history lost": the earlier lock is never rewritten or removed.
	code(unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_finance_period_transitions SET reason='rewritten' WHERE org_id=$1`, org), "55000")
	code(unitSchemaExec(t, fx.legalFixtureState, `DELETE FROM org_finance_period_transitions WHERE org_id=$1 AND revision=2`, org), "55000")
	if err := unitSchemaExec(t, fx.legalFixtureState, `TRUNCATE org_finance_period_transitions,org_finance_periods`); err == nil {
		t.Fatal("period history truncated")
	}
	code(unitSchemaExec(t, fx.legalFixtureState, `DELETE FROM org_finance_periods WHERE org_id=$1`, org), "55000")
	code(unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_finance_periods SET state='LOCKED',revision=4 WHERE org_id=$1`, org), "23514")
	code(unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_finance_periods SET state='OPEN' WHERE org_id=$1`, org), "23514")
	insert := `INSERT INTO org_finance_period_transitions(org_id,period_month,period_id,revision,kind,from_state,to_state,authority,reason,actor_account_id,actor_membership_id,operation_id,request_hash) SELECT $1,'2026-10-01',id,$2,$3,$4,$5,$6,$7,$8,$9,$10,repeat('a',64) FROM org_finance_periods WHERE org_id=$1 AND period_month='2026-10-01'`
	// A reopening without its reason, a gap, a forged state or authority.
	code(unitSchemaExec(t, fx.legalFixtureState, insert, org, 4, "RELOCK", "REOPENED", "LOCKED", "fin.period.close", "reason", fx.actor.AccountID, fx.member, onboardingID(t)), "23514")
	code(unitSchemaExec(t, fx.legalFixtureState, insert, org, 5, "REOPEN", "LOCKED", "REOPENED", "fin.period.reopen", "Synthetic gap", fx.actor.AccountID, fx.member, onboardingID(t)), "23514")
	code(unitSchemaExec(t, fx.legalFixtureState, insert, org, 4, "RELOCK", "REOPENED", "LOCKED", "fin.period.reopen", "", fx.actor.AccountID, fx.member, onboardingID(t)), "23514")
	code(unitSchemaExec(t, fx.legalFixtureState, insert, org, 3, "REOPEN", "LOCKED", "REOPENED", "fin.period.reopen", "Synthetic overwrite", fx.actor.AccountID, fx.member, onboardingID(t)), "23514")
	// A transition whose operation has no audit event fails at commit.
	if err := unitSchemaExec(t, fx.legalFixtureState, insert, org, 4, "RELOCK", "REOPENED", "LOCKED", "fin.period.close", "", fx.actor.AccountID, fx.member, onboardingID(t)); err == nil {
		t.Fatal("unaudited transition committed")
	}
	reopenInsert := `INSERT INTO org_finance_period_transitions(org_id,period_month,period_id,revision,kind,from_state,to_state,authority,reason,actor_account_id,actor_membership_id,operation_id,request_hash) VALUES($1,'2026-11-01',$2,2,'LOCK','OPEN','LOCKED','fin.period.close','',$3,$4,$5,repeat('a',64))`
	if periodPrivateSnapshot(t, fx.legalFixtureState) != before {
		t.Fatal("refused direct SQL changed period history")
	}
	// RLS: without the organization setting nothing is visible or writable,
	// and a scoped transaction cannot write another organization's rows.
	var visible int
	if err := fx.f.Runtime.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_finance_period_transitions)+(SELECT count(*) FROM org_finance_periods)`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("period rows visible without organization scope", visible, err)
	}
	if _, err := fx.f.Runtime.Exec(t.Context(), reopenInsert, org, onboardingID(t), fx.actor.AccountID, fx.member, onboardingID(t)); err == nil {
		t.Fatal("unscoped period insert accepted")
	}
	other := fx.f.Organizations[0].Scope(t)
	err := database.WithOrgTx(t.Context(), fx.f.Runtime, other, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), reopenInsert, org, onboardingID(t), fx.actor.AccountID, fx.member, onboardingID(t))
		return err
	})
	if err == nil {
		t.Fatal("cross-organization period insert accepted")
	}
	if periodPrivateSnapshot(t, fx.legalFixtureState) != before {
		t.Fatal("unscoped SQL changed period history")
	}
}
