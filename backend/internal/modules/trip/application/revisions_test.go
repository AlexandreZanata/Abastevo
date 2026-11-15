package application_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	amend "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgamend/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

var revisionAt = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// history builds stored entries in sequence order from compact steps.
func history(steps ...app.ReviewEntry) []app.ReviewEntry {
	out := make([]app.ReviewEntry, len(steps))
	for i, s := range steps {
		s.Sequence = int64(i + 1)
		if s.ActorMembershipID == "" {
			s.ActorMembershipID, s.ActorEmployeeID = id(3), id(30)
		}
		s.OperationID, s.RecordedAt = id(100+i), revisionAt.Add(time.Duration(i)*time.Minute)
		out[i] = s
	}
	return out
}

func step(kind review.Kind, revision int64, state review.State, reason string) app.ReviewEntry {
	return app.ReviewEntry{Kind: kind, Revision: revision, State: state, Reason: reason}
}

var (
	submitted1 = step(review.Submit, 1, review.Submitted, "")
	opened1    = step(review.Open, 1, review.InReview, "")
	approved1  = step(review.Approve, 1, review.Approved, "")
	amended2   = app.ReviewEntry{Kind: app.Amend, Revision: 2, State: review.Submitted, Reason: "Leitura final digitada errada", ActorMembershipID: id(7), ActorEmployeeID: id(70)}
)

func mustRevisions(t *testing.T, entries []app.ReviewEntry) app.Revisions {
	t.Helper()
	r, err := app.TripRevisions(entries)
	if err != nil {
		t.Fatal("valid stored history refused", err)
	}
	return r
}

// EDR-234 / B-BR-EMP-363/365: the field segment with its resubmission and
// one segment per amendment, with the approved revision in effect.
func TestTripRevisionsReadFieldResubmissionsAndAmendmentSegments(t *testing.T) {
	loop := mustRevisions(t, history(submitted1, opened1, step(review.Return, 1, review.Returned, "Informe o motivo da parada"),
		step(review.Resubmit, 2, review.Submitted, ""), step(review.Open, 2, review.InReview, ""), step(review.Approve, 2, review.Approved, "")))
	if loop.State() != review.Approved || loop.Latest() != 2 || loop.Effective() != 2 || loop.FieldOpen() {
		t.Fatal("the VAL-01 return and resubmission loop", loop.State(), loop.Latest(), loop.Effective())
	}
	if s, ok := loop.RevisionState(1); !ok || s != review.Returned {
		t.Fatal("the returned revision was not kept", s)
	}
	pending := mustRevisions(t, history(submitted1, opened1, approved1, amended2))
	if pending.State() != review.Submitted || pending.Latest() != 2 || pending.Effective() != 1 || pending.FieldOpen() {
		t.Fatal("a pending amendment changed the effective revision", pending.Effective())
	}
	if e, ok := pending.Amendment(); !ok || e.ActorMembershipID != id(7) || e.Revision != 2 {
		t.Fatal("the amendment segment lost its author")
	}
	rejected := mustRevisions(t, history(submitted1, opened1, approved1, amended2, step(review.Open, 2, review.InReview, ""), step(review.Reject, 2, review.Rejected, "Sem evidência")))
	if rejected.State() != review.Rejected || rejected.Effective() != 1 || rejected.Latest() != 2 {
		t.Fatal("a rejected amendment changed the effective revision")
	}
	edited := mustRevisions(t, history(submitted1, opened1, approved1, amended2, step(review.Open, 2, review.InReview, ""), step(review.Return, 2, review.Returned, "Confirme o valor"),
		app.ReviewEntry{Kind: app.Amend, Revision: 3, State: review.Submitted, Reason: "Valor confirmado", ActorMembershipID: id(7), ActorEmployeeID: id(70)},
		step(review.Open, 3, review.InReview, ""), step(review.Approve, 3, review.Approved, "")))
	if edited.Effective() != 3 || edited.Latest() != 3 || edited.State() != review.Approved {
		t.Fatal("an approved edit of a returned amendment is not in effect", edited.Effective())
	}
	for name, broken := range map[string][]app.ReviewEntry{
		"amend before approval":      history(submitted1, app.ReviewEntry{Kind: app.Amend, Revision: 2, State: review.Submitted, Reason: "x"}),
		"amend skipping a revision":  history(submitted1, opened1, approved1, app.ReviewEntry{Kind: app.Amend, Revision: 3, State: review.Submitted, Reason: "x"}),
		"amend without reason":       history(submitted1, opened1, approved1, app.ReviewEntry{Kind: app.Amend, Revision: 2, State: review.Submitted}),
		"amend beside pending":       history(submitted1, opened1, approved1, amended2, app.ReviewEntry{Kind: app.Amend, Revision: 3, State: review.Submitted, Reason: "x"}),
		"resubmit an amendment":      history(submitted1, opened1, approved1, amended2, step(review.Open, 2, review.InReview, ""), step(review.Return, 2, review.Returned, "x"), step(review.Resubmit, 3, review.Submitted, "")),
		"resubmit an approved trip":  history(submitted1, opened1, approved1, step(review.Resubmit, 2, review.Submitted, "")),
		"decision of an old segment": history(submitted1, opened1, approved1, amended2, step(review.Open, 1, review.InReview, "")),
		"second submit":              history(submitted1, step(review.Submit, 1, review.Submitted, "")),
	} {
		if _, err := app.TripRevisions(broken); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("impossible revision history accepted", name, err)
		}
	}
}

