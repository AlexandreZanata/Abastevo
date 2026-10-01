package application

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type intakeFixture struct {
	ID            string `json:"id"`
	PolicyVersion string `json:"policy_version"`
	Cases         []struct {
		ID      string `json:"id"`
		Claimed struct {
			Verdict           string   `json:"verdict"`
			PermissionGranted bool     `json:"permission_granted"`
			HasFix            bool     `json:"has_fix"`
			SourceInfoPresent bool     `json:"source_info_present"`
			Simulated         bool     `json:"simulated"`
			AccuracyM         *float64 `json:"accuracy_m"`
			CapturedAt        *string  `json:"captured_at"`
			Manual            bool     `json:"manual"`
		} `json:"claimed"`
		ReceiptAt   string `json:"receipt_at"`
		Verdict     string `json:"verdict"`
		AllowsClaim bool   `json:"allows_claim"`
		Refusal     string `json:"refusal"`
	} `json:"cases"`
	Teleport []struct {
		ID             string  `json:"id"`
		DistanceM      float64 `json:"distance_m"`
		ElapsedSeconds int64   `json:"elapsed_seconds"`
		Teleport       bool    `json:"teleport"`
	} `json:"teleport"`
}

// TestIntakeFixtureReplay replays every server-verification vector:
// claimed metadata must reproduce its verdict on server time, and
// every forged/stale/replayed/skewed/timeless VERIFIED claim refuses
// with the fixed code. RED proven by trusting client age instead of
// server math: stale-at-receipt and replayed-old-fix verify.
func TestIntakeFixtureReplay(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..",
		"contracts", "testdata", "location", "intake-v1.json"))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	var fx intakeFixture
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatalf("fixture JSON: %v", err)
	}
	if fx.ID != "location-intake-v1" || fx.PolicyVersion != LocationV1 {
		t.Fatalf("fixture header wrong: id=%q policy=%q", fx.ID, fx.PolicyVersion)
	}
	if len(fx.Cases) == 0 || len(fx.Teleport) == 0 {
		t.Fatal("fixture must carry claim and teleport vectors")
	}
	for _, c := range fx.Cases {
		var captured *time.Time
		if c.Claimed.CapturedAt != nil {
			ts, err := time.Parse(time.RFC3339, *c.Claimed.CapturedAt)
			if err != nil {
				t.Fatalf("%s captured_at: %v", c.ID, err)
			}
			captured = &ts
		}
		receipt, err := time.Parse(time.RFC3339, c.ReceiptAt)
		if err != nil {
			t.Fatalf("%s receipt_at: %v", c.ID, err)
		}
		got, err := VerifyDeviceFix(DeviceFix{
			ClaimedVerdict:    c.Claimed.Verdict,
			PermissionGranted: c.Claimed.PermissionGranted,
			HasFix:            c.Claimed.HasFix,
			SourceInfoPresent: c.Claimed.SourceInfoPresent,
			Simulated:         c.Claimed.Simulated,
			AccuracyMeters:    c.Claimed.AccuracyM,
			Manual:            c.Claimed.Manual,
			CapturedAt:        captured,
		}, receipt)
		if c.Refusal != "" {
			var rej *FixRejection
			if !errors.As(err, &rej) || rej.Reason != c.Refusal {
				t.Errorf("%s: must refuse %q, got %+v (%v)",
					c.ID, c.Refusal, got, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: honest claim refused: %v", c.ID, err)
			continue
		}
		if got.Risk.Verdict != c.Verdict || got.AllowsClaim != c.AllowsClaim {
			t.Errorf("%s: verdict/claim = %q/%v, want %q/%v",
				c.ID, got.Risk.Verdict, got.AllowsClaim, c.Verdict, c.AllowsClaim)
		}
	}
	for _, tc := range fx.Teleport {
		if got := TeleportRisk(tc.DistanceM, tc.ElapsedSeconds); got != tc.Teleport {
			t.Errorf("teleport %s: got %v, want %v", tc.ID, got, tc.Teleport)
		}
	}
}

// TestVerifyRequiresReceiptTime proves programmer-error handling:
// zero receipt never verifies, even for perfect metadata.
func TestVerifyRequiresReceiptTime(t *testing.T) {
	acc := 25.0
	now := time.Now()
	if _, err := VerifyDeviceFix(DeviceFix{
		ClaimedVerdict: FixVerified, PermissionGranted: true, HasFix: true,
		SourceInfoPresent: true, AccuracyMeters: &acc, CapturedAt: &now,
	}, time.Time{}); err == nil {
		t.Error("zero receipt accepted")
	}
}

// TestRefusalsNeverCarryCoordinates proves log safety: every refusal
// and every verified output serializes to fixed vocabulary plus
// bands, and no decimal coordinate tail from the transient fix can
// appear in any error string the worker logs.
func TestRefusalsNeverCarryCoordinates(t *testing.T) {
	lat, lon := -23.5505199, -46.6333094
	acc := 25.0
	captured := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	receipt := captured.Add(30 * time.Second)
	inputs := []DeviceFix{
		{ClaimedVerdict: "TRUSTED", PermissionGranted: true, HasFix: true,
			SourceInfoPresent: true, AccuracyMeters: &acc, CapturedAt: &captured,
			Latitude: &lat, Longitude: &lon},
		{ClaimedVerdict: FixVerified, PermissionGranted: true, HasFix: true,
			SourceInfoPresent: false, AccuracyMeters: &acc, CapturedAt: &captured,
			Latitude: &lat, Longitude: &lon},
		{ClaimedVerdict: FixVerified, PermissionGranted: true, HasFix: true,
			SourceInfoPresent: true, AccuracyMeters: &acc,
			CapturedAt: func() *time.Time { old := captured.Add(-time.Hour); return &old }(),
			Latitude:   &lat, Longitude: &lon},
	}
	for i, in := range inputs {
		_, err := VerifyDeviceFix(in, receipt)
		if err == nil {
			t.Fatalf("case %d accepted", i)
		}
		for _, leak := range []string{"23.5505", "46.6333", "-23.", "-46.", "lat", "lon"} {
			if strings.Contains(strings.ToLower(err.Error()), leak) {
				t.Errorf("case %d leaks coordinates into %q", i, err)
			}
		}
		var rej *FixRejection
		if !errors.As(err, &rej) {
			t.Errorf("case %d must refuse with a fixed code, got %T", i, err)
		}
	}
	// Verified outputs also serialize coordinate-free.
	verified, err := VerifyDeviceFix(DeviceFix{
		ClaimedVerdict: FixVerified, PermissionGranted: true, HasFix: true,
		SourceInfoPresent: true, AccuracyMeters: &acc, CapturedAt: &captured,
		Latitude: &lat, Longitude: &lon,
	}, receipt)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(verified)
	for _, banned := range []string{"latitude", "longitude", "23.5505", "46.6333"} {
		if strings.Contains(strings.ToLower(string(out)), banned) {
			t.Errorf("verified output leaks coordinates: %s", out)
		}
	}
}
