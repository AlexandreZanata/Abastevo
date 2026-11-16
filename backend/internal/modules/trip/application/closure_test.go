package application_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func closureScope(t *testing.T) kernel.OrgScope {
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

func closureInput() app.CloseInput {
	return app.CloseInput{OperationID: id(90), TripID: id(40), ExpectedRevision: 1, Reason: "Motorista sem sinal; viagem encerrada pela supervisao", MissingReason: "Leitura final nao registrada"}
}

// B-BR-EMP-399/403: the closure command validates identities, a positive
// revision and both reasons (METER-001 text profile); its digest binds every
// field and the zero command has none.
func TestCloseCommandShapeReasonsAndDigest(t *testing.T) {
	scope := closureScope(t)
	command, err := app.NewCloseCommand(closureInput())
	if err != nil {
		t.Fatal("a valid closure was refused", err)
	}
	digest := command.Digest(scope)
	if len(digest) != 64 || strings.ToLower(digest) != digest || strings.Trim(digest, "0123456789abcdef") != "" {
		t.Fatal("closure digest is not lowercase SHA-256 hex", digest)
	}
	if v, err := command.Snapshot(); err != nil || v.TripID != id(40) || v.ExpectedRevision != 1 || v.MissingReason != closureInput().MissingReason {
		t.Fatal("closure snapshot", err)
	}
	for name, edit := range map[string]func(*app.CloseInput){
		"reason":         func(v *app.CloseInput) { v.Reason = "Outro motivo" },
		"missing reason": func(v *app.CloseInput) { v.MissingReason = "Painel apagado" },
		"revision":       func(v *app.CloseInput) { v.ExpectedRevision = 2 },
		"operation":      func(v *app.CloseInput) { v.OperationID = id(91) },
		"trip":           func(v *app.CloseInput) { v.TripID = id(41) },
	} {
		v := closureInput()
		edit(&v)
		other, err := app.NewCloseCommand(v)
		if err != nil || other.Digest(scope) == digest {
			t.Fatal("a closure field does not bind the digest", name, err)
		}
	}
	for name, edit := range map[string]func(*app.CloseInput){
		"empty reason":             func(v *app.CloseInput) { v.Reason = "" },
		"blank reason":             func(v *app.CloseInput) { v.Reason = " 　 " },
		"control in reason":        func(v *app.CloseInput) { v.Reason = "Sem\tsinal" },
		"281-scalar reason":        func(v *app.CloseInput) { v.Reason = strings.Repeat("é", 281) },
		"empty missing reason":     func(v *app.CloseInput) { v.MissingReason = "" },
		"control in missing":       func(v *app.CloseInput) { v.MissingReason = "Painel\napagado" },
		"281-scalar missing":       func(v *app.CloseInput) { v.MissingReason = strings.Repeat("é", 281) },
		"zero revision":            func(v *app.CloseInput) { v.ExpectedRevision = 0 },
		"revision above cap":       func(v *app.CloseInput) { v.ExpectedRevision = app.MaxReviewRevision + 1 },
		"non-canonical trip":       func(v *app.CloseInput) { v.TripID = strings.ToUpper(id(40)) },
		"no operation":             func(v *app.CloseInput) { v.OperationID = "" },
		"non-canonical operation":  func(v *app.CloseInput) { v.OperationID = "op-1" },
		"zero operation identifer": func(v *app.CloseInput) { v.OperationID = "00000000-0000-0000-0000-000000000000" },
	} {
		v := closureInput()
		edit(&v)
		if _, err := app.NewCloseCommand(v); !errors.Is(err, app.ErrInvalidClosure) {
			t.Fatal("invalid closure admitted", name, err)
		}
	}
	long := closureInput()
	long.Reason, long.MissingReason = strings.Repeat("é", 280), strings.Repeat("é", 280)
	if _, err := app.NewCloseCommand(long); err != nil {
		t.Fatal("280-scalar reasons refused", err)
	}
	if (app.CloseCommand{}).Digest(scope) != "" || command.Digest(kernel.OrgScope{}) != "" {
		t.Fatal("a zero command or scope has a digest")
	}
	if _, err := (app.CloseCommand{}).Snapshot(); !errors.Is(err, app.ErrInvalidClosure) {
		t.Fatal("the zero command has a snapshot", err)
	}
}

func TestClosePoliciesDrift(t *testing.T) {
	write, read := app.ClosePolicy(app.ClosureWriteAction), app.ClosePolicy(app.ClosureReadAction)
	if write.Permission != "trip.trip.close_for_other" || !write.Fresh || !write.Effect.Write() || write.Validate() != nil ||
		read.Permission != "trip.trip.close_for_other" || read.Fresh || read.Effect.Write() || read.Validate() != nil {
		t.Fatal("closure policies drift")
	}
	if app.ClosePolicy(0).Permission != "" {
		t.Fatal("an unknown closure action has a policy")
	}
}

func closureRecord(t *testing.T) app.Record {
	t.Helper()
	at, err := orgtime.ParseInstant("2026-11-14T12:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	start := int64(184_320_000)
	return app.Record{ID: id(40), OperationID: id(19), AssetID: id(10), DriverID: id(30), AuthorMembershipID: id(3), AuthorEmployeeID: id(30), MeterID: id(11), SegmentID: id(12), ReadingID: id(13),
		ReadingKind: "ODOMETER", ReadingValue: &start, Declared: orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400},
		Reporting: orgtime.ReportingAttribution{Day: "2026-11-14", Zone: "America/Cuiaba", Source: "unit"}, State: trip.InProgress}
}

// EDR-257: the stored trip is closed through the pure domain at receipt; the
// result has no finish and NOT_MEASURED usage; the author or driver closing
// is a self-closure; a completed trip is INVALID_TRANSITION; a bad
// missing-reading reason is an invalid closure. A stored closure rebuilds
// the closed trip, whose later events are after it.
func TestCloseDomainKeepsTheFinalEvidenceMissing(t *testing.T) {
	scope := closureScope(t)
	record := closureRecord(t)
	closed, err := app.CloseDomain(scope, record, id(7), id(70), "Leitura final nao registrada")
	if err != nil || closed.State() != trip.Completed {
		t.Fatal("closure through the domain", err)
	}
	if _, ok := closed.Finished(); ok {
		t.Fatal("a closure invented a finish")
	}
	if u := closed.MeterUsage(); u.Status() != trip.NotMeasured || u.Reason() != trip.ReasonReadingAbsent {
		t.Fatal("closed usage", u.Status(), u.Reason())
	}
	if _, err := app.CloseDomain(scope, record, id(3), id(70), "Leitura final nao registrada"); !errors.Is(err, trip.ErrSelfClosure) {
		t.Fatal("the author closed its own trip", err)
	}
	if _, err := app.CloseDomain(scope, record, id(7), id(30), "Leitura final nao registrada"); !errors.Is(err, trip.ErrSelfClosure) {
		t.Fatal("the driver closed its own trip", err)
	}
	if _, err := app.CloseDomain(scope, record, id(7), id(70), "Painel\tapagado"); !errors.Is(err, app.ErrInvalidClosure) {
		t.Fatal("an invalid missing-reading reason closed the trip", err)
	}
	completed := record
	completed.State = trip.Completed
	end, _ := orgtime.ParseInstant("2026-11-14T15:00:00Z")
	completed.Finish = &app.FinishRecord{OperationID: id(21), AuthorMembershipID: id(3), Declared: orgtime.Declaration{OccurredAt: end, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400},
		MissingReason: &[]string{"Painel apagado"}[0], EndDay: "2026-11-14"}
	if _, err := app.CloseDomain(scope, completed, id(7), id(70), "Leitura final nao registrada"); !errors.Is(err, trip.ErrInvalidTransition) {
		t.Fatal("a finished trip was closed", err)
	}
	stored := record
	stored.State = trip.Completed
	stored.Closure = &app.ClosureRecord{OperationID: id(90), CloserMembershipID: id(7), CloserEmployeeID: id(70), Reason: "Sem sinal", MissingReason: "Leitura final nao registrada", ClosedAt: end.Add(time.Hour)}
	rebuilt, err := app.StoredDomain(scope, stored)
	if err != nil || rebuilt.State() != trip.Completed {
		t.Fatal("a stored closure was not rebuilt", err)
	}
	if c, ok := rebuilt.Closed(); !ok || c.Closer().ID() != id(7) || c.CloserEmployee().ID() != id(70) || c.Absence().Reason() != "Leitura final nao registrada" {
		t.Fatal("rebuilt closure facts", ok)
	}
	in := app.EventInput{EventID: id(95), TripID: id(40), AuthorMembershipID: id(3), Type: trip.LogbookStop, Declared: orgtime.Declaration{OccurredAt: end, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}}
	if _, err := app.EventDomain(scope, stored, in); !errors.Is(err, trip.ErrEventAfterFinish) {
		t.Fatal("an event on a closed trip was not after the closure", err)
	}
	if _, err := app.EventDomain(scope, record, in); err != nil {
		t.Fatal("the same event on the running trip", err)
	}
	both := stored
	both.Finish = completed.Finish
	if _, err := app.StoredDomain(scope, both); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("a trip with both a finish and a closure was rebuilt", err)
	}
	unclosed := record
	unclosed.Closure = stored.Closure
	if _, err := app.StoredDomain(scope, unclosed); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("a running trip with a closure was rebuilt", err)
	}
}

