package mail

import (
	"context"
	"testing"
)

// SEC01-B: the memory outbox is a bounded deterministic test
// adapter — retained issuances never grow without a ceiling, and
// production wiring uses it only behind explicit local delivery
// configuration (never silently, never SMTP).

func TestOutboxCapsRetainedIssuances(t *testing.T) {
	o := NewOutbox(testClock{}, 3)
	ctx := context.Background()
	for _, tc := range []struct{ address, code string }{
		{"a@example.invalid", "111111"},
		{"b@example.invalid", "222222"},
		{"c@example.invalid", "333333"},
		{"d@example.invalid", "444444"},
	} {
		if err := o.SendCode(ctx, tc.address, tc.code); err != nil {
			t.Fatal(err)
		}
	}
	got := o.All()
	if len(got) != 3 {
		t.Fatalf("retained = %d, want the 3 most recent", len(got))
	}
	if got[0].Code != "222222" || got[2].Code != "444444" {
		t.Errorf("ring kept %+v, want newest three in order", got)
	}
}

func TestOutboxClampsNonPositiveCap(t *testing.T) {
	for _, max := range []int{0, -5} {
		o := NewOutbox(testClock{}, max)
		if o.maxRetained != DefaultMaxRetained {
			t.Errorf("max %d clamped to %d, want %d", max, o.maxRetained, DefaultMaxRetained)
		}
	}
}

type testClock struct{}

func (testClock) NowUnix() int64 { return 1_700_000_000 }
