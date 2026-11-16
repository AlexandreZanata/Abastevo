//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	expensepg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/adapters"
	expenseapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	expense "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/domain"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuelpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/adapters"
	fuelapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
)

// expenseState is the fueling fixture plus the real expense.submit owner
// composed from the same authority sources, fleet lifecycle port and expense
// store as the runtime (EMP-EXP-002).
type expenseState struct {
	fuelingState
	expenseAuthority *orgpg.ExpenseAuthority
	direct           syncapp.Handler
	directs          *syncpg.Store
}

func expenseIntakeConfig(fx syncFixtureState) syncpg.Config {
	config := fuelingIntakeConfig(fx)
	config.NoAsyncPlans[syncapp.OperationKey{Type: expenseapp.OperationType, Schema: 1}] = map[string]syncapp.FollowupMode{protocol.Accepted: syncapp.NoAsyncFollowup, protocol.Rejected: syncapp.NoAsyncFollowup}
	return config
}

func expenseFixture(t *testing.T) expenseState {
	t.Helper()
	x := expenseState{fuelingState: fuelingFixture(t)}
	meterAuthority, ok := x.meters.Authority.(*orgpg.MeterAuthority)
	if !ok {
		t.Fatal("fixture meter authority is not the real one")
	}
	x.expenseAuthority = (*orgpg.ExpenseAuthority)(meterAuthority)
	handler, err := expensepg.NewIntakeHandler(x.expenseAuthority, x.meters, x.expenses)
	if err != nil {
		t.Fatal(err)
	}
	x.direct, x.directs = handler, syncpg.NewStore(expenseIntakeConfig(x.syncFixtureState))
	return x
}

// directOp builds a signed expense.submit of member d on the vehicle: a toll
// of R$ 12,50 paid by the employee (BUC-EMP-226).
func (x expenseState) directOp(t *testing.T, d syncFixtureState, declared time.Time, edit func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	payload := map[string]any{"asset_id": x.vehicle.AssetID, "category": "TOLL", "paid_total": "1250", "company_amount": "1250", "payer": "EMPLOYEE", "trip_id": nil}
	return syncHTTPOperation(t, d.deviceFixtureState, d.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = expenseapp.OperationType
		raw["entity"] = map[string]any{"type": expenseapp.EntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(declared)
		raw["entry_zone"] = "America/Cuiaba"
		raw["device_offset_seconds"] = -14400
		raw["payload"] = payload
		if edit != nil {
			edit(raw, payload)
		}
	})
}

func (x expenseState) processDirect(t *testing.T, d syncFixtureState, op protocol.Operation) (protocol.Acknowledgement, error) {
	t.Helper()
	return x.directs.Process(t.Context(), d.source, op, protocol.DefaultLimits(), x.direct)
}

type expenseCounts struct{ expenses, effects, claims, receipts, audits int }

func (x expenseState) directCount(t *testing.T, op protocol.Operation) expenseCounts {
	t.Helper()
	var c expenseCounts
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_expenses WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),(SELECT count(*) FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2),
(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND operation_id=$2)`, x.scope.OrgID().String(), op.ID()).Scan(&c.expenses, &c.effects, &c.claims, &c.receipts, &c.audits); err != nil {
		t.Fatal(err)
	}
	return c
}

func (x expenseState) directAccepted(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 || len(ack.Warnings()) != 0 {
		t.Fatal("legitimate expense was not accepted", err, ack.Result(), ack.Code())
	}
	if c := x.directCount(t, op); c != (expenseCounts{expenses: 1, effects: 1, claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("accepted expense not stored exactly once: %+v", c)
	}
}

func (x expenseState) directNothing(t *testing.T, op protocol.Operation) {
	t.Helper()
	if c := x.directCount(t, op); c != (expenseCounts{}) {
		t.Fatalf("refused expense left state: %+v", c)
	}
}

func (x expenseState) directMalformed(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if err != nil || ack.Result() != protocol.Rejected || ack.Code() != "MALFORMED" {
		t.Fatal("expected a final MALFORMED receipt", err, ack.Result(), ack.Code())
	}
	if c := x.directCount(t, op); c != (expenseCounts{claims: 1, receipts: 1, audits: 1}) {
		t.Fatalf("malformed expense stored a fact: %+v", c)
	}
}

func (x expenseState) directUnavailable(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error) {
	t.Helper()
	if !errors.Is(err, syncapp.ErrUnavailable) || errors.Is(err, syncapp.ErrSourceDenied) || len(ack.CanonicalBytes()) != 0 {
		t.Fatal("expected RETRY/UNAVAILABLE with nothing stored", err)
	}
	x.directNothing(t, op)
}

func (x expenseState) directRefused(t *testing.T, op protocol.Operation, ack protocol.Acknowledgement, err error, want orgapi.Code) {
	t.Helper()
	if code, _ := orgapi.Registered(err); !errors.Is(err, syncapp.ErrSourceDenied) || code != want || len(ack.CanonicalBytes()) != 0 {
		t.Fatalf("expected %s refusal, got %v", want, err)
	}
	x.directNothing(t, op)
}

// costOf counts the expenses of one fueling and returns the identity of the one.
func (x fuelingState) costOf(t *testing.T, fueling string) (int, string) {
	t.Helper()
	var n int
	var id string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*),COALESCE(min(id::text),'') FROM org_expenses WHERE org_id=$1 AND fueling_id=$2`, x.scope.OrgID().String(), fueling).Scan(&n, &id); err != nil {
		t.Fatal(err)
	}
	return n, id
}

