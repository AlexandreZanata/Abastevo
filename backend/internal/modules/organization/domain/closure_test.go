package domain_test

import (
	"errors"
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestClosureWindowMatchesCompanyConfirmationAndServerCutoff(t *testing.T) {
	now := time.Date(2026, 1, 5, 16, 0, 0, 0, time.UTC)
	for _, days := range []int{0, 7, 30, 90} {
		end, err := d.ClosureDeadline("Frota Exemplo", "Frota Exemplo", now, days)
		want := days
		if days == 0 {
			want = 30
		}
		if err != nil || end.Sub(now) != time.Duration(want)*24*time.Hour || d.ClosureDue(end.Add(-time.Microsecond), end) || !d.ClosureDue(end, end) {
			t.Fatalf("closure window %d: %v %v", days, end, err)
		}
	}
	for _, days := range []int{-1, 6, 91} {
		if _, err := d.ClosureDeadline("Frota Exemplo", "Frota Exemplo", now, days); !errors.Is(err, d.ErrInvalid) {
			t.Errorf("invalid closure window %d was admitted: %v", days, err)
		}
	}
	if _, err := d.ClosureDeadline("Frota Exemplo", "frota exemplo", now, 7); !errors.Is(err, d.ErrInvalid) {
		t.Fatal("a different name confirmed closure")
	}
}
