package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	chargingprepared "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/prepared"
	"github.com/go-chi/chi/v5"
)

func TestChargingSurfacesAreRegistered(t *testing.T) {
	router := chi.NewRouter()
	registerChargingSurfaces(router,
		chargingprepared.ObservationStore{},
		func(context.Context, string, string) (string, error) { return "", nil },
		func() bool { return true },
		nil,
	)
	found := map[string]bool{}
	if err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		found[method+" "+route] = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"POST /v1/charging/observations",
		"PUT /v1/charging/observations/{id}",
		"DELETE /v1/charging/observations/{id}",
		"POST /v1/charging/observations/mine",
		"POST /v1/charging/observations/{id}/helpfulness",
		"POST /v1/charging/observations/{id}/report",
		"GET /v1/charging/observations/history",
		"POST /v1/charging/tariffs/rank",
	} {
		if !found[want] {
			t.Errorf("missing route %s", want)
		}
	}
}

func TestChargingCommandRouteRejectsForgedSession(t *testing.T) {
	router := chi.NewRouter()
	registerChargingSurfaces(router,
		chargingprepared.ObservationStore{},
		func(context.Context, string, string) (string, error) { return "", context.Canceled },
		func() bool { return true },
		nil,
	)
	body := []byte(`{"session":{"family_id":"bbbbbbbb-1111-4111-8111-000000000001","access_token":"forged"},"id":"cccccccc-1111-4111-8111-000000000001","connector_code":"TYPE2","condition":"STANDARD","observed_day":"2026-10-22"}`)
	req := httptest.NewRequest("POST", "/v1/charging/observations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("forged charging command -> %d, want 401 (route missing?)", w.Code)
	}
}

func TestChargingReadSurfacesHonorFeatureGate(t *testing.T) {
	router := chi.NewRouter()
	registerChargingSurfaces(router,
		chargingprepared.ObservationStore{},
		func(context.Context, string, string) (string, error) { return "", nil },
		func() bool { return false },
		nil,
	)
	for _, probe := range []struct {
		method string
		path   string
	}{
		{"GET", "/v1/charging/observations/history?place_id=11111111-1111-4111-8111-111111111111&limit=10"},
		{"POST", "/v1/charging/tariffs/rank"},
	} {
		req := httptest.NewRequest(probe.method, probe.path, bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s %s -> %d, want 404 when charging reads are disabled", probe.method, probe.path, w.Code)
		}
	}
}
