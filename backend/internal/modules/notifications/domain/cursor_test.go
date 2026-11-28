package domain

import (
	"errors"
	"testing"
	"time"
)

func TestPositionRoundTripKeepsMicroseconds(t *testing.T) {
	p := Position{CreatedAt: time.Date(2026, 11, 28, 9, 0, 0, 123456000, time.UTC), ID: comment}
	key := EncodePosition(p)
	if key != "n1|1795856400123456|"+comment {
		t.Fatalf("key %q", key)
	}
	got, err := DecodePosition(key)
	if err != nil || !got.CreatedAt.Equal(p.CreatedAt) || got.ID != p.ID {
		t.Fatalf("round trip %+v %v", got, err)
	}
}

func TestPositionRejectsForeignKeys(t *testing.T) {
	for _, key := range []string{
		"",
		"1795856400123456|" + comment,
		"n2|1795856400123456|" + comment,
		"n1||" + comment,
		"n1|0|" + comment,
		"n1|-5|" + comment,
		"n1|01795856400123456|" + comment,
		"n1|abc|" + comment,
		"n1|1795856400123456|not-a-uuid",
		"n1|1795856400123456",
		"n1|99999999999999999999|" + comment,
		"n1|1795856400123456|" + comment + "|x",
	} {
		if _, err := DecodePosition(key); !errors.Is(err, ErrInvalid) {
			t.Errorf("admitted %q", key)
		}
	}
}
