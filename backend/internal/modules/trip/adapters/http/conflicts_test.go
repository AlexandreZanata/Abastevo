package http

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	review "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func conflictID(n int) string {
	return "7b0e2a54-0000-4000-8000-0000000097" + string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// EMP-TRIP-007 (EDR-263): strict bodies, the refusal vocabulary, the two
// admitted policies and one projection per held trip owner.
func TestConflictRequestsRecordsAndProjection(t *testing.T) {
	var resolve conflictResolveRequest
	if err := resolve.UnmarshalJSON([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000009703","revision":1,"decision":"REFUSE","reason":"Veículo em uso"}`)); err != nil || resolve.Reason != "Veículo em uso" || *resolve.Revision != 1 {
		t.Fatal("valid resolution body refused", err)
	}
	for name, raw := range map[string]string{
		"unknown key":    `{"schema_version":1,"operation_id":"x","revision":1,"decision":"ACCEPT","asset_id":"x"}`,
		"lone surrogate": `{"schema_version":1,"operation_id":"x","revision":1,"decision":"REFUSE","reason":"\ud800"}`,
		"null reason":    `{"schema_version":1,"operation_id":"x","revision":1,"decision":"REFUSE","reason":null}`,
	} {
		var v conflictResolveRequest
		if v.UnmarshalJSON([]byte(raw)) == nil {
			t.Fatal("malformed resolution body admitted", name)
		}
	}
	var list conflictListRequest
	if list.UnmarshalJSON([]byte(`{"limit":5,"state":"OPEN","cursor":""}`)) != nil || list.State != "OPEN" || list.UnmarshalJSON([]byte(`{"limit":5,"reviewer":"x"}`)) == nil {
		t.Fatal("list body strictness")
	}
	if !errors.Is(reviewMapped(app.ErrConflictUnacceptable), orgapi.Conflict) || !errors.Is(reviewMapped(review.ErrSelfApproval), orgapi.SelfApproval) ||
		!errors.Is(reviewMapped(review.ErrReasonRequired), orgapi.ValidationFailed) || !errors.Is(reviewMapped(app.ErrOperationReused), orgapi.OperationReused) {
		t.Fatal("conflict refusal mapping drift")
	}
	org, _ := kernel.ParseOrgID(conflictID(1))
	scope, _ := kernel.NewOrgScope(org)
	if conflictRule(true) != (orgapi.Rule{Permission: "trip.trip.review", Write: true, Fresh: true, RequireRevision: true, CompanyOnly: true, Success: 200}) ||
		conflictRule(false) != (orgapi.Rule{Permission: "trip.trip.review", CompanyOnly: true, Success: 200}) {
		t.Fatal("conflict route rules drift")
	}
	if (ConflictRecords{}).Check(context.Background(), scope, access.Subject{}, app.AmendPolicy(), permissions.ResourceFacts{OrgID: org, ID: org.String()}, nil) == nil {
		t.Fatal("conflict records admitted a foreign policy")
	}
	at := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	declared := orgtime.Declaration{OccurredAt: at, EntryZone: "America/Cuiaba", DeviceOffsetSeconds: -14400}
	reason, text, revision := "Painel apagado", "Parada", int64(1)
	base := app.ConflictView{ReviewItemID: conflictID(2), OperationID: conflictID(3), PayloadDigest: "sha256:" + strings.Repeat("a", 64), State: app.ConflictOpen, Revision: 1,
		AuthorAccountID: conflictID(4), AuthorMembershipID: conflictID(5), AuthorEmployeeID: conflictID(6), EnrollmentID: conflictID(7), ReceivedAt: at.Add(time.Hour),
		ExpiresAt: at.Add(61 * 24 * time.Hour), Trips: []app.ConflictTrip{{TripID: conflictID(8), State: trip.InProgress, StartedAt: at}}}
	for kind, change := range map[string]func(*app.ConflictView){
		app.FinishOperationType: func(v *app.ConflictView) {
			v.Code, v.Fact = app.TripAlreadyClosed, app.ConflictFact{TripID: conflictID(8), Declared: declared, Finish: &app.HeldStartFact{TripID: conflictID(8), Declared: declared, MissingReason: &reason}}
		},
		app.EventOperationType: func(v *app.ConflictView) {
			v.Code, v.Fact = app.StaleRevision, app.ConflictFact{TripID: conflictID(8), Declared: declared, Event: &app.HeldEventFact{EventID: conflictID(9), EventType: "STOP", Text: &text}}
		},
		app.PauseOperationType: func(v *app.ConflictView) {
			v.Code, v.Fact = app.StaleRevision, app.ConflictFact{TripID: conflictID(8), Declared: declared, Transition: &app.HeldTransitionFact{Command: "PAUSE", ExpectedRevision: 2}}
		},
		app.SubmitOperationType: func(v *app.ConflictView) {
			v.Code, v.Fact = app.StaleRevision, app.ConflictFact{TripID: conflictID(8), Declared: declared, SubmittedRevision: &revision}
		},
	} {
		v := base
		v.OperationType = kind
		change(&v)
		out, err := projectConflict(v)
		if err != nil {
			t.Fatal("projection", kind, err)
		}
		fact := out["fact"].(map[string]any)
		if len(fact) != 12 || fact["asset_id"] != nil || fact["driver_employee_id"] != nil || out["resolution"] != nil || out["operation_type"] != kind {
			t.Fatal("projection shape", kind, fact)
		}
		switch kind {
		case app.FinishOperationType:
			if fact["missing_reading_reason"] != reason || fact["reading"] != nil || fact["event"] != nil || fact["submitted_revision"] != nil {
				t.Fatal("finish fact", fact)
			}
		case app.EventOperationType:
			if fact["event"].(map[string]any)["text"] != text || fact["missing_reading_reason"] != nil {
				t.Fatal("event fact", fact)
			}
		case app.PauseOperationType:
			if tr := fact["transition"].(map[string]any); tr["command"] != "PAUSE" || tr["expected_revision"] != int64(2) || tr["reason"] != nil || fact["event"] != nil {
				t.Fatal("pause fact", fact)
			}
		case app.SubmitOperationType:
			if fact["submitted_revision"] != revision || fact["event"] != nil {
				t.Fatal("submission fact", fact)
			}
		}
	}
	bad := base
	bad.OperationType, bad.Code = app.StartOperationType, app.AssetBusy
	if _, err := projectConflict(bad); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal("an incomplete view was projected", err)
	}
}
