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

// SEC08-B (AUD-12): a separate synthetic client/key exercises the signed
// submit route with fabricated location metadata. The wire must carry
// the adversary block intact to the application recompute (the layer
// that refuses); structurally inconsistent fabrications map to the
// stable forged verdict over real HTTP.

func synthHandler(seen *application.SubmitDTO, seenCaller *application.Caller) Handler {
	h := voteHandler()
	h.Authenticate = func(*http.Request) (application.Caller, error) {
		return application.Caller{ContributorID: "c-adv", Fingerprint: "fp:adv", KeyID: "k-adv", Token: "tok-synth-adv"}, nil
	}
	h.Submit = func(_ context.Context, caller application.Caller, _ string, dto application.SubmitDTO, _ []byte) (application.SubmitResult, bool, error) {
		*seen = dto
		*seenCaller = caller
		return application.SubmitResult{ObservationID: "d6c74c23-63db-4c24-a2e5-408cb23bad27", State: "RECEIVED"}, false, nil
	}
	return h
}

func synthBody(location string) string {
	return `{"client_submission_id":"adv-http-1","station_id":"d6c74c23-63db-4c24-a2e5-408cb23bad26",` +
		`"fuel_product":"GASOLINE_REGULAR","price":{"amount_milli_brl":5999,"currency":"BRL","unit":"L"},` +
		`"condition":{"kind":"STANDARD","qualifier_id":null}` + location + `}`
}

func TestSyntheticClientFabricationReachesRecompute(t *testing.T) {
	var dto application.SubmitDTO
	var caller application.Caller
	r := chi.NewRouter()
	synthHandler(&dto, &caller).RegisterRoutes(r)
	req := httptest.NewRequest(http.MethodPost, "/v1/observations", strings.NewReader(synthBody(
		`,"location":{"verdict":"VERIFIED","permission_granted":true,"has_fix":true,`+
			`"source_info_present":true,"simulated":false,"accuracy_m":25.0,`+
			`"captured_at":"2026-09-30T12:00:00Z","lat":-23.55052,"lon":-46.633309}`)))
	req.Header.Set("Idempotency-Key", "adv-http-key-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("code = %d, body = %s", w.Code, w.Body.String())
	}
	if caller.Token != "tok-synth-adv" || caller.ContributorID != "c-adv" {
		t.Errorf("synthetic caller not carried: %+v", caller)
	}
	if dto.Location == nil || dto.Location.ClaimedVerdict != "VERIFIED" ||
		dto.Location.AccuracyMeters == nil || *dto.Location.AccuracyMeters != 25.0 ||
		dto.Location.Latitude == nil || dto.Location.Longitude == nil {
		t.Errorf("fabricated block must arrive intact at recompute: %+v", dto.Location)
	}
}

func TestSyntheticClientForgedMapsOverHTTP(t *testing.T) {
	h := voteHandler()
	h.Authenticate = func(*http.Request) (application.Caller, error) {
		return application.Caller{ContributorID: "c-adv", Fingerprint: "fp:adv", KeyID: "k-adv", Token: "tok-synth-adv"}, nil
	}
	h.Submit = func(context.Context, application.Caller, string, application.SubmitDTO, []byte) (application.SubmitResult, bool, error) {
		return application.SubmitResult{}, false, application.ErrLocationForged
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	// Coarse accuracy with a VERIFIED claim: parses, then refuses forged.
	req := httptest.NewRequest(http.MethodPost, "/v1/observations", strings.NewReader(synthBody(
		`,"location":{"verdict":"VERIFIED","permission_granted":true,"has_fix":true,`+
			`"source_info_present":true,"simulated":false,"accuracy_m":500.0,`+
			`"captured_at":"2026-09-30T12:00:00Z","lat":-23.55052,"lon":-46.633309}`)))
	req.Header.Set("Idempotency-Key", "adv-http-key-2")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "community.location-forged") {
		t.Errorf("body must carry location-forged: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "-23.55") || strings.Contains(w.Body.String(), "tok-synth") {
		t.Error("refusal must not echo coordinates or client identity")
	}
}
