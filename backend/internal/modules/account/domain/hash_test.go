package domain

import (
	"regexp"
	"strings"
	"testing"
)

func TestHasherRoundTrip(t *testing.T) {
	h := SHA256Hasher{}
	salt, err := h.NewSalt()
	if err != nil {
		t.Fatal(err)
	}
	if len(salt) != 32 {
		t.Errorf("salt length = %d, want 32 hex chars", len(salt))
	}
	stored := h.Hash(salt, "482916")
	if stored == "482916" || strings.Contains(stored, "482916") {
		t.Error("stored verifier must not contain the code")
	}
	if !h.Equal(stored, salt, "482916") {
		t.Error("correct code must verify")
	}
	if h.Equal(stored, salt, "482917") {
		t.Error("wrong code must not verify")
	}
	other, _ := h.NewSalt()
	if h.Equal(stored, other, "482916") {
		t.Error("wrong salt must not verify")
	}
	if h.Equal("not-hex!!", salt, "482916") {
		t.Error("corrupt verifier must not verify")
	}
}

func TestGenerateCodeShape(t *testing.T) {
	shape := regexp.MustCompile(`^[0-9]{6}$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		c, err := GenerateCode()
		if err != nil {
			t.Fatal(err)
		}
		if !shape.MatchString(c) {
			t.Fatalf("code %q must be 6 digits", c)
		}
		seen[c] = true
	}
	if len(seen) < 190 {
		t.Errorf("200 codes yielded only %d distinct values", len(seen))
	}
}

func TestGenerateTokenAndAlias(t *testing.T) {
	tok, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 64 {
		t.Errorf("token length = %d, want 64 hex chars", len(tok))
	}
	shape := regexp.MustCompile(`^[abcdefghjkmnpqrstuvwxyz23456789]{4}-[abcdefghjkmnpqrstuvwxyz23456789]{4}-[abcdefghjkmnpqrstuvwxyz23456789]{4}$`)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		a, err := GenerateAlias()
		if err != nil {
			t.Fatal(err)
		}
		if !shape.MatchString(a) {
			t.Fatalf("alias %q must be 4-4-4 unambiguous lowercase", a)
		}
		seen[a] = true
	}
	if len(seen) != 200 {
		t.Errorf("200 aliases yielded only %d distinct values", len(seen))
	}
}

func TestAddressHashIsPIIFree(t *testing.T) {
	h1 := AddressHash("Case-01@Example.Invalid")
	h2 := AddressHash("  case-01@example.invalid ")
	if h1 != h2 {
		t.Error("lookup key must be case/space-insensitive")
	}
	if strings.Contains(h1, "case-01") || len(h1) != 64 {
		t.Errorf("lookup key must be an opaque hash, got %q", h1)
	}
	if NormalizeAddress("  AbC@X.Invalid ") != "abc@x.invalid" {
		t.Error("normalization must trim and lowercase")
	}
}
