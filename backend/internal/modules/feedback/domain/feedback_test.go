package domain

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestRatingRange(t *testing.T) {
	for _, stars := range []Rating{1, 2, 3, 4, 5} {
		if err := stars.Validate(); err != nil {
			t.Errorf("rating %d must validate: %v", stars, err)
		}
	}
	for _, stars := range []Rating{-1, 0, 6, 100} {
		if err := stars.Validate(); err == nil {
			t.Errorf("rating %d must refuse", stars)
		}
	}
}

func TestCommentBounds(t *testing.T) {
	got, err := ParseComment("Preço bom ⛽")
	if err != nil {
		t.Fatalf("valid comment rejected: %v", err)
	}
	if got.Scalars != 11 {
		t.Errorf("scalars = %d, want 11", got.Scalars)
	}
	if _, err := ParseComment(strings.Repeat("x", 280)); err != nil {
		t.Errorf("280 scalars must pass: %v", err)
	}
	if _, err := ParseComment(strings.Repeat("x", 281)); err == nil {
		t.Error("281 scalars must refuse without truncation")
	}
	for _, raw := range []string{"", "   ", "\r\n\t "} {
		if _, err := ParseComment(raw); err == nil {
			t.Errorf("blank %q must refuse", raw)
		}
	}
	if got, err := ParseComment("a\r\nb"); err != nil || got.Text != "a\nb" || got.Scalars != 3 {
		t.Errorf("CRLF must normalize, got %+v err=%v", got, err)
	}
	invalid := string([]byte{0xff, 0xfe, '(', ')'})
	if !utf8.ValidString(invalid) {
		if _, err := ParseComment(invalid); err == nil {
			t.Error("invalid UTF-8 must refuse")
		}
	} else {
		t.Fatal("test vector must be invalid UTF-8")
	}
}

func TestCommentAstralBoundary(t *testing.T) {
	// Astral-plane emoji are 4 bytes / 2 UTF-16 units each but count
	// as one scalar: 280 emoji (1120 bytes) must pass, 281 must
	// refuse. This pins scalar counting against byte/UTF-16 length
	// across Go/Kotlin/Swift (P22-T04, B-BR-F03).
	pump := strings.Repeat("⛽", 280)
	got, err := ParseComment(pump)
	if err != nil {
		t.Fatalf("280 astral scalars must pass: %v", err)
	}
	if got.Scalars != 280 {
		t.Errorf("scalars = %d, want 280", got.Scalars)
	}
	if _, err := ParseComment(strings.Repeat("⛽", 281)); err == nil {
		t.Error("281 astral scalars must refuse without truncation")
	}
	// Lone CR folds like CRLF; mixed breaks plus emoji count once each.
	got, err = ParseComment("a\rb\r\n⛽")
	if err != nil || got.Text != "a\nb\n⛽" || got.Scalars != 5 {
		t.Errorf("CR/CRLF/emoji must normalize to 5 scalars, got %+v err=%v", got, err)
	}
}

func TestAgreementMath(t *testing.T) {
	got, err := ComputeAgreement(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasVotes || got.BasisPoints != 6666 || got.Valid != 2 || got.Invalid != 1 {
		t.Errorf("2/1 must be 6666bps with counts, got %+v", got)
	}
	for _, tc := range []struct {
		valid, invalid, want int64
	}{
		{0, 3, 0},
		{3, 0, 10000},
		{1, 1, 5000},
		{1, 3, 2500},
		{999999, 1, 9999},
	} {
		got, err := ComputeAgreement(tc.valid, tc.invalid)
		if err != nil {
			t.Fatal(err)
		}
		if !got.HasVotes || got.BasisPoints != tc.want {
			t.Errorf("%d/%d = %d, want %d", tc.valid, tc.invalid, got.BasisPoints, tc.want)
		}
	}
	empty, err := ComputeAgreement(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if empty.HasVotes {
		t.Errorf("zero votes must be null, got %+v", empty)
	}
	if _, err := ComputeAgreement(-1, 0); err == nil {
		t.Error("negative counts must refuse")
	}
}

func TestTargetValidation(t *testing.T) {
	ok := Target{StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26", Product: "GASOLINE_ADDITIVED"}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid target refused: %v", err)
	}
	for _, bad := range []Target{{}, {StationID: "x"}, {Product: "x"}} {
		if err := bad.Validate(); err == nil {
			t.Errorf("blank target %+v must refuse", bad)
		}
	}
}
