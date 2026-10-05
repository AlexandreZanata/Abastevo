package adapters

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
)

type fakeProfileStore struct {
	profile application.StoredProfile
	ops     map[string]application.StoredOperator
}

func (f *fakeProfileStore) EnsureUnclaimed(_ context.Context, _ string) error { return nil }

func (f *fakeProfileStore) Profile(_ context.Context, _ string) (application.StoredProfile, error) {
	return f.profile, nil
}

func (f *fakeProfileStore) RecordOperator(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func (f *fakeProfileStore) CloseOperator(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (f *fakeProfileStore) CurrentOperator(_ context.Context, stationID string) (application.StoredOperator, bool, error) {
	op, ok := f.ops[stationID]
	return op, ok, nil
}

func (f *fakeProfileStore) UpdateProjection(_ context.Context, _ string, expectedRevision int, fields map[string]string) (application.StoredProfile, error) {
	return application.StoredProfile{PolicyVersion: "profile-v1", Revision: expectedRevision + 1, Business: fields}, nil
}

func serveProfile(store *fakeProfileStore) (string, int) {
	handler := Handler{
		Store: store,
		Read: func(_ context.Context, _ string) (string, string, *float64, *float64, error) {
			lat, lon := -23.55, -46.63
			return "Posto Central", "reviewed", &lat, &lon, nil
		},
	}
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	req := httptest.NewRequest(http.MethodGet, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/profile", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder.Body.String(), recorder.Code
}

func TestProfileReadIsPublicAndHonest(t *testing.T) {
	body, code := serveProfile(&fakeProfileStore{
		profile: application.StoredProfile{PolicyVersion: "profile-v1", Revision: 1, Business: map[string]string{}},
		ops:     map[string]application.StoredOperator{},
	})
	if code != http.StatusOK {
		t.Fatalf("code = %d: %s", code, body)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc["has_badge"] != false {
		t.Fatalf("unclaimed profile must not badge: %v", doc)
	}
	if doc["policy_version"] != "profile-v1" {
		t.Fatalf("doc = %v", doc)
	}
	for _, forbidden := range []string{"cpf", "private_key", "password", "account", "grant", "price"} {
		if strings.Contains(strings.ToLower(body), `"`+forbidden+`"`) {
			t.Fatalf("private field leaked: %s", forbidden)
		}
	}
}

func TestProfileReadShowsOperatorWithoutBadge(t *testing.T) {
	body, code := serveProfile(&fakeProfileStore{
		profile: application.StoredProfile{PolicyVersion: "profile-v1", Revision: 1, Business: map[string]string{}},
		ops: map[string]application.StoredOperator{
			"d6c74c23-63db-4c24-a2e5-408cb23bad26": {ID: "op-1", CNPJ: "04218406000104", Source: "registry"},
		},
	})
	if code != http.StatusOK {
		t.Fatalf("code = %d", code)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	operator, _ := doc["operator"].(map[string]any)
	if operator["cnpj"] != "04218406000104" {
		t.Fatalf("operator = %v", operator)
	}
	if doc["has_badge"] != false {
		t.Fatal("operator link alone must not badge (grants arrive in P31)")
	}
}

func TestPublicProfileBadgePortAndFailureAreHonest(t *testing.T) {
	stationID := "d6c74c23-63db-4c24-a2e5-408cb23bad26"
	for _, failed := range []bool{false, true} {
		handler := Handler{
			Store: &fakeProfileStore{profile: application.StoredProfile{Revision: 1}, ops: map[string]application.StoredOperator{stationID: {CNPJ: "04218406000104", Source: "registry"}}},
			Read: func(context.Context, string) (string, string, *float64, *float64, error) {
				return "Posto", "unknown", nil, nil, nil
			},
			Representation: func(_ context.Context, id, cnpj string) (bool, error) {
				if id != stationID || cnpj != "04218406000104" {
					t.Fatal("badge identity mismatch")
				}
				if failed {
					return false, context.DeadlineExceeded
				}
				return true, nil
			},
		}
		router := chi.NewRouter()
		handler.RegisterRoutes(router)
		res := httptest.NewRecorder()
		router.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/stations/"+stationID+"/profile", nil))
		if failed {
			if res.Code != http.StatusServiceUnavailable {
				t.Fatal("badge lookup failure guessed success")
			}
		} else if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"has_badge":true`) {
			t.Fatal("live representation not shown")
		}
		for _, private := range []string{`"account_id"`, `"grant_id"`, `"declaration"`} {
			if strings.Contains(res.Body.String(), private) {
				t.Fatal("private badge data leaked")
			}
		}
	}
}
