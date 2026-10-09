package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var testSecret = []byte("test-secret-32-bytes-long-xxxxxx")

func TestCursorRoundTrip(t *testing.T) {
	now := time.Now()
	token := Seal(testSecret, "fh1", "key-9", now)
	last, err := Open(testSecret, token, "fh1", now)
	if err != nil || last != "key-9" {
		t.Errorf("round trip = %q, %v", last, err)
	}
}

func TestCursorRefusals(t *testing.T) {
	now := time.Now()
	token := Seal(testSecret, "fh1", "k", now)
	if _, err := Open(testSecret, token, "other", now); err == nil {
		t.Error("filter change accepted")
	}
	if _, err := Open(testSecret, token, "fh1", now.Add(CursorTTL+time.Second)); err == nil {
		t.Error("expired cursor accepted")
	}
	broken := token[:len(token)-4] + "AAAA"
	if _, err := Open(testSecret, broken, "fh1", now); err == nil {
		t.Error("tampered cursor accepted")
	}
	for _, bad := range []string{"", "!!!", "aGk"} {
		if _, err := Open(testSecret, bad, "fh1", now); err == nil {
			t.Errorf("garbage %q accepted", bad)
		}
	}
	if _, err := Open([]byte("wrong-secret-32-bytes-long-xxxx"), token, "fh1", now); err == nil {
		t.Error("wrong secret accepted")
	}
	if got, err := ParseCursor(testSecret, "", "fh1", now); err != nil || got != "" {
		t.Errorf("empty cursor = %q, %v", got, err)
	}
}

func TestParseLimit(t *testing.T) {
	if n, err := ParseLimit(""); err != nil || n != DefaultLimit {
		t.Errorf("default = %d, %v", n, err)
	}
	if n, err := ParseLimit("100"); err != nil || n != 100 {
		t.Errorf("max = %d, %v", n, err)
	}
	for _, raw := range []string{"0", "101", "-3", "abc", "20.5", ""} {
		if raw == "" {
			continue
		}
		if _, err := ParseLimit(raw); err == nil {
			t.Errorf("limit %q accepted", raw)
		}
	}
}

func TestErrorEnvelopeShape(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/stations", nil)
	WriteError(w, r, 400, "station.bad-limit", "safe message", []Detail{{Field: "limit", Code: "range"}})
	res := w.Result()
	if res.StatusCode != 400 {
		t.Errorf("status = %d", res.StatusCode)
	}
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("error cache = %q", cc)
	}
	body := w.Body.String()
	for _, want := range []string{`"code":"station.bad-limit"`, `"trace_id"`, `"field":"limit"`} {
		if !strings.Contains(body, want) {
			t.Errorf("envelope missing %s: %s", want, body)
		}
	}
}

func TestWriteJSONWithETagDecouplesIdentity(t *testing.T) {
	stable := []byte(`{"items":[{"id":1}]}`)
	first := []byte(`{"items":[{"id":1}],"generated_at":"2026-10-09T12:00:00Z"}`)
	second := []byte(`{"items":[{"id":1}],"generated_at":"2026-10-09T12:00:01Z"}`)
	etag := ETag(stable)
	r1 := httptest.NewRequest("GET", "/v1/stations", nil)
	w1 := httptest.NewRecorder()
	WriteJSONWithETag(w1, r1, 200, "public, max-age=60", first, etag)
	if w1.Code != 200 || w1.Header().Get("ETag") != etag {
		t.Fatalf("first render = %d etag %q", w1.Code, w1.Header().Get("ETag"))
	}
	r2 := httptest.NewRequest("GET", "/v1/stations", nil)
	r2.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	WriteJSONWithETag(w2, r2, 200, "public, max-age=60", second, etag)
	if w2.Code != 304 || w2.Body.Len() != 0 {
		t.Fatalf("volatile revalidation = %d bytes %d, want bodiless 304", w2.Code, w2.Body.Len())
	}
}

func TestETagConditional(t *testing.T) {
	body := []byte(`{"items":[]}`)
	etag := ETag(body)
	r := httptest.NewRequest("GET", "/v1/stations", nil)
	r.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()
	WriteJSON(w, r, 200, "public, max-age=60", body)
	if w.Code != 304 {
		t.Errorf("status = %d, want 304", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Error("304 carried a body")
	}
	r2 := httptest.NewRequest("GET", "/v1/stations", nil)
	w2 := httptest.NewRecorder()
	WriteJSON(w2, r2, 200, "no-store", body)
	if w2.Code != 200 || w2.Body.String() != string(body) {
		t.Errorf("plain render failed: %d", w2.Code)
	}
	if w2.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("cache header = %q", w2.Header().Get("Cache-Control"))
	}
}
