package domain_test

import (
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
)

var (
	allStates = []domain.ReviewState{
		domain.StateDraft, domain.StateSubmitted, domain.StateInReview, domain.StateReturned,
		domain.StateApproved, domain.StateRejected, domain.StateCancelled, domain.StateVoided, "UNKNOWN",
	}
	allIntents = []domain.Intent{domain.IntentEdit, domain.IntentAmend, domain.IntentVoid, domain.IntentAdjust}
	allResults = []domain.ReviewState{
		domain.StateApproved, domain.StateReturned, domain.StateRejected, domain.StateCancelled,
		domain.StateSubmitted, domain.StateVoided,
	}
	receipt = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
)

func periods() []domain.Period {
	return []domain.Period{
		{State: domain.PeriodOpen},
		{State: domain.PeriodLocked},
		{State: domain.PeriodReopened, Reopening: &domain.Reopening{AuthorityProven: true, Reason: "Late receipt"}},
		{State: domain.PeriodReopened, Reopening: &domain.Reopening{AuthorityProven: false, Reason: "Late receipt"}},
		{State: domain.PeriodReopened},
		{State: domain.PeriodOpen, Reopening: &domain.Reopening{AuthorityProven: true, Reason: "Late receipt"}},
		{State: "CLOSED"},
	}
}

func writablePeriod(p domain.Period) bool {
	return p.State == domain.PeriodOpen && p.Reopening == nil ||
		p.State == domain.PeriodReopened && p.Reopening != nil && p.Reopening.AuthorityProven
}

func pendingFacts(f domain.Facts) bool {
	switch f.State {
	case domain.StateDraft, domain.StateSubmitted, domain.StateInReview, domain.StateReturned:
		return f.Effective > 0 && f.Latest > f.Effective
	}
	return false
}

func sameMarker(a, b *domain.Recalculation) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func stripMarker(d domain.Decision) domain.Decision {
	d.Derived = nil
	return d
}

func stripResolution(r domain.Resolution) domain.Resolution {
	r.Derived = nil
	return r
}

func closedError(t *testing.T, err error) {
	t.Helper()
	if errorCode(err) == "UNEXPECTED" {
		t.Fatalf("refusal outside the closed vocabulary: %v", err)
	}
}

func requests(intent domain.Intent, effective, latest int64) []domain.Request {
	postings := []*domain.Period{nil}
	if intent == domain.IntentAdjust {
		postings = nil
		for _, p := range periods()[:3] {
			postings = append(postings, &p)
		}
	}
	var out []domain.Request
	for _, expected := range []int64{latest - 1, latest} {
		for _, revision := range []int64{0, latest, latest + 1} {
			for _, base := range []int64{0, effective, latest} {
				for _, granted := range []bool{false, true} {
					for _, reason := range []string{"", "Pump quantity corrected"} {
						for _, posting := range postings {
							r := domain.Request{
								Intent: intent, Expected: expected, Revision: revision, Base: base,
								ActorID: uuid(31), At: receipt, Reason: reason,
								Authority: domain.Authority{Edit: granted, Amend: granted, Void: granted},
							}
							if posting != nil {
								r.PostingDay, r.PostingPeriod = "2026-11-14", posting
							}
							out = append(out, r)
						}
					}
				}
			}
		}
	}
	return out
}

// Exhaustive small-domain enumeration of the append-only, conflict, period and
// authority invariants. Each successful decision must retain every revision.
func TestDecideInvariants(t *testing.T) {
	successes := map[domain.Outcome]int{}
	for _, state := range allStates[:8] {
		for effective := int64(0); effective <= 2; effective++ {
			for latest := int64(1); latest <= 3; latest++ {
				// Unknown tokens and misplaced reopenings are pinned by the vectors.
				for _, period := range periods()[:5] {
					facts := domain.Facts{
						RecordID: uuid(1), State: state, Effective: effective, Latest: latest,
						ReportingDay: "2026-10-31", Period: period,
					}
					for _, intent := range allIntents {
						for _, r := range requests(intent, effective, latest) {
							if d, ok := checkDecision(t, facts, r); ok {
								successes[d.Outcome]++
							}
						}
					}
				}
			}
		}
	}
	for _, outcome := range []domain.Outcome{domain.OutcomeEdit, domain.OutcomeAmendment, domain.OutcomeVoid, domain.OutcomeAdjustment} {
		if successes[outcome] == 0 {
			t.Errorf("enumeration never reached %s", outcome)
		}
	}
}

