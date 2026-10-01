package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type locationFixture struct {
	ID            string `json:"id"`
	PolicyVersion string `json:"policy_version"`
	Bounds        struct {
		MaxFixAgeSeconds    int64   `json:"max_fix_age_seconds"`
		MaxClockSkewSeconds int64   `json:"max_clock_skew_seconds"`
		MaxClaimAccuracyM   float64 `json:"max_claim_accuracy_m"`
	} `json:"bounds"`
	Cases []struct {
		ID    string `json:"id"`
		Input struct {
			PermissionGranted bool     `json:"permission_granted"`
			HasFix            bool     `json:"has_fix"`
			SourceInfoPresent bool     `json:"source_info_present"`
			Simulated         bool     `json:"simulated"`
			AccuracyM         *float64 `json:"accuracy_m"`
			FixAgeSeconds     *int64   `json:"fix_age_seconds"`
			ClockSkewSeconds  *int64   `json:"clock_skew_seconds"`
			Manual            bool     `json:"manual"`
		} `json:"input"`
		Verdict     string `json:"verdict"`
		Reason      string `json:"reason"`
		Freshness   string `json:"freshness"`
		Accuracy    string `json:"accuracy"`
		AllowsClaim bool   `json:"allows_claim"`
	} `json:"cases"`
}

// TestLocationFixtureReplay replays every golden vector against the
// frozen classifier so fixtures and implementation cannot drift
// (P14/P15 pattern). RED proven by loosening a precedence rule:
// trusting a client simulated=false without OS source info flips
// forged-client-flag-ignored and source-missing-beats-simulated to
// VERIFIED; GREEN on restore.
func TestLocationFixtureReplay(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..",
		"contracts", "testdata", "location", "risk-v1.json"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var fx locationFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("fixture JSON: %v", err)
	}
	if fx.ID != "location-risk-v1" || fx.PolicyVersion != LocationV1 {
		t.Fatalf("fixture header wrong: id=%q policy=%q", fx.ID, fx.PolicyVersion)
	}
	if fx.Bounds.MaxFixAgeSeconds != MaxFixAgeSeconds ||
		fx.Bounds.MaxClockSkewSeconds != MaxClockSkewSeconds ||
		fx.Bounds.MaxClaimAccuracyM != MaxClaimAccuracyM {
		t.Fatalf("fixture bounds drifted from code: %+v", fx.Bounds)
	}
	if len(fx.Cases) == 0 {
		t.Fatal("fixture must carry vectors")
	}
	for _, c := range fx.Cases {
		risk := ClassifyFix(FixInput{
			PermissionGranted: c.Input.PermissionGranted,
			HasFix:            c.Input.HasFix,
			SourceInfoPresent: c.Input.SourceInfoPresent,
			Simulated:         c.Input.Simulated,
			AccuracyMeters:    c.Input.AccuracyM,
			FixAgeSeconds:     c.Input.FixAgeSeconds,
			ClockSkewSeconds:  c.Input.ClockSkewSeconds,
			Manual:            c.Input.Manual,
		})
		if risk.Verdict != c.Verdict || risk.Reason != c.Reason {
			t.Errorf("%s: verdict/reason = %q/%q, want %q/%q",
				c.ID, risk.Verdict, risk.Reason, c.Verdict, c.Reason)
		}
		if risk.Freshness != c.Freshness {
			t.Errorf("%s: freshness = %q, want %q", c.ID, risk.Freshness, c.Freshness)
		}
		if risk.Accuracy != c.Accuracy {
			t.Errorf("%s: accuracy = %q, want %q", c.ID, risk.Accuracy, c.Accuracy)
		}
		if risk.AllowsClaim != c.AllowsClaim {
			t.Errorf("%s: allows_claim = %v, want %v", c.ID, risk.AllowsClaim, c.AllowsClaim)
		}
		if risk.PolicyVersion != LocationV1 {
			t.Errorf("%s: policy = %q", c.ID, risk.PolicyVersion)
		}
	}
}

// TestClaimGateBlocksUnknownProximity proves the acceptance rule:
// only a VERIFIED fix yields a position claim; an UNKNOWN result can
// never become verified proximity, whatever accuracy/distance the
// caller passes alongside it.
func TestClaimGateBlocksUnknownProximity(t *testing.T) {
	age := int64(10)
	acc := 20.0
	verified := ClassifyFix(FixInput{
		PermissionGranted: true, HasFix: true, SourceInfoPresent: true,
		AccuracyMeters: &acc, FixAgeSeconds: &age,
	})
	if !verified.AllowsClaim {
		t.Fatalf("fresh accurate fix must verify, got %+v", verified)
	}
	if claim := ClaimForFix(verified, 20, 10); claim == nil {
		t.Fatal("verified fix must carry a claim")
	}
	for _, tc := range []struct {
		name string
		in   FixInput
	}{
		{"no-fix", FixInput{PermissionGranted: true}},
		{"source-missing", FixInput{PermissionGranted: true, HasFix: true,
			AccuracyMeters: &acc, FixAgeSeconds: &age}},
		{"simulated", FixInput{PermissionGranted: true, HasFix: true,
			SourceInfoPresent: true, Simulated: true,
			AccuracyMeters: &acc, FixAgeSeconds: &age}},
		{"manual", FixInput{PermissionGranted: true, HasFix: true,
			SourceInfoPresent: true, AccuracyMeters: &acc,
			FixAgeSeconds: &age, Manual: true}},
	} {
		risk := ClassifyFix(tc.in)
		if risk.AllowsClaim {
			t.Errorf("%s: must not allow a claim, got %+v", tc.name, risk)
		}
		if claim := ClaimForFix(risk, 20, 10); claim != nil {
			t.Errorf("%s: non-verified fix must yield nil claim", tc.name)
		}
	}
}

// TestFixBandsNeverCarryCoordinates guards the persisted vocabulary:
// the risk struct has no coordinate fields, so bands and codes are the
// only values that can ever leave this layer.
func TestFixBandsNeverCarryCoordinates(t *testing.T) {
	age := int64(5)
	acc := 10.0
	risk := ClassifyFix(FixInput{
		PermissionGranted: true, HasFix: true, SourceInfoPresent: true,
		AccuracyMeters: &acc, FixAgeSeconds: &age,
	})
	raw, err := json.Marshal(risk)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"lat", "lon", "latitude", "longitude", "fix_x", "fix_y"} {
		if containsFold(string(raw), banned) {
			t.Errorf("risk payload must never carry coordinates (%q): %s", banned, raw)
		}
	}
}

func containsFold(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			h := haystack[i+j]
			n := needle[j]
			if 'A' <= h && h <= 'Z' {
				h += 'a' - 'A'
			}
			if 'A' <= n && n <= 'Z' {
				n += 'a' - 'A'
			}
			if h != n {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
