package orgapi_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// B-BR-EMP-295: RESERVATION_CONFLICT is 409 and reveals only the window.
func TestReservationConflictRevealsOnlyTheWindow(t *testing.T) {
	from, to := time.Date(2026, 12, 1, 11, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 15, 0, 0, 123000, time.UTC)
	render := func(err error) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		orgapi.WriteError(w, httptest.NewRequest(http.MethodPost, "/", nil), err)
		var body map[string]any
		if json.Unmarshal(w.Body.Bytes(), &body) != nil {
			t.Fatal("invalid error body")
		}
		return w.Code, body["error"].(map[string]any)
	}
	status, body := render(orgapi.ReservationWindowConflict(from, &to))
	details := body["details"].([]any)
	if status != 409 || body["code"] != "RESERVATION_CONFLICT" || len(details) != 2 {
		t.Fatal("conflict envelope", status, body)
	}
	for i, want := range []map[string]any{{"field": "conflict_window", "code": "STARTS_AT", "instant": "2026-12-01T11:00:00Z"}, {"field": "conflict_window", "code": "ENDS_AT", "instant": "2026-12-01T15:00:00.000123Z"}} {
		got := details[i].(map[string]any)
		if len(got) != len(want) || got["field"] != want["field"] || got["code"] != want["code"] || got["instant"] != want["instant"] {
			t.Fatal("conflict detail", got)
		}
	}
	if _, body := render(orgapi.ReservationWindowConflict(from, nil)); len(body["details"].([]any)) != 1 {
		t.Fatal("open-ended window detail", body)
	}
	for _, bad := range []error{orgapi.ReservationWindowConflict(time.Time{}, nil), orgapi.ReservationWindowConflict(to, &from), orgapi.ReservationWindowConflict(from.Add(time.Nanosecond), nil)} {
		if !errors.Is(bad, orgapi.Unavailable) {
			t.Fatal("invalid window accepted", bad)
		}
	}
	if status, body := render(orgapi.ReservationConflict); status != 409 || len(body["details"].([]any)) != 0 {
		t.Fatal("bare conflict", status, body)
	}
}
