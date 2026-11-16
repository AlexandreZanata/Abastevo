package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/safety/domain"
)

const (
	account  = "aaaaaaaa-0000-4000-8000-00000000000a"
	family   = "fafafafa-0000-4000-8000-00000000000f"
	content  = "c0c0c0c0-0000-4000-8000-00000000000b"
	blockID  = "b1b1b1b1-0000-4000-8000-000000000001"
	authorID = "bbbbbbbb-0000-4000-8000-00000000000b"
)

type fakeStore struct {
	blocks    []application.BlockCommand
	reports   []application.ReportCommand
	unblocks  []application.UnblockCommand
	hidden    [][]domain.ContentRef
	err       error
	guardSeen bool
}

func (f *fakeStore) runGuard(ctx context.Context, g application.Guard) error {
	if g != nil {
		f.guardSeen = true
		return g(ctx, nil)
	}
	return nil
}

func (f *fakeStore) Block(ctx context.Context, cmd application.BlockCommand) (domain.BlockResult, error) {
	if err := f.runGuard(ctx, cmd.Guard); err != nil {
		return domain.BlockResult{}, err
	}
	f.blocks = append(f.blocks, cmd)
	if f.err != nil {
		return domain.BlockResult{}, f.err
	}
	return domain.BlockResult{Block: domain.Block{ID: blockID, Label: "k7q2-m9zx-42ab", SourceKind: cmd.Ref.Kind, CreatedAt: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}}, nil
}

func (f *fakeStore) Unblock(ctx context.Context, cmd application.UnblockCommand) error {
	if err := f.runGuard(ctx, cmd.Guard); err != nil {
		return err
	}
	f.unblocks = append(f.unblocks, cmd)
	return f.err
}

func (f *fakeStore) ListBlocks(context.Context, string) ([]domain.Block, error) {
	return []domain.Block{{ID: blockID, Label: "", SourceKind: domain.KindForumComment, CreatedAt: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}}, f.err
}

func (f *fakeStore) Hidden(_ context.Context, _ string, refs []domain.ContentRef) ([]domain.ContentRef, error) {
	f.hidden = append(f.hidden, refs)
	if f.err != nil {
		return nil, f.err
	}
	return refs[:1], nil
}

func (f *fakeStore) ReportUser(ctx context.Context, cmd application.ReportCommand) (domain.ReportReceipt, error) {
	if err := f.runGuard(ctx, cmd.Guard); err != nil {
		return domain.ReportReceipt{}, err
	}
	f.reports = append(f.reports, cmd)
	if f.err != nil {
		return domain.ReportReceipt{}, f.err
	}
	return domain.ReportReceipt{ID: "r0r0r0r0-0000-4000-8000-000000000001", Reason: cmd.Reason, CreatedAt: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}, nil
}