// rawState runs one statement in an organization transaction of the runtime
// role and returns its SQLSTATE ("" when it committed). It never touches the
// test, so racing goroutines may call it.
func rawState(ctx context.Context, fx legalFixtureState, scope kernel.OrgScope, statement string, args ...any) (string, error) {
	err := database.WithOrgTx(ctx, fx.f.Runtime, scope, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, statement, args...)
		return err
	})
	var pe interface{ SQLState() string }
	if errors.As(err, &pe) {
		return pe.SQLState(), nil
	}
	return "", err
}

func sqlState(t *testing.T, fx legalFixtureState, scope kernel.OrgScope, statement string, args ...any) string {
	t.Helper()
	code, err := rawState(t.Context(), fx, scope, statement, args...)
	if err != nil {
		t.Fatal("unexpected non-SQL failure", err)
	}
	return code
}

// secondCost is a raw second FUELING expense of a fueling, with every fact
// equal to the stored fueling's so that only uniqueness can refuse it.
const secondCost = `INSERT INTO org_expenses(org_id,id,origin_kind,source_id,fueling_id,source_revision,operation_id,payload_digest,asset_id,author_membership_id,author_employee_id,enrollment_id,
 category,paid_centavos,company_centavos,payer,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,reporting_day,reporting_zone,reporting_source,unit_id,cost_centre_id,review_state,revision)
VALUES($1,$2,'FUELING',$3,$3,1,$4,$5,$6,$7,$8,$9,'FUEL',$10,$10,$11,$12,$13,'America/Cuiaba',-14400,($12::timestamptz AT TIME ZONE $14::text)::date,$14,'unit',$15,NULL,'SUBMITTED',1)`

type costFacts struct {
	op       protocol.Operation
	paid     int64
	payer    string
	declared time.Time
}

func (x fuelingState) secondCostArgs(t *testing.T, d syncFixtureState, f costFacts) []any {
	t.Helper()
	var zone string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT time_zone FROM org_units WHERE org_id=$1 AND id=$2`, x.scope.OrgID().String(), x.unit).Scan(&zone); err != nil {
		t.Fatal(err)
	}
	floor := f.declared.UTC().Truncate(time.Microsecond)
	return []any{x.scope.OrgID().String(), onboardingID(t), f.op.Snapshot().Entity.ID, f.op.ID(), f.op.PayloadDigest(), x.vehicle.AssetID, d.source.MembershipID, d.person.employee, d.source.EnrollmentID,
		f.paid, f.payer, floor, int16(f.declared.Nanosecond() % 1000), zone, x.unit}
}

// recordedTotals reads the organization's stored expenses through the owner
// and computes the EMP-EXP-001 review-basis totals of one month.
func (x fuelingState) recordedTotals(t *testing.T, ids []string, basis expense.Basis, start, end string) expense.CostTotals {
	t.Helper()
	var entries []expense.Entry
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		for _, id := range ids {
			record, err := x.expenses.ReadExpenseTx(t.Context(), syncapp.Borrow(tx), x.scope, id)
			if err != nil {
				return err
			}
			value, err := record.Expense(x.scope)
			if err != nil {
				return err
			}
			state, err := expense.ParseReviewState(record.ReviewState)
			if err != nil {
				return err
			}
			entries = append(entries, expense.Entry{Expense: value, State: state})
		}
		return nil
	}); err != nil {
		t.Fatal("stored expenses unreadable", err)
	}
	period, err := expense.NewReportingPeriod(start, end)
	if err != nil {
		t.Fatal(err)
	}
	totals, err := expense.Totals(x.scope, period, basis, entries)
	if err != nil {
		t.Fatal("totals refused the stored expenses", err)
	}
	return totals
}

// VAL-13 intake part (BUC-EMP-225, B-BR-EMP-444..446): one fueling of 97 350
// paid by the employee has exactly one FUEL expense of 97 350 whatever is
// resent; the projection re-run and the catch-up change nothing; a second
// cost is refused by the database; the period counts 97 350 once.
func TestExpenseFuelingProjectionStoresOneCostPerFueling(t *testing.T) {
	x := expenseFixture(t)
	d := x.syncFixtureState
	declared := x.declaredNow(t)
	op := x.op(t, d, declared, func(_, p map[string]any) { p["paid_total"], p["payer"] = "97350", "EMPLOYEE" })
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 0, 1)
	org, fueling := x.scope.OrgID().String(), op.Snapshot().Entity.ID
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT count(*)=1 AND bool_and(x.origin_kind='FUELING' AND x.source_id=f.id AND x.fueling_id=f.id AND x.source_revision=1 AND x.id<>f.id
 AND x.operation_id=f.operation_id AND x.payload_digest=f.payload_digest AND x.asset_id=f.asset_id AND x.author_membership_id=f.author_membership_id
 AND x.author_employee_id=f.author_employee_id AND x.enrollment_id=f.enrollment_id AND x.category='FUEL' AND x.paid_centavos=97350 AND x.company_centavos=97350 AND x.payer='EMPLOYEE'
 AND(x.occurred_at,x.occurred_submicro,x.entry_zone,x.device_offset_seconds)=(f.occurred_at,f.occurred_submicro,f.entry_zone,f.device_offset_seconds) AND x.occurred_submicro=123
 AND x.unit_id=$3 AND x.cost_centre_id IS NULL AND x.reporting_source='unit' AND x.reporting_zone=(SELECT time_zone FROM org_units WHERE org_id=$1 AND id=$3)
 AND x.reporting_day=(f.occurred_at AT TIME ZONE x.reporting_zone)::date AND x.review_state='SUBMITTED' AND x.revision=1
 AND x.received_at>=f.received_at AND x.received_at<=(SELECT received_at FROM org_sync_acknowledgements a WHERE a.org_id=f.org_id AND a.operation_id=f.operation_id))
 FROM org_fuelings f JOIN org_expenses x ON(x.org_id,x.fueling_id)=(f.org_id,f.id) WHERE f.org_id=$1 AND f.operation_id=$2`, org, op.ID(), x.unit).Scan(&exact); err != nil || !exact {
		t.Fatal("stored expense is not the exact cost of the stored fueling", err)
	}
	_, cost := x.costOf(t, fueling)
	// Resends and digest reuse never add, change or replace the cost.
	for range 2 {
		replay, err := x.process(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK", err)
		}
	}
	reused := x.op(t, d, declared, func(raw, p map[string]any) {
		raw["operation_id"], raw["entity"] = op.ID(), map[string]any{"type": fuelapp.EntityType, "id": fueling, "expected_revision": nil}
		p["paid_total"], p["payer"] = "194700", "EMPLOYEE"
	})
	refusal, err := x.process(t, d, reused)
	if err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same operation was not refused", err, refusal.Code())
	}
	x.accepted(t, op, ack, nil, 0, 1)
	// EDR-290: running the projection again for the stored fueling, and the
	// catch-up, find the stored cost unchanged and write nothing.
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		if err := x.expenses.ProjectFuelingTx(t.Context(), syncapp.Borrow(tx), x.scope, fueling, 1); err != nil {
			return err
		}
		inserted, err := x.expenses.ProjectPendingFuelingsTx(t.Context(), syncapp.Borrow(tx), x.scope)
		if err == nil && inserted != 0 {
			t.Error("catch-up inserted a second cost", inserted)
		}
		return err
	}); err != nil {
		t.Fatal("idempotent projection re-run", err)
	}
	if n, again := x.costOf(t, fueling); n != 1 || again != cost {
		t.Fatal("re-running the projection changed the cost", n)
	}
	// A second cost for the fueling, with every fact equal, is refused by
	// the unique origin key; it is not stored.
	if code := sqlState(t, x.legalFixtureState, x.scope, secondCost, x.secondCostArgs(t, d, costFacts{op: op, paid: 97350, payer: "EMPLOYEE", declared: declared})...); code != "23505" {
		t.Fatal("a second cost for one fueling was not refused by uniqueness", code)
	}
	if n, _ := x.costOf(t, fueling); n != 1 {
		t.Fatal("second cost stored", n)
	}
	// The month counts the fueling once: 97 350 including pending, 0 approved.
	day := orgtime.FormatInstant(declared)[:8] + "01"
	month, _ := time.Parse("2006-01-02", day)
	end := month.AddDate(0, 1, 0).Format("2006-01-02")
	pending := x.recordedTotals(t, []string{cost}, expense.BasisIncludesPending, day, end)
	if pending.FigureCentavos().Int64() != 97350 || pending.PendingCount() != 1 || !pending.IncludesPending() || pending.ApprovedCentavos().Sign() != 0 {
		t.Fatal("including-pending figure is not one cost of 97 350", pending.FigureCentavos())
	}
	if approved := x.recordedTotals(t, []string{cost}, expense.BasisApproved, day, end); approved.FigureCentavos().Sign() != 0 {
		t.Fatal("a submitted cost was counted as approved")
	}
}

