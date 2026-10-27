package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	subdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func contractsPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	base := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "contracts")
	return filepath.Join(append([]string{base}, parts...)...)
}

type golden struct {
	Version       string              `json:"version"`
	PolicyVersion int                 `json:"policy_version"`
	Tiers         map[string][]string `json:"tiers"`
}

// TestCatalog_ENT05_MatchesGoldenAndPremium proves the code catalog equals the
// golden JSON and the PRO bundle equals the paid Premium bundle.
func TestCatalog_ENT05_MatchesGoldenAndPremium(t *testing.T) {
	raw, err := os.ReadFile(contractsPath(t, "admin", "tier-catalog-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want golden
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	if want.Version != CatalogVersion || want.PolicyVersion != PolicyVersion {
		t.Fatalf("golden metadata=%+v", want)
	}
	for _, tier := range []Tier{TierPRO, TierContributor, TierEnterprise} {
		got, ok := Capabilities(tier, PolicyVersion)
		if !ok || !slices.Equal(got, want.Tiers[string(tier)]) {
			t.Fatalf("tier %s got=%v want=%v", tier, got, want.Tiers[string(tier)])
		}
	}
	pro, _ := Capabilities(TierPRO, PolicyVersion)
	if !slices.Equal(pro, subdomain.PremiumCapabilities()) {
		t.Fatalf("PRO=%v Premium=%v", pro, subdomain.PremiumCapabilities())
	}
}

// TestCatalog_ENT05_DefensiveCopy keeps callers from mutating the frozen
// catalog.
func TestCatalog_ENT05_DefensiveCopy(t *testing.T) {
	first, _ := Capabilities(TierContributor, PolicyVersion)
	first[0] = "invented.capability"
	second, _ := Capabilities(TierContributor, PolicyVersion)
	if second[0] == "invented.capability" {
		t.Fatal("catalog slice shared with caller")
	}
	if _, ok := Capabilities(TierPRO, PolicyVersion+1); ok {
		t.Fatal("unknown policy version accepted")
	}
	if _, ok := Capabilities(Tier("free"), PolicyVersion); ok {
		t.Fatal("unknown tier accepted")
	}
}

// TestEndAt_ENT04 covers the 1 hour .. 5 years bounds.
func TestEndAt_ENT04(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if !ValidEndAt(now, nil) {
		t.Fatal("open-ended grant refused")
	}
	minimum, maximum := now.Add(MinGrantDuration), now.Add(MaxGrantDuration)
	if !ValidEndAt(now, &minimum) || !ValidEndAt(now, &maximum) {
		t.Fatal("boundary end_at refused")
	}
	below, above := minimum.Add(-time.Nanosecond), maximum.Add(time.Nanosecond)
	if ValidEndAt(now, &below) || ValidEndAt(now, &above) {
		t.Fatal("out-of-bounds end_at accepted")
	}
}

// TestGrant_ENT06_ENT07_ActiveWindow covers read-time expiry and revocation.
func TestGrant_ENT06_ENT07_ActiveWindow(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	end := now.Add(time.Hour)
	active := Grant{State: StateActive, StartAt: now, EndAt: &end}
	if !active.ActiveAt(now) || !active.ActiveAt(end.Add(-time.Nanosecond)) {
		t.Fatal("active window refused")
	}
	if active.ActiveAt(end) {
		t.Fatal("end boundary treated as active")
	}
	open := Grant{State: StateActive, StartAt: now}
	if !open.ActiveAt(now.Add(5 * 365 * 24 * time.Hour)) {
		t.Fatal("open-ended grant expired")
	}
	revoked := active
	revoked.State = StateRevoked
	if revoked.ActiveAt(now) {
		t.Fatal("revoked grant active")
	}
}
