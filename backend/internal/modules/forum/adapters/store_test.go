package adapters

import (
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func TestCursorEncodingAndDecoding(t *testing.T) {
	const validUUID = "00000000-0000-0000-0000-000000000001"
	const seq = int64(42)

	token := EncodeCursor(seq, validUUID)
	if token == "" {
		t.Fatal("expected non-empty cursor token")
	}

	gotSeq, gotID, err := DecodeCursor(token)
	if err != nil {
		t.Fatalf("unexpected error decoding cursor: %v", err)
	}
	if gotSeq != seq {
		t.Errorf("got seq %d, want %d", gotSeq, seq)
	}
	if gotID != validUUID {
		t.Errorf("got id %q, want %q", gotID, validUUID)
	}
}

func TestCursorDecoding_InvalidInputs(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"whitespace", "   "},
		{"malformed_base64", "!!!not-base64!!!"},
		{"missing_separator", "MTIzNDU"}, // base64 for "12345"
		{"non_numeric_seq", "YWJjOjAwMDAwMDAwLTAwMDAtMDAwMC0wMDAwLTAwMDAwMDAwMDAwMQ"}, // "abc:00000000-..."
		{"zero_seq", "MDowMDAwMDAwMC0wMDAwLTAwMDAtMDAwMC0wMDAwMDAwMDAwMDE"},           // "0:00000000-..."
		{"negative_seq", "LTE6MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAx"},      // "-1:00000000-..."
		{"malformed_uuid", "NDI6bm90LWEtdXVpZA"},                                      // "42:not-a-uuid"
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := DecodeCursor(tc.token)
			if !errors.Is(err, domain.ErrInvalidCursor) {
				t.Fatalf("expected ErrInvalidCursor, got %v", err)
			}
		})
	}
}

func TestUUIDHelpers(t *testing.T) {
	const original = "12345678-1234-1234-1234-123456789abc"
	u, err := mustUUID(original)
	if err != nil {
		t.Fatalf("mustUUID failed on valid uuid: %v", err)
	}
	if !u.Valid {
		t.Fatal("expected u.Valid to be true")
	}

	formatted := uuidString(u)
	if !strings.EqualFold(formatted, original) {
		t.Fatalf("uuid roundtrip mismatch: got %q, want %q", formatted, original)
	}

	// Malformed cases
	invalidCases := []string{
		"",
		"not-a-uuid",
		"12345678-1234-1234-1234-123456789ab",   // 31 hex digits
		"12345678-1234-1234-1234-123456789abcd", // 33 hex digits
		"12345678-1234-1234-1234-123456789abg",  // non-hex
	}
	for _, inv := range invalidCases {
		_, err := mustUUID(inv)
		if err == nil {
			t.Errorf("mustUUID should have failed for %q", inv)
		}
	}
}
