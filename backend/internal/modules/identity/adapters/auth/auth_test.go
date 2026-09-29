package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSingleValueRefuses(t *testing.T) {
	r := httptest.NewRequest("GET", "/v1/x", nil)
	if _, err := singleValue(r.Header, HeaderSignature); err == nil {
		t.Error("missing header accepted")
	}
	r.Header.Add(HeaderSignature, "a")
	r.Header.Add(HeaderSignature, "b")
	if _, err := singleValue(r.Header, HeaderSignature); err == nil {
		t.Error("duplicated header accepted")
	}
	r2 := httptest.NewRequest("GET", "/v1/x", nil)
	r2.Header.Set(HeaderSignature, "  abc  ")
	if got, err := singleValue(r2.Header, HeaderSignature); err != nil || got != "abc" {
		t.Errorf("single = %q, %v", got, err)
	}
}

func TestBaseLinesShape(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/observations?deep=true", strings.NewReader(`{"a":1}`))
	r.Header.Set("Content-Type", "application/json")
	lines := BaseLines(r, "api.example.invalid", "1", "2", "fp:x", "ch.c", []byte(`{"a":1}`), true)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		`"@method": POST`, `"@authority": api.example.invalid`,
		`"@path": /v1/observations`, `"@query": deep=true`,
		`"content-type": application/json`, `"content-digest": "sha-512=:`,
		`"created": 1`, `"expires": 2`, `"keyid": "fp:x"`, `"nonce": "ch.c"`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("base missing %s:\n%s", want, joined)
		}
	}
	bare := BaseLines(r, "h", "1", "2", "k", "n", nil, false)
	for _, l := range bare {
		if strings.HasPrefix(l, `"content-`) {
			t.Errorf("bodyless base carries content line: %q", l)
		}
	}
	if len(bare) != 8 {
		t.Errorf("bodyless base has %d lines, want 8", len(bare))
	}
}
