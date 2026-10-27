package http

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// TestPublicCursor_CTB10_DoesNotExposeAccountID: the public next_cursor is
// opaque. Its decoded bytes never contain the account id it resumes from,
// it round-trips with the right secret and fails closed otherwise.
func TestPublicCursor_CTB10_DoesNotExposeAccountID(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	since := time.Date(2026, 3, 1, 9, 30, 0, 123456000, time.UTC)
	const accountID = "11111111-2222-4333-8444-555555555555"
	token, err := sealPublicCursor(secret, since, accountID, now)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{accountID, strings.ReplaceAll(accountID, "-", ""), "2026-03-01"} {
		if strings.Contains(string(raw), leak) || strings.Contains(token, leak) {
			t.Fatalf("cursor exposes %q", leak)
		}
	}
	gotSince, gotID, err := openPublicCursor(secret, token, now.Add(time.Minute))
	if err != nil || !gotSince.Equal(since) || gotID != accountID {
		t.Fatalf("roundtrip since=%v id=%q err=%v", gotSince, gotID, err)
	}
	if _, _, err := openPublicCursor([]byte(strings.Repeat("o", 32)), token, now); err == nil {
		t.Fatal("cursor opened with another secret")
	}
	tampered := []byte(token)
	tampered[len(tampered)-2] ^= 1
	if _, _, err := openPublicCursor(secret, string(tampered), now); err == nil {
		t.Fatal("tampered cursor opened")
	}
	again, err := sealPublicCursor(secret, since, accountID, now)
	if err != nil || again == token {
		t.Fatalf("cursor is deterministic: err=%v", err)
	}
}