// B-BR-EMP-366 / BR-REV-003: an amendment revision is decided with the
// amendment's author excluded too; an earlier revision is CONFLICT.
func TestDecideRevisionOnAnAmendmentExcludesItsAuthor(t *testing.T) {
	pending := mustRevisions(t, history(submitted1, opened1, approved1, amended2))
	author := app.Person{AccountID: id(1), MembershipID: id(3), Employees: []string{id(30)}}
	driver := app.Person{Employees: []string{id(31)}}
	amender := app.Person{AccountID: id(8), MembershipID: id(7), Employees: []string{id(70)}}
	reviewer := app.Person{AccountID: id(5), MembershipID: id(6), Employees: []string{id(60)}}
	in := app.DecisionInput{OperationID: id(90), TripID: id(40), ExpectedRevision: 2, Decision: review.Approve}
	added, err := app.DecideRevision(pending, in, reviewer, author, driver, &amender)
	if err != nil || len(added) != 2 || added[0].Revision != 2 || added[1].Revision != 2 || added[1].State != review.Approved {
		t.Fatal("the amendment revision was not opened and approved at its absolute revision", err)
	}
	if _, err := app.DecideRevision(pending, in, amender, author, driver, &amender); !errors.Is(err, review.ErrSelfApproval) {
		t.Fatal("the amendment's author approved it", err)
	}
	stale := in
	stale.ExpectedRevision = 1
	if _, err := app.DecideRevision(pending, stale, reviewer, author, driver, &amender); !errors.Is(err, review.ErrConflict) {
		t.Fatal("a decision naming the approved revision was not CONFLICT", err)
	}
	if _, err := app.SubmitEntry(pending, review.Submit, 2, author, author); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("a field submission entered an amendment segment", err)
	}
	if _, err := app.SubmitKind(trip.Completed, pending, 2); !errors.Is(err, app.ErrSubmitStale) {
		t.Fatal("a submission of an amended trip is not stale", err)
	}
}

func correctionOf(kind meter.Kind, value int64) *app.Correction {
	return &app.Correction{Kind: kind, Value: value}
}

func amendmentCommand(t *testing.T, edit func(*app.AmendmentInput)) app.AmendmentCommand {
	t.Helper()
	in := app.AmendmentInput{OperationID: id(80), TripID: id(40), ExpectedRevision: 1, Reason: "Leitura final digitada errada", End: correctionOf(meter.Odometer, 184_600_000)}
	if edit != nil {
		edit(&in)
	}
	c, err := app.NewAmendmentCommand(in)
	if err != nil {
		t.Fatal("valid amendment refused", err)
	}
	return c
}

