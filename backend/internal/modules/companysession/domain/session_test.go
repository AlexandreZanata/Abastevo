package domain_test

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/domain"
	"strings"
	"testing"
	"time"
)

func TestCompanySessionBoundariesAndSecretNamespaces(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	for _, elapsed := range []time.Duration{0, domain.IdleTimeout - time.Nanosecond} {
		if !domain.Live(now, now, now.Add(domain.AbsoluteLifetime), now.Add(elapsed)) {
			t.Fatal("live session refused")
		}
	}
	for _, when := range []time.Time{now.Add(-time.Second), now.Add(domain.IdleTimeout), now.Add(domain.AbsoluteLifetime)} {
		if domain.Live(now, now, now.Add(domain.AbsoluteLifetime), when) {
			t.Fatal("expired/rollback session accepted")
		}
	}
	if domain.Live(now, now.Add(time.Minute), now.Add(domain.AbsoluteLifetime), now) {
		t.Fatal("future last-seen accepted")
	}
	if !domain.Fresh(now, now.Add(domain.FreshWindow-time.Nanosecond)) || domain.Fresh(now, now.Add(domain.FreshWindow)) || domain.Fresh(now, now.Add(-time.Second)) {
		t.Fatal("freshness boundary")
	}
	for _, pair := range []struct{ value, prefix string }{{domain.TokenPrefix + strings.Repeat("a", 64), domain.TokenPrefix}, {domain.CSRFPrefix + strings.Repeat("a", 64), domain.CSRFPrefix}} {
		if !domain.ValidSecret(pair.value, pair.prefix) {
			t.Fatal("valid generated secret refused")
		}
		if domain.ValidSecret(pair.value, domain.TokenPrefix) == domain.ValidSecret(pair.value, domain.CSRFPrefix) {
			t.Fatal("secret domains overlap")
		}
	}
	for _, bad := range []string{"", strings.Repeat("a", 64), domain.TokenPrefix + strings.Repeat("A", 64), domain.TokenPrefix + strings.Repeat("a", 63), "abv_admin_key"} {
		if domain.ValidSecret(bad, domain.TokenPrefix) {
			t.Fatal("invalid cookie accepted")
		}
	}
	if !domain.ValidID("7b0e2a54-0000-4000-8000-000000000001") || domain.ValidID("00000000-0000-0000-0000-000000000000") {
		t.Fatal("identity validation")
	}
}
