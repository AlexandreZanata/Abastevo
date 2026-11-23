package adapters

import (
	"bytes"
	"regexp"
	"testing"
)

func TestAppAccountTokenIsStableKeyedUUID(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	token, err := NewAppAccountToken(key, "production", "com.anpfuel.app")
	if err != nil {
		t.Fatal(err)
	}
	account := "0b7f3f1e-5c1d-4a3b-9f0e-2d4c6b8a1e3f"
	first := token(account)
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-8[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(first) {
		t.Fatalf("not a version 8 UUID: %s", first)
	}
	if token(account) != first {
		t.Fatal("binding must be stable")
	}
	if token("0b7f3f1e-5c1d-4a3b-9f0e-2d4c6b8a1e40") == first {
		t.Fatal("accounts must not share a binding")
	}
	other, _ := NewAppAccountToken(bytes.Repeat([]byte{8}, 32), "production", "com.anpfuel.app")
	sandbox, _ := NewAppAccountToken(key, "test", "com.anpfuel.app")
	if other(account) == first || sandbox(account) == first {
		t.Fatal("key and environment must separate bindings")
	}
	if token("not-an-account") != "" {
		t.Fatal("invalid account must not bind")
	}
	if _, err := NewAppAccountToken(key[:31], "production", "com.anpfuel.app"); err == nil {
		t.Fatal("short key accepted")
	}
	play, _ := NewAccountBinding(key, "production", "com.anpfuel.app")
	if len(play(account)) != 64 {
		t.Fatal("Play obfuscatedAccountId must be 64 hex characters")
	}
}