// gatedCosts holds the intake transaction open after the real projection.
type gatedCosts struct {
	inner              *expensepg.Store
	projected, release chan struct{}
}

func (g gatedCosts) ProjectFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, fueling string, revision int64) error {
	err := g.inner.ProjectFuelingTx(ctx, db, scope, fueling, revision)
	g.projected <- struct{}{}
	<-g.release
	return err
}

// waitRuntimeBlocked waits until n runtime sessions wait on a lock.
func waitRuntimeBlocked(t *testing.T, fx legalFixtureState, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked int
		if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND usename=$1 AND wait_event_type='Lock' AND pg_blocking_pids(pid)<>'{}'`, fx.f.RuntimeRole).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked >= n {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("no lock wait observed")
		}
	}
}

// B-BR-EMP-444/EDR-290: a second cost for one fueling races the fueling's
// own transaction in both orders and never commits. Queued behind the open
// fueling transaction it meets the stored cost (23505); taken first, before
// the fueling exists, it finds no source (23503) and the fueling then
// commits with its one cost.
func TestExpenseSecondCostForOneFuelingRacesBothOrders(t *testing.T) {
	x := expenseFixture(t)
	d := x.syncFixtureState
	t.Run("fueling-transaction-first", func(t *testing.T) {
		gate := gatedCosts{inner: x.expenses, projected: make(chan struct{}, 1), release: make(chan struct{})}
		handler, err := fuelpg.NewIntakeHandler(x.authority, x.meters, x.store, gate)
		if err != nil {
			t.Fatal(err)
		}
		declared := x.declaredNow(t)
		op := x.op(t, d, declared, nil)
		type result struct {
			ack protocol.Acknowledgement
			err error
		}
		fueled := make(chan result, 1)
		go func() {
			ack, err := x.fuelings.Process(context.Background(), d.source, op, protocol.DefaultLimits(), handler)
			fueled <- result{ack, err}
		}()
		select {
		case <-gate.projected:
		case got := <-fueled:
			t.Fatal("the fueling ended before its projection", got.err)
		}
		type raw struct {
			code string
			err  error
		}
		second := make(chan raw, 1)
		args := x.secondCostArgs(t, d, costFacts{op: op, paid: 97350, payer: "COMPANY", declared: declared})
		go func() {
			code, err := rawState(context.Background(), x.legalFixtureState, x.scope, secondCost, args...)
			second <- raw{code, err}
		}()
		waitRuntimeBlocked(t, x.legalFixtureState, 1)
		close(gate.release)
		got, queued := <-fueled, <-second
		x.accepted(t, op, got.ack, got.err, 0, 1)
		if queued.err != nil || queued.code != "23505" {
			t.Fatal("the queued second cost was not refused by uniqueness", queued.code, queued.err)
		}
		if n, _ := x.costOf(t, op.Snapshot().Entity.ID); n != 1 {
			t.Fatal("two costs for one fueling", n)
		}
	})
	t.Run("second-cost-first", func(t *testing.T) {
		declared := x.declaredNow(t)
		op := x.op(t, d, declared, nil)
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		holder, err := x.f.Runtime.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = holder.Rollback(context.Background()) }()
		if _, err := holder.Exec(ctx, `SELECT set_config('anpfuel.org_id',$1,true)`, x.scope.OrgID().String()); err != nil {
			t.Fatal(err)
		}
		if _, err := holder.Exec(ctx, `SELECT 1 FROM organizations WHERE org_id=$1 FOR UPDATE`, x.scope.OrgID().String()); err != nil {
			t.Fatal(err)
		}
		type result struct {
			ack protocol.Acknowledgement
			err error
		}
		fueled := make(chan result, 1)
		args := x.secondCostArgs(t, d, costFacts{op: op, paid: 97350, payer: "COMPANY", declared: declared})
		go func() {
			ack, err := x.fuelings.Process(context.Background(), d.source, op, protocol.DefaultLimits(), x.fueling)
			fueled <- result{ack, err}
		}()
		waitRuntimeBlocked(t, x.legalFixtureState, 1)
		_, err = holder.Exec(ctx, secondCost, args...)
		rollback := holder.Rollback(ctx)
		got := <-fueled
		var pe interface{ SQLState() string }
		if !errors.As(err, &pe) || pe.SQLState() != "23503" || rollback != nil {
			t.Fatal("a cost for a fueling not yet stored was not refused as a missing source", err, rollback)
		}
		x.accepted(t, op, got.ack, got.err, 0, 1)
		if n, _ := x.costOf(t, op.Snapshot().Entity.ID); n != 1 {
			t.Fatal("two costs for one fueling", n)
		}
	})
}

type failingCosts struct{ inner *expensepg.Store }

func (f failingCosts) ProjectFuelingTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, fueling string, revision int64) error {
	if err := f.inner.ProjectFuelingTx(ctx, db, scope, fueling, revision); err != nil {
		return err
	}
	return errors.New("synthetic failure after the real expense insert")
}

// skippedCosts is the defect the deferred check exists for: a projection
// that silently stores nothing.
type skippedCosts struct{}

func (skippedCosts) ProjectFuelingTx(context.Context, syncapp.DBTX, kernel.OrgScope, string, int64) error {
	return nil
}

// B-BR-EMP-445 (EDR-287): neither half commits alone. A failure after the
// real expense insert or a projection that stores nothing leaves no fueling,
// reading, expense, claim or receipt; the same operation then records both.
// An expense naming a fueling that is not stored, or a DIRECT expense naming
// a fueling, is refused by the database.
func TestExpenseFuelingPartialTransactionStoresNeither(t *testing.T) {
	x := expenseFixture(t)
	d := x.syncFixtureState
	declared := x.declaredNow(t)
	op := x.op(t, d, declared, nil)
	for name, costs := range map[string]fuelapp.CostProjection{"failure-after-the-expense": failingCosts{x.expenses}, "projection-stores-nothing": skippedCosts{}} {
		t.Run(name, func(t *testing.T) {
			handler, err := fuelpg.NewIntakeHandler(x.authority, x.meters, x.store, costs)
			if err != nil {
				t.Fatal(err)
			}
			ack, err := x.fuelings.Process(t.Context(), d.source, op, protocol.DefaultLimits(), handler)
			x.unavailable(t, op, ack, err)
		})
	}
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 0, 1)
	args := x.secondCostArgs(t, d, costFacts{op: op, paid: 97350, payer: "COMPANY", declared: declared})
	args[2] = onboardingID(t)
	if code := sqlState(t, x.legalFixtureState, x.scope, secondCost, args...); code != "23503" {
		t.Fatal("an expense without its fueling was not refused", code)
	}
	if code := sqlState(t, x.legalFixtureState, x.scope, strings.Replace(secondCost, "'FUELING',$3,$3", "'DIRECT',$2,$3", 1), x.secondCostArgs(t, d, costFacts{op: op, paid: 97350, payer: "COMPANY", declared: declared})...); code != "23514" {
		t.Fatal("a DIRECT expense naming a fueling was not refused", code)
	}
	for _, kind := range []string{"PAYMENT", "REIMBURSEMENT", "DOCUMENT", "WORK_ORDER"} {
		if code := sqlState(t, x.legalFixtureState, x.scope, strings.Replace(secondCost, "'FUELING',$3,$3", "'"+kind+"',$3,$3", 1), x.secondCostArgs(t, d, costFacts{op: op, paid: 97350, payer: "COMPANY", declared: declared})...); code != "23514" {
			t.Fatal("a settlement or unowned origin was stored as a cost", kind, code)
		}
	}
	if n, _ := x.costOf(t, op.Snapshot().Entity.ID); n != 1 {
		t.Fatal("refused inserts changed the one cost", n)
	}
}

// BR-TEN-005 / V-ISO: another organization's real fueling is never a source
// here, and a row of this organization cannot be written from another.
func TestExpenseForeignFuelingIsNeverASource(t *testing.T) {
	x := expenseFixture(t)
	d := x.syncFixtureState
	declared := x.declaredNow(t)
	op := x.op(t, d, declared, nil)
	ack, err := x.process(t, d, op)
	x.accepted(t, op, ack, err, 0, 1)
	foreign := foreignAsset(t, x.fuelingState)
	var otherOrg string
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT org_id::text FROM org_assets WHERE id=$1`, foreign).Scan(&otherOrg); err != nil {
		t.Fatal(err)
	}
	other := scopeFor(t, otherOrg)
	args := x.secondCostArgs(t, d, costFacts{op: op, paid: 97350, payer: "COMPANY", declared: declared})
	args[0] = otherOrg
	if code := sqlState(t, x.legalFixtureState, other, secondCost, args...); code != "23503" {
		t.Fatal("another organization referenced this organization's fueling", code)
	}
	args = x.secondCostArgs(t, d, costFacts{op: op, paid: 97350, payer: "COMPANY", declared: declared})
	if code := sqlState(t, x.legalFixtureState, other, secondCost, args...); code != "42501" {
		t.Fatal("a row of this organization was written from another scope", code)
	}
	var visible int
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, other, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM org_expenses`).Scan(&visible)
	}); err != nil || visible != 0 {
		t.Fatal("expense visible to another organization", visible, err)
	}
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, other, func(tx pgx.Tx) error {
		err := x.expenses.ProjectFuelingTx(t.Context(), syncapp.Borrow(tx), other, op.Snapshot().Entity.ID, 1)
		if !errors.Is(err, expenseapp.ErrNotFound) {
			t.Error("another organization projected this organization's fueling", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n, _ := x.costOf(t, op.Snapshot().Entity.ID); n != 1 {
		t.Fatal("foreign attempts changed the one cost", n)
	}
}

// BUC-EMP-226 / B-BR-EMP-447/449: a signed toll is stored once with the
// vehicle's unit at its instant; resends return the stored ACK; digest reuse
// is refused; twelve identical deliveries leave one expense.
func TestExpenseDirectIntakeStoresOneExpenseAndReplays(t *testing.T) {
	x := expenseFixture(t)
	d := x.syncFixtureState
	declared := x.declaredNow(t)
	op := x.directOp(t, d, declared, nil)
	ack, err := x.processDirect(t, d, op)
	x.directAccepted(t, op, ack, err)
	org := x.scope.OrgID().String()
	var exact bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT x.origin_kind='DIRECT' AND x.source_id=x.id AND x.fueling_id IS NULL AND x.source_revision=1 AND x.id=$3 AND x.payload_digest=$4
 AND x.asset_id=$5 AND x.author_membership_id=$6 AND x.author_employee_id=$7 AND x.enrollment_id=$8 AND x.category='TOLL' AND x.paid_centavos=1250 AND x.company_centavos=1250
 AND x.payer='EMPLOYEE' AND x.occurred_submicro=123 AND x.entry_zone='America/Cuiaba' AND x.device_offset_seconds=-14400 AND x.unit_id=$9 AND x.cost_centre_id IS NULL
 AND x.reporting_source='unit' AND x.reporting_zone=(SELECT time_zone FROM org_units WHERE org_id=$1 AND id=$9) AND x.reporting_day=(x.occurred_at AT TIME ZONE x.reporting_zone)::date
 AND x.review_state='SUBMITTED' AND x.revision=1 AND x.received_at<=(SELECT received_at FROM org_sync_acknowledgements a WHERE a.org_id=x.org_id AND a.operation_id=x.operation_id)
 FROM org_expenses x WHERE x.org_id=$1 AND x.operation_id=$2`, org, op.ID(), op.Snapshot().Entity.ID, op.PayloadDigest(), x.vehicle.AssetID, d.source.MembershipID, d.person.employee, d.source.EnrollmentID, x.unit).Scan(&exact); err != nil || !exact {
		t.Fatal("stored expense is not the exact original fact", err)
	}
	for range 2 {
		replay, err := x.processDirect(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("resend did not return the stored ACK", err)
		}
	}
	reused := x.directOp(t, d, declared, func(raw, p map[string]any) {
		raw["operation_id"], raw["entity"] = op.ID(), map[string]any{"type": expenseapp.EntityType, "id": op.Snapshot().Entity.ID, "expected_revision": nil}
		p["paid_total"], p["company_amount"] = "2500", "2500"
	})
	refusal, err := x.processDirect(t, d, reused)
	if err != nil || refusal.Result() != protocol.Rejected || refusal.Code() != "OPERATION_ID_REUSED" {
		t.Fatal("mismatched digest under the same operation was not refused", err, refusal.Code())
	}
	x.directAccepted(t, op, ack, nil)
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT paid_centavos=1250 AND payload_digest=$3 FROM org_expenses WHERE org_id=$1 AND operation_id=$2`, org, op.ID(), op.PayloadDigest()).Scan(&exact); err != nil || !exact {
		t.Fatal("digest reuse rewrote the stored expense", err)
	}
	// A partial company amount and a known zero are stored exactly.
	partial := x.directOp(t, d, declared, func(_, p map[string]any) {
		p["category"], p["paid_total"], p["company_amount"], p["payer"] = "OTHER", "10000", "8000", "COMPANY"
	})
	ack, err = x.processDirect(t, d, partial)
	x.directAccepted(t, partial, ack, err)
	zero := x.directOp(t, d, declared, func(_, p map[string]any) { p["category"], p["paid_total"], p["company_amount"] = "FINE", "0", "0" })
	ack, err = x.processDirect(t, d, zero)
	x.directAccepted(t, zero, ack, err)
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT(SELECT paid_centavos=10000 AND company_centavos=8000 AND payer='COMPANY' AND category='OTHER' FROM org_expenses WHERE org_id=$1 AND operation_id=$2)
 AND(SELECT paid_centavos=0 AND company_centavos=0 AND category='FINE' FROM org_expenses WHERE org_id=$1 AND operation_id=$3)`, org, partial.ID(), zero.ID()).Scan(&exact); err != nil || !exact {
		t.Fatal("exact amounts changed", err)
	}
	// Twelve concurrent identical deliveries on a twelve-connection pool.
	config := expenseIntakeConfig(x.syncFixtureState)
	config.Pool = x.f.RuntimePool(t, 12)
	store := syncpg.NewStore(config)
	raced := x.directOp(t, d, declared, nil)
	var wg sync.WaitGroup
	acks := make([]protocol.Acknowledgement, 12)
	errs := make([]error, 12)
	for i := range 12 {
		wg.Go(func() {
			acks[i], errs[i] = store.Process(context.Background(), d.source, raced, protocol.DefaultLimits(), x.direct)
		})
	}
	wg.Wait()
	var original []byte
	for i := range 12 {
		if errs[i] != nil {
			if !errors.Is(errs[i], syncapp.ErrUnavailable) || errors.Is(errs[i], syncapp.ErrSourceDenied) {
				t.Fatal("concurrent duplicate failed outside RETRY", errs[i])
			}
			continue
		}
		if acks[i].Result() != protocol.Accepted || original != nil && !bytes.Equal(original, acks[i].CanonicalBytes()) {
			t.Fatal("concurrent duplicates produced different results")
		}
		original = acks[i].CanonicalBytes()
	}
	if original == nil {
		t.Fatal("no concurrent duplicate was accepted")
	}
	x.directAccepted(t, raced, acks[0], nil)
}

