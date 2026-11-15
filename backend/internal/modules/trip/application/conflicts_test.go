package application_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

var conflictAt = time.Date(2026, 11, 15, 12, 0, 0, 123, time.UTC)

// heldStartView is an OPEN held trip.start declared at conflictAt.
func heldStartView(trips ...app.ConflictTrip) app.ConflictView {
	declared := orgtime.Declaration{OccurredAt: conflictAt, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
	value := int64(184_320_000)
	return app.ConflictView{ReviewItemID: id(70), OperationID: id(71), OperationType: app.StartOperationType, PayloadDigest: "sha256:" + strings.Repeat("a", 64),
		Code: app.AssetBusy, State: app.ConflictOpen, Revision: 1, AuthorAccountID: id(72), AuthorMembershipID: id(73), AuthorEmployeeID: id(74), EnrollmentID: id(75),
		ReceivedAt: conflictAt.Add(time.Hour), ExpiresAt: conflictAt.Add(61 * 24 * time.Hour),
		Fact: app.ConflictFact{TripID: id(76), Declared: declared, Start: &app.HeldStartFact{TripID: id(76), AssetID: id(77), DriverID: id(74), Declared: declared,
			MeterID: id(78), SegmentID: id(79), ReadingID: id(80), ReadingKind: "ODOMETER", ReadingValue: &value}}, Trips: trips}
}

func ptr(t time.Time) *time.Time { return &t }

func finished(id string, from, to time.Time) app.ConflictTrip {
	return app.ConflictTrip{TripID: id, State: trip.Completed, StartedAt: from, FinishedAt: &to}
}

func decisionOf(t *testing.T, kind app.ConflictDecisionKind, reason string) app.ConflictDecisionInput {
	t.Helper()
	command, err := app.NewConflictDecision(app.ConflictDecisionInput{OperationID: id(90), ReviewItemID: id(70), ExpectedRevision: 1, Decision: kind, Reason: reason})
	if err != nil {
		t.Fatal(err)
	}
	v, err := command.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return v
}

var currentFacts = app.ConflictFacts{AssetActive: true, AuthorCurrent: true, DriverActive: true}

// B-BR-EMP-409: a held start is acceptable only when no stored trip of its
// asset is in progress or finished after its declared instant.
func TestConflictAcceptancePreservesOneActiveTripPerAsset(t *testing.T) {
	accept := decisionOf(t, app.ConflictAccept, "")
	before := finished(id(81), conflictAt.Add(-3*time.Hour), conflictAt.Add(-time.Hour))
	exactly := finished(id(82), conflictAt.Add(-2*time.Hour), conflictAt)
	for name, c := range map[string]struct {
		trips []app.ConflictTrip
		want  error
	}{
		"no trip":                             {nil, nil},
		"finished before":                     {[]app.ConflictTrip{before}, nil},
		"finished exactly at the start":       {[]app.ConflictTrip{exactly}, nil},
		"finished a nanosecond after":         {[]app.ConflictTrip{finished(id(83), conflictAt.Add(-time.Hour), conflictAt.Add(time.Nanosecond))}, app.ErrConflictUnacceptable},
		"in progress since before":            {[]app.ConflictTrip{{TripID: id(84), State: trip.InProgress, StartedAt: conflictAt.Add(-time.Hour)}}, app.ErrConflictUnacceptable},
		"started later, still running":        {[]app.ConflictTrip{{TripID: id(85), State: trip.InProgress, StartedAt: conflictAt.Add(5 * time.Minute)}}, app.ErrConflictUnacceptable},
		"started later and finished (VAL-07)": {[]app.ConflictTrip{before, finished(id(86), conflictAt.Add(5*time.Minute), conflictAt.Add(4*time.Hour))}, app.ErrConflictUnacceptable},
		"paused trip":                         {[]app.ConflictTrip{{TripID: id(87), State: trip.Paused, StartedAt: conflictAt.Add(-time.Hour)}}, app.ErrConflictUnacceptable},
		"cancelled before the start":          {[]app.ConflictTrip{{TripID: id(91), State: trip.Cancelled, StartedAt: conflictAt.Add(-2 * time.Hour), FinishedAt: ptr(conflictAt.Add(-time.Hour))}}, nil},
		"cancelled after the start":           {[]app.ConflictTrip{{TripID: id(92), State: trip.Cancelled, StartedAt: conflictAt.Add(-2 * time.Hour), FinishedAt: ptr(conflictAt.Add(time.Minute))}}, app.ErrConflictUnacceptable},
	} {
		t.Run(name, func(t *testing.T) {
			err := app.DecideConflict(heldStartView(c.trips...), accept, currentFacts)
			if c.want == nil && err != nil || c.want != nil && (!errors.Is(err, c.want) || !errors.Is(err, review.ErrConflict)) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
	if !finished(id(88), conflictAt.Add(-time.Hour), conflictAt.Add(time.Second)).Overlaps(conflictAt) || before.Overlaps(conflictAt) || exactly.Overlaps(conflictAt) {
		t.Fatal("overlap rule drift")
	}
}

// B-BR-EMP-408/409: the asset, the author and the driver must still admit a
// trip; a refusal is always admitted; only a held start can be accepted.
func TestConflictDecisionRulesFollowTheItemAndTheFacts(t *testing.T) {
	accept, refuse := decisionOf(t, app.ConflictAccept, "Liberado pela gestão"), decisionOf(t, app.ConflictRefuse, "Veículo em uso por outra viagem")
	disposedAfter, disposedAt := conflictAt.Add(time.Hour), conflictAt
	for name, facts := range map[string]app.ConflictFacts{
		"asset not active":            {AuthorCurrent: true, DriverActive: true},
		"disposed at the declaration": {AssetActive: true, DisposedAt: &disposedAt, AuthorCurrent: true, DriverActive: true},
		"author ended":                {AssetActive: true, DriverActive: true},
		"driver merged or inactive":   {AssetActive: true, AuthorCurrent: true},
	} {
		if err := app.DecideConflict(heldStartView(), accept, facts); !errors.Is(err, app.ErrConflictUnacceptable) {
			t.Fatal(name, err)
		}
		if err := app.DecideConflict(heldStartView(), refuse, facts); err != nil {
			t.Fatal("refusal not admitted", name, err)
		}
	}
	if err := app.DecideConflict(heldStartView(), accept, app.ConflictFacts{AssetActive: true, DisposedAt: &disposedAfter, AuthorCurrent: true, DriverActive: true}); err != nil {
		t.Fatal("a disposal after the declaration refused the acceptance", err)
	}
	for _, kind := range []string{app.FinishOperationType, app.EventOperationType, app.SubmitOperationType, app.PauseOperationType, app.ResumeOperationType, app.CancelOperationType} {
		view := heldStartView()
		view.OperationType, view.Fact.Start = kind, nil
		if err := app.DecideConflict(view, accept, currentFacts); !errors.Is(err, app.ErrConflictUnacceptable) {
			t.Fatal("accepted a held", kind, err)
		}
		if err := app.DecideConflict(view, refuse, currentFacts); err != nil {
			t.Fatal("refusal of a held", kind, err)
		}
	}
	// BR-REV-007: a resolved item or another revision is a CONFLICT first.
	stale := refuse
	stale.ExpectedRevision = 2
	resolved := heldStartView()
	resolved.Revision, resolved.State = 2, app.ConflictRefused
	if err := app.DecideConflict(heldStartView(), stale, currentFacts); !errors.Is(err, review.ErrConflict) || errors.Is(err, app.ErrConflictUnacceptable) {
		t.Fatal("stale revision", err)
	}
	if err := app.DecideConflict(resolved, refuse, currentFacts); !errors.Is(err, review.ErrConflict) {
		t.Fatal("second resolution", err)
	}
}

// B-BR-EMP-410: author and declared driver identities, after merges.
func TestConflictSelfReviewCoversAuthorAndDriver(t *testing.T) {
	author := app.ConflictParty{AccountID: id(1), MembershipID: id(2), Employees: []string{id(3), id(4)}}
	driver := app.ConflictParty{Employees: []string{id(5)}}
	for name, reviewer := range map[string]app.ConflictParty{
		"same account":           {AccountID: id(1), MembershipID: id(9), Employees: []string{id(10)}},
		"same membership":        {AccountID: id(9), MembershipID: id(2), Employees: []string{id(10)}},
		"merged into the author": {AccountID: id(9), MembershipID: id(8), Employees: []string{id(11), id(4)}},
		"the declared driver":    {AccountID: id(9), MembershipID: id(8), Employees: []string{id(5)}},
	} {
		if !app.SelfReview(reviewer, author, driver) {
			t.Fatal("self review admitted", name)
		}
	}
	if app.SelfReview(app.ConflictParty{AccountID: id(9), MembershipID: id(8), Employees: []string{id(10)}}, author, driver) {
		t.Fatal("an independent reviewer was refused")
	}
}

// B-BR-EMP-408/410: the command shape, the reason profile and the digest.
func TestConflictDecisionCommandShapeAndDigest(t *testing.T) {
	base := app.ConflictDecisionInput{OperationID: id(90), ReviewItemID: id(70), ExpectedRevision: 1, Decision: app.ConflictRefuse, Reason: "Motivo"}
	for name, c := range map[string]struct {
		change func(*app.ConflictDecisionInput)
		want   error
	}{
		"refusal without reason": {func(v *app.ConflictDecisionInput) { v.Reason = "" }, review.ErrReasonRequired},
		"blank refusal":          {func(v *app.ConflictDecisionInput) { v.Reason = "   " }, review.ErrReasonRequired},
		"blank acceptance note":  {func(v *app.ConflictDecisionInput) { v.Decision, v.Reason = app.ConflictAccept, "  " }, review.ErrInvalid},
		"control character":      {func(v *app.ConflictDecisionInput) { v.Reason = "linha\nnova" }, review.ErrInvalid},
		"281 scalars":            {func(v *app.ConflictDecisionInput) { v.Reason = strings.Repeat("é", 281) }, review.ErrInvalid},
		"unknown decision":       {func(v *app.ConflictDecisionInput) { v.Decision = "ATTACH" }, review.ErrInvalid},
		"zero revision":          {func(v *app.ConflictDecisionInput) { v.ExpectedRevision = 0 }, review.ErrInvalid},
		"non-canonical item":     {func(v *app.ConflictDecisionInput) { v.ReviewItemID = strings.ToUpper(id(70)) }, review.ErrInvalid},
		"no operation":           {func(v *app.ConflictDecisionInput) { v.OperationID = "" }, review.ErrInvalid},
	} {
		v := base
		c.change(&v)
		if _, err := app.NewConflictDecision(v); !errors.Is(err, c.want) {
			t.Fatal(name, err)
		}
	}
	accept := base
	accept.Decision, accept.Reason = app.ConflictAccept, ""
	a, err := app.NewConflictDecision(accept)
	r, err2 := app.NewConflictDecision(base)
	if err != nil || err2 != nil {
		t.Fatal(err, err2)
	}
	org, _ := kernel.ParseOrgID(id(99))
	scope, _ := kernel.NewOrgScope(org)
	other, _ := kernel.ParseOrgID(id(98))
	otherScope, _ := kernel.NewOrgScope(other)
	if d := a.Digest(scope); len(d) != 64 || d == r.Digest(scope) || d == a.Digest(otherScope) || d != a.Digest(scope) {
		t.Fatal("digest does not bind the command and the organization")
	}
	if _, err := (app.ConflictDecision{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) || (app.ConflictDecision{}).Digest(scope) != "" {
		t.Fatal("the zero command is usable", err)
	}
	if app.ConflictPolicy(true) != app.ReviewPolicy(app.ReviewDecideAction) || app.ConflictPolicy(false) != app.ReviewPolicy(app.ReviewReadAction) {
		t.Fatal("conflict policies are not the trip.trip.review policies")
	}
}

// B-BR-EMP-407/411: the projection admits only held trip codes, a coherent
// fact per owner and a resolution that matches its item and state.
func TestConflictViewValidationAndHeldCodes(t *testing.T) {
	for _, c := range []struct {
		kind, code string
		ok         bool
	}{
		{app.StartOperationType, app.AssetBusy, true}, {app.StartOperationType, app.AssetUnavailable, true}, {app.StartOperationType, app.PermissionChanged, true},
		{app.StartOperationType, app.TripAlreadyClosed, false}, {app.FinishOperationType, app.TripAlreadyClosed, true}, {app.FinishOperationType, app.AssetBusy, false},
		{app.EventOperationType, app.TripAlreadyClosed, true}, {app.EventOperationType, app.StaleRevision, true}, {app.SubmitOperationType, app.StaleRevision, true},
		{app.SubmitOperationType, app.TripAlreadyClosed, false}, {"meter.reading.record", app.PermissionChanged, false},
		{app.PauseOperationType, app.TripAlreadyClosed, true}, {app.ResumeOperationType, app.StaleRevision, true}, {app.CancelOperationType, app.StaleRevision, true},
		{app.CancelOperationType, app.AssetBusy, false},
	} {
		if app.HeldCode(c.kind, c.code) != c.ok {
			t.Fatal("held code", c.kind, c.code)
		}
	}
	open := heldStartView(finished(id(81), conflictAt.Add(-3*time.Hour), conflictAt.Add(-time.Hour)))
	if open.Validate() != nil {
		t.Fatal("valid open view refused")
	}
	clone := open.Clone()
	clone.Trips[0].TripID = id(1)
	if open.Trips[0].TripID != id(81) {
		t.Fatal("clone aliases the trips")
	}
	entity := int64(1)
	accepted, err := protocol.NewReviewResolution(protocol.ReviewResolutionSnapshot{OperationID: open.OperationID, PayloadDigest: open.PayloadDigest, ReviewItemID: open.ReviewItemID,
		ResolutionID: id(90), ReviewRevision: 2, Outcome: protocol.ReviewAccepted, ResolvedAt: orgtime.FormatInstant(conflictAt.Add(2 * time.Hour)), EntityRevision: &entity})
	if err != nil {
		t.Fatal(err)
	}
	done := open
	done.State, done.Revision, done.Resolution = app.ConflictAccepted, 2, &accepted
	if done.Validate() != nil {
		t.Fatal("valid accepted view refused")
	}
	for name, change := range map[string]func(*app.ConflictView){
		"meter code":   func(v *app.ConflictView) { v.Code = "ORPHAN_REFERENCE" },
		"finish shape": func(v *app.ConflictView) { v.OperationType = app.FinishOperationType },
		"accepted finish": func(v *app.ConflictView) {
			v.OperationType, v.Code, v.Fact.Finish, v.Fact.Start = app.FinishOperationType, app.TripAlreadyClosed, v.Fact.Start, nil
		},
		"resolution as refused": func(v *app.ConflictView) { v.State = app.ConflictRefused },
		"open with resolution":  func(v *app.ConflictView) { v.State, v.Revision = app.ConflictOpen, 1 },
		"resolved before receipt": func(v *app.ConflictView) {
			v.ReceivedAt = conflictAt.Add(3 * time.Hour)
			v.ExpiresAt = v.ReceivedAt.Add(time.Hour)
		},
		"completed without finish": func(v *app.ConflictView) {
			v.Trips = []app.ConflictTrip{{TripID: id(81), State: trip.Completed, StartedAt: conflictAt}}
		},
		"duplicate trip": func(v *app.ConflictView) { v.Trips = append(v.Trips, v.Trips[0]) },
		"cancelled without end": func(v *app.ConflictView) {
			v.Trips = []app.ConflictTrip{{TripID: id(81), State: trip.Cancelled, StartedAt: conflictAt}}
		},
		"pause without transition": func(v *app.ConflictView) { v.OperationType, v.Code = app.PauseOperationType, app.StaleRevision },
		"bad digest":               func(v *app.ConflictView) { v.PayloadDigest = "sha256:" + strings.Repeat("A", 64) },
	} {
		v := done.Clone()
		change(&v)
		if v.Validate() == nil {
			t.Fatal("invalid view admitted", name)
		}
	}
	for _, f := range []app.ConflictFilter{{Limit: 0}, {Limit: 101}, {Limit: 5, State: "OPENED"}, {Limit: 5, AfterID: "x"}} {
		if f.Validate() == nil {
			t.Fatal("invalid filter admitted", f)
		}
	}
	if (app.ConflictFilter{Limit: 100, State: app.ConflictExpired, AfterID: id(1)}).Validate() != nil {
		t.Fatal("valid filter refused")
	}
	// EMP-TRIP-011: a held cancellation names its trip, revision and reason.
	reason := "Viagem cancelada"
	held := heldStartView()
	held.OperationType, held.Code, held.Fact.Start = app.CancelOperationType, app.TripAlreadyClosed, nil
	held.Fact.Transition = &app.HeldTransitionFact{Command: "CANCEL", ExpectedRevision: 3, Reason: &reason}
	if held.Validate() != nil {
		t.Fatal("valid held cancellation refused")
	}
	held.Fact.Transition = &app.HeldTransitionFact{Command: "CANCEL", ExpectedRevision: 3}
	if held.Validate() == nil {
		t.Fatal("a held cancellation without its reason was admitted")
	}
	held.Fact.Transition = &app.HeldTransitionFact{Command: "PAUSE", ExpectedRevision: 3}
	if held.Validate() == nil {
		t.Fatal("a held cancellation carrying a pause was admitted")
	}
}

// B-BR-EMP-408: an accepted held start rebuilds the EMP-TRIP-001 trip it
// declared with the zones of its declared instant.
func TestHeldStartTripRebuildsTheDeclaredTrip(t *testing.T) {
	org, _ := kernel.ParseOrgID(id(99))
	scope, _ := kernel.NewOrgScope(org)
	view := heldStartView()
	started, err := app.HeldStartTrip(scope, view, orgtime.ReportingZones{Organization: "America/Cuiaba"})
	if err != nil || started.State() != trip.InProgress || started.Reference().ID() != id(76) || started.Reporting().Day != "2026-11-15" || started.Reporting().Zone != "America/Cuiaba" {
		t.Fatal("held start trip", err)
	}
	absent := heldStartView()
	reason := "Painel apagado"
	absent.Fact.Start.ReadingID, absent.Fact.Start.ReadingValue, absent.Fact.Start.MissingReason = "", nil, &reason
	if _, err := app.HeldStartTrip(scope, absent, orgtime.ReportingZones{Organization: "UTC"}); err != nil {
		t.Fatal("held start with a stated absence", err)
	}
	finish := heldStartView()
	finish.OperationType = app.FinishOperationType
	broken := heldStartView()
	broken.Fact.Start.ReadingValue = nil
	for name, v := range map[string]app.ConflictView{"finish": finish, "reading without value": broken} {
		if _, err := app.HeldStartTrip(scope, v, orgtime.ReportingZones{Organization: "UTC"}); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("rebuilt", name, err)
		}
	}
}
