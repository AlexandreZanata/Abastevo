package adapters

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

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
)

type fakeClaimStore struct {
	mu     sync.Mutex
	claims map[string]application.ClaimRow
	decls  map[string]application.DeclarationRow
	active map[string]string
	quota  int64
}

func (f *fakeClaimStore) CreateClaim(_ context.Context, id, accountID, stationID, operatorCNPJ, operatorSource, role, scopes, clientKey string) (application.ClaimRow, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.claims {
		if row.AccountID == accountID && row.ClientKey == clientKey {
			return application.ClaimRow{}, false, nil
		}
	}
	row := application.ClaimRow{ID: id, AccountID: accountID, StationID: stationID, OperatorCNPJ: operatorCNPJ, OperatorSource: operatorSource, Role: role, Scopes: strings.Split(scopes, ","), State: "draft", ClientKey: clientKey}
	if f.claims == nil {
		f.claims = map[string]application.ClaimRow{}
	}
	f.claims[id] = row
	_ = scopes
	return row, true, nil
}

func (f *fakeClaimStore) Claim(_ context.Context, id string) (application.ClaimRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.claims[id]
	if !ok {
		return application.ClaimRow{}, application.ErrClaimNotFound
	}
	return row, nil
}

func (f *fakeClaimStore) ClaimByKey(_ context.Context, accountID, clientKey string) (application.ClaimRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, row := range f.claims {
		if row.AccountID == accountID && row.ClientKey == clientKey {
			return row, nil
		}
	}
	return application.ClaimRow{}, application.ErrClaimNotFound
}

