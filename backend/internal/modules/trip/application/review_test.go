package application_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func submission(t *testing.T, change func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	return operation(t, app.SubmitOperationType, map[string]any{}, func(e, p map[string]any) {
		e["entity"] = map[string]any{"type": app.ReviewEntityType, "id": id(40), "expected_revision": 1}
		e["depends_on"] = []any{id(21)}
		if change != nil {
			change(e, p)
		}
	})
}

// B-BR-EMP-342 / EDR-220: the envelope names the trip, the review revision
// and the finish dependency; the payload is exactly empty.
func TestParseSubmitKeepsTheExactSubmission(t *testing.T) {
	op := submission(t, nil)
	command, err := app.ParseSubmit(op)
	if err != nil {
		t.Fatal(err)
	}
	v, err := command.Snapshot()
	if err != nil || v.TripID != id(40) || v.Revision != 1 || v.AuthorMembershipID != id(3) || v.OperationID != op.ID() || v.PayloadDigest != op.PayloadDigest() ||
		len(v.DependsOn) != 1 || v.DependsOn[0] != id(21) || v.Declared.OccurredAt.Nanosecond()%1000 != 123 || v.Declared.EntryZone != "America/Cuiaba" {
		t.Fatal("parsed submission lost its exact envelope", err)
	}
	v.DependsOn[0] = "changed"
	if again, _ := command.Snapshot(); again.DependsOn[0] != id(21) {
		t.Fatal("the snapshot shares the dependency slice")
	}
	if _, err := app.ParseSubmit(submission(t, func(e, _ map[string]any) { e["depends_on"] = []any{} })); err != nil {
		t.Fatal("a submission without its finish dependency was refused", err)
	}
	if _, err := (app.SubmitCommand{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("the zero command is usable", err)
	}
}

func TestParseSubmitRefusesEveryOtherShapeAsMalformed(t *testing.T) {
	for name, change := range map[string]func(e, p map[string]any){
		"payload key":    func(_, p map[string]any) { p["trip_id"] = id(40) },
		"payload reason": func(_, p map[string]any) { p["reason"] = "" },
		"trip entity":    func(e, _ map[string]any) { e["entity"].(map[string]any)["type"] = app.EntityType },
		"no revision":    func(e, _ map[string]any) { e["entity"].(map[string]any)["expected_revision"] = nil },
		"revision above cap": func(e, _ map[string]any) {
			e["entity"].(map[string]any)["expected_revision"] = app.MaxReviewRevision + 1
		},
		"two dependencies":    func(e, _ map[string]any) { e["depends_on"] = []any{id(21), id(22)} },
		"soft reference":      func(e, _ map[string]any) { e["soft_references"] = []any{map[string]any{"type": "trip", "id": id(41)}} },
		"another type":        func(e, _ map[string]any) { e["type"] = app.FinishOperationType },
		"unknown entry zone":  func(e, _ map[string]any) { e["entry_zone"] = "Mars/Olympus" },
		"offset out of range": func(e, _ map[string]any) { e["device_offset_seconds"] = 90000 },
	} {
		t.Run(name, func(t *testing.T) {
			op, ok := sealed(t, func(e, p map[string]any) {
				e["entity"] = map[string]any{"type": app.ReviewEntityType, "id": id(40), "expected_revision": 1}
				change(e, p)
			})
			if !ok {
				return // the frozen protocol refuses the envelope before any owner
			}
			if _, err := app.ParseSubmit(op); !errors.Is(err, app.ErrMalformed) {
				t.Fatal("not MALFORMED", err)
			}
		})
	}
	if _, err := app.ParseSubmit(operation(t, app.SubmitOperationType, map[string]any{"x": nil}, func(e, _ map[string]any) {
		e["entity"] = map[string]any{"type": app.ReviewEntityType, "id": id(40), "expected_revision": 1}
	})); !errors.Is(err, app.ErrMalformed) {
		t.Fatal("a null payload member was admitted", err)
	}
}

// sealed builds the operation fixture without failing on an envelope that
// the frozen protocol itself refuses.
func sealed(t *testing.T, change func(e, p map[string]any)) (protocol.Operation, bool) {
	t.Helper()
	body := map[string]any{}
	envelope := map[string]any{"schema_version": 1, "operation_id": id(20), "owner_account_id": id(1), "api_origin": origin, "org_id": id(65), "membership_id": id(3),
		"enrollment_id": id(4), "type": app.SubmitOperationType, "operation_schema": 1, "entity": map[string]any{"type": app.ReviewEntityType, "id": id(40), "expected_revision": 1},
		"occurred_at": "2026-11-14T12:00:00.000000123Z", "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "depends_on": []any{},
		"soft_references": []any{}, "attachments": []any{}, "payload": body}
	change(envelope, body)
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(id(1), origin, id(65))
	if err != nil {
		t.Fatal(err)
	}
	op, err := protocol.Seal(raw, binding)
	return op, err == nil
}

// EDR-223: the trip's review state is the APPR-001 case of its stored
// history; only a COMPLETED trip whose case is DRAFT at the named revision
// is submitted, and completion alone never approves.
func TestReviewCaseBindsTheStoredHistoryToTheTripStanding(t *testing.T) {
	at := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	entry := func(seq int64, kind review.Kind, state review.State, reason string) app.ReviewEntry {
		return app.ReviewEntry{Sequence: seq, Kind: kind, Revision: 1, State: state, Reason: reason, ActorMembershipID: id(3), ActorEmployeeID: id(30), OperationID: id(50 + int(seq)), RecordedAt: at}
	}
	draft, err := app.ReviewCase(nil)
	if err != nil || draft.State() != review.Draft || draft.Revision() != 1 {
		t.Fatal("a trip without entries is not a DRAFT revision 1", err)
	}
	standing, err := app.Standing(trip.Completed, draft)
	if err != nil || !standing.Completed() || standing.Approved() || standing.Review() != trip.ReviewDraft {
		t.Fatal("operational completion mistaken for approval", err)
	}
	if err := app.SubmitGate(trip.Completed, draft, 1); err != nil {
		t.Fatal("a completed draft is not submittable", err)
	}
	for _, state := range []trip.State{trip.InProgress, trip.Planned, trip.Paused} {
		if err := app.SubmitGate(state, draft, 1); !errors.Is(err, app.ErrSubmitPending) {
			t.Fatal("a non-completed trip is submittable", state, err)
		}
	}
	if err := app.SubmitGate(trip.Completed, draft, 2); !errors.Is(err, app.ErrSubmitStale) {
		t.Fatal("a stale revision is submittable", err)
	}
	history := []app.ReviewEntry{entry(1, review.Submit, review.Submitted, ""), entry(2, review.Open, review.InReview, ""), entry(3, review.Return, review.Returned, "Informe o motivo da parada")}
	returned, err := app.ReviewCase(history)
	if err != nil || returned.State() != review.Returned || returned.Revision() != 1 {
		t.Fatal("stored history replay", err)
	}
	if standing, _ := app.Standing(trip.Completed, returned); standing.Review() != trip.ReviewReturned || standing.Approved() {
		t.Fatal("returned standing")
	}
	if err := app.SubmitGate(trip.Completed, returned, 1); !errors.Is(err, app.ErrSubmitStale) {
		t.Fatal("a returned case is submittable before EMP-TRIP-015", err)
	}
	for name, broken := range map[string][]app.ReviewEntry{
		"gap":               {entry(2, review.Submit, review.Submitted, "")},
		"impossible":        {entry(1, review.Open, review.InReview, "")},
		"cancel not owned":  {entry(1, review.Cancel, review.Cancelled, "")},
		"no actor":          {{Sequence: 1, Kind: review.Submit, Revision: 1, State: review.Submitted, OperationID: id(51), RecordedAt: at}},
		"no receipt":        {{Sequence: 1, Kind: review.Submit, Revision: 1, State: review.Submitted, ActorMembershipID: id(3), ActorEmployeeID: id(30), OperationID: id(51)}},
		"reasonless return": {entry(1, review.Submit, review.Submitted, ""), entry(2, review.Open, review.InReview, ""), entry(3, review.Return, review.Returned, "")},
	} {
		if _, err := app.ReviewCase(broken); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("impossible stored history accepted", name, err)
		}
	}
}

