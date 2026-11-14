//go:build integration

package adapters_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// B-BR-EMP-215/216/218, BUC-EMP-110: one eligible event becomes one scoped
// notice with a re-authorizable panel link; expired authority receives nothing.
func TestNotificationDispatchesOnceToCurrentAuthorizedRecipientsWithLink(t *testing.T) {
	x := setup(t)
	x.member(t, x.a, 601, "OWNER")
	x.member(t, x.a, 602, "FINANCE") // loses the role before dispatch
	x.member(t, x.a, 603, "DRIVER")  // never holds rpt.export.run
	x.member(t, x.a, 604, "FINANCE") // membership ends before dispatch
	x.member(t, x.a, 605, "FINANCE") // suspended before dispatch
	x.member(t, x.a, 606, "FINANCE")
	x.member(t, x.b, 701, "OWNER")
	report := key(900)
	p := x.mustPublish(t, x.a, account(601), report, reportRequest("report:"+report+":ready", report, 601, 602, 603, 604, 605, 606))
	if p.registered[0].Deduplicated || p.registered[0].Key != "report:"+report+":ready" {
		t.Fatal("first registration reported as duplicate")
	}
	request := p.registered[0].RequestID
	if x.count(t, `SELECT count(*) FROM org_notification_requests WHERE org_id=$1 AND dispatched_at IS NULL AND source_event_id=$2`, x.a.OrgID().String(), p.op.ID) != 1 ||
		x.count(t, `SELECT count(*) FROM org_notification_notices`) != 0 || x.count(t, `SELECT count(*) FROM job_queue WHERE kind='org-event'`) != 1 {
		t.Fatal("producer transaction did not leave one pending request and one job")
	}
	x.endRole(t, x.a, 602)
	x.endMembership(t, x.a, 604)
	x.suspend(t, x.a, 605)
	if n := x.drain(t); n != 1 {
		t.Fatal("jobs handled", n)
	}
	want := map[string]string{key(601): "DELIVERED", key(602): "SUPPRESSED:INELIGIBLE", key(603): "SUPPRESSED:INELIGIBLE", key(604): "SUPPRESSED:INELIGIBLE", key(605): "SUPPRESSED:INELIGIBLE", key(606): "DELIVERED"}
	got := x.history(t, x.a, request)
	if len(got) != len(want) {
		t.Fatal("history", got)
	}
	for m, outcome := range want {
		if got[m] != outcome {
			t.Fatal("history outcome", m, got[m], outcome)
		}
	}
	if x.count(t, `SELECT count(*) FROM org_notification_notices WHERE org_id=$1`, x.a.OrgID().String()) != 2 ||
		x.count(t, `SELECT count(*) FROM org_notification_requests WHERE dispatched_at IS NOT NULL`) != 1 ||
		x.count(t, `SELECT count(*) FROM org_event_receipts WHERE consumer=$1`, domain.ConsumerName) != 1 {
		t.Fatal("dispatch effect not exactly once")
	}
	page, err := x.store.ListInbox(t.Context(), x.a, self(601), app.InboxQuery{Limit: 10})
	if err != nil || len(page.Notices) != 1 || page.Next != 0 {
		t.Fatal("owner inbox", err, page)
	}
	n := page.Notices[0]
	if n.Kind != reportKind || n.Template != "test.report.ready.v1" || n.Link != "/reports/"+report || n.Params["report_id"] != report || n.Params["format"] != "CSV" || len(n.Params) != 2 || !n.ReadAt.IsZero() || n.CreatedAt.IsZero() || n.Position < 1 {
		t.Fatal("notice view drift", n)
	}
	// A redelivered or repeated dispatch of the same event changes nothing.
	if err := database.WithOrgTx(t.Context(), x.pool, x.a, func(tx pgx.Tx) error { return x.store.Dispatch(t.Context(), x.a, tx, p.op) }); err != nil {
		t.Fatal(err)
	}
	if x.count(t, `SELECT count(*) FROM org_notification_notices`) != 2 || x.count(t, `SELECT count(*) FROM org_notification_deliveries`) != 6 {
		t.Fatal("repeated dispatch created effects")
	}
	// Listing re-checks the current recipient on every call.
	if page, err := x.store.ListInbox(t.Context(), x.a, self(603), app.InboxQuery{Limit: 10}); err != nil || len(page.Notices) != 0 {
		t.Fatal("current member without notices", err)
	}
	for name, c := range map[string]struct {
		scope kernel.OrgScope
		self  app.Self
		want  error
	}{
		"ended member":          {x.a, self(604), domain.ErrNotFound},
		"suspended member":      {x.a, self(605), domain.ErrSuspended},
		"foreign organization":  {x.b, self(601), domain.ErrNotFound},
		"another member's path": {x.a, app.Self{AccountID: account(603), MembershipID: key(601)}, domain.ErrNotFound},
		"foreign account":       {x.a, app.Self{AccountID: account(701), MembershipID: key(601)}, domain.ErrNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := x.store.ListInbox(t.Context(), c.scope, c.self, app.InboxQuery{Limit: 10}); !errors.Is(err, c.want) {
				t.Fatal("inbox not denied", err)
			}
		})
	}
	if page, err := x.store.ListInbox(t.Context(), x.b, self(701), app.InboxQuery{Limit: 10}); err != nil || len(page.Notices) != 0 {
		t.Fatal("foreign organization saw notices", err)
	}
	// The suspended member regains nothing retroactively: the decision was at dispatch.
	x.exec(t, x.a, `UPDATE org_memberships SET state='ACTIVE',suspension_origin='',revision=revision+1 WHERE org_id=$1 AND id=$2`, x.a.OrgID().String(), key(605))
	if page, err := x.store.ListInbox(t.Context(), x.a, self(605), app.InboxQuery{Limit: 10}); err != nil || len(page.Notices) != 0 {
		t.Fatal("ineligible recipient received a late notice", err)
	}
}

