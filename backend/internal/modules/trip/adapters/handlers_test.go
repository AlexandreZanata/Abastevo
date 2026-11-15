package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func orgtimeDeclaration(at time.Time) orgtime.Declaration {
	return orgtime.Declaration{OccurredAt: at, EntryZone: "UTC"}
}

// B-BR-EMP-313: the held condition order under the locks.
func TestStartHoldOrderIsPermissionThenAvailabilityThenBusy(t *testing.T) {
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	v := app.StartInput{Event: app.Event{Declared: orgtimeDeclaration(at)}}
	disposed := at
	admitted := fleet.StartDecision{Admitted: true}
	cases := []struct {
		name        string
		disposition app.Disposition
		facts       app.StartFacts
		decision    fleet.StartDecision
		want        string
	}{
		{"apply", app.Apply, app.StartFacts{AssetState: "ACTIVE"}, admitted, ""},
		{"busy", app.Apply, app.StartFacts{AssetState: "ACTIVE", ActiveTripID: "x"}, admitted, app.AssetBusy},
		{"inactive", app.Apply, app.StartFacts{AssetState: "INACTIVE", ActiveTripID: "x"}, admitted, app.AssetUnavailable},
		{"disposed-at-instant", app.Apply, app.StartFacts{AssetState: "ACTIVE", DisposedAt: &disposed}, admitted, app.AssetUnavailable},
		{"changed-right-first", app.HoldPermissionChanged, app.StartFacts{AssetState: "DISPOSED", ActiveTripID: "x"}, admitted, app.PermissionChanged},
		{"disposal-ended-assignment", app.HoldDisposalEnded, app.StartFacts{AssetState: "ACTIVE", ActiveTripID: "x"}, admitted, app.PermissionChanged},
		{"blocked-or-reserved", app.Apply, app.StartFacts{AssetState: "ACTIVE"}, fleet.StartDecision{Code: fleet.AssetUnavailable}, app.AssetUnavailable},
		{"held-instant-busy", app.Apply, app.StartFacts{AssetState: "ACTIVE"}, fleet.StartDecision{Code: fleet.AssetBusy}, app.AssetBusy},
		{"decision-before-current-busy", app.Apply, app.StartFacts{AssetState: "ACTIVE", ActiveTripID: "x"}, fleet.StartDecision{Code: fleet.AssetUnavailable}, app.AssetUnavailable},
	}
	for _, tc := range cases {
		if got := startHold(tc.disposition, tc.facts, tc.decision, v); got != tc.want {
			t.Fatal(tc.name, got)
		}
	}
	before := at.Add(time.Nanosecond)
	if got := startHold(app.Apply, app.StartFacts{AssetState: "ACTIVE", DisposedAt: &before}, admitted, v); got != "" {
		t.Fatal("a start declared before the disposal was held", got)
	}
}

// EDR-202: EMP-TRIP-001 refusals at receipt map onto the frozen vocabulary.
func TestFinishRefusalsUseTheFrozenVocabulary(t *testing.T) {
	if _, err := finishRefusal(trip.ErrForbidden); !errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("another member's finish is not a whole-response refusal", err)
	} else if code, _ := orgapi.Registered(err); code != orgapi.Forbidden {
		t.Fatal("refusal code", code)
	}
	for _, cause := range []error{trip.ErrFinishBeforeStart, trip.ErrInvalid, trip.ErrScope, meter.ErrInvalid, meter.ErrScope} {
		if outcome, err := finishRefusal(cause); err != nil || outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" {
			t.Fatal("refusal is not MALFORMED", cause, outcome, err)
		}
	}
	if _, err := finishRefusal(errors.New("other")); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("an unknown cause is not a retry", err)
	}
}

func TestOwnersNeedRealPortsAndDeclareTheirResults(t *testing.T) {
	if _, err := NewStartHandler(nil, nil, nil, nil); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("start owner without ports")
	}
	if _, err := NewFinishHandler(nil, nil, (*Store)(nil)); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("finish owner without ports")
	}
	for kind, d := range map[string]syncapp.Descriptor{app.StartOperationType: (&startHandler{}).Descriptor(), app.FinishOperationType: (&finishHandler{}).Descriptor()} {
		if d.Validate() != nil || d.Type != kind || len(d.Results()) != 3 || d.WritePolicy.Permission != "trip.trip.start" || d.ReadPolicy.Permission != "trip.trip.read" {
			t.Fatal("descriptor drift", kind)
		}
	}
	if !readingFits(app.Event{}, nil) || readingFits(app.Event{Reading: &app.Reading{}}, nil) || readingFits(app.Event{}, &fleetapp.MeterView{}) {
		t.Fatal("reading fit without a meter view")
	}
}