// B-BR-EMP-447: wrong shapes are final MALFORMED receipts with no fact;
// recognised later forms keep no claim and stay RETRY/UNAVAILABLE.
func TestExpenseDirectIntakeRefusesMalformedAndRetainsLaterForms(t *testing.T) {
	x := expenseFixture(t)
	d := x.syncFixtureState
	now := x.declaredNow(t)
	draft, _, err := fleetWrite(t, x.legalFixtureState, x.assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	first := x.directOp(t, d, now, nil)
	ack, err := x.processDirect(t, d, first)
	x.directAccepted(t, first, ack, err)
	for name, edit := range map[string]func(e, p map[string]any){
		"fuel-is-a-fueling":  func(_, p map[string]any) { p["category"] = "FUEL" },
		"company-above-paid": func(_, p map[string]any) { p["company_amount"] = "1251" },
		"decimal-amount":     func(_, p map[string]any) { p["paid_total"] = "12.50" },
		"unknown-payer":      func(_, p map[string]any) { p["payer"] = "SUPPLIER" },
		"unknown-key":        func(_, p map[string]any) { p["note"] = "pedágio" },
		"draft-asset":        func(_, p map[string]any) { p["asset_id"] = draft.AssetID },
		"expected-revision":  func(e, _ map[string]any) { e["entity"].(map[string]any)["expected_revision"] = 1 },
		"identity-of-another-operation": func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": expenseapp.EntityType, "id": first.Snapshot().Entity.ID, "expected_revision": nil}
		},
	} {
		t.Run(name, func(t *testing.T) {
			op := x.directOp(t, d, now, edit)
			ack, err := x.processDirect(t, d, op)
			x.directMalformed(t, op, ack, err)
			replay, err := x.processDirect(t, d, op)
			if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
				t.Fatal("malformed receipt not replayed", err)
			}
		})
	}
	for name, edit := range map[string]func(e, p map[string]any){
		"trip-owner-unavailable":     func(_, p map[string]any) { p["trip_id"] = onboardingID(t) },
		"configured-catalogue-later": func(_, p map[string]any) { p["category"] = "MEALS" },
		"receipt-bytes-unavailable": func(e, _ map[string]any) {
			e["attachments"] = []any{map[string]any{"id": onboardingID(t), "sha256": strings.Repeat("a", 64), "bytes": 10, "mime": "image/jpeg"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			op := x.directOp(t, d, now, edit)
			ack, err := x.processDirect(t, d, op)
			x.directUnavailable(t, op, ack, err)
		})
	}
}

