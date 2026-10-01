package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
)

func submitBodyWithLocation(location string) string {
	return `{"client_submission_id":"sub-1","station_id":"d6c74c23-63db-4c24-a2e5-408cb23bad26",` +
		`"fuel_product":"GASOLINE_REGULAR","price":{"amount_milli_brl":5999,"currency":"BRL","unit":"L"},` +
		`"condition":{"kind":"STANDARD","qualifier_id":null}` + location + `}`
}

func TestParseSubmitLocationValid(t *testing.T) {
	dto, err := parseSubmitBody([]byte(submitBodyWithLocation(`,"location":{"verdict":"VERIFIED",` +
		`"permission_granted":true,"has_fix":true,"source_info_present":true,` +
		`"simulated":false,"accuracy_m":25.0,"clock_skew_s":10,"manual":false,` +
		`"captured_at":"2026-09-30T12:00:00Z","lat":-23.55052,"lon":-46.633309}`)))
	if err != nil {
		t.Fatalf("valid location refused: %v", err)
	}
	if dto.Location == nil || dto.Location.ClaimedVerdict != "VERIFIED" {
		t.Fatalf("location not parsed: %+v", dto.Location)
	}
	if dto.Location.AccuracyMeters == nil || *dto.Location.AccuracyMeters != 25.0 {
		t.Errorf("accuracy = %+v", dto.Location.AccuracyMeters)
	}
	if dto.Location.ClockSkewSeconds == nil || *dto.Location.ClockSkewSeconds != 10 {
		t.Errorf("skew = %+v", dto.Location.ClockSkewSeconds)
	}
	if dto.Location.CapturedAt == nil || dto.Location.Latitude == nil || dto.Location.Longitude == nil {
		t.Errorf("timestamps/coords not parsed: %+v", dto.Location)
	}
}

func TestParseSubmitLocationShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"empty-verdict", `,"location":{"verdict":"","permission_granted":true}`},
		{"negative-accuracy", `,"location":{"verdict":"VERIFIED","accuracy_m":-1.0}`},
		{"fractional-skew", `,"location":{"verdict":"VERIFIED","clock_skew_s":10.5}`},
		{"lat-out-of-range", `,"location":{"verdict":"VERIFIED","lat":-91.0}`},
		{"lon-out-of-range", `,"location":{"verdict":"VERIFIED","lon":181.0}`},
		{"bad-captured-at", `,"location":{"verdict":"VERIFIED","captured_at":"yesterday"}`},
		{"unknown-field", `,"location":{"verdict":"VERIFIED","mock":true}`},
	} {
		if _, err := parseSubmitBody([]byte(submitBodyWithLocation(tc.body))); err == nil {
			t.Errorf("%s: malformed location accepted", tc.name)
		}
	}
	// Absent location preserves the old shape exactly.
	dto, err := parseSubmitBody([]byte(submitBodyWithLocation("")))
	if err != nil {
		t.Fatalf("location-less body refused: %v", err)
	}
	if dto.Location != nil {
		t.Errorf("absent location must stay nil: %+v", dto.Location)
	}
}

func TestSubmitMapsForgedLocation(t *testing.T) {
	h := voteHandler()
	h.Submit = func(context.Context, application.Caller, string, application.SubmitDTO, []byte) (application.SubmitResult, bool, error) {
		return application.SubmitResult{}, false, application.ErrLocationForged
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	req := httptest.NewRequest(http.MethodPost, "/v1/observations", strings.NewReader(submitBodyWithLocation("")))
	req.Header.Set("Idempotency-Key", "k-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "community.location-forged") {
		t.Errorf("body must carry location-forged: %s", w.Body.String())
	}
	// Forged verdicts still parse (semantics refuse later, not here).
	dto, err := parseSubmitBody([]byte(submitBodyWithLocation(`,"location":{"verdict":"TRUSTED"}`)))
	if err != nil {
		t.Fatalf("unknown verdict must parse for later refusal: %v", err)
	}
	if dto.Location == nil || dto.Location.ClaimedVerdict != "TRUSTED" {
		t.Errorf("verdict not carried: %+v", dto.Location)
	}
}