func TestQueriesRefuseUnboundInputs(t *testing.T) {
	ctx := context.Background()
	if _, err := (OpenWork{}).OpenAssetWorkTx(ctx, nil, kernel.OrgScope{}, "x"); !errors.Is(err, fleetapp.ErrUnavailable) {
		t.Fatal("open work without a transaction", err)
	}
	if _, err := ActiveTripsOfAssetAtDBTX(ctx, nil, kernel.OrgScope{}, "x", time.Now()); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("active trips without a transaction", err)
	}
	if _, err := (ActiveUse{}).ActiveUseDBTX(ctx, nil, kernel.OrgScope{}, "x", time.Now()); !errors.Is(err, fleetapp.ErrUnavailable) {
		t.Fatal("active use without a transaction", err)
	}
	var s *Store
	if _, err := s.AuthorEmployeeTx(ctx, nil, kernel.OrgScope{}, "x"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil store answered", err)
	}
	if err := s.CheckHeldTx(ctx, nil, syncapp.Source{}, protocol.Operation{}, syncapp.Outcome{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil store proved a held fact", err)
	}
	if _, err := s.CheckOwnReceiptTx(ctx, nil, syncapp.Source{}, protocol.Operation{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil store proved an own receipt", err)
	}
}

// EMP-TRIP-003 (EDR-207): the logbook owner needs real ports, declares the
// trip policies and maps refusals onto the frozen vocabulary; the logbook
// store refuses unbound inputs.
func TestEventOwnerPortsRefusalsAndUnboundStore(t *testing.T) {
	if _, err := NewEventHandler(nil, nil, nil, nil); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("event owner without ports")
	}
	if d := (&eventHandler{}).Descriptor(); d.Validate() != nil || d.Type != app.EventOperationType || len(d.Results()) != 3 ||
		d.WritePolicy.Permission != "trip.trip.start" || d.ReadPolicy.Permission != "trip.trip.read" {
		t.Fatal("event descriptor drift")
	}
	if _, err := eventRefusal(trip.ErrForbidden); !errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("another member's event is not a whole-response refusal", err)
	} else if code, _ := orgapi.Registered(err); code != orgapi.Forbidden {
		t.Fatal("refusal code", code)
	}
	for _, cause := range []error{trip.ErrEventBeforeStart, trip.ErrInvalid, trip.ErrScope} {
		if outcome, err := eventRefusal(cause); err != nil || outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" {
			t.Fatal("refusal is not MALFORMED", cause, outcome, err)
		}
	}
	for _, cause := range []error{trip.ErrUnsupportedEvent, trip.ErrEventAfterFinish, errors.New("other")} {
		if _, err := eventRefusal(cause); !errors.Is(err, syncapp.ErrUnavailable) {
			t.Fatal("a cause outside the refusal map is not a retry", cause, err)
		}
	}
	ctx := context.Background()
	var s *EventStore
	if _, err := s.EventFactsTx(ctx, nil, kernel.OrgScope{}, "x"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil logbook store answered", err)
	}
	if _, err := s.TimelineTx(ctx, nil, kernel.OrgScope{}, "x"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil logbook store read a timeline", err)
	}
	if err := s.CheckHeldTx(ctx, nil, syncapp.Source{}, protocol.Operation{}, syncapp.Outcome{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil logbook store proved a held event", err)
	}
	if _, err := s.CheckOwnReceiptTx(ctx, nil, syncapp.Source{}, protocol.Operation{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil logbook store proved an own receipt", err)
	}
	if _, err := (StartImplements{}).ImplementsAtStartDBTX(ctx, nil, kernel.OrgScope{}, "x"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("start implements without ports", err)
	}
}

// EMP-TRIP-011 (EDR-253): the lifecycle owners need real ports and a known
// kind, declare the trip policies, and map a refusal at receipt onto a held
// code or the frozen vocabulary.
func TestLifecycleOwnerPortsHoldsAndRefusals(t *testing.T) {
	if _, err := NewTransitionHandler(app.PauseOperationType, nil, nil, nil, nil); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("lifecycle owner without ports")
	}
	if _, err := NewTransitionHandler(app.FinishOperationType, nil, nil, nil, nil); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("finish accepted as a lifecycle kind")
	}
	for _, kind := range app.LifecycleOperationTypes {
		if d := (&transitionHandler{kind: kind}).Descriptor(); d.Validate() != nil || d.Type != kind || len(d.Results()) != 3 ||
			d.WritePolicy.Permission != "trip.trip.start" || d.ReadPolicy.Permission != "trip.trip.read" {
			t.Fatal("lifecycle descriptor drift", kind)
		}
	}
	open, closed := app.Record{State: trip.InProgress}, app.Record{State: trip.Cancelled}
	for _, c := range []struct {
		record app.Record
		err    error
		code   string
	}{
		{closed, trip.ErrInvalidTransition, app.TripAlreadyClosed}, {app.Record{State: trip.Completed}, trip.ErrInvalidTransition, app.TripAlreadyClosed},
		{open, trip.ErrInvalidTransition, app.StaleRevision}, {open, trip.ErrConflict, app.StaleRevision},
		{open, trip.ErrForbidden, ""}, {open, trip.ErrBeforeStart, ""}, {open, nil, ""},
	} {
		if got := transitionHold(c.record, c.err); got != c.code {
			t.Fatal("held code", c.record.State, c.err, got)
		}
	}
	if _, err := transitionRefusal(trip.ErrForbidden); !errors.Is(err, syncapp.ErrSourceDenied) {
		t.Fatal("another member's command is not a whole-response refusal", err)
	} else if code, _ := orgapi.Registered(err); code != orgapi.Forbidden {
		t.Fatal("refusal code", code)
	}
	for _, cause := range []error{trip.ErrBeforeStart, trip.ErrInvalid, trip.ErrScope, trip.ErrIncomplete} {
		if outcome, err := transitionRefusal(cause); err != nil || outcome.Result != protocol.Rejected || outcome.Code != "MALFORMED" {
			t.Fatal("refusal is not MALFORMED", cause, outcome, err)
		}
	}
	if _, err := transitionRefusal(errors.New("other")); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("an unknown cause is not a retry", err)
	}
	ctx := context.Background()
	var s *Store
	if _, err := s.TransitionFactsTx(ctx, nil, kernel.OrgScope{}, "x"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil store answered lifecycle facts", err)
	}
	if _, err := s.HoldTransitionTx(ctx, nil, syncapp.Source{}, protocol.Operation{}, app.TransitionCommand{}, app.StaleRevision); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil store held a lifecycle command", err)
	}
	if err := s.InsertTransitionTx(ctx, nil, kernel.OrgScope{}, syncapp.Source{}, app.TransitionCommand{}, trip.Trip{}, "x"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil store inserted a lifecycle row", err)
	}
}