// B-BR-EMP-365: shape, reason profile, corrections and a digest binding
// every field.
func TestAmendmentCommandShapeAndDigest(t *testing.T) {
	scope := tripScope(t)
	base := amendmentCommand(t, nil)
	digest := base.Digest(scope)
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		t.Fatal("amendment digest is not lowercase SHA-256 hex", digest)
	}
	for name, edit := range map[string]func(*app.AmendmentInput){
		"reason":   func(v *app.AmendmentInput) { v.Reason = "Outra razão" },
		"value":    func(v *app.AmendmentInput) { v.End = correctionOf(meter.Odometer, 184_600_001) },
		"start":    func(v *app.AmendmentInput) { v.Start = correctionOf(meter.Odometer, 184_310_000) },
		"revision": func(v *app.AmendmentInput) { v.ExpectedRevision = 2 },
	} {
		if amendmentCommand(t, edit).Digest(scope) == digest {
			t.Fatal("a field does not bind the digest", name)
		}
	}
	for name, c := range map[string]struct {
		edit func(*app.AmendmentInput)
		want error
	}{
		"no reason":          {func(v *app.AmendmentInput) { v.Reason = "" }, period.ErrReasonRequired},
		"blank reason":       {func(v *app.AmendmentInput) { v.Reason = " 　 " }, period.ErrReasonRequired},
		"control reason":     {func(v *app.AmendmentInput) { v.Reason = "a\tb" }, period.ErrReasonInvalid},
		"281 scalars":        {func(v *app.AmendmentInput) { v.Reason = strings.Repeat("é", 281) }, period.ErrReasonInvalid},
		"no correction":      {func(v *app.AmendmentInput) { v.End = nil }, app.ErrInvalidAmendment},
		"unknown kind":       {func(v *app.AmendmentInput) { v.End = correctionOf(meter.Kind("KILOMETRE"), 1) }, app.ErrInvalidAmendment},
		"negative value":     {func(v *app.AmendmentInput) { v.End = correctionOf(meter.Odometer, -1) }, app.ErrInvalidAmendment},
		"zero revision":      {func(v *app.AmendmentInput) { v.ExpectedRevision = 0 }, review.ErrInvalid},
		"revision above cap": {func(v *app.AmendmentInput) { v.ExpectedRevision = app.MaxReviewRevision + 1 }, review.ErrInvalid},
		"non-canonical trip": {func(v *app.AmendmentInput) { v.TripID = strings.ToUpper(id(40)) }, review.ErrInvalid},
	} {
		in := app.AmendmentInput{OperationID: id(80), TripID: id(40), ExpectedRevision: 1, Reason: "Leitura final digitada errada", End: correctionOf(meter.Odometer, 184_600_000)}
		c.edit(&in)
		if _, err := app.NewAmendmentCommand(in); !errors.Is(err, c.want) {
			t.Fatal("invalid amendment admitted", name, err)
		}
	}
	if (app.AmendmentCommand{}).Digest(scope) != "" {
		t.Fatal("the zero command has a digest")
	}
	if p := app.AmendPolicy(); p != fleetapp.MeterReviewPolicy(true) || p.Validate() != nil {
		t.Fatal("the amendment policy is not the METER-004 correction policy", p)
	}
}

func tripScope(t *testing.T) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(id(65))
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

