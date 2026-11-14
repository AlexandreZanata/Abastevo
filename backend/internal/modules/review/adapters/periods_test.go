package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPeriodStorePreservesOnlySafeRefusals(t *testing.T) {
	for _, expected := range []error{access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrSetupIncomplete, access.ErrFreshAuthRequired, access.ErrEntitlementRequired, access.ErrUnauthenticated, permission.ErrPermissionDenied, app.ErrOperationReused, app.ErrNotFound, app.ErrUnauthenticated, period.ErrConflict, period.ErrTransition, period.ErrInvalid, period.ErrReasonRequired, period.ErrReasonInvalid} {
		if got := storeError(context.Background(), expected); !errors.Is(got, expected) {
			t.Fatal("registered refusal became unavailability", expected)
		}
	}
	if got := storeError(context.Background(), audit.ErrReused); !errors.Is(got, app.ErrOperationReused) {
		t.Fatal("audit operation reuse not mapped", got)
	}
	if got := storeError(context.Background(), pgx.ErrNoRows); !errors.Is(got, app.ErrNotFound) {
		t.Fatal("missing row not opaque not-found", got)
	}
	for _, err := range []error{errors.New("synthetic private provider secret"), &pgconn.PgError{Code: "XX000", Message: "synthetic private record"}, &pgconn.PgError{Code: "23514", Message: "synthetic private reason"}, &pgconn.PgError{Code: "23505", ConstraintName: "unregistered_private"}} {
		if got := storeError(context.Background(), err); got != app.ErrUnavailable {
			t.Fatal("unregistered provider details escaped", got)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := storeError(ctx, errors.New("synthetic")); !errors.Is(got, context.Canceled) {
		t.Fatal("cancellation hidden", got)
	}
	if storeError(context.Background(), nil) != nil {
		t.Fatal("nil error invented a refusal")
	}
}

func TestPeriodAuditVocabulary(t *testing.T) {
	defs := PeriodAuditDefinitions()
	catalog, err := audit.NewCatalog(defs)
	if err != nil || len(defs) != 3 {
		t.Fatal("period audit vocabulary", err)
	}
	for _, action := range []string{"fin.period.locked", "fin.period.reopened", "fin.period.relocked"} {
		e := audit.Event{OperationID: "a6000000-0000-4000-8000-000000000001", ActorID: "a6000000-0000-4000-8000-000000000002", Action: action, TargetType: "finance_period", TargetID: "a6000000-0000-4000-8000-000000000003", RequestHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
		if catalog.Validate(e) != nil {
			t.Fatal("period action not registered", action)
		}
	}
}

func TestPeriodStoreAndGateFailClosedWithoutSources(t *testing.T) {
	org, _ := kernel.ParseOrgID("a6000000-0000-4000-8000-000000000001")
	scope, _ := kernel.NewOrgScope(org)
	actor := app.PeriodActor{AccountID: "a6000000-0000-4000-8000-000000000002", SessionID: "a6000000-0000-4000-8000-000000000003"}
	member := "a6000000-0000-4000-8000-000000000004"
	command, err := app.NewPeriodCommand(app.PeriodCommandInput{Action: period.ActionClose, OperationID: "a6000000-0000-4000-8000-000000000005", Month: "2026-10", ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	month, _ := period.ParseMonth("2026-10")
	var absent *PeriodStore
	for _, s := range []*PeriodStore{absent, {}} {
		if _, _, err := s.TransitionPeriod(context.Background(), scope, actor, member, "a6000000-0000-4000-8000-000000000006", command); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("store without sources accepted a command", err)
		}
		if _, err := s.ReadPeriod(context.Background(), scope, actor, member, month); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("store without sources read", err)
		}
		if _, err := s.ListPeriods(context.Background(), scope, actor, member, app.PeriodFilter{Limit: 1}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("store without sources listed", err)
		}
	}
	s := &PeriodStore{}
	if _, _, err := s.TransitionPeriod(context.Background(), kernel.OrgScope{}, actor, member, "a6000000-0000-4000-8000-000000000006", command); err == nil {
		t.Fatal("invalid scope accepted")
	}
	if _, _, err := s.TransitionPeriod(context.Background(), scope, app.PeriodActor{}, member, "a6000000-0000-4000-8000-000000000006", command); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("invalid actor accepted", err)
	}
	if _, _, err := s.TransitionPeriod(context.Background(), scope, actor, member, "bad", command); !errors.Is(err, period.ErrInvalid) {
		t.Fatal("invalid new period identity accepted", err)
	}
	if _, err := s.ReadPeriod(context.Background(), scope, actor, member, period.Month{}); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("zero month read", err)
	}
	if _, err := (PeriodGate{}).StateForDayTx(context.Background(), nil, scope, "2026-10-01"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("gate without transaction", err)
	}
	if _, err := (PeriodGate{}).StateForOccurrenceTx(context.Background(), nil, scope, time.Now(), ""); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("gate without transaction", err)
	}
}
