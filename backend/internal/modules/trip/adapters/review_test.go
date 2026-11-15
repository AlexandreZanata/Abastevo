package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/jackc/pgx/v5"
)

// EDR-220: the submission owner exists only with real ports and declares
// exactly ACCEPTED and REJECTED (no held path).
func TestSubmitOwnerNeedsRealPortsAndHasNoHeldPath(t *testing.T) {
	if h, err := NewSubmitHandler(nil, nil, nil, nil); h != nil || !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("a submission owner without ports was composed", err)
	}
	var store *Store
	if h, err := NewSubmitHandler(nil, nil, store, store); h != nil || !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("a submission owner with nil ports was composed", err)
	}
	d := (&submitHandler{}).Descriptor()
	if d.Type != app.SubmitOperationType || d.Schema != 1 || d.WritePolicy != app.Policy(app.RecordAction) || d.ReadPolicy != app.Policy(app.ReadAction) ||
		len(d.TerminalResults) != 2 || d.TerminalResults[0] != protocol.Accepted || d.TerminalResults[1] != protocol.Rejected || d.Validate() != nil {
		t.Fatal("submission descriptor drift", d.TerminalResults)
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
	if !storedSubmit(source, v, record, []app.ReviewEntry{good}) {
		t.Fatal("the exact entry was not proven")
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
		if storedSubmit(source, v, record, []app.ReviewEntry{e}) {
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
	if len(ReviewAuditDefinitions()) != 3 {
		t.Fatal("decision audit actions drift")
	}
}