// B-BR-EMP-365/366 and BR-AMD-002/004 through the orgamend kernel: AMEND
// from the approved revision, EDIT of a returned amendment, the pending,
// never-approved, stale and locked refusals, and Resolve.
func TestProposeAndResolveAmendmentsThroughTheKernel(t *testing.T) {
	open := amend.Period{State: amend.PeriodOpen}
	locked := amend.Period{State: amend.PeriodLocked}
	approved := mustRevisions(t, history(submitted1, opened1, approved1))
	d, err := app.ProposeAmendment(approved, id(40), "2026-11-15", open, amendmentCommand(t, nil), id(8), revisionAt)
	if err != nil || d.Outcome != amend.OutcomeAmendment || d.Revision != 2 || d.Base != 1 || d.Effective != 1 || d.Reason != "Leitura final digitada errada" {
		t.Fatal("an amendment of the approved revision", err, d.Outcome, d.Revision, d.Base)
	}
	if _, err := app.ProposeAmendment(approved, id(40), "2026-11-15", locked, amendmentCommand(t, nil), id(8), revisionAt); !errors.Is(err, amend.ErrPeriodLocked) {
		t.Fatal("an amendment in a locked month", err)
	}
	stale := amendmentCommand(t, func(v *app.AmendmentInput) { v.ExpectedRevision = 2 })
	if _, err := app.ProposeAmendment(approved, id(40), "2026-11-15", open, stale, id(8), revisionAt); !errors.Is(err, amend.ErrConflict) {
		t.Fatal("a stale amendment", err)
	}
	draft := mustRevisions(t, history(submitted1))
	if _, err := app.ProposeAmendment(draft, id(40), "2026-11-15", open, amendmentCommand(t, nil), id(8), revisionAt); !errors.Is(err, amend.ErrNotApproved) {
		t.Fatal("an amendment of a never-approved trip", err)
	}
	pending := mustRevisions(t, history(submitted1, opened1, approved1, amended2))
	second := amendmentCommand(t, func(v *app.AmendmentInput) { v.ExpectedRevision = 2 })
	if _, err := app.ProposeAmendment(pending, id(40), "2026-11-15", open, second, id(8), revisionAt); !errors.Is(err, amend.ErrAmendmentPending) {
		t.Fatal("a second amendment beside a pending one", err)
	}
	returned := mustRevisions(t, history(submitted1, opened1, approved1, amended2, step(review.Open, 2, review.InReview, ""), step(review.Return, 2, review.Returned, "Confirme o valor")))
	edit, err := app.ProposeAmendment(returned, id(40), "2026-11-15", open, second, id(8), revisionAt)
	if err != nil || edit.Outcome != amend.OutcomeEdit || edit.Revision != 3 || edit.Base != 2 || edit.Effective != 1 {
		t.Fatal("an edit of a returned amendment", err, edit.Outcome)
	}
	if _, err := app.ProposeAmendment(returned, id(40), "2026-11-15", locked, second, id(8), revisionAt); !errors.Is(err, amend.ErrPeriodLocked) {
		t.Fatal("an edit of a returned amendment in a locked month", err)
	}
	// Resolve: approval makes the amendment effective only while writable.
	res, err := app.ResolveAmendment(pending, id(40), "2026-11-15", open, review.Approved, revisionAt)
	if err != nil || res.Effective != 2 || res.Superseded != 1 || res.Derived == nil || res.Derived.Revision != 2 {
		t.Fatal("an approved amendment did not become effective with its recalculation marker", err)
	}
	if _, err := app.ResolveAmendment(pending, id(40), "2026-11-15", locked, review.Approved, revisionAt); !errors.Is(err, amend.ErrPeriodLocked) {
		t.Fatal("an amendment approved into a locked month", err)
	}
	if res, err := app.ResolveAmendment(pending, id(40), "2026-11-15", locked, review.Rejected, revisionAt); err != nil || res.Effective != 1 {
		t.Fatal("a rejected amendment changed the effective revision", err)
	}
	if _, err := app.ResolveAmendment(approved, id(40), "2026-11-15", open, review.Approved, revisionAt); !errors.Is(err, amend.ErrConflict) {
		t.Fatal("a resolution without a pending amendment", err)
	}
}