// expenseRolePerson is a synthetic member whose only grant, from its first
// retained period, is role on scope, with its own enrolled device.
func expenseRolePerson(t *testing.T, fx deviceFixtureState, role, scopeType, target string) devicePerson {
	t.Helper()
	key, err := fx.accounts.CreateKeyAccount(t.Context(), "synexpense"+onboardingID(t)[:8])
	if err != nil {
		t.Fatal(err)
	}
	login, err := fx.accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	p := devicePerson{auth: login, employee: onboardingID(t), member: onboardingID(t)}
	org := fx.scope.OrgID().String()
	if target == "" {
		target = org
	}
	for _, command := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, []any{org, p.employee}},
		{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state)VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{org, p.member, p.employee, login.Account.ID}},
		{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,$4,$5,$6,clock_timestamp(),$7,'Synthetic first retained expense period')`, []any{org, onboardingID(t), p.member, role, scopeType, target, fx.member}},
		{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, p.member}},
	} {
		if err := unitSchemaExec(t, fx.legalFixtureState, command.sql, command.args...); err != nil {
			t.Fatal("actual role history fixture failed", err)
		}
	}
	p.signer, p.actor = deviceAddSigner(t, fx, p)
	return p
}

func (x expenseState) member(t *testing.T, role, scopeType, target string) syncFixtureState {
	t.Helper()
	d := x.syncFixtureState
	d.person = expenseRolePerson(t, x.deviceFixtureState, role, scopeType, target)
	d.source = meterHistoricalSource(t, x.deviceFixtureState, d.person)
	return d
}

// B-BR-EMP-448: authority at receipt through the real signed intake.
func TestExpenseDirectIntakeAuthorityAtReceipt(t *testing.T) {
	x := expenseFixture(t)
	asset := x.installed.AssetID
	self := func(t *testing.T, d syncFixtureState, declared time.Time) protocol.Operation {
		return x.directOp(t, d, declared, func(_, p map[string]any) { p["asset_id"] = asset })
	}
	t.Run("foreign-asset-not-found", func(t *testing.T) {
		foreign := foreignAsset(t, x.fuelingState)
		op := x.directOp(t, x.syncFixtureState, x.declaredNow(t), func(_, p map[string]any) { p["asset_id"] = foreign })
		ack, err := x.processDirect(t, x.syncFixtureState, op)
		x.directRefused(t, op, ack, err, orgapi.NotFound)
	})
	t.Run("unknown-asset-not-found", func(t *testing.T) {
		op := x.directOp(t, x.syncFixtureState, x.declaredNow(t), func(_, p map[string]any) { p["asset_id"] = onboardingID(t) })
		ack, err := x.processDirect(t, x.syncFixtureState, op)
		x.directRefused(t, op, ack, err, orgapi.NotFound)
	})
	for _, role := range []string{"AUDITOR", "APPROVER"} {
		t.Run(strings.ToLower(role)+"-forbidden", func(t *testing.T) {
			d := x.member(t, role, "ORG", "")
			op := x.directOp(t, d, x.declaredNow(t), nil)
			ack, err := x.processDirect(t, d, op)
			x.directRefused(t, op, ack, err, orgapi.Forbidden)
		})
	}
	t.Run("finance-org-own-expense-accepted", func(t *testing.T) {
		d := x.member(t, "FINANCE", "ORG", "")
		op := x.directOp(t, d, x.declaredNow(t), nil)
		ack, err := x.processDirect(t, d, op)
		x.directAccepted(t, op, ack, err)
	})
	t.Run("structural-unit-grant-unavailable", func(t *testing.T) {
		d := x.member(t, "SUPERVISOR", "UNIT", x.unit)
		op := x.directOp(t, d, x.declaredNow(t), nil)
		ack, err := x.processDirect(t, d, op)
		if !errors.Is(err, syncapp.ErrSourceDenied) || !errors.Is(err, fa.ErrUnavailable) || len(ack.CanonicalBytes()) != 0 {
			t.Fatal("a structural grant alone was not an unavailable source", err)
		}
		x.directNothing(t, op)
	})
	t.Run("self-never-assigned-forbidden", func(t *testing.T) {
		d := x.driver(t)
		op := self(t, d, x.declaredNow(t))
		ack, err := x.processDirect(t, d, op)
		x.directRefused(t, op, ack, err, orgapi.Forbidden)
	})
	t.Run("self-assigned-own-expense-and-replay", func(t *testing.T) {
		d := x.driver(t)
		_, row := x.assign(t, d.person.employee, asset, fleet.PrimaryAssignment, nil, nil)
		op := self(t, d, row.from)
		ack, err := x.processDirect(t, d, op)
		x.directAccepted(t, op, ack, err)
		replay, err := x.processDirect(t, d, op)
		if err != nil || !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
			t.Fatal("SELF receipt replay", err)
		}
		t.Run("declared-before-assignment-held-unavailable", func(t *testing.T) {
			declared := row.from.Add(-time.Nanosecond)
			var disposition expenseapp.Disposition
			if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
				var err error
				disposition, err = x.expenseAuthority.ClassifyExpenseTx(t.Context(), syncapp.Borrow(tx), x.scope, d.source, asset, declared)
				return err
			}); err != nil || disposition != expenseapp.HoldPermissionChanged {
				t.Fatal("changed assignment is not a retained hold", disposition, err)
			}
			before := self(t, d, declared)
			ack, err := x.processDirect(t, d, before)
			x.directUnavailable(t, before, ack, err)
		})
		t.Run("receipt-read-needs-current-read", func(t *testing.T) {
			if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
				return x.expenseAuthority.CheckExpenseTx(t.Context(), syncapp.Borrow(tx), x.scope, d.source, expenseapp.Target{ExpenseID: op.Snapshot().Entity.ID, AssetID: asset, UnitID: x.unit}, expenseapp.ReadAction)
			}); err != nil {
				t.Fatal("own stored expense not readable by its SELF author", err)
			}
		})
	})
	t.Run("other-asset-assigned-forbidden", func(t *testing.T) {
		d := x.driver(t)
		x.assign(t, d.person.employee, x.vehicle.AssetID, fleet.PrimaryAssignment, nil, nil)
		op := self(t, d, x.declaredNow(t))
		ack, err := x.processDirect(t, d, op)
		x.directRefused(t, op, ack, err, orgapi.Forbidden)
	})
}

// A direct expense declared at or after the asset's disposal stores nothing
// and stays unavailable; one declared before it is recorded.
func TestExpenseDirectIntakeDisposalStoresNothingAtOrAfterTheInstant(t *testing.T) {
	x := expenseFixture(t)
	d := x.syncFixtureState
	disposal := lifecycleDBNow(t, x.legalFixtureState).Truncate(time.Microsecond).Add(-time.Microsecond).Add(500 * time.Nanosecond)
	dispose := lifecycleInputFor(t, x.legalFixtureState, fleet.Dispose, x.vehicle.AssetID, x.vehicleAt)
	dispose.At = disposal
	if _, _, err := lifecycleWrite(t, x.legalFixtureState, x.assets, lifecycleCommand(t, dispose)); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{disposal, disposal.Add(time.Nanosecond)} {
		op := x.directOp(t, d, at, nil)
		ack, err := x.processDirect(t, d, op)
		x.directUnavailable(t, op, ack, err)
	}
	op := x.directOp(t, d, disposal.Add(-time.Millisecond), nil)
	ack, err := x.processDirect(t, d, op)
	x.directAccepted(t, op, ack, err)
}

// Forced RLS, organization isolation and insert-only history for the
// ordinary runtime role; usage sources treat expenses as associations.
func TestExpenseSchemaIsForcedRLSAndInsertOnly(t *testing.T) {
	x := expenseFixture(t)
	d := x.syncFixtureState
	op := x.directOp(t, d, x.declaredNow(t), nil)
	ack, err := x.processDirect(t, d, op)
	x.directAccepted(t, op, ack, err)
	var forced bool
	if err := x.f.Admin.QueryRow(t.Context(), `SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='org_expenses'::regclass`).Scan(&forced); err != nil || !forced {
		t.Fatal("expense table is not forced-RLS", err)
	}
	var visible int
	if err := x.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM org_expenses`).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("expense rows visible without an organization setting", visible, err)
	}
	var pe interface{ SQLState() string }
	if _, err := x.f.Runtime.Exec(t.Context(), `INSERT INTO org_expenses(org_id,id,origin_kind,source_id,source_revision,operation_id,payload_digest,asset_id,author_membership_id,author_employee_id,enrollment_id,
 category,paid_centavos,company_centavos,payer,occurred_at,occurred_submicro,entry_zone,device_offset_seconds,reporting_day,reporting_zone,reporting_source,review_state,revision)
VALUES($1,$2,'DIRECT',$2,1,$3,$4,$5,$6,$7,$8,'TOLL',1250,1250,'EMPLOYEE',clock_timestamp(),0,'UTC',0,current_date,'UTC','organization','SUBMITTED',1)`,
		x.scope.OrgID().String(), onboardingID(t), op.ID(), op.PayloadDigest(), x.vehicle.AssetID, d.source.MembershipID, d.person.employee, d.source.EnrollmentID); !errors.As(err, &pe) || pe.SQLState() != "42501" {
		t.Fatal("insert accepted without an organization setting", err)
	}
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(t.Context(), "GRANT UPDATE,DELETE,TRUNCATE ON org_expenses TO "+role); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`UPDATE org_expenses SET company_centavos=0`, `UPDATE org_expenses SET review_state='SUBMITTED'`, `DELETE FROM org_expenses`, `TRUNCATE org_expenses`} {
		if code := sqlState(t, x.legalFixtureState, x.scope, statement); code != "55000" {
			t.Fatal("expense history accepted ordinary maintenance", statement, code)
		}
	}
	x.directAccepted(t, op, ack, nil)
	// The usage sources answer for the referenced asset and unit (the keys
	// are listed associations, never current use).
	if err := database.WithOrgTx(t.Context(), x.f.Runtime, x.scope, func(tx pgx.Tx) error {
		var asset, unit string
		return tx.QueryRow(t.Context(), `SELECT org_asset_current_usage($1,$2)::text,org_unit_current_usage($1,$3)::text`, x.scope.OrgID().String(), x.vehicle.AssetID, x.unit).Scan(&asset, &unit)
	}); err != nil {
		t.Fatal("usage sources refused the expense keys", err)
	}
}