func router(store *fakeStore, guardErr error, signedErr error) *chi.Mux {
	h := Handler{
		Service: application.Service{Store: store},
		Sessions: func(_ context.Context, fam, token string) (string, error) {
			if fam == family && token == "tok" {
				return account, nil
			}
			return "", errors.New("expired")
		},
		SignedOwner: func(*http.Request, string) (string, string, error) {
			if signedErr != nil {
				return "", "", signedErr
			}
			return "c1c1c1c1-0000-4000-8000-000000000001", "k1k1k1k1-0000-4000-8000-000000000001", nil
		},
		Guard: func(context.Context, pgx.Tx, string, string, string, string, string) error { return guardErr },
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func post(r http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func session(extra string) string {
	body := `{"family_id":"` + family + `","access_token":"tok"`
	if extra != "" {
		body += "," + extra
	}
	return body + "}"
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("error body %q: %v", w.Body.String(), err)
	}
	return env.Error.Code
}

// B-BR-SAFE-03: every route refuses missing, expired and unsigned
// sessions before any store work.
func TestSafetyRoutesRequireSignedSession(t *testing.T) {
	paths := []string{"/v1/community/user-reports", "/v1/community/blocks", "/v1/community/blocks/list", "/v1/community/blocks/hidden", "/v1/community/blocks/" + blockID + "/remove"}
	bodies := map[string]string{
		"no session":      `{}`,
		"expired session": `{"family_id":"` + family + `","access_token":"old"}`,
		"bad family":      `{"family_id":"x","access_token":"tok"}`,
	}
	for _, path := range paths {
		for name, body := range bodies {
			store := &fakeStore{}
			w := post(router(store, nil, nil), path, body)
			if w.Code != http.StatusUnauthorized || errorCode(t, w) != "safety.auth-required" {
				t.Fatalf("%s %s: %d %s", path, name, w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%s %s: cache-control %q", path, name, w.Header().Get("Cache-Control"))
			}
			if len(store.blocks)+len(store.reports)+len(store.unblocks)+len(store.hidden) != 0 {
				t.Fatalf("%s %s reached the store", path, name)
			}
		}
		store := &fakeStore{}
		w := post(router(store, nil, errors.New("unbound key")), path, session(""))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s unsigned: %d", path, w.Code)
		}
	}
}

func TestSafetyRejectsNonJSONAndUnknownFields(t *testing.T) {
	store := &fakeStore{}
	r := router(store, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/community/blocks", strings.NewReader(session(`"content_kind":"FORUM_COMMENT","content_id":"`+content+`"`)))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("text/plain = %d", w.Code)
	}
	w = post(r, "/v1/community/blocks", session(`"content_kind":"FORUM_COMMENT","content_id":"`+content+`","account_id":"`+authorID+`"`))
	if w.Code != http.StatusBadRequest || len(store.blocks) != 0 {
		t.Fatalf("client-named account accepted: %d %s", w.Code, w.Body.String())
	}
}

func TestSafetyBlockResponseShape(t *testing.T) {
	store := &fakeStore{}
	w := post(router(store, nil, nil), "/v1/community/blocks", session(`"content_kind":"STATION_COMMENT","content_id":"`+strings.ToUpper(content)+`"`))
	if w.Code != http.StatusOK {
		t.Fatalf("block = %d %s", w.Code, w.Body.String())
	}
	if !store.guardSeen || len(store.blocks) != 1 || store.blocks[0].AccountID != account || store.blocks[0].Ref.ID != content {
		t.Fatalf("store command = %+v (guard %v)", store.blocks, store.guardSeen)
	}
	want := `{"block":{"block_id":"` + blockID + `","label":"k7q2-m9zx-42ab","source_kind":"STATION_COMMENT","created_at":"2026-11-15T12:00:00Z"},"replayed":false}`
	if strings.TrimSpace(w.Body.String()) != want {
		t.Fatalf("body = %s\nwant %s", w.Body.String(), want)
	}
	if strings.Contains(w.Body.String(), account) || strings.Contains(w.Body.String(), authorID) {
		t.Fatal("response carries an account id")
	}
}

func TestSafetyReportValidationAndShape(t *testing.T) {
	store := &fakeStore{}
	r := router(store, nil, nil)
	for name, extra := range map[string]string{
		"unknown reason": `"content_kind":"FORUM_POST","content_id":"` + content + `","reason":"OFF_TOPIC"`,
		"long detail":    `"content_kind":"FORUM_POST","content_id":"` + content + `","reason":"SPAM","detail":"` + strings.Repeat("a", 201) + `"`,
		"bad kind":       `"content_kind":"EDITORIAL","content_id":"` + content + `","reason":"SPAM"`,
		"bad id":         `"content_kind":"FORUM_POST","content_id":"x","reason":"SPAM"`,
	} {
		w := post(r, "/v1/community/user-reports", session(extra))
		if w.Code != http.StatusBadRequest || errorCode(t, w) != "safety.bad-request" {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	if len(store.reports) != 0 {
		t.Fatal("invalid reports reached the store")
	}
	w := post(r, "/v1/community/user-reports", session(`"content_kind":"FORUM_POST","content_id":"`+content+`","reason":"HATE","detail":"  ofensas  "`))
	if w.Code != http.StatusOK || len(store.reports) != 1 || store.reports[0].Detail != "ofensas" {
		t.Fatalf("report = %d %s %+v", w.Code, w.Body.String(), store.reports)
	}
	if body := w.Body.String(); !strings.Contains(body, `"reason":"HATE"`) || strings.Contains(body, "ofensas") || strings.Contains(body, "case") {
		t.Fatalf("receipt leaks detail or case: %s", body)
	}
}

func TestSafetyErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
		retry  string
	}{
		{domain.ErrSelfTarget, 400, "safety.self-target", ""},
		{domain.ErrNotFound, 404, "safety.not-found", ""},
		{domain.ErrBlockLimit, 409, "safety.block-limit", ""},
		{&domain.QuotaError{RetryAfterSeconds: 3600}, 429, "safety.rate-limited", "3600"},
		{ErrGuardRefused, 401, "safety.auth-required", ""},
		{errors.New("pg: connection reset by peer at 10.0.0.1"), 503, "safety.unavailable", ""},
	}
	for _, tc := range cases {
		store := &fakeStore{err: tc.err}
		w := post(router(store, nil, nil), "/v1/community/blocks", session(`"content_kind":"FORUM_COMMENT","content_id":"`+content+`"`))
		if w.Code != tc.status || errorCode(t, w) != tc.code || w.Header().Get("Retry-After") != tc.retry {
			t.Fatalf("%v: %d %s retry %q", tc.err, w.Code, w.Body.String(), w.Header().Get("Retry-After"))
		}
		if strings.Contains(w.Body.String(), "10.0.0.1") {
			t.Fatal("internal error text leaked")
		}
	}
	// A guard refusal inside the transaction is a 401, not a 503.
	store := &fakeStore{}
	w := post(router(store, ErrGuardRefused, nil), "/v1/community/user-reports", session(`"content_kind":"FORUM_POST","content_id":"`+content+`","reason":"SPAM"`))
	if w.Code != http.StatusUnauthorized || len(store.reports) != 0 {
		t.Fatalf("guard refusal = %d, stored %d", w.Code, len(store.reports))
	}
}

func TestSafetyHiddenListUnblock(t *testing.T) {
	store := &fakeStore{}
	r := router(store, nil, nil)
	w := post(r, "/v1/community/blocks/hidden", session(`"items":[{"content_kind":"FORUM_COMMENT","content_id":"`+content+`"},{"content_kind":"STATION_COMMENT","content_id":"`+blockID+`"}]`))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"hidden":[{"content_kind":"FORUM_COMMENT","content_id":"`+content+`"}]}` {
		t.Fatalf("hidden = %d %s", w.Code, w.Body.String())
	}
	items := make([]string, 101)
	for i := range items {
		items[i] = `{"content_kind":"FORUM_COMMENT","content_id":"` + content + `"}`
	}
	if w := post(r, "/v1/community/blocks/hidden", session(`"items":[`+strings.Join(items, ",")+`]`)); w.Code != http.StatusBadRequest {
		t.Fatalf("101 refs = %d", w.Code)
	}
	if w := post(r, "/v1/community/blocks/hidden", session(`"items":[]`)); w.Code != http.StatusBadRequest {
		t.Fatalf("0 refs = %d", w.Code)
	}
	w = post(r, "/v1/community/blocks/list", session(""))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"items":[{"block_id":"`+blockID+`","label":"","source_kind":"FORUM_COMMENT","created_at":"2026-11-15T12:00:00Z"}]}` {
		t.Fatalf("list = %d %s", w.Code, w.Body.String())
	}
	w = post(r, "/v1/community/blocks/not-a-uuid/remove", session(""))
	if w.Code != http.StatusNotFound || len(store.unblocks) != 0 {
		t.Fatalf("malformed block id = %d", w.Code)
	}
	w = post(r, "/v1/community/blocks/"+blockID+"/remove", session(""))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"removed":true}` || store.unblocks[0].BlockID != blockID {
		t.Fatalf("remove = %d %s", w.Code, w.Body.String())
	}
}

// B-BR-SAFE-03: the request proof covers the exact body bytes, so the
// signature verifier must still read the body after JSON decoding.
func TestSafetySignedOwnerSeesExactBody(t *testing.T) {
	body := session(`"content_kind":"FORUM_COMMENT","content_id":"` + content + `"`)
	var seen string
	h := Handler{
		Service: application.Service{Store: &fakeStore{}},
		Sessions: func(context.Context, string, string) (string, error) {
			return account, nil
		},
		SignedOwner: func(r *http.Request, _ string) (string, string, error) {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				return "", "", err
			}
			seen = string(raw)
			if seen != body {
				return "", "", errors.New("digest mismatch")
			}
			return "c1c1c1c1-0000-4000-8000-000000000001", "k1k1k1k1-0000-4000-8000-000000000001", nil
		},
	}
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	w := post(r, "/v1/community/blocks", body)
	if w.Code != http.StatusOK || seen != body {
		t.Fatalf("signed block = %d, verifier saw %q", w.Code, seen)
	}
}
