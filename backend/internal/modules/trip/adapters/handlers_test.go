package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
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
	cases := []struct {
		name        string
		disposition app.Disposition
		facts       app.StartFacts
		want        string
	}{
		{"apply", app.Apply, app.StartFacts{AssetState: "ACTIVE"}, ""},
		{"busy", app.Apply, app.StartFacts{AssetState: "ACTIVE", ActiveTripID: "x"}, app.AssetBusy},
		{"inactive", app.Apply, app.StartFacts{AssetState: "INACTIVE", ActiveTripID: "x"}, app.AssetUnavailable},
		{"disposed-at-instant", app.Apply, app.StartFacts{AssetState: "ACTIVE", DisposedAt: &disposed}, app.AssetUnavailable},
		{"changed-right-first", app.HoldPermissionChanged, app.StartFacts{AssetState: "DISPOSED", ActiveTripID: "x"}, app.PermissionChanged},
		{"disposal-ended-assignment", app.HoldDisposalEnded, app.StartFacts{AssetState: "ACTIVE", ActiveTripID: "x"}, app.PermissionChanged},
	}
	for _, tc := range cases {
		if got := startHold(tc.disposition, tc.facts, v); got != tc.want {
			t.Fatal(tc.name, got)
		}
	}
	before := at.Add(time.Nanosecond)
	if got := startHold(app.Apply, app.StartFacts{AssetState: "ACTIVE", DisposedAt: &before}, v); got != "" {
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
	if _, err := NewStartHandler(nil, nil, nil); !errors.Is(err, syncapp.ErrUnavailable) {
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