// B-BR-EMP-344/345: the reviewer decides one named revision; a SUBMITTED
// case is opened first; author and beneficiary never decide.
func TestDecideAppliesTheLifecycleWithSeparationOfDuties(t *testing.T) {
	submitted, err := review.Replay([]review.Entry{{Kind: review.Submit, Revision: 1, State: review.Submitted}})
	if err != nil {
		t.Fatal(err)
	}
	author := app.Person{AccountID: id(1), MembershipID: id(3), Employees: []string{id(30)}}
	driver := app.Person{Employees: []string{id(31), id(32)}}
	reviewer := app.Person{AccountID: id(5), MembershipID: id(6), Employees: []string{id(60)}}
	in := app.DecisionInput{OperationID: id(70), TripID: id(40), ExpectedRevision: 1, Decision: review.Return, Reason: "Informe o motivo da parada"}
	next, err := app.Decide(submitted, in, reviewer, author, driver)
	if err != nil || next.State() != review.Returned || len(next.History()) != 3 || next.History()[1].Kind != review.Open || next.History()[2].Reason != in.Reason {
		t.Fatal("a SUBMITTED case was not opened and returned", err)
	}
	if again, err := app.Decide(next, in, reviewer, author, driver); !errors.Is(err, review.ErrInvalidTransition) || again.State() != "" {
		t.Fatal("a decided revision was decided again", err)
	}
	stale := in
	stale.ExpectedRevision = 2
	if _, err := app.Decide(submitted, stale, reviewer, author, driver); !errors.Is(err, review.ErrConflict) {
		t.Fatal("a stale revision was decided", err)
	}
	for name, self := range map[string]app.Person{
		"author account":     {AccountID: id(1), MembershipID: id(8), Employees: []string{id(80)}},
		"author membership":  {AccountID: id(9), MembershipID: id(3), Employees: []string{id(80)}},
		"author employee":    {AccountID: id(9), MembershipID: id(8), Employees: []string{id(30)}},
		"driver merge chain": {AccountID: id(9), MembershipID: id(8), Employees: []string{id(81), id(32)}},
	} {
		if _, err := app.Decide(submitted, in, self, author, driver); !errors.Is(err, review.ErrSelfApproval) {
			t.Fatal("self-approval was not refused", name, err)
		}
	}
	draft := review.NewDraft()
	if _, err := app.Decide(draft, in, reviewer, author, driver); !errors.Is(err, review.ErrInvalidTransition) {
		t.Fatal("a draft was decided", err)
	}
	submittedNext, err := app.Submit(draft, 1, author, author)
	if err != nil || submittedNext.State() != review.Submitted {
		t.Fatal("the author's submission", err)
	}
	if _, err := app.Submit(draft, 1, reviewer, author); !errors.Is(err, review.ErrForbidden) {
		t.Fatal("another member submitted", err)
	}
}