// B-BR-EMP-217: muted optional kinds are suppressed, mandatory kinds are not,
// and the per-recipient hourly bound records LIMITED instead of dropping.
func TestNotificationPreferencesMandatoryKindsAndHourlyBound(t *testing.T) {
	x := setup(t)
	x.member(t, x.a, 601, "OWNER")
	x.member(t, x.a, 603, "DRIVER")
	out, err := x.store.SetPreference(t.Context(), x.a, self(603), app.PreferenceCommand{OperationID: newID(t), Kind: alertKind, Muted: true})
	if err != nil || out.Preference.Revision != 1 || !out.Preference.Muted || out.Replayed {
		t.Fatal("mute", err, out)
	}
	// Storage inconsistency: a mandatory kind can never be muted by its rows.
	if _, err := x.f.Admin.Exec(t.Context(), `INSERT INTO org_notification_preferences(org_id,membership_id,kind,muted,revision) VALUES($1,$2,$3,true,1)`, x.a.OrgID().String(), key(603), accessKind); err != nil {
		t.Fatal(err)
	}
	p := x.mustPublish(t, x.a, account(601), key(900), alertRequest("alert:1", 601, 603), accessRequest("access:1", 603))
	x.drain(t)
	alert, access := x.history(t, x.a, p.registered[0].RequestID), x.history(t, x.a, p.registered[1].RequestID)
	if alert[key(601)] != "DELIVERED" || alert[key(603)] != "SUPPRESSED:MUTED" || access[key(603)] != "DELIVERED" {
		t.Fatal("preference outcome", alert, access)
	}
	if _, err := x.store.SetPreference(t.Context(), x.a, self(603), app.PreferenceCommand{OperationID: newID(t), Kind: accessKind, Muted: true, Revision: 1}); !errors.Is(err, domain.ErrValidation) {
		t.Fatal("mandatory kind muted through the store", err)
	}
	if _, err := x.store.SetPreference(t.Context(), x.a, self(603), app.PreferenceCommand{OperationID: newID(t), Kind: "test.unknown.kind", Muted: true}); !errors.Is(err, domain.ErrValidation) {
		t.Fatal("unknown kind stored", err)
	}
	// 1 + 21 alerts for the owner: at most HourlyLimit are delivered per window.
	var requests []string
	for batch, size := range []int{10, 10, 1} {
		var rs []domain.Request
		for i := range size {
			rs = append(rs, alertRequest("alert:bound:"+string(rune('a'+batch))+":"+string(rune('a'+i)), 601))
		}
		pub := x.mustPublish(t, x.a, account(601), key(901+batch), rs...)
		for _, r := range pub.registered {
			requests = append(requests, r.RequestID)
		}
	}
	x.drain(t)
	delivered := x.count(t, `SELECT count(*) FROM org_notification_deliveries WHERE membership_id=$1 AND outcome='DELIVERED' AND request_id IN (SELECT id FROM org_notification_requests WHERE kind=$2)`, key(601), alertKind)
	limited := x.count(t, `SELECT count(*) FROM org_notification_deliveries WHERE membership_id=$1 AND reason='LIMITED'`, key(601))
	windows := x.count(t, `SELECT count(*) FROM org_notification_budgets WHERE membership_id=$1 AND kind=$2`, key(601), alertKind)
	used := x.count(t, `SELECT COALESCE(sum(used),0) FROM org_notification_budgets WHERE membership_id=$1 AND kind=$2`, key(601), alertKind)
	if delivered+limited != 22 || used != delivered || x.count(t, `SELECT count(*) FROM org_notification_budgets WHERE used>$1`, domain.HourlyLimit) != 0 {
		t.Fatal("bound accounting", delivered, limited, used)
	}
	if windows == 1 && (delivered != domain.HourlyLimit || limited != 2) {
		t.Fatal("single-window bound", delivered, limited)
	}
	if x.count(t, `SELECT count(*) FROM org_notification_notices WHERE membership_id=$1`, key(601)) != delivered {
		t.Fatal("limited recipients received notices")
	}
	if x.count(t, `SELECT count(*) FROM org_notification_deliveries`) != 3+len(requests) {
		t.Fatal("a recipient outcome was silently dropped")
	}
	// The bound is per kind: a mandatory notice still arrives for the owner.
	x.mustPublish(t, x.a, account(601), key(905), accessRequest("access:owner", 601))
	x.drain(t)
	if x.count(t, `SELECT count(*) FROM org_notification_deliveries d JOIN org_notification_requests r ON (r.org_id,r.id)=(d.org_id,d.request_id) WHERE d.membership_id=$1 AND r.kind=$2 AND d.outcome='DELIVERED'`, key(601), accessKind) != 1 {
		t.Fatal("bound leaked across kinds")
	}
}

