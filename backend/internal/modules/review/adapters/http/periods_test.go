package http

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/domain/period"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

func periodID(n int) string { return fmt.Sprintf("a6000000-0000-4000-8000-%012d", n) }

func TestPeriodErrorsMapToRegisteredCodes(t *testing.T) {
	for _, c := range []struct {
		err  error
		want orgapi.Code
	}{
		{period.ErrConflict, orgapi.Conflict},
		{period.ErrTransition, orgapi.Conflict},
		{period.ErrReasonRequired, orgapi.ValidationFailed},
		{period.ErrReasonInvalid, orgapi.ValidationFailed},
		{period.ErrInvalid, orgapi.ValidationFailed},
		{app.ErrNotFound, orgapi.NotFound},
		{app.ErrSurfaceDisabled, orgapi.NotFound},
		{app.ErrOperationReused, orgapi.OperationReused},
		{app.ErrUnauthenticated, orgapi.Unauthenticated},
		{app.ErrUnavailable, orgapi.Unavailable},
		{access.ErrNoMembership, orgapi.NotFound},
		{access.ErrMembershipSuspended, orgapi.MembershipSuspended},
		{access.ErrOrgSuspended, orgapi.OrgSuspended},
		{access.ErrOrgArchived, orgapi.OrgArchived},
		{access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired},
		{access.ErrEntitlementRequired, orgapi.EntitlementRequired},
		{orgapi.Malformed, orgapi.Malformed},
		{errors.New("synthetic private provider detail"), orgapi.Unavailable},
	} {
		if got, ok := orgapi.Registered(periodMapped(c.err)); !ok || got != c.want {
			t.Fatalf("%v mapped to %v, want %v", c.err, got, c.want)
		}
	}
	denied := permissions.DenyPermission("fin.period.reopen")
	if got := periodMapped(denied); !errors.Is(got, orgapi.Forbidden) || !strings.Contains(fmt.Sprintf("%#v", got), "fin.period.reopen") {
		t.Fatal("permission denial lost its registered key", got)
	}
}

func TestPeriodRequestsDecodeStrictlyAndLosslessly(t *testing.T) {
	var reopen periodReopenRequest
	if err := decodePeriod([]byte(`{"schema_version":1,"operation_id":"`+periodID(1)+`","revision":2,"reason":"  Synthetic 😀 reason "}`), &reopen); err != nil || reopen.Reason != "  Synthetic \U0001F600 reason " || *reopen.Revision != 2 {
		t.Fatal("valid reopen refused or reason not verbatim", err)
	}
	for _, raw := range []string{
		`{"schema_version":1,"operation_id":"` + periodID(1) + `","revision":2,"reason":"lone \udc00"}`,
		`{"schema_version":1,"operation_id":"` + periodID(1) + `","revision":2,"reason":"lone \ud83d end"}`,
		"{\"schema_version\":1,\"operation_id\":\"" + periodID(1) + "\",\"revision\":2,\"reason\":\"bad \xff\"}",
		`{"schema_version":1,"operation_id":"` + periodID(1) + `","revision":2,"reason":"x","state":"LOCKED"}`,
		`{"schema_version":1,"operation_id":"` + periodID(1) + `","Revision":2,"reason":"x"}`,
		`{"schema_version":1,"operation_id":"` + periodID(1) + `","revision":2,"reason":null}`,
		`[]`,
	} {
		var v periodReopenRequest
		if decodePeriod([]byte(raw), &v) == nil {
			t.Fatal("lossy or non-canonical request accepted", raw)
		}
	}
	var close periodCloseRequest
	if decodePeriod([]byte(`{"schema_version":1,"operation_id":"`+periodID(1)+`","revision":1,"reason":"x"}`), &close) == nil {
		t.Fatal("close accepted a reason")
	}
	var list periodListRequest
	if err := decodePeriod([]byte(`{"limit":2,"state":"LOCKED"}`), &list); err != nil || list.Limit != 2 || list.State != "LOCKED" {
		t.Fatal("list request", err)
	}
	var read periodReadRequest
	if err := decodePeriod([]byte(`{}`), &read); err != nil {
		t.Fatal("empty read request", err)
	}
	if decodePeriod([]byte(`{"period":"2026-10"}`), &read) == nil {
		t.Fatal("read accepted a body selector")
	}
}