// B-BR-EMP-367 / trips §10: the usage of each named revision with the
// TRIP-001 rule; 290 000 m until the approval, 280 000 m after.
func TestRevisionUsageNamesItsRevision(t *testing.T) {
	scope := tripScope(t)
	start, end := int64(184_320_000), int64(184_610_000)
	declared := orgtime.Declaration{OccurredAt: time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC), EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
	finished := declared
	finished.OccurredAt = declared.OccurredAt.Add(3 * time.Hour)
	attribution, err := orgtime.AttributeReportingDay(declared.OccurredAt, orgtime.ReportingZones{Organization: "America/Cuiaba"})
	if err != nil {
		t.Fatal(err)
	}
	record := app.Record{ID: id(40), OperationID: id(41), PayloadDigest: "sha256:" + strings.Repeat("a", 64), AssetID: id(42), DriverID: id(30), AuthorMembershipID: id(3),
		AuthorEmployeeID: id(30), EnrollmentID: id(4), Declared: declared, ReceivedAt: declared.OccurredAt, MeterID: id(43), SegmentID: id(44), ReadingID: id(45),
		ReadingKind: string(meter.Odometer), ReadingValue: &start, Reporting: attribution, State: trip.Completed,
		Finish: &app.FinishRecord{OperationID: id(46), PayloadDigest: "sha256:" + strings.Repeat("b", 64), AuthorMembershipID: id(3), Declared: finished,
			ReceivedAt: finished.OccurredAt, MeterID: id(43), SegmentID: id(44), ReadingID: id(47), ReadingKind: string(meter.Odometer), ReadingValue: &end, EndDay: "2026-11-15"}}
	amendments := []app.AmendmentRecord{{Revision: 2, Base: 1, Intent: amend.IntentAmend, End: &app.CorrectionRecord{MeterID: id(43), ReadingID: id(47), NewReadingID: id(48), From: end, To: 184_600_000}}}
	pending := mustRevisions(t, history(submitted1, opened1, approved1, amended2))
	s, e := app.Values(record, amendments, pending, pending.Effective())
	usage, err := app.UsageOf(scope, record, pending.Effective(), s, e)
	if err != nil || usage.Status != trip.Measured || usage.Delta != 290_000 || usage.Revision != 1 || usage.Kind != meter.Odometer {
		t.Fatal("the effective revision before approval", err, usage.Delta)
	}
	s, e = app.Values(record, amendments, pending, 2)
	if usage, err = app.UsageOf(scope, record, 2, s, e); err != nil || usage.Delta != 280_000 || usage.Revision != 2 {
		t.Fatal("the pending amendment's own usage", err, usage.Delta)
	}
	approved := mustRevisions(t, history(submitted1, opened1, approved1, amended2, step(review.Open, 2, review.InReview, ""), step(review.Approve, 2, review.Approved, "")))
	s, e = app.Values(record, amendments, approved, approved.Effective())
	if usage, err = app.UsageOf(scope, record, approved.Effective(), s, e); err != nil || usage.Delta != 280_000 || usage.Revision != 2 {
		t.Fatal("the approved amendment's usage (trips §10)", err, usage.Delta)
	}
	if *record.Finish.ReadingValue != end {
		t.Fatal("deriving a revision rewrote the stored value")
	}
	// A correction below the start is never an invented distance.
	backwards := []app.AmendmentRecord{{Revision: 2, Base: 1, Intent: amend.IntentAmend, End: &app.CorrectionRecord{MeterID: id(43), ReadingID: id(47), NewReadingID: id(48), From: end, To: 184_000_000}}}
	s, e = app.Values(record, backwards, approved, 2)
	if usage, err = app.UsageOf(scope, record, 2, s, e); err != nil || usage.Status != trip.NotMeasured || usage.Reason != trip.ReasonDecreasing {
		t.Fatal("a decreasing correction invented a distance", err, usage.Status)
	}
	// A rejected amendment never contributes to a later revision's values.
	rejected := mustRevisions(t, history(submitted1, opened1, approved1, amended2, step(review.Open, 2, review.InReview, ""), step(review.Reject, 2, review.Rejected, "Sem evidência")))
	s, e = app.Values(record, amendments, rejected, 1)
	if *s != start || *e != end {
		t.Fatal("a rejected amendment's value leaked into the effective revision")
	}
}