func (f *fakeClaimStore) ListOwnedClaims(_ context.Context, accountID string, _, _ int) ([]application.ClaimRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []application.ClaimRow
	for _, row := range f.claims {
		if row.AccountID == accountID {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeClaimStore) CountOpenClaims(_ context.Context, _ string) (int64, error) {
	return f.quota, nil
}

func (f *fakeClaimStore) SetClaimState(_ context.Context, id, expected, state string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.claims[id]
	if !ok || row.State != expected {
		return 0, nil
	}
	row.State = state
	f.claims[id] = row
	return 1, nil
}

func (f *fakeClaimStore) CreateDeclaration(_ context.Context, id, claimID string, version int, nonceDigest, expectedDigest, declaration string, expiresAt time.Time) (application.DeclarationRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row := application.DeclarationRow{ID: id, ClaimID: claimID, Version: version, NonceDigest: nonceDigest, ExpectedDigest: expectedDigest, Declaration: declaration, State: "active", ExpiresAt: expiresAt}
	if f.decls == nil {
		f.decls = map[string]application.DeclarationRow{}
		f.active = map[string]string{}
	}
	f.decls[id] = row
	f.active[claimID] = id
	return row, nil
}

func (f *fakeClaimStore) SupersedeDeclarations(_ context.Context, claimID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for id, row := range f.decls {
		if row.ClaimID == claimID && row.State == "active" {
			row.State = "superseded"
			f.decls[id] = row
		}
	}
	return nil
}

func (f *fakeClaimStore) ActiveDeclaration(_ context.Context, claimID string) (application.DeclarationRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.active[claimID]
	if !ok {
		return application.DeclarationRow{}, application.ErrClaimNotFound
	}
	if row := f.decls[id]; row.State == "active" {
		return row, nil
	}
	return application.DeclarationRow{}, application.ErrClaimNotFound
}

func (f *fakeClaimStore) GetDeclaration(_ context.Context, id string) (application.DeclarationRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.decls[id]
	if !ok {
		return application.DeclarationRow{}, application.ErrClaimNotFound
	}
	return row, nil
}

func claimTestPorts(store *fakeClaimStore) application.ClaimPorts {
	n := 0
	return application.ClaimPorts{
		Store: store,
		Clock: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) {
			n++
			return "claim-id-" + string(rune('0'+n)), nil
		},
		OperatorOf: func(context.Context, string) (string, string, bool, error) {
			return "04218406000104", "registry", true, nil
		},
	}
}

func claimTestHandler(store *fakeClaimStore) ClaimHandler {
	return ClaimHandler{
		Ports: claimTestPorts(store),
		Sessions: func(context.Context, string, string) (string, error) {
			return "acc-1", nil
		},
	}
}

func doClaimRequest(h ClaimHandler, method, target, body string) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	h.RegisterRoutes(router)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func openBody(key string) string {
	return `{"family_id": "fam", "access_token": "tok", "role": "administrator", "scopes": ["profile.edit"], "client_key": "` + key + `"}`
}

func TestClaimOpenReplayAndConflict(t *testing.T) {
	h := claimTestHandler(&fakeClaimStore{})
	first := doClaimRequest(h, http.MethodPost, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/claims", openBody("key-1"))
	if first.Code != http.StatusCreated {
		t.Fatalf("open = %d: %s", first.Code, first.Body.String())
	}
	if first.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("private response without no-store")
	}
	var created map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created["declaration"] == "" || created["version"].(float64) != 1 {
		t.Fatalf("declaration missing: %v", created)
	}
	second := doClaimRequest(h, http.MethodPost, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/claims", openBody("key-1"))
	if second.Code != http.StatusOK {
		t.Fatalf("replay = %d", second.Code)
	}
	var replayed map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &replayed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if replayed["id"] != created["id"] || replayed["declaration"] != created["declaration"] {
		t.Fatal("replay diverged (new nonce would break idempotency)")
	}
	changed := strings.Replace(openBody("key-1"), "administrator", "manager", 1)
	conflict := doClaimRequest(h, http.MethodPost, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/claims", changed)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("changed = %d, want 409", conflict.Code)
	}
}

func TestClaimRejectsBadSessionAndQuota(t *testing.T) {
	denied := func(context.Context, string, string) (string, error) {
		return "", errors.New("session-invalid")
	}
	h := ClaimHandler{Ports: claimTestPorts(&fakeClaimStore{}), Sessions: denied}
	res := doClaimRequest(h, http.MethodPost, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/claims", openBody("key-1"))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("bad session = %d, want 401", res.Code)
	}
	store := &fakeClaimStore{quota: 3}
	h = claimTestHandler(store)
	res = doClaimRequest(h, http.MethodPost, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/claims", openBody("key-1"))
	if res.Code != http.StatusTooManyRequests {
		t.Fatalf("quota = %d, want 429", res.Code)
	}
}

func TestClaimStatusReissueCancelAreOwnerOnly(t *testing.T) {
	h := claimTestHandler(&fakeClaimStore{})
	created := doClaimRequest(h, http.MethodPost, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/claims", openBody("key-1"))
	var doc map[string]any
	if err := json.Unmarshal(created.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	id, _ := doc["id"].(string)
	session := `{"family_id": "fam", "access_token": "tok"}`

	ok := doClaimRequest(h, http.MethodPost, "/v1/profile/claims/"+id+"/status", session)
	if ok.Code != http.StatusOK {
		t.Fatalf("owner status = %d", ok.Code)
	}
	foreign := doClaimRequest(ClaimHandler{Ports: h.Ports, Sessions: func(context.Context, string, string) (string, error) {
		return "acc-2", nil
	}}, http.MethodPost, "/v1/profile/claims/"+id+"/status", session)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign status = %d, want 404 (no oracle, no leakage)", foreign.Code)
	}
	reissued := doClaimRequest(h, http.MethodPost, "/v1/profile/claims/"+id+"/reissue", session)
	var second map[string]any
	if err := json.Unmarshal(reissued.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if reissued.Code != http.StatusOK || second["version"].(float64) != 2 || second["declaration"] == doc["declaration"] {
		t.Fatalf("reissue = %d: %v", reissued.Code, second)
	}
	cancelled := doClaimRequest(h, http.MethodPost, "/v1/profile/claims/"+id+"/cancel", session)
	if cancelled.Code != http.StatusOK {
		t.Fatalf("cancel = %d", cancelled.Code)
	}
	again := doClaimRequest(h, http.MethodPost, "/v1/profile/claims/"+id+"/cancel", session)
	if again.Code != http.StatusConflict {
		t.Fatalf("double cancel = %d, want 409", again.Code)
	}
	mine := doClaimRequest(h, http.MethodPost, "/v1/profile/claims/mine", session)
	if mine.Code != http.StatusOK || !strings.Contains(mine.Body.String(), id) {
		t.Fatalf("mine = %d: %s", mine.Code, mine.Body.String())
	}
}
