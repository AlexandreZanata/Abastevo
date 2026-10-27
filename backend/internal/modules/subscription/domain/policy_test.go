package domain

import (
	"testing"
	"time"
)

func TestVerifiedCapabilitiesBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	for _, state := range []State{Pending, Active, Canceled, Grace, Hold, Paused, Expired, Revoked, Unknown, State("future-state")} {
		t.Run(string(state), func(t *testing.T) {
			s := Snapshot{State: state, VerifiedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Hour)}
			got := Evaluate(&s, now, true)
			want := state == Active || state == Canceled || state == Grace
			if (len(got.Capabilities) == 7) != want {
				t.Fatalf("state %s rights %+v", state, got)
			}
			if !got.RetainedDataAccess {
				t.Fatal("retained-data exit must remain free")
			}
		})
	}
	base := Snapshot{State: Active, VerifiedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Hour)}
	for _, name := range []string{"expired", "stale", "future-proof", "inactive", "retired", "unverified"} {
		t.Run(name, func(t *testing.T) {
			s := base
			live := true
			switch name {
			case "expired":
				s.ValidUntil = now
			case "stale":
				s.VerifiedAt = now.Add(-ProofFreshness)
			case "future-proof":
				s.VerifiedAt = now.Add(time.Second)
			case "inactive":
				live = false
			case "retired":
				s.Retired = true
			case "unverified":
				s.VerifiedAt = time.Time{}
			}
			got := Evaluate(&s, now, live)
			if len(got.Capabilities) != 0 {
				t.Fatal("stale/expired/revoked proof granted rights")
			}
		})
	}
	got := Evaluate(nil, now, true)
	if got.State != None || len(got.Capabilities) != 0 || !got.RetainedDataAccess {
		t.Fatal(got)
	}
}
func TestTokenAndOperationBounds(t *testing.T) {
	if !ValidID("11111111-1111-4111-8111-111111111111") {
		t.Fatal("valid operation refused")
	}
	for _, v := range []string{"", "secret token", "token" + string(rune(10)), "☃"} {
		if ValidToken(v) {
			t.Fatal("invalid token accepted")
		}
	}
	if ValidID("anything") {
		t.Fatal("invalid id")
	}
	if TokenDigest("production", "com.anpfuel", "opaque") == TokenDigest("test", "com.anpfuel", "opaque") {
		t.Fatal("environment collision")
	}
}

func TestCapabilityResultsDoNotShareMutableSlice(t *testing.T) {
	now := time.Now().UTC()
	s := Snapshot{State: Active, VerifiedAt: now, ValidUntil: now.Add(time.Hour)}
	first := Evaluate(&s, now, true)
	first.Capabilities[0] = "invented.trust"
	second := Evaluate(&s, now, true)
	if second.Capabilities[0] != "cloud.sync" {
		t.Fatal("caller mutated global policy")
	}
}

func TestMultipleVehicleCapabilityUsesLiveVerifiedPremium(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	for _, state := range []State{Active, Canceled, Grace, Pending, Hold, Paused, Expired, Revoked, Unknown} {
		snapshot := &Snapshot{State: state, VerifiedAt: now, ValidUntil: now.Add(time.Hour)}
		found := false
		for _, c := range Evaluate(snapshot, now, true).Capabilities {
			found = found || c == "vehicles.multiple"
		}
		if want := state == Active || state == Canceled || state == Grace; found != want {
			t.Fatalf("%s multiple=%v want=%v", state, found, want)
		}
	}
}

// TestAdminGrant_ENT08_UnionOfSources proves web and admin capabilities merge
// without losing either source or its tier metadata.
func TestAdminGrant_ENT08_UnionOfSources(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	webUntil := now.Add(time.Hour)
	adminUntil := now.Add(2 * time.Hour)
	s := &Snapshot{
		WebCapabilities:   []string{"cloud.sync"},
		WebValidUntil:     webUntil,
		AdminCapabilities: []string{"professional.write"},
		AdminValidUntil:   adminUntil,
		Tiers:             []TierStatus{{Tier: "PRO", Source: "admin", ValidUntil: &adminUntil}},
	}
	status := Evaluate(s, now, true)
	if status.State != Active || status.Reason != "verified-web" {
		t.Fatalf("status=%+v", status)
	}
	for _, capability := range []string{"cloud.sync", "professional.write"} {
		found := false
		for _, got := range status.Capabilities {
			found = found || got == capability
		}
		if !found {
			t.Fatalf("missing %s in %v", capability, status.Capabilities)
		}
	}
	if len(status.Tiers) != 2 {
		t.Fatalf("tiers=%+v", status.Tiers)
	}
}

// TestAdminGrant_ENT09_OpenEndedGrantHasNoValidUntil: an open-ended admin
// grant keeps access until revoked, so the merged valid_until is null rather
// than a past paid expiry or a shorter bounded grant.
func TestAdminGrant_ENT09_OpenEndedGrantHasNoValidUntil(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	month := now.Add(30 * 24 * time.Hour)
	for name, s := range map[string]*Snapshot{
		"expired paid plus open-ended admin": {
			State: Expired, ValidUntil: now.Add(-24 * time.Hour), VerifiedAt: now,
			AdminCapabilities: []string{"cloud.sync"},
			Tiers:             []TierStatus{{Tier: "PRO", Source: "admin"}},
		},
		"open-ended plus bounded admin": {
			AdminCapabilities: []string{"cloud.sync"},
			AdminValidUntil:   month,
			Tiers: []TierStatus{
				{Tier: "PRO", Source: "admin"},
				{Tier: "ENTERPRISE", Source: "admin", ValidUntil: &month},
			},
		},
	} {
		status := Evaluate(s, now, true)
		if status.State != Active || status.ValidUntil != nil {
			t.Fatalf("%s: state=%s valid_until=%v", name, status.State, status.ValidUntil)
		}
	}
	bounded := Evaluate(&Snapshot{
		AdminCapabilities: []string{"cloud.sync"},
		AdminValidUntil:   month,
		Tiers:             []TierStatus{{Tier: "PRO", Source: "admin", ValidUntil: &month}},
	}, now, true)
	if bounded.ValidUntil == nil || !bounded.ValidUntil.Equal(month) {
		t.Fatalf("bounded valid_until=%v", bounded.ValidUntil)
	}
}
