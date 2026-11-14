package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/go-chi/chi/v5"
)

const ownResultsBody = `{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000000001","family_id":"7b0e2a54-0000-4000-8000-000000000002","access_token":"synthetic-private-proof","membership_id":"7b0e2a54-0000-4000-8000-000000000003","enrollment_id":"7b0e2a54-0000-4000-8000-000000000004","selectors":[]}`

type ownResultsRepo struct{ calls int }

func (r *ownResultsRepo) OwnMeterReviewResults(context.Context, app.Source, []protocol.ReviewSelector) ([]protocol.ReviewResult, error) {
	r.calls++
	return nil, app.ErrUnavailable
}
func TestOwnReviewResultsRejectsAlternateAuthorityBeforeAnySourceOrRepository(t *testing.T) {
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Set("Cookie", "synthetic") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, func(r *http.Request) { r.Header.Set("X-CSRF", "synthetic") }, func(r *http.Request) { r.Header.Set("Signature-Input", "synthetic") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, func(r *http.Request) {
		r.Header.Add("Origin", "https://company.example.invalid")
		r.Header.Add("Origin", "https://company.example.invalid")
	}, func(r *http.Request) { r.Host = "foreign.example.invalid" }} {
		repo := &ownResultsRepo{}
		r := httptest.NewRequest(http.MethodPost, "https://company.example.invalid/v1/orgs/7b0e2a54-0000-4000-8000-000000000001/sync/review-results", strings.NewReader(ownResultsBody))
		r.Header.Set("Content-Type", "application/json")
		edit(r)
		w := httptest.NewRecorder()
		w.Header().Set("ETag", "old")
		reviewResultsEndpoint{ReviewResultsOptions{Enabled: true, Origin: "https://company.example.invalid", Repository: repo}}.ServeHTTP(w, r)
		if w.Code != 401 || repo.calls != 0 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" || strings.Contains(w.Body.String(), "synthetic-private") {
			t.Fatal("alternate proof or private response escaped", w.Code)
		}
	}
}
func TestOwnReviewResultsRejectsMalformedCompleteWrapperBeforeSource(t *testing.T) {
	for _, body := range []string{`{}`, strings.Repeat(" ", 65537), strings.TrimSuffix(ownResultsBody, "}") + `,"schema_version":1}`, strings.Replace(ownResultsBody, `"selectors":[]`, `"selectors":[null]`, 1), strings.Replace(ownResultsBody, `"access_token":"synthetic-private-proof"`, `"access_token":null`, 1)} {
		repo := &ownResultsRepo{}
		r := httptest.NewRequest(http.MethodPost, "https://company.example.invalid/v1/orgs/7b0e2a54-0000-4000-8000-000000000001/sync/review-results", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		reviewResultsEndpoint{ReviewResultsOptions{Enabled: true, Origin: "https://company.example.invalid", Repository: repo}}.ServeHTTP(w, r)
		if w.Code != 400 || repo.calls != 0 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("malformed suffix reached source/repository", w.Code)
		}
	}
}
func TestOwnReviewResultsRegistrationDoesNotAcceptMissingOrTypedNilProvider(t *testing.T) {
	var absent *ownResultsRepo
	valid := ReviewResultsOptions{Enabled: true, Origin: "https://company.example.invalid", Verifier: auth.Verifier{Authority: "company.example.invalid"}, Devices: &changingDevice{}, Repository: &ownResultsRepo{}}
	for _, edit := range []func(*ReviewResultsOptions){func(v *ReviewResultsOptions) { v.Repository = nil }, func(v *ReviewResultsOptions) { v.Repository = absent }, func(v *ReviewResultsOptions) { v.Devices = (*changingDevice)(nil) }, func(v *ReviewResultsOptions) { v.Verifier.Authority = "foreign" }, func(v *ReviewResultsOptions) { v.Origin = "http://company.example.invalid" }} {
		v := valid
		edit(&v)
		if RegisterReviewResults(chi.NewRouter(), v) == nil {
			t.Fatal("missing source registered")
		}
	}
	if RegisterReviewResults(chi.NewRouter(), valid) != nil {
		t.Fatal("coherent descriptor failed registration")
	}
}