func checkDecision(t *testing.T, f domain.Facts, r domain.Request) (domain.Decision, bool) {
	t.Helper()
	d, err := domain.Decide(f, r)
	if err != nil {
		closedError(t, err)
		if d != (domain.Decision{}) {
			t.Fatal("a refusal returned a partial decision")
		}
		return d, false
	}
	// Refusals are pinned by the shared vectors; replay every acceptance.
	again, againErr := domain.Decide(f, r)
	if againErr != nil || !sameMarker(d.Derived, again.Derived) || stripMarker(d) != stripMarker(again) ||
		d.Derived != nil && d.Derived == again.Derived {
		t.Fatal("decision is not deterministic")
	}
	switch {
	case r.Expected != f.Latest:
		t.Fatal("stale command succeeded")
	case f.State == domain.StateVoided:
		t.Fatal("voided record changed")
	case !r.Authority.Edit && !r.Authority.Amend && !r.Authority.Void:
		t.Fatal("unauthorized command succeeded")
	case d.Effective != f.Effective:
		t.Fatal("a decision changed the effective revision")
	case d.RecordID != f.RecordID || d.ActorID != r.ActorID || !d.At.Equal(r.At) || d.ReportingDay != f.ReportingDay:
		t.Fatal("decision lost its author, time or record")
	}
	switch d.Outcome {
	case domain.OutcomeEdit:
		if r.Intent != domain.IntentEdit || d.Revision != f.Latest+1 || d.Base != f.Latest || d.Voided || d.Derived != nil ||
			(f.State != domain.StateDraft && f.State != domain.StateReturned) {
			t.Fatalf("edit broke the append-only chain: %+v", d)
		}
	case domain.OutcomeAmendment:
		if r.Intent != domain.IntentAmend || f.Effective == 0 || pendingFacts(f) || d.Revision != f.Latest+1 ||
			d.Base != f.Effective || d.Voided || d.Derived != nil || r.Reason == "" || !writablePeriod(f.Period) {
			t.Fatalf("amendment broke the append-only chain: %+v", d)
		}
	case domain.OutcomeVoid:
		if r.Intent != domain.IntentVoid || f.Effective == 0 || pendingFacts(f) || d.Revision != 0 || d.Base != f.Effective ||
			!d.Voided || r.Reason == "" || !writablePeriod(f.Period) || d.Derived == nil ||
			*d.Derived != (domain.Recalculation{RecordID: f.RecordID, Revision: f.Effective, Cause: domain.CauseVoided, Day: f.ReportingDay, At: d.At}) {
			t.Fatalf("void lost history: %+v", d)
		}
	case domain.OutcomeAdjustment:
		if r.Intent != domain.IntentAdjust || f.Effective == 0 || pendingFacts(f) || d.Revision != 0 || d.Base != f.Effective ||
			d.Voided || r.Reason == "" || f.Period.State != domain.PeriodLocked || !writablePeriod(*r.PostingPeriod) ||
			d.PostingDay <= f.ReportingDay || d.Derived == nil ||
			*d.Derived != (domain.Recalculation{RecordID: f.RecordID, Revision: f.Effective, Cause: domain.CauseAdjusted, Day: d.PostingDay, At: d.At}) {
			t.Fatalf("adjustment broke the original link: %+v", d)
		}
	default:
		t.Fatalf("unknown outcome %q", d.Outcome)
	}
	return d, true
}

func TestResolveInvariants(t *testing.T) {
	approvals := 0
	for _, state := range allStates {
		for effective := int64(0); effective <= 2; effective++ {
			for latest := int64(1); latest <= 3; latest++ {
				for _, period := range periods() {
					facts := domain.Facts{
						RecordID: uuid(1), State: state, Effective: effective, Latest: latest,
						ReportingDay: "2026-10-31", Period: period,
					}
					for _, result := range allResults {
						for _, judged := range []int64{latest - 1, latest, latest + 1} {
							v := domain.Review{Revision: judged, Result: result, At: receipt}
							res, err := domain.Resolve(facts, v)
							again, againErr := domain.Resolve(facts, v)
							if !sameMarker(res.Derived, again.Derived) || res.Derived != nil && res.Derived == again.Derived ||
								stripResolution(res) != stripResolution(again) || againErr != err {
								t.Fatal("resolution is not deterministic")
							}
							if err != nil {
								closedError(t, err)
								if res != (domain.Resolution{}) {
									t.Fatal("a refusal returned a partial resolution")
								}
								continue
							}
							if judged != latest || !pendingFacts(facts) || res.Revision != latest || res.RecordID != facts.RecordID {
								t.Fatalf("resolved a revision that is not the pending amendment: %+v", res)
							}
							switch result {
							case domain.StateApproved:
								approvals++
								want := domain.Recalculation{RecordID: facts.RecordID, Revision: latest, Superseded: effective, Cause: domain.CauseAmended, Day: facts.ReportingDay, At: receipt}
								if !writablePeriod(period) || res.Effective != latest || res.Superseded != effective || res.Pending ||
									res.Derived == nil || *res.Derived != want {
									t.Fatalf("approval did not supersede and retain: %+v", res)
								}
							case domain.StateReturned, domain.StateRejected, domain.StateCancelled:
								if res.Effective != effective || res.Superseded != 0 || res.Derived != nil || res.Pending != (result == domain.StateReturned) {
									t.Fatalf("non-approval changed the effective revision: %+v", res)
								}
							default:
								t.Fatalf("accepted a non-outcome result %q", result)
							}
						}
					}
				}
			}
		}
	}
	if approvals == 0 {
		t.Error("enumeration never approved an amendment")
	}
}
