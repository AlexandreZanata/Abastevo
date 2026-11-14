//go:build integration

package adapters_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	eventpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/adapters"
	eventapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/application"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	notifypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Synthetic test-only producer vocabulary. No production producer exists yet.
const (
	eventName  = "test.report.completed"
	targetType = "test.report"
	reportKind = "test.report.ready"
	alertKind  = "test.budget.alert"
	accessKind = "test.access.changed"
)

func key(n int) string     { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }
func account(n int) string { return key(n + 100) }

func scopeOf(t *testing.T, n int) kernel.OrgScope {
	t.Helper()
	id, err := kernel.ParseOrgID(key(n))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func testCatalog(t *testing.T) domain.Catalog {
	t.Helper()
	c, err := domain.NewCatalog([]domain.Kind{
		{Name: reportKind, Permission: "rpt.export.run", Templates: []string{"test.report.ready.v1", "test.report.ready.v2"},
			Params: map[string]domain.Param{"report_id": {Type: domain.ParamID}, "format": {Type: domain.ParamCode, Codes: []string{"CSV", "PDF"}}},
			Link:   "/reports/{report_id}"},
		{Name: alertKind, Templates: []string{"test.budget.alert.v1"}, Params: map[string]domain.Param{"count": {Type: domain.ParamCount}}},
		{Name: accessKind, Mandatory: true, Templates: []string{"test.access.changed.v1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type fx struct {
	f       *orgtest.Fixture
	pool    *pgxpool.Pool
	a, b    kernel.OrgScope
	store   *notifypg.Store
	events  eventapp.Service
	handler *eventpg.Handler
	queue   *jobs.Queue
}

// GrantNotifications provisions only this disposable runtime role.
func grantNotifications(t *testing.T, f *orgtest.Fixture) {
	t.Helper()
	r := pgx.Identifier{f.RuntimeRole}.Sanitize()
	if _, err := f.Admin.Exec(t.Context(), "GRANT USAGE ON SCHEMA public TO "+r+
		";GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships,org_role_assignments TO "+r+
		";GRANT SELECT,UPDATE ON accounts TO "+r+";GRANT SELECT,INSERT ON payment_billing_exits TO "+r+
		";GRANT SELECT,INSERT ON org_audit_events,org_event_intents,org_event_receipts TO "+r+
		";GRANT SELECT,INSERT,UPDATE ON job_queue TO "+r+
		";GRANT SELECT,INSERT ON org_notification_recipients,org_notification_deliveries,org_notification_commands TO "+r+
		";GRANT SELECT,INSERT,UPDATE ON org_notification_requests,org_notification_notices,org_notification_preferences,org_notification_budgets TO "+r); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) *fx {
	t.Helper()
	f := orgtest.New(t)
	f.GrantTeamReductions(t)
	f.GrantSettingsLifecycle(t)
	f.GrantOwnershipLifecycle(t)
	grantNotifications(t, f)
	x := &fx{f: f, pool: f.RuntimePool(t, 12), a: scopeOf(t, 301), b: scopeOf(t, 302)}
	for _, s := range []kernel.OrgScope{x.a, x.b} {
		v, err := org.NewPending(s, org.Profile{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := orgpg.NewStore(f.Runtime).Create(t.Context(), s, v); err != nil {
			t.Fatal(err)
		}
	}
	x.store = &notifypg.Store{Pool: x.pool, Catalog: testCatalog(t), Authority: accesspg.NewStore(x.pool), NewID: jobs.NewUUIDv4}
	ecat, err := event.NewCatalog([]event.Definition{{Name: eventName, TargetType: targetType, Consumers: []string{domain.ConsumerName}}})
	if err != nil {
		t.Fatal(err)
	}
	acat, err := audit.NewCatalog([]audit.Definition{{Action: eventName, TargetType: targetType}})
	if err != nil {
		t.Fatal(err)
	}
	x.events = eventapp.Service{Enabled: true, Catalog: ecat, Audit: auditpg.NewStore(x.pool, acat), Writer: eventpg.NewStore(x.pool)}
	x.handler, err = eventpg.NewHandler(eventpg.NewStore(x.pool), ecat, map[string]eventpg.Effect{domain.ConsumerName: x.store.Dispatch})
	if err != nil {
		t.Fatal(err)
	}
	x.queue = jobs.NewQueue(x.pool)
	return x
}

// member seeds an active synthetic account/employee/membership with one role.
func (x *fx) member(t *testing.T, s kernel.OrgScope, n int, role string) {
	t.Helper()
	if _, err := x.f.Admin.Exec(t.Context(), `INSERT INTO accounts(id,alias,status) VALUES($1,$2,'active')`, account(n), fmt.Sprintf("synthetic-notice-%d", n)); err != nil {
		t.Fatal(err)
	}
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, s, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, s.OrgID().String(), key(n+200)); err != nil {
			return err
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, s.OrgID().String(), key(n), key(n+200), account(n)); err != nil {
			return err
		}
		kind, target := "SELF", key(n)
		if role == "OWNER" {
			kind, target = "ORG", s.OrgID().String()
		}
		if _, err := tx.Exec(t.Context(), `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$3,'synthetic notice role')`,
			s.OrgID().String(), key(n+300), key(n), role, kind, target, time.Now().UTC().Add(-time.Minute)); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, s.OrgID().String(), key(n))
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func (x *fx) exec(t *testing.T, s kernel.OrgScope, sql string, args ...any) {
	t.Helper()
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, s, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), sql, args...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
func (x *fx) endMembership(t *testing.T, s kernel.OrgScope, n int) {
	x.exec(t, s, `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, s.OrgID().String(), key(n))
}
func (x *fx) suspend(t *testing.T, s kernel.OrgScope, n int) {
	x.exec(t, s, `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE org_id=$1 AND id=$2`, s.OrgID().String(), key(n))
}
func (x *fx) endRole(t *testing.T, s kernel.OrgScope, n int) {
	x.exec(t, s, `UPDATE org_role_assignments SET ended_at=clock_timestamp(),revision=revision+1 WHERE org_id=$1 AND id=$2`, s.OrgID().String(), key(n+300))
}

func newID(t *testing.T) string {
	t.Helper()
	id, err := jobs.NewUUIDv4()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type published struct {
	op         event.Event
	registered []domain.Registered
	inner      error
	err        error
}

// publish is the test-only producer: one audited source change whose work
// registers notice requests through the producer port, plus notif.dispatch.
func (x *fx) publish(ctx context.Context, s kernel.OrgScope, actor, target string, requests ...domain.Request) published {
	op, _ := jobs.NewUUIDv4()
	sum := sha256.Sum256([]byte("synthetic-producer:" + op))
	a := audit.Event{OperationID: op, ActorID: actor, Action: eventName, TargetType: targetType, TargetID: target, RequestHash: hex.EncodeToString(sum[:])}
	e := event.Event{ID: op, Name: eventName, TargetType: targetType, TargetID: target, Revision: 1}
	out := published{op: e}
	_, out.err = x.events.Record(ctx, s, a, e, []string{domain.ConsumerName}, func(ctx context.Context, scope kernel.OrgScope, tx pgx.Tx) error {
		out.registered, out.inner = x.store.RequestTx(ctx, tx, scope, e, requests)
		return out.inner
	})
	return out
}

func (x *fx) mustPublish(t *testing.T, s kernel.OrgScope, actor, target string, requests ...domain.Request) published {
	t.Helper()
	p := x.publish(t.Context(), s, actor, target, requests...)
	if p.err != nil || p.inner != nil || len(p.registered) != len(requests) {
		t.Fatal("producer refused", p.err, p.inner)
	}
	return p
}

// drain claims every queued org-event job through the real orgevent handler.
func (x *fx) drain(t *testing.T) int {
	t.Helper()
	n := 0
	for {
		job, err := x.queue.Claim(t.Context(), event.JobKind, "synthetic-notify-worker", time.Minute)
		if errors.Is(err, jobs.ErrNoJob) {
			return n
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := x.handler.Handle(t.Context(), job); err != nil {
			t.Fatal("dispatch failed", err)
		}
		if err := x.queue.Complete(t.Context(), job.ID, job.LeaseToken); err != nil {
			t.Fatal(err)
		}
		n++
	}
}

func (x *fx) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := x.f.Admin.QueryRow(t.Context(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// history maps recipient -> OUTCOME[:REASON] for one request (admin view).
func (x *fx) history(t *testing.T, s kernel.OrgScope, request string) map[string]string {
	t.Helper()
	rows, err := x.f.Admin.Query(t.Context(), `SELECT membership_id::text,channel,outcome,COALESCE(reason,''),notice_id IS NOT NULL FROM org_notification_deliveries WHERE org_id=$1 AND request_id=$2`, s.OrgID().String(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var m, channel, outcome, reason string
		var notice bool
		if err := rows.Scan(&m, &channel, &outcome, &reason, &notice); err != nil {
			t.Fatal(err)
		}
		if channel != string(domain.ChannelInApp) || notice != (outcome == string(domain.Delivered)) || out[m] != "" {
			t.Fatal("history row drift", m, channel, outcome, notice)
		}
		if reason != "" {
			outcome += ":" + reason
		}
		out[m] = outcome
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	return out
}

func reportRequest(idempotency, report string, recipients ...int) domain.Request {
	r := domain.Request{Key: idempotency, Kind: reportKind, Template: "test.report.ready.v1", Params: map[string]string{"report_id": report, "format": "CSV"}}
	for _, n := range recipients {
		r.Recipients = append(r.Recipients, key(n))
	}
	return r
}
func alertRequest(idempotency string, recipients ...int) domain.Request {
	r := domain.Request{Key: idempotency, Kind: alertKind, Template: "test.budget.alert.v1", Params: map[string]string{"count": "3"}}
	for _, n := range recipients {
		r.Recipients = append(r.Recipients, key(n))
	}
	return r
}
func accessRequest(idempotency string, recipients ...int) domain.Request {
	r := domain.Request{Key: idempotency, Kind: accessKind, Template: "test.access.changed.v1"}
	for _, n := range recipients {
		r.Recipients = append(r.Recipients, key(n))
	}
	return r
}

func self(n int) app.Self { return app.Self{AccountID: account(n), MembershipID: key(n)} }
