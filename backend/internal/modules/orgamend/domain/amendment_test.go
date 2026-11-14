package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
)

func approvedFacts(period domain.Period) domain.Facts {
	return domain.Facts{
		RecordID: uuid(1), State: domain.StateApproved, Effective: 1, Latest: 1,
		ReportingDay: "2026-10-31", Period: period,
	}
}

func amendRequest(at time.Time) domain.Request {
	return domain.Request{
		Intent: domain.IntentAmend, Expected: 1, Revision: 2, Base: 1, ActorID: uuid(31), At: at,
		Reason: "Pump quantity corrected", Authority: domain.Authority{Amend: true},
	}
}

func TestDecisionTimeIsNormalizedServerReceipt(t *testing.T) {
	local := time.FixedZone("BRT", -3*3600)
	// A monotonic reading is process-local and must not survive into the value.
	at := time.Now().In(local)
	d, err := domain.Decide(approvedFacts(domain.Period{State: domain.PeriodOpen}), amendRequest(at))
	if err != nil {
		t.Fatal(err)
	}
	if d.At.Location() != time.UTC || !d.At.Equal(at) || d.At != d.At.Round(-1) {
		t.Fatal("decision time is not the normalized wall-clock receipt")
	}
	v, err := domain.Resolve(domain.Facts{
		RecordID: uuid(1), State: domain.StateInReview, Effective: 1, Latest: 2,
		ReportingDay: "2026-10-31", Period: domain.Period{State: domain.PeriodOpen},
	}, domain.Review{Revision: 2, Result: domain.StateApproved, At: at})
	if err != nil {
		t.Fatal(err)
	}
	if v.Derived.At.Location() != time.UTC || !v.Derived.At.Equal(at) || v.Derived.At != v.Derived.At.Round(-1) {
		t.Fatal("marker time is not the normalized wall-clock decision time")
	}
}

func TestBoundaryInstantsAndDays(t *testing.T) {
	period := domain.Period{State: domain.PeriodOpen}
	for name, at := range map[string]time.Time{
		"year 10000": time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
		"zero":       {},
	} {
		if _, err := domain.Decide(approvedFacts(period), amendRequest(at)); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s receipt accepted", name)
		}
	}
	if _, err := domain.Decide(approvedFacts(period), amendRequest(time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC))); err != nil {
		t.Error("latest bounded receipt refused")
	}
	facts := approvedFacts(period)
	facts.ReportingDay = "10000-01-01"
	if _, err := domain.Decide(facts, amendRequest(receipt)); !errors.Is(err, domain.ErrInvalid) {
		t.Error("five-digit year day accepted")
	}
	facts.ReportingDay = "0001-01-01"
	if _, err := domain.Decide(facts, amendRequest(receipt)); err != nil {
		t.Error("first calendar day refused")
	}
}

func TestDecideDoesNotRetainOrMutateInputs(t *testing.T) {
	reopening := &domain.Reopening{AuthorityProven: true, Reason: "Late receipt correction"}
	posting := &domain.Period{State: domain.PeriodReopened, Reopening: &domain.Reopening{AuthorityProven: true, Reason: "Posting reopening"}}
	facts := approvedFacts(domain.Period{State: domain.PeriodLocked})
	r := amendRequest(receipt)
	r.Intent, r.Revision, r.PostingDay, r.PostingPeriod = domain.IntentAdjust, 0, "2026-11-14", posting
	first, err := domain.Decide(facts, r)
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.Decide(facts, r)
	if err != nil {
		t.Fatal(err)
	}
	if first.Derived == second.Derived {
		t.Fatal("markers share storage across decisions")
	}
	first.Derived.Revision = 99
	if second.Derived.Revision != 1 {
		t.Fatal("a caller can rewrite another decision's marker")
	}
	if *posting.Reopening != (domain.Reopening{AuthorityProven: true, Reason: "Posting reopening"}) || posting.State != domain.PeriodReopened {
		t.Fatal("posting period facts were mutated")
	}
	reopened := approvedFacts(domain.Period{State: domain.PeriodReopened, Reopening: reopening})
	if _, err := domain.Decide(reopened, amendRequest(receipt)); err != nil {
		t.Fatal(err)
	}
	if *reopening != (domain.Reopening{AuthorityProven: true, Reason: "Late receipt correction"}) {
		t.Fatal("reopening facts were mutated")
	}
}

// A decision applied by the consumer, replayed against the new facts, never
// produces a second effect (V-IDEM).
func TestAppliedDecisionsReplayAsRefusals(t *testing.T) {
	open := domain.Period{State: domain.PeriodOpen}
	amend := amendRequest(receipt)
	d, err := domain.Decide(approvedFacts(open), amend)
	if err != nil {
		t.Fatal(err)
	}
	afterAmend := domain.Facts{RecordID: uuid(1), State: domain.StateSubmitted, Effective: d.Effective, Latest: d.Revision, ReportingDay: d.ReportingDay, Period: open}
	if _, err := domain.Decide(afterAmend, amend); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("amendment replay was not a conflict")
	}
	review := domain.Review{Revision: d.Revision, Result: domain.StateApproved, At: receipt}
	res, err := domain.Resolve(afterAmend, review)
	if err != nil {
		t.Fatal(err)
	}
	afterApproval := domain.Facts{RecordID: uuid(1), State: domain.StateApproved, Effective: res.Effective, Latest: res.Revision, ReportingDay: d.ReportingDay, Period: open}
	if _, err := domain.Resolve(afterApproval, review); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("approval replay was not a conflict")
	}
	void := amendRequest(receipt)
	void.Intent, void.Expected, void.Revision, void.Base, void.Authority = domain.IntentVoid, 2, 0, 2, domain.Authority{Void: true}
	v, err := domain.Decide(afterApproval, void)
	if err != nil || !v.Voided || v.Derived.Revision != 2 {
		t.Fatal("void of the amended record failed", err)
	}
	afterVoid := afterApproval
	afterVoid.State = domain.StateVoided
	if _, err := domain.Decide(afterVoid, void); !errors.Is(err, domain.ErrAlreadyVoided) {
		t.Fatal("void replay was not refused")
	}
}
