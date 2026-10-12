package http

import (
	"encoding/json"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	legal "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/legalterms"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublishedTermsAreExactAnonymousAndCollectionOff(t *testing.T) {
	r := chi.NewRouter()
	Handler{Authenticate: func(_ *http.Request) (privacyapp.Caller, error) {
		t.Fatal("public terms authenticated")
		return privacyapp.Caller{}, nil
	}}.RegisterRoutes(r)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/v1/privacy/training-terms", nil))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var out struct {
		Version             string `json:"version"`
		Digest              string `json:"digest"`
		Text                string `json:"text"`
		CollectionAvailable bool   `json:"collection_available"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Version != legal.Version || out.Digest != legal.Digest || out.Text != legal.Text || out.CollectionAvailable {
		t.Fatal("terms diverged or collection enabled")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unexpected cache policy")
	}
}