// B-BR-EMP-215: the producer key returns the first request; changed content
// under the same key is refused and its whole producer transaction rolls back.
func TestNotificationIdempotencyKeyReturnsFirstAndRefusesChangedContent(t *testing.T) {
	x := setup(t)
	x.member(t, x.a, 601, "OWNER")
	report := key(900)
	first := x.mustPublish(t, x.a, account(601), report, reportRequest("report:same", report, 601))
	second := x.mustPublish(t, x.a, account(601), report, reportRequest("report:same", report, 601))
	if !second.registered[0].Deduplicated || second.registered[0].RequestID != first.registered[0].RequestID || first.registered[0].Deduplicated {
		t.Fatal("retry did not return the first request", first.registered, second.registered)
	}
	changed := reportRequest("report:same", report, 601)
	changed.Template = "test.report.ready.v2"
	refused := x.publish(t.Context(), x.a, account(601), report, changed)
	if !errors.Is(refused.inner, domain.ErrReused) || refused.err == nil {
		t.Fatal("changed content under the same key accepted", refused.inner, refused.err)
	}
	if x.count(t, `SELECT count(*) FROM org_audit_events WHERE operation_id=$1`, refused.op.ID) != 0 || x.count(t, `SELECT count(*) FROM org_event_intents WHERE event_id=$1`, refused.op.ID) != 0 {
		t.Fatal("refused producer transaction committed")
	}
	if x.count(t, `SELECT count(*) FROM org_notification_requests`) != 1 || x.drain(t) != 2 {
		t.Fatal("deduplicated producer created another request")
	}
	if x.count(t, `SELECT count(*) FROM org_notification_notices`) != 1 || x.count(t, `SELECT count(*) FROM org_notification_deliveries`) != 1 {
		t.Fatal("one eligible event produced more than one notice")
	}
}