func TestDecisionCommandShapeReasonAndDigest(t *testing.T) {
	base := app.DecisionInput{OperationID: id(70), TripID: id(40), ExpectedRevision: 1, Decision: review.Approve}
	command, err := app.NewDecisionCommand(base)
	if err != nil {
		t.Fatal("an approval without a reason is refused", err)
	}
	org, err := kernel.ParseOrgID(id(65))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	digest := command.Digest(scope)
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		t.Fatal("decision digest is not lowercase SHA-256 hex", digest)
	}
	withReason := base
	withReason.Reason = "Synthetic approval note"
	other, err := app.NewDecisionCommand(withReason)
	if err != nil || other.Digest(scope) == digest {
		t.Fatal("the reason does not bind the digest", err)
	}
	for name, c := range map[string]struct {
		edit func(*app.DecisionInput)
		want error
	}{
		"return without reason": {func(v *app.DecisionInput) { v.Decision = review.Return }, review.ErrReasonRequired},
		"reject blank reason":   {func(v *app.DecisionInput) { v.Decision, v.Reason = review.Reject, " 　 " }, review.ErrReasonRequired},
		"control in reason":     {func(v *app.DecisionInput) { v.Decision, v.Reason = review.Return, "Synthetic\treason" }, review.ErrInvalid},
		"281 scalars":           {func(v *app.DecisionInput) { v.Decision, v.Reason = review.Return, strings.Repeat("é", 281) }, review.ErrInvalid},
		"open is no decision":   {func(v *app.DecisionInput) { v.Decision = review.Open }, review.ErrInvalid},
		"submit is no decision": {func(v *app.DecisionInput) { v.Decision = review.Submit }, review.ErrInvalid},
		"zero revision":         {func(v *app.DecisionInput) { v.ExpectedRevision = 0 }, review.ErrInvalid},
		"revision above cap":    {func(v *app.DecisionInput) { v.ExpectedRevision = app.MaxReviewRevision + 1 }, review.ErrInvalid},
		"non-canonical trip":    {func(v *app.DecisionInput) { v.TripID = strings.ToUpper(id(40)) }, review.ErrInvalid},
		"no operation":          {func(v *app.DecisionInput) { v.OperationID = "" }, review.ErrInvalid},
	} {
		v := base
		c.edit(&v)
		if _, err := app.NewDecisionCommand(v); !errors.Is(err, c.want) {
			t.Fatal("invalid decision command admitted", name, err)
		}
	}
	if ok, err := app.NewDecisionCommand(app.DecisionInput{OperationID: id(70), TripID: id(40), ExpectedRevision: 1, Decision: review.Return, Reason: strings.Repeat("é", 280)}); err != nil {
		t.Fatal("a 280-scalar reason was refused", err, ok)
	}
	if got := (app.DecisionCommand{}).Digest(scope); got != "" {
		t.Fatal("the zero command has a digest")
	}
}

func TestReviewPoliciesAndQueueFilter(t *testing.T) {
	decide, read := app.ReviewPolicy(app.ReviewDecideAction), app.ReviewPolicy(app.ReviewReadAction)
	if decide.Permission != "trip.trip.review" || !decide.Fresh || !decide.Effect.Write() || read.Permission != "trip.trip.review" || read.Fresh || read.Effect.Write() || decide.Validate() != nil || read.Validate() != nil {
		t.Fatal("review policies drift")
	}
	if p := app.ReviewPolicy(0); p.Permission != "" {
		t.Fatal("an unknown review action has a policy")
	}
	for _, bad := range []app.QueueFilter{{Limit: 0}, {Limit: 101}, {Limit: 1, AfterID: "x"}} {
		if bad.Validate() == nil {
			t.Fatal("invalid queue filter accepted", bad.Limit)
		}
	}
	if (app.QueueFilter{Limit: 100, AfterID: id(40)}).Validate() != nil {
		t.Fatal("valid queue filter refused")
	}
}
