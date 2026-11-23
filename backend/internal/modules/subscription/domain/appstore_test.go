package domain

import (
	"testing"
	"time"
)

func TestAppStoreStateMapsEveryStatus(t *testing.T) {
	cases := []struct {
		status    int
		autoRenew bool
		revoked   bool
		want      State
	}{
		{AppStoreStatusActive, true, false, Active},
		{AppStoreStatusActive, false, false, Canceled},
		{AppStoreStatusExpired, true, false, Expired},
		{AppStoreStatusBillingRetry, true, false, Hold},
		{AppStoreStatusGrace, true, false, Grace},
		{AppStoreStatusRevoked, false, false, Revoked},
		{AppStoreStatusActive, true, true, Revoked},
		{0, true, false, Unknown},
		{6, true, false, Unknown},
	}
	for _, c := range cases {
		if got := AppStoreState(c.status, c.autoRenew, c.revoked); got != c.want {
			t.Fatalf("status %d autoRenew %t revoked %t: got %s want %s", c.status, c.autoRenew, c.revoked, got, c.want)
		}
	}
}

func TestAppStoreValidUntilUsesGraceOnlyInGrace(t *testing.T) {
	expires := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	grace := expires.Add(6 * 24 * time.Hour)
	if got := AppStoreValidUntil(Grace, expires, grace); !got.Equal(grace) {
		t.Fatalf("grace end not used: %s", got)
	}
	if got := AppStoreValidUntil(Hold, expires, grace); !got.Equal(expires) {
		t.Fatalf("hold must not extend access: %s", got)
	}
	if got := AppStoreValidUntil(Grace, expires, time.Time{}); !got.Equal(expires) {
		t.Fatalf("missing grace end must keep expiry: %s", got)
	}
}

func TestStoreTokenDigestSeparatesStoresAndKeepsPlayDigest(t *testing.T) {
	play := StoreTokenDigest(ProviderPlay, "production", "com.anpfuel.app", "token")
	if play != TokenDigest("production", "com.anpfuel.app", "token") {
		t.Fatal("Play digest changed")
	}
	apple := StoreTokenDigest(ProviderAppStore, "production", "com.anpfuel.app", "token")
	if apple == play || len(apple) != 64 {
		t.Fatalf("App Store digest must differ from Play: %s", apple)
	}
	if StoreTokenDigest("web", "production", "com.anpfuel.app", "token") != "" {
		t.Fatal("unknown provider must not produce a digest")
	}
}

func TestEvaluateLabelsTierWithIssuingStore(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	snapshot := &Snapshot{Provider: ProviderAppStore, State: Active, VerifiedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Hour), ProductID: "p", BasePlanID: "p"}
	status := Evaluate(snapshot, now, true)
	if len(status.Tiers) != 1 || status.Tiers[0].Source != ProviderAppStore {
		t.Fatalf("tiers %+v", status.Tiers)
	}
	snapshot.Provider = ""
	status = Evaluate(snapshot, now, true)
	if status.Tiers[0].Source != ProviderPlay {
		t.Fatalf("legacy rows are Play purchases: %+v", status.Tiers)
	}
}
