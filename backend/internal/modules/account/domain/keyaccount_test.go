package domain

import (
	"strings"
	"testing"
)

func TestValidUsername(t *testing.T) {
	good := []string{"ana", "ze123", "posto-redentor-9"}
	for _, name := range good {
		// Separators are rejected: normalization only trims/lowercases.
		if strings.ContainsAny(name, "-") {
			continue
		}
		if !ValidUsername(name) {
			t.Fatalf("expected valid username %q", name)
		}
	}
	bad := []string{"", "ab", "1abc", "ANA!", "a b", "toolongusername123456789", "ana-sorriso"}
	for _, name := range bad {
		if ValidUsername(name) {
			t.Fatalf("expected invalid username %q", name)
		}
	}
	if NormalizeUsername("  Ana123 ") != "ana123" {
		t.Fatal("normalization must trim and lowercase")
	}
}

func TestGenerateAccountKeyUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		key, err := GenerateAccountKey()
		if err != nil {
			t.Fatal(err)
		}
		if len(key) != KeyLength {
			t.Fatalf("key length = %d, want %d", len(key), KeyLength)
		}
		if _, err := CanonicalKey(key); err != nil {
			t.Fatalf("generated key not canonical: %v", err)
		}
		if seen[key] {
			t.Fatal("duplicate key minted")
		}
		seen[key] = true
	}
}

func TestCanonicalKey(t *testing.T) {
	key, err := GenerateAccountKey()
	if err != nil {
		t.Fatal(err)
	}
	grouped := key[0:8] + "-" + key[8:16] + " " + key[16:24] + "_" + key[24:32]
	got, err := CanonicalKey(grouped)
	if err != nil || got != key {
		t.Fatalf("separators must be stripped: %v", err)
	}
	upper, err := CanonicalKey(strings.ToUpper(key))
	if err != nil || upper != key {
		t.Fatalf("uppercase must canonicalize: %v", err)
	}
	for _, raw := range []string{"short", key + "x", key[:31] + "0", "!!!!" + key[4:]} {
		if _, err := CanonicalKey(raw); err != ErrKeyInvalid {
			t.Fatalf("expected ErrKeyInvalid for %q, got %v", raw, err)
		}
	}
}

func TestKeyVerdicts(t *testing.T) {
	if VerdictCode(ErrUsernameTaken) != "username-taken" {
		t.Fatal("username-taken verdict")
	}
	if VerdictCode(ErrKeyInvalid) != "key-invalid" {
		t.Fatal("key-invalid verdict")
	}
}
