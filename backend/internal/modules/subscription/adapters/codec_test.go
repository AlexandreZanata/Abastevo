package adapters

import (
	"bytes"
	"testing"
)

func TestTokenEnvelopeAuthentication(t *testing.T) {
	c, err := NewTokenCodec(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.Seal("synthetic-opaque-token", "owner-env-digest")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := c.Seal("synthetic-opaque-token", "owner-env-digest")
	if bytes.Equal(a, b) || bytes.Contains(a, []byte("synthetic-opaque-token")) {
		t.Fatal("plaintext/nonrandom token")
	}
	got, err := c.Open(a, "owner-env-digest")
	if err != nil || got != "synthetic-opaque-token" {
		t.Fatal("roundtrip failed")
	}
	if _, err = c.Open(a, "foreign-owner"); err == nil {
		t.Fatal("foreign AAD accepted")
	}
	a[len(a)-1] ^= 1
	if _, err = c.Open(a, "owner-env-digest"); err == nil {
		t.Fatal("tamper accepted")
	}
	if _, err = NewTokenCodec([]byte("short")); err == nil {
		t.Fatal("weak key")
	}
}
