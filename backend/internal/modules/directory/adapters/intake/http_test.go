package intake

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
)

type fakeStore struct {
	mu   sync.Mutex
	rows map[string]application.SuggestionRow
}

func (f *fakeStore) CreateSuggestion(_ context.Context, id, accountID, clientKey string, proposal []byte, evidenceRef string) (application.SuggestionRow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := accountID + "|" + clientKey
	if _, ok := f.rows[key]; ok {
		return application.SuggestionRow{}, false, nil
	}
	row := application.SuggestionRow{ID: id, AccountID: accountID, ClientSubmissionID: clientKey, Proposal: proposal, EvidenceRef: evidenceRef, State: application.SuggestionPending, CreatedAt: time.Now()}
	if f.rows == nil {
		f.rows = map[string]application.SuggestionRow{}
	}
	f.rows[key] = row
	return row, true, nil
}

func (f *fakeStore) SuggestionByKey(_ context.Context, accountID, clientKey string) (application.SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if row, ok := f.rows[accountID+"|"+clientKey]; ok {
		return row, nil
	}
	return application.SuggestionRow{}, application.ErrSuggestionNotFound
}

func (f *fakeStore) GetSuggestion(_ context.Context, id string) (application.SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return application.SuggestionRow{}, application.ErrSuggestionNotFound
}

func (f *fakeStore) ListOwned(_ context.Context, accountID string, _, _ int) ([]application.SuggestionRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []application.SuggestionRow
	for _, row := range f.rows {
		if row.AccountID == accountID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeStore) CountRecent(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (f *fakeStore) CancelSuggestion(_ context.Context, id, accountID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, row := range f.rows {
		if row.ID == id && row.AccountID == accountID && row.State == application.SuggestionPending {
			row.State = application.SuggestionCancelled
			f.rows[key] = row
			return 1, nil
		}
	}
	return 0, nil
}

func testHandler(sessions func(ctx context.Context, familyID, accessToken string) (string, error)) Handler {
	n := 0
	return Handler{
		Service: application.IntakeService{
			Store: &fakeStore{},
			Clock: func() time.Time { return time.Now() },
			NewID: func() (string, error) { n++; return "sug-http-" + string(rune('0'+n)), nil },
		},
		Sessions: sessions,
	}
}

func signedIn(_ context.Context, _, _ string) (string, error) { return "acc-1", nil }

func doRequest(h Handler, method, target, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func submitBody(key string) string {
	return `{"family_id": "fam", "access_token": "tok", "client_submission_id": "` + key + `", "proposal": {"display_name": "Posto Novo", "municipality_code": "3550308", "state": "SP", "cnpj_normalized": "04218406000104"}}`
}

func TestSubmitReplayAndConflict(t *testing.T) {
	h := testHandler(signedIn)
	first := doRequest(h, http.MethodPost, "/v1/stations/suggestions", submitBody("key-1"))
	if first.Code != http.StatusCreated {
		t.Fatalf("submit = %d: %s", first.Code, first.Body.String())
	}
	if first.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private response without no-store")
	}
	var created map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	second := doRequest(h, http.MethodPost, "/v1/stations/suggestions", submitBody("key-1"))
	if second.Code != http.StatusOK {
		t.Fatalf("replay = %d", second.Code)
	}
	var replayed map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &replayed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if replayed["id"] != created["id"] {
		t.Fatalf("replay id changed: %v vs %v", replayed["id"], created["id"])
	}
	changed := strings.Replace(submitBody("key-1"), "Posto Novo", "Posto Trocado", 1)
	conflict := doRequest(h, http.MethodPost, "/v1/stations/suggestions", changed)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("changed body = %d, want 409", conflict.Code)
	}
}

func TestSubmitRejectsBadSessionAndInput(t *testing.T) {
	denied := func(context.Context, string, string) (string, error) {
		return "", errors.New("session-invalid")
	}
	h := testHandler(denied)
	res := doRequest(h, http.MethodPost, "/v1/stations/suggestions", submitBody("key-1"))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("bad session = %d, want 401", res.Code)
	}
	h = testHandler(signedIn)
	res = doRequest(h, http.MethodPost, "/v1/stations/suggestions", `{"family_id": "fam"}`)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("bad input = %d, want 400", res.Code)
	}
}

func TestStatusCancelAreOwnerOnly(t *testing.T) {
	h := testHandler(signedIn)
	created := doRequest(h, http.MethodPost, "/v1/stations/suggestions", submitBody("key-1"))
	var doc map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	id, _ := doc["id"].(string)

	ok := doRequest(h, http.MethodPost, "/v1/stations/suggestions/"+id+"/status", `{"family_id": "fam", "access_token": "tok"}`)
	if ok.Code != http.StatusOK {
		t.Fatalf("owner status = %d", ok.Code)
	}
	// Foreign owners see nothing (same store, different session).
	foreign := doRequest(Handler{Service: h.Service, Sessions: func(context.Context, string, string) (string, error) {
		return "acc-2", nil
	}}, http.MethodPost, "/v1/stations/suggestions/"+id+"/status", `{"family_id": "fam", "access_token": "tok"}`)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign status = %d, want 404 (no oracle)", foreign.Code)
	}
	cancelled := doRequest(h, http.MethodPost, "/v1/stations/suggestions/"+id+"/cancel", `{"family_id": "fam", "access_token": "tok"}`)
	if cancelled.Code != http.StatusOK {
		t.Fatalf("cancel = %d", cancelled.Code)
	}
	again := doRequest(h, http.MethodPost, "/v1/stations/suggestions/"+id+"/cancel", `{"family_id": "fam", "access_token": "tok"}`)
	if again.Code != http.StatusConflict {
		t.Fatalf("double cancel = %d, want 409", again.Code)
	}
	mine := doRequest(h, http.MethodPost, "/v1/stations/suggestions/mine", `{"family_id": "fam", "access_token": "tok"}`)
	if mine.Code != http.StatusOK || !strings.Contains(mine.Body.String(), id) {
		t.Fatalf("mine = %d: %s", mine.Code, mine.Body.String())
	}
}