// The default enterprise runtime registers the real expense.submit owner: a
// signed TLS request records the toll and a resend returns the same ACK.
func TestExpenseIntakeDefaultRuntimeSignedTLSAcceptsAndReplays(t *testing.T) {
	fx := meterIntakeHTTP(t)
	fuelingGrants(t, fx.legalFixtureState)
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	assets := fleetStore(t, fx.legalFixtureState)
	input := fleetCreateInput()
	input.UnitID = unit
	created, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	installed, _, err := meterInstall(t, fx.legalFixtureState, meterStore(t, fx.legalFixtureState), meterInstallCommand(t, meterInstallInput(t, created.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision})); err != nil {
		t.Fatal(err)
	}
	op := syncHTTPOperation(t, fx.deviceFixtureState, fx.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = expenseapp.OperationType
		raw["entity"] = map[string]any{"type": expenseapp.EntityType, "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		raw["payload"] = map[string]any{"asset_id": created.AssetID, "category": "PARKING", "paid_total": "2000", "company_amount": "2000", "payer": "COMPANY", "trip_id": nil}
	})
	ack := fx.dispatch(t, op, 200)
	if ack.Result() != protocol.Accepted || ack.Snapshot().EntityRevision == nil || *ack.Snapshot().EntityRevision != 1 {
		t.Fatal("default runtime did not accept the signed expense", ack.Result(), ack.Code())
	}
	replay := fx.dispatch(t, op, 200)
	if !bytes.Equal(replay.CanonicalBytes(), ack.CanonicalBytes()) {
		t.Fatal("default runtime resend changed the ACK")
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*)=1 AND bool_and(origin_kind='DIRECT' AND category='PARKING' AND paid_centavos=2000 AND company_centavos=2000 AND unit_id=$3)
 FROM org_expenses WHERE org_id=$1 AND operation_id=$2`, fx.scope.OrgID().String(), op.ID(), unit).Scan(&exact); err != nil || !exact {
		t.Fatal("default runtime expense not stored exactly once", err)
	}
}
