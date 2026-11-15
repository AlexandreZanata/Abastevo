package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	reviewapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EDR-220/235: the submission owner exists only with real ports and declares
// ACCEPTED, REJECTED and, for a held stale submission, ACCEPTED_FOR_REVIEW.
func TestSubmitOwnerNeedsRealPortsAndHoldsStaleSubmissions(t *testing.T) {
	if h, err := NewSubmitHandler(nil, nil, nil, nil); h != nil || !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("a submission owner without ports was composed", err)
	}
	var store *Store
	if h, err := NewSubmitHandler(nil, nil, store, store); h != nil || !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("a submission owner with nil ports was composed", err)
	}
	d := (&submitHandler{}).Descriptor()
	if d.Type != app.SubmitOperationType || d.Schema != 1 || d.WritePolicy != app.Policy(app.RecordAction) || d.ReadPolicy != app.Policy(app.ReadAction) ||
		len(d.TerminalResults) != 3 || d.TerminalResults[0] != protocol.Accepted || d.TerminalResults[1] != protocol.Rejected || d.TerminalResults[2] != protocol.AcceptedForReview || d.Validate() != nil {
		t.Fatal("submission descriptor drift", d.TerminalResults)
	}
	if _, ok := (&submitHandler{}).ReviewProof().(SubmitProof); !ok {
		t.Fatal("the held-submission proof is not composed")
	}
	var h *submitHandler
	if err := h.Admit(context.Background(), nil, syncapp.Source{}, protocol.Operation{}, syncapp.Write); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("an unready owner admitted", err)
	}
}

// The stored SUBMIT entry is exactly this operation's author fact.
func TestStoredSubmitProvesTheExactEntry(t *testing.T) {
	source := syncapp.Source{MembershipID: "m"}
	v := app.SubmitInput{OperationID: "op", Revision: 1}
	record := app.Record{AuthorMembershipID: "m", AuthorEmployeeID: "e"}
	good := app.ReviewEntry{Sequence: 1, Kind: review.Submit, Revision: 1, State: review.Submitted, ActorMembershipID: "m", ActorEmployeeID: "e", OperationID: "op"}
	if revision, ok := storedSubmit(source, v, record, []app.ReviewEntry{good}); !ok || revision != 1 {
		t.Fatal("the exact entry was not proven")
	}
	resubmit := good
	resubmit.Sequence, resubmit.Kind, resubmit.Revision = 4, review.Resubmit, 2
	if revision, ok := storedSubmit(source, v, record, []app.ReviewEntry{resubmit}); !ok || revision != 2 {
		t.Fatal("the exact RESUBMIT entry was not proven (EMP-TRIP-015)")
	}
	for name, edit := range map[string]func(*app.ReviewEntry){
		"other operation": func(e *app.ReviewEntry) { e.OperationID = "other" },
		"other member":    func(e *app.ReviewEntry) { e.ActorMembershipID = "x" },
		"other employee":  func(e *app.ReviewEntry) { e.ActorEmployeeID = "x" },
		"other revision":  func(e *app.ReviewEntry) { e.Revision = 2 },
		"not a submit":    func(e *app.ReviewEntry) { e.Kind, e.State = review.Open, review.InReview },
		"a reason":        func(e *app.ReviewEntry) { e.Reason = "x" },
		"not first":       func(e *app.ReviewEntry) { e.Sequence = 2 },
	} {
		e := good
		edit(&e)
		if _, ok := storedSubmit(source, v, record, []app.ReviewEntry{e}); ok {
			t.Fatal("a different entry was proven", name)
		}
	}
}

// The review store fails closed without its ports and keeps the domain and
// reuse vocabulary distinct from storage failures.
func TestReviewStoreFailsClosedAndMapsFailures(t *testing.T) {
	var s *ReviewStore
	scope := kernel.OrgScope{}
	if _, _, err := s.DecideTripReview(context.Background(), scope, app.ReviewActor{}, "", app.DecisionCommand{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an unready store decided", err)
	}
	if _, err := s.ReadTripReview(context.Background(), scope, app.ReviewActor{}, "", ""); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an unready store read", err)
	}
	if _, err := s.ListTripReviews(context.Background(), scope, app.ReviewActor{}, "", app.QueueFilter{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an unready store listed", err)
	}
	ctx := context.Background()
	for _, c := range []struct{ in, want error }{
		{audit.ErrReused, app.ErrOperationReused}, {pgx.ErrNoRows, app.ErrNotFound}, {review.ErrSelfApproval, review.ErrSelfApproval},
		{review.ErrConflict, review.ErrConflict}, {app.ErrReviewUnauthorized, app.ErrReviewUnauthorized},
	} {
		if got := reviewFailure(ctx, c.in); !errors.Is(got, c.want) {
			t.Fatal("review failure mapping", c.in, got)
		}
	}
	if len(ReviewAuditDefinitions()) != 4 || ReviewAuditDefinitions()[3].Action != app.AmendAuditAction {
		t.Fatal("decision and amendment audit actions drift")
	}
}

type periodlessAuthority struct{}

func (periodlessAuthority) CheckTripReviewTx(context.Context, pgx.Tx, kernel.OrgScope, app.ReviewActor, string, string, bool) (app.Reviewer, error) {
	return app.Reviewer{}, nil
}

type periodlessAudit struct{}

func (periodlessAudit) RecordChangeTx(context.Context, pgx.Tx, kernel.OrgScope, audit.Event, func(context.Context, pgx.Tx) error) (bool, error) {
	return false, nil
}

type periodGateStub struct{ reviewapp.PeriodGate }

// EMP-TRIP-016 (EDR-232): a review store without the period gate is not
// composed, so no decision can skip the lock of its reporting month.
func TestReviewStoreNeedsThePeriodGate(t *testing.T) {
	s := &ReviewStore{Pool: &pgxpool.Pool{}, Authority: periodlessAuthority{}, Audit: periodlessAudit{}}
	if s.ready() {
		t.Fatal("a review store without the period gate is ready")
	}
	if _, _, err := s.DecideTripReview(context.Background(), kernel.OrgScope{}, app.ReviewActor{}, "", app.DecisionCommand{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("a store without the period gate decided", err)
	}
	s.Periods = periodGateStub{}
	if !s.ready() {
		t.Fatal("a fully composed review store is not ready")
	}
}