// B-BR-EMP-401: the member who closed the trip never decides its review, as
// an author whatever role it holds; another reviewer still decides.
func TestDecideRevisionExcludesTheCloser(t *testing.T) {
	at := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	submitted, err := app.TripRevisions([]app.ReviewEntry{{Sequence: 1, Kind: review.Submit, Revision: 1, State: review.Submitted, ActorMembershipID: id(3), ActorEmployeeID: id(30), OperationID: id(51), RecordedAt: at}})
	if err != nil {
		t.Fatal(err)
	}
	author := app.Person{AccountID: id(1), MembershipID: id(3), Employees: []string{id(30)}}
	driver := app.Person{Employees: []string{id(30)}}
	closer := app.Person{AccountID: id(8), MembershipID: id(7), Employees: []string{id(70), id(71)}}
	reviewer := app.Person{AccountID: id(5), MembershipID: id(6), Employees: []string{id(60)}}
	in := app.DecisionInput{OperationID: id(92), TripID: id(40), ExpectedRevision: 1, Decision: review.Approve}
	if _, err := app.DecideRevision(submitted, in, reviewer, author, driver, nil, closer); err != nil {
		t.Fatal("another reviewer could not decide a closed trip", err)
	}
	for name, self := range map[string]app.Person{
		"closer":             closer,
		"closer account":     {AccountID: id(8), MembershipID: id(9), Employees: []string{id(90)}},
		"closer merge chain": {AccountID: id(9), MembershipID: id(9), Employees: []string{id(91), id(71)}},
	} {
		if _, err := app.DecideRevision(submitted, in, self, author, driver, nil, closer); !errors.Is(err, review.ErrSelfApproval) {
			t.Fatal("the closer decided the closed trip", name, err)
		}
	}
}