// BUC-EMP-110: concurrent duplicate producers leave one request and concurrent
// duplicate dispatchers (bypassing the orgevent receipt) leave one notice.
func TestNotificationConcurrentDuplicateProducersAndDispatchers(t *testing.T) {
	x := setup(t)
	x.member(t, x.a, 601, "OWNER")
	x.member(t, x.a, 606, "FINANCE")
	report := key(900)
	var wg sync.WaitGroup
	results := make(chan published, 12)
	start := make(chan struct{})
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- x.publish(context.Background(), x.a, account(601), report, reportRequest("report:race", report, 601, 606))
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	firsts := 0
	var request string
	for p := range results {
		if p.err != nil || p.inner != nil || len(p.registered) != 1 {
			t.Fatal("concurrent duplicate producer failed", p.err, p.inner)
		}
		if !p.registered[0].Deduplicated {
			firsts++
		}
		if request != "" && p.registered[0].RequestID != request {
			t.Fatal("duplicate producers saw different requests")
		}
		request = p.registered[0].RequestID
	}
	if firsts != 1 || x.count(t, `SELECT count(*) FROM org_notification_requests`) != 1 {
		t.Fatal("concurrent duplicates created requests", firsts)
	}
	var source event.Event
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT source_event_id::text,source_event_name,source_target_type,source_target_id::text FROM org_notification_requests`).Scan(&source.ID, &source.Name, &source.TargetType, &source.TargetID); err != nil {
		t.Fatal(err)
	}
	source.Revision = 1
	errs := make(chan error, 12)
	begin := make(chan struct{})
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-begin
			errs <- database.WithOrgTx(context.Background(), x.pool, x.a, func(tx pgx.Tx) error { return x.store.Dispatch(context.Background(), x.a, tx, source) })
		}()
	}
	close(begin)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent dispatch failed", err)
		}
	}
	if x.count(t, `SELECT count(*) FROM org_notification_notices`) != 2 || x.count(t, `SELECT count(*) FROM org_notification_deliveries`) != 2 ||
		x.count(t, `SELECT COALESCE(sum(used),0) FROM org_notification_budgets`) != 2 {
		t.Fatal("concurrent dispatch duplicated effects")
	}
	if n := x.drain(t); n != 12 {
		t.Fatal("queued producer jobs", n)
	}
	if x.count(t, `SELECT count(*) FROM org_notification_notices`) != 2 || x.count(t, `SELECT count(*) FROM org_event_receipts`) != 12 {
		t.Fatal("queued redelivery duplicated notices")
	}
	// Different events contend on the same recipients' budget rows: every
	// dispatch commits (no lock-order deadlock) and each budget counts exactly.
	var sources []event.Event
	for i := range 6 {
		p := x.mustPublish(t, x.a, account(601), key(910+i), alertRequest("alert:race:"+string(rune('a'+i)), 601, 606), accessRequest("access:race:"+string(rune('a'+i)), 606, 601))
		sources = append(sources, p.op)
	}
	errs = make(chan error, len(sources))
	begin = make(chan struct{})
	for _, source := range sources {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-begin
			errs <- database.WithOrgTx(context.Background(), x.pool, x.a, func(tx pgx.Tx) error { return x.store.Dispatch(context.Background(), x.a, tx, source) })
		}()
	}
	close(begin)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("cross-event concurrent dispatch failed", err)
		}
	}
	for _, kind := range []string{alertKind, accessKind} {
		for _, m := range []int{601, 606} {
			if used := x.count(t, `SELECT COALESCE(sum(used),0) FROM org_notification_budgets WHERE membership_id=$1 AND kind=$2`, key(m), kind); used != 6 {
				t.Fatal("budget lost an update", kind, m, used)
			}
		}
	}
	if x.count(t, `SELECT count(*) FROM org_notification_notices`) != 2+24 {
		t.Fatal("cross-event notices", x.count(t, `SELECT count(*) FROM org_notification_notices`))
	}
}

// B-BR-EMP-215: private content, foreign recipients and unaudited sources are
// refused at the producer port before anything commits.
func TestNotificationProducerPortRefusesPrivateContentAndForeignTargets(t *testing.T) {
	x := setup(t)
	x.member(t, x.a, 601, "OWNER")
	x.member(t, x.b, 701, "OWNER")
	report := key(900)
	attachment := reportRequest("report:attachment", report, 601)
	attachment.Params["attachment"] = key(950)
	text := reportRequest("report:text", report, 601)
	text.Params["format"] = "Synthetic Driver Name"
	for name, r := range map[string]domain.Request{
		"private attachment":  attachment,
		"free text":           text,
		"foreign recipient":   reportRequest("report:foreign", report, 701),
		"unknown recipient":   reportRequest("report:unknown", report, 999),
		"foreign + own mixed": reportRequest("report:mixed", report, 601, 701),
	} {
		t.Run(name, func(t *testing.T) {
			p := x.publish(t.Context(), x.a, account(601), report, r)
			if !errors.Is(p.inner, domain.ErrInvalid) || p.err == nil {
				t.Fatal("unsafe request accepted", p.inner, p.err)
			}
		})
	}
	if x.count(t, `SELECT count(*) FROM org_notification_requests`) != 0 || x.count(t, `SELECT count(*) FROM org_audit_events`) != 0 || x.count(t, `SELECT count(*) FROM job_queue`) != 0 {
		t.Fatal("refused producer work committed")
	}
	// A request must belong to an audited source operation of the same scope.
	unaudited := event.Event{ID: key(951), Name: eventName, TargetType: targetType, TargetID: report, Revision: 1}
	err := database.WithOrgTx(t.Context(), x.pool, x.a, func(tx pgx.Tx) error {
		_, err := x.store.RequestTx(t.Context(), tx, x.a, unaudited, []domain.Request{reportRequest("report:unaudited", report, 601)})
		return err
	})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("request without an audited source accepted", err)
	}
	err = database.WithOrgTx(t.Context(), x.pool, x.a, func(tx pgx.Tx) error {
		_, err := x.store.RequestTx(t.Context(), tx, x.b, unaudited, []domain.Request{reportRequest("report:scope", report, 701)})
		return err
	})
	if !errors.Is(err, domain.ErrDenied) {
		t.Fatal("mismatched producer scope accepted", err)
	}
	for _, bad := range []event.Event{{}, {ID: "x", Name: eventName, TargetType: targetType, TargetID: report, Revision: 1}, {ID: key(951), Name: "bad", TargetType: targetType, TargetID: report, Revision: 1}, {ID: key(951), Name: eventName, TargetType: targetType, TargetID: report}} {
		err = database.WithOrgTx(t.Context(), x.pool, x.a, func(tx pgx.Tx) error {
			_, err := x.store.RequestTx(t.Context(), tx, x.a, bad, []domain.Request{reportRequest("report:bad-source", report, 601)})
			return err
		})
		if !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("invalid source event accepted", err)
		}
	}
	if _, err := x.store.RequestTx(t.Context(), nil, x.a, unaudited, nil); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatal("producer port without transaction", err)
	}
	// Dispatch for a different source than the stored binding is denied and
	// leaves the request pending for its own event.
	p := x.mustPublish(t, x.a, account(601), report, reportRequest("report:bound", report, 601))
	forged := p.op
	forged.TargetID = key(952)
	err = database.WithOrgTx(t.Context(), x.pool, x.a, func(tx pgx.Tx) error { return x.store.Dispatch(t.Context(), x.a, tx, forged) })
	if !errors.Is(err, domain.ErrDenied) || x.count(t, `SELECT count(*) FROM org_notification_requests WHERE dispatched_at IS NULL`) != 1 {
		t.Fatal("forged source dispatched", err)
	}
	if x.drain(t) != 1 || x.count(t, `SELECT count(*) FROM org_notification_notices`) != 1 {
		t.Fatal("bound source did not dispatch")
	}
}

// BR-TEN-007/BR-PRIV-003: forced RLS hides every table without the scope and
// immutable history cannot be rewritten even by the migration owner.
func TestNotificationRLSAndAppendOnlyGuards(t *testing.T) {
	x := setup(t)
	x.member(t, x.a, 601, "OWNER")
	report := key(900)
	x.mustPublish(t, x.a, account(601), report, reportRequest("report:rls", report, 601))
	x.drain(t)
	tables := []string{"org_notification_requests", "org_notification_recipients", "org_notification_notices", "org_notification_deliveries", "org_notification_preferences", "org_notification_budgets", "org_notification_commands"}
	for _, table := range tables {
		var forced, enabled bool
		if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity,relforcerowsecurity FROM pg_class WHERE oid=$1::regclass`, table).Scan(&enabled, &forced); err != nil || !enabled || !forced {
			t.Fatal("RLS not forced", table, err)
		}
		var n int
		if err := x.pool.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatal("unscoped runtime read rows", table, n, err)
		}
		if err := database.WithOrgTx(t.Context(), x.pool, x.b, func(tx pgx.Tx) error { return tx.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n) }); err != nil || n != 0 {
			t.Fatal("foreign scope read rows", table, n, err)
		}
	}
	if _, err := x.pool.Exec(t.Context(), `INSERT INTO org_notification_preferences(org_id,membership_id,kind,muted,revision) VALUES($1,$2,$3,true,1)`, x.a.OrgID().String(), key(601), alertKind); !sqlState(err, "42501") {
		t.Fatal("unscoped insert accepted", err)
	}
	if err := database.WithOrgTx(t.Context(), x.pool, x.b, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_notification_preferences(org_id,membership_id,kind,muted,revision) VALUES($1,$2,$3,true,1)`, x.a.OrgID().String(), key(601), alertKind)
		return err
	}); !sqlState(err, "42501") {
		t.Fatal("foreign-scope insert accepted", err)
	}
	if _, err := x.store.SetPreference(t.Context(), x.a, self(601), app.PreferenceCommand{OperationID: newID(t), Kind: alertKind, Muted: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := x.store.MarkRead(t.Context(), x.a, self(601), app.ReadCommand{OperationID: newID(t), NoticeID: x.noticeOf(t, 601)}); err != nil {
		t.Fatal(err)
	}
	for name, sql := range map[string]string{
		"rewrite history":       `UPDATE org_notification_deliveries SET outcome='SUPPRESSED',reason='MUTED',notice_id=NULL`,
		"delete history":        `DELETE FROM org_notification_deliveries`,
		"truncate history":      `TRUNCATE org_notification_deliveries`,
		"move recipient":        `UPDATE org_notification_recipients SET membership_id=membership_id`,
		"delete receipt":        `DELETE FROM org_notification_commands`,
		"change request":        `UPDATE org_notification_requests SET kind='test.other.kind'`,
		"redispatch request":    `UPDATE org_notification_requests SET dispatched_at=clock_timestamp()`,
		"delete request":        `DELETE FROM org_notification_requests`,
		"unread notice":         `UPDATE org_notification_notices SET read_at=NULL`,
		"move notice":           `UPDATE org_notification_notices SET membership_id=membership_id,read_at=read_at`,
		"delete notice":         `DELETE FROM org_notification_notices`,
		"skip revision":         `UPDATE org_notification_preferences SET revision=revision+2`,
		"delete preference":     `DELETE FROM org_notification_preferences`,
		"budget jump":           `UPDATE org_notification_budgets SET used=used+5`,
		"budget beyond the cap": `UPDATE org_notification_budgets SET used=21`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := x.f.Admin.Exec(t.Context(), sql); err == nil {
				t.Fatal("guard accepted", sql)
			}
		})
	}
	_, err := x.f.Admin.Exec(t.Context(), `INSERT INTO org_notification_requests(org_id,id,idempotency_key,source_event_id,source_event_name,source_target_type,source_target_id,kind,template,params,content_hash,recipient_count)
		SELECT org_id,$1,'orphan',source_event_id,source_event_name,source_target_type,source_target_id,kind,template,params,content_hash,1 FROM org_notification_requests LIMIT 1`, key(953))
	if !sqlState(err, "23514") {
		t.Fatal("request without recipients committed", err)
	}
	if _, err := x.f.Admin.Exec(t.Context(), `INSERT INTO org_notification_preferences(org_id,membership_id,kind,muted,revision) VALUES($1,$2,$3,true,4)`, x.a.OrgID().String(), key(601), accessKind); !sqlState(err, "23514") {
		t.Fatal("preference started beyond revision 1", err)
	}
}

// B-BR-EMP-218: own-inbox commands are replay-safe, revisioned and scoped.
func TestNotificationMemberCommandsReplayRevisionAndOwnership(t *testing.T) {
	x := setup(t)
	x.member(t, x.a, 601, "OWNER")
	x.member(t, x.a, 603, "DRIVER")
	x.mustPublish(t, x.a, account(601), key(900), accessRequest("access:1", 601, 603))
	x.drain(t)
	mine, theirs := x.noticeOf(t, 601), x.noticeOf(t, 603)
	op := newID(t)
	read, err := x.store.MarkRead(t.Context(), x.a, self(601), app.ReadCommand{OperationID: op, NoticeID: mine})
	if err != nil || read.Replayed || read.Notice.ReadAt.IsZero() || read.Notice.ID != mine {
		t.Fatal("mark read", err, read)
	}
	replay, err := x.store.MarkRead(t.Context(), x.a, self(601), app.ReadCommand{OperationID: op, NoticeID: mine})
	if err != nil || !replay.Replayed || !replay.Notice.ReadAt.Equal(read.Notice.ReadAt) {
		t.Fatal("replay", err, replay)
	}
	if _, err := x.store.MarkRead(t.Context(), x.a, self(601), app.ReadCommand{OperationID: op, NoticeID: theirs}); !errors.Is(err, domain.ErrReused) {
		t.Fatal("reused operation with another target", err)
	}
	if _, err := x.store.MarkRead(t.Context(), x.a, self(603), app.ReadCommand{OperationID: op, NoticeID: theirs}); !errors.Is(err, domain.ErrReused) {
		t.Fatal("another member reused the operation", err)
	}
	if _, err := x.store.MarkRead(t.Context(), x.a, self(601), app.ReadCommand{OperationID: newID(t), NoticeID: theirs}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("marked another member's notice", err)
	}
	if _, err := x.store.MarkRead(t.Context(), x.a, self(601), app.ReadCommand{OperationID: newID(t), NoticeID: key(999)}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("unknown notice", err)
	}
	again, err := x.store.MarkRead(t.Context(), x.a, self(601), app.ReadCommand{OperationID: newID(t), NoticeID: mine})
	if err != nil || again.Replayed || !again.Notice.ReadAt.Equal(read.Notice.ReadAt) {
		t.Fatal("idempotent read changed the read time", err)
	}
	if x.count(t, `SELECT count(*) FROM org_notification_notices WHERE id=$1 AND read_at IS NULL`, theirs) != 1 {
		t.Fatal("other member's read state changed")
	}
	if page, err := x.store.ListInbox(t.Context(), x.a, self(601), app.InboxQuery{UnreadOnly: true, Limit: 10}); err != nil || len(page.Notices) != 0 {
		t.Fatal("unread filter", err)
	}
	set := func(op string, muted bool, revision int64) (app.PreferenceResult, error) {
		return x.store.SetPreference(t.Context(), x.a, self(601), app.PreferenceCommand{OperationID: op, Kind: alertKind, Muted: muted, Revision: revision})
	}
	op = newID(t)
	if out, err := set(op, true, 0); err != nil || out.Replayed || out.Preference.Revision != 1 || !out.Preference.Muted {
		t.Fatal("create preference", err, out)
	}
	if out, err := set(op, true, 0); err != nil || !out.Replayed || out.Preference.Revision != 1 {
		t.Fatal("preference replay", err, out)
	}
	if _, err := set(op, false, 0); !errors.Is(err, domain.ErrReused) {
		t.Fatal("reused preference operation", err)
	}
	for _, stale := range []int64{0, 2} {
		if _, err := set(newID(t), false, stale); !errors.Is(err, domain.ErrConflict) {
			t.Fatal("stale revision accepted", stale, err)
		}
	}
	if out, err := set(newID(t), false, 1); err != nil || out.Preference.Revision != 2 || out.Preference.Muted {
		t.Fatal("update preference", err, out)
	}
	stored, err := x.store.Preferences(t.Context(), x.a, self(601))
	if err != nil || len(stored) != 1 || stored[0].Kind != alertKind || stored[0].Revision != 2 || stored[0].Muted {
		t.Fatal("stored preferences", err, stored)
	}
	// Concurrent identical commands: one change, the rest replay it.
	same := newID(t)
	type result struct {
		out app.PreferenceResult
		err error
	}
	var wg sync.WaitGroup
	outs := make(chan result, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := set(same, true, 2)
			outs <- result{out, err}
		}()
	}
	wg.Wait()
	close(outs)
	changes := 0
	for r := range outs {
		if r.err != nil || r.out.Preference.Revision != 3 {
			t.Fatal("concurrent replay", r.err, r.out)
		}
		if !r.out.Replayed {
			changes++
		}
	}
	// Concurrent different commands on one revision: exactly one wins.
	outs = make(chan result, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := set(newID(t), false, 3)
			outs <- result{out, err}
		}()
	}
	wg.Wait()
	close(outs)
	wins := 0
	for r := range outs {
		switch {
		case r.err == nil:
			wins++
		case !errors.Is(r.err, domain.ErrConflict):
			t.Fatal("unexpected concurrent preference failure", r.err)
		}
	}
	if changes != 1 || wins != 1 || x.count(t, `SELECT revision FROM org_notification_preferences WHERE membership_id=$1 AND kind=$2`, key(601), alertKind) != 4 {
		t.Fatal("concurrent preference commands", changes, wins)
	}
	// Ended membership: every own-inbox command is refused at once (BR-AUTH-014).
	x.endMembership(t, x.a, 603)
	if _, err := x.store.MarkRead(t.Context(), x.a, self(603), app.ReadCommand{OperationID: newID(t), NoticeID: theirs}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("ended member marked a notice", err)
	}
	if _, err := x.store.Preferences(t.Context(), x.a, self(603)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("ended member read preferences", err)
	}
	if _, err := x.store.SetPreference(t.Context(), x.a, self(603), app.PreferenceCommand{OperationID: newID(t), Kind: alertKind, Muted: true}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("ended member changed preferences", err)
	}
}

func TestNotificationInboxKeysetPagination(t *testing.T) {
	x := setup(t)
	x.member(t, x.a, 601, "OWNER")
	for i := range 5 {
		x.mustPublish(t, x.a, account(601), key(900+i), accessRequest("access:page:"+string(rune('a'+i)), 601))
	}
	x.drain(t)
	seen := map[string]bool{}
	var after, last int64
	pages := 0
	for {
		page, err := x.store.ListInbox(t.Context(), x.a, self(601), app.InboxQuery{Limit: 2, After: after})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, n := range page.Notices {
			if seen[n.ID] || (last != 0 && n.Position >= last) {
				t.Fatal("page order or duplicate", n.ID)
			}
			seen[n.ID], last = true, n.Position
		}
		if page.Next == 0 {
			break
		}
		if len(page.Notices) != 2 || page.Next != page.Notices[1].Position {
			t.Fatal("next cursor drift", page)
		}
		after = page.Next
	}
	if len(seen) != 5 || pages != 3 {
		t.Fatal("pagination", len(seen), pages)
	}
}

func (x *fx) noticeOf(t *testing.T, member int) string {
	t.Helper()
	var id string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_notification_notices WHERE membership_id=$1 ORDER BY position LIMIT 1`, key(member)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func sqlState(err error, code string) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == code
}
