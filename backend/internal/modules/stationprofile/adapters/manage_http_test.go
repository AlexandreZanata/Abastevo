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

func manageTestPorts() application.ManagePorts {
	return application.ManagePorts{
		Grants: &manageGrantStore{grant: application.GrantRow{
			ID: "grant-1", AccountID: "acc-1", StationID: "station-1",
			OperatorCNPJ: "04218406000104", Role: "administrator",
			Scopes: []string{"profile.edit", "reply.official"}, Status: "active",
		}},
		Profiles: &manageProfileStore{revision: 1},
		OperatorOf: func(context.Context, string) (string, bool, error) {
			return "04218406000104", true, nil
		},
		AccountLive: func(context.Context, string) (bool, error) { return true, nil },
		SubmitReply: func(_ context.Context, _, _, _, text string) (string, error) {
			if len([]rune(text)) > 280 {
				return "", errTextTooLong
			}
			return "comment-1", nil
		},
		Attribute: func(_ context.Context, _, _, _, _ string) error { return nil },
	}
}

type textTooLongError string

func (e textTooLongError) Error() string { return string(e) }

var errTextTooLong = textTooLongError("text-too-long")

type manageGrantStore struct {
	grant application.GrantRow
}

func (f *manageGrantStore) DecideAtomically(context.Context, application.DecisionInput, *application.GrantInput, string) error {
	return nil
}

func (f *manageGrantStore) ActiveGrant(_ context.Context, _, _ string) (application.GrantRow, bool, error) {
	if f.grant.ID == "" {
		return application.GrantRow{}, false, nil
	}
	return f.grant, true, nil
}

type manageProfileStore struct {
	revision int
	fields   map[string]string
}

func (f *manageProfileStore) EnsureUnclaimed(context.Context, string) error { return nil }

func (f *manageProfileStore) Profile(_ context.Context, _ string) (application.StoredProfile, error) {
	return application.StoredProfile{PolicyVersion: "profile-v1", Revision: f.revision, Business: f.fields}, nil
}

func (f *manageProfileStore) RecordOperator(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func (f *manageProfileStore) CloseOperator(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (f *manageProfileStore) CurrentOperator(context.Context, string) (application.StoredOperator, bool, error) {
	return application.StoredOperator{}, false, nil
}

func (f *manageProfileStore) UpdateProjection(_ context.Context, _ string, expectedRevision int, fields map[string]string) (application.StoredProfile, error) {
	if expectedRevision != f.revision {
		return application.StoredProfile{}, application.ErrManageVersion
	}
	f.revision++
	f.fields = fields
	return application.StoredProfile{PolicyVersion: "profile-v1", Revision: f.revision, Business: fields}, nil
}

func manageTestHandler() ManageHandler {
	ports := manageTestPorts()
	return ManageHandler{
		EditPorts:  func(*http.Request) application.ManagePorts { return ports },
		ReplyPorts: func(*http.Request) application.ManagePorts { return ports },
		Sessions: func(context.Context, string, string) (string, error) {
			return "acc-1", nil
		},
	}
}

func doManageRequest(h ManageHandler, method, target, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func TestManageEditEnforcesPolicyAndVersion(t *testing.T) {
	h := manageTestHandler()
	ok := doManageRequest(h, http.MethodPost, "/v1/stations/station-1/profile",
		`{"family_id": "fam", "access_token": "tok", "expected_revision": 1, "fields": {"phone": "+55-11-99999-0000"}}`)
	if ok.Code != http.StatusOK {
		t.Fatalf("edit = %d: %s", ok.Code, ok.Body.String())
	}
	if ok.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("management response without no-store")
	}
	forbidden := doManageRequest(h, http.MethodPost, "/v1/stations/station-1/profile",
		`{"family_id": "fam", "access_token": "tok", "expected_revision": 2, "fields": {"cnpj": "04218406000104"}}`)
	if forbidden.Code != http.StatusBadRequest {
		t.Fatalf("canonical field = %d, want 400", forbidden.Code)
	}
	stale := doManageRequest(h, http.MethodPost, "/v1/stations/station-1/profile",
		`{"family_id": "fam", "access_token": "tok", "expected_revision": 1, "fields": {"phone": "x"}}`)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale revision = %d, want 409", stale.Code)
	}
}

func TestManageReplyPostsOfficially(t *testing.T) {
	h := manageTestHandler()
	ok := doManageRequest(h, http.MethodPost, "/v1/stations/station-1/profile/replies",
		`{"family_id": "fam", "access_token": "tok", "product": "ETHANOL", "text": "Obrigado pela visita!"}`)
	if ok.Code != http.StatusCreated {
		t.Fatalf("reply = %d: %s", ok.Code, ok.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(ok.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if doc["comment_id"] != "comment-1" {
		t.Fatalf("doc = %v", doc)
	}
	long := strings.Repeat("a", 281)
	oversize := doManageRequest(h, http.MethodPost, "/v1/stations/station-1/profile/replies",
		`{"family_id": "fam", "access_token": "tok", "product": "ETHANOL", "text": "`+long+`"}`)
	if oversize.Code == http.StatusCreated {
		t.Fatal("281-scalar reply must not publish")
	}
}