func TestPeriodProjectionsAreExactAndFailClosed(t *testing.T) {
	at := time.Date(2026, 11, 14, 12, 0, 0, 123456789, time.UTC)
	history := []app.PeriodTransitionView{
		{Revision: 2, Kind: period.Lock, From: period.Open, To: period.Locked, Authority: period.ClosePermission, ActorAccountID: periodID(2), ActorMembershipID: periodID(4), OperationID: periodID(30), RecordedAt: at},
		{Revision: 3, Kind: period.Reopen, From: period.Locked, To: period.Reopened, Authority: period.ReopenPermission, Reason: "Synthetic reason", ActorAccountID: periodID(2), ActorMembershipID: periodID(4), OperationID: periodID(31), RecordedAt: at.Add(time.Minute)},
	}
	view := app.PeriodView{Month: "2026-10", Start: "2026-10-01", End: "2026-11-01", PeriodID: periodID(40), State: period.Reopened, Revision: 3, History: history}
	out, err := projectPeriod(view, true)
	if err != nil || len(out) != 9 || out["period"] != "2026-10" || out["start_day"] != "2026-10-01" || out["end_day"] != "2026-11-01" || out["period_id"] != periodID(40) || out["state"] != period.Reopened || out["revision"] != int64(3) || out["data_mode"] != "SYNTHETIC" || out["time_basis"] != "REPORTING_DAY" {
		t.Fatal("period projection", out, err)
	}
	entries, ok := out["history"].([]map[string]any)
	if !ok || len(entries) != 2 || len(entries[1]) != 10 || entries[1]["reason"] != "Synthetic reason" || entries[1]["recorded_at"] != "2026-11-14T12:01:00.123456789Z" || entries[0]["from_state"] != period.Open {
		t.Fatal("history projection", entries)
	}
	listed, err := projectPeriod(view, false)
	if err != nil || listed["history"] != nil || len(listed) != 8 {
		t.Fatal("list projection must omit history", listed, err)
	}
	open, err := projectPeriod(app.PeriodView{Month: "2026-10", Start: "2026-10-01", End: "2026-11-01", State: period.Open, Revision: 1, History: []app.PeriodTransitionView{}}, true)
	if err != nil || open["period_id"] != nil {
		t.Fatal("implicit open projection", open, err)
	}
	if _, err := projectPeriod(app.PeriodView{Month: "2026-13"}, true); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal("invalid view projected")
	}
	ack, err := projectAck(app.PeriodAck{Month: "2026-10", PeriodID: periodID(40), Kind: period.Lock, State: period.Locked, Revision: 2, RecordedAt: at}, true)
	if err != nil || len(ack) != 8 || ack["replayed"] != true || ack["kind"] != period.Lock || ack["recorded_at"] != "2026-11-14T12:00:00.123456789Z" {
		t.Fatal("ack projection", ack, err)
	}
	if _, err := projectAck(app.PeriodAck{Month: "2026-10", Kind: period.Lock, State: period.Locked, Revision: 2, RecordedAt: at}, false); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal("ack without period identity projected")
	}
}

func TestPeriodRecordsAndRegistrationFailClosed(t *testing.T) {
	org, _ := kernel.ParseOrgID(periodID(1))
	scope, _ := kernel.NewOrgScope(org)
	facts := permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}
	rules := []permissions.Rule{{}}
	if (PeriodRecords{}).Check(context.Background(), scope, access.Subject{}, app.PeriodPolicy(app.PeriodClose), facts, rules) == nil {
		t.Fatal("record check accepted a subject outside the scope")
	}
	if (PeriodRecords{}).Check(context.Background(), kernel.OrgScope{}, access.Subject{}, app.PeriodPolicy(app.PeriodRead), facts, rules) == nil {
		t.Fatal("record check accepted an invalid scope")
	}
	if err := RegisterPeriods(chi.NewRouter(), app.PeriodService{}, company.Handler{}, orgapi.Gateway{}, access.Authorization{}); err != nil {
		t.Fatal("disabled period surface must mount nothing", err)
	}
	if err := RegisterPeriods(chi.NewRouter(), app.PeriodService{Enabled: true}, company.Handler{}, orgapi.Gateway{}, access.Authorization{}); err == nil {
		t.Fatal("incomplete period surface registered")
	}
}
