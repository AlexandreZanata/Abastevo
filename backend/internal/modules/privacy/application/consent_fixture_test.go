package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// CS09A contract freeze: golden consent vectors pin the wire shapes
// (frozen purpose + current terms + revision-ordered GRANTED/REVOKED,
// default-OFF status). Valid vectors must construct through the
// domain; stale-terms vectors must refuse. No pixels, GPS, keys or
// image bytes travel in these shapes.

type consentVector struct {
	ID    string `json:"id"`
	Valid bool   `json:"valid"`
	Value struct {
		Purpose      string `json:"purpose"`
		TermsVersion string `json:"terms_version"`
		TermsDigest  string `json:"terms_digest"`
		Revision     uint64 `json:"revision"`
		State        string `json:"state"`
		Granted      bool   `json:"granted"`
		NeedsRegrant bool   `json:"needs_regrant"`
	} `json:"value"`
}

func TestConsentGoldenVectors(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "contracts", "testdata", "privacy-consent")
	matches, err := filepath.Glob(filepath.Join(dir, "consent-*.json"))
	if err != nil || len(matches) != 4 {
		t.Fatalf("expected 4 consent vectors, got %d (%v)", len(matches), err)
	}
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(path), err)
		}
		var v consentVector
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("decode %s: %v", filepath.Base(path), err)
		}
		switch v.ID {
		case "consent-grant-valid", "consent-grant-stale-terms":
			c, _, err := domain.GrantConsent(nil, domain.ConsentParams{
				ContributorID: "c1111111-0000-4000-8000-000000000001", ContributorRef: "tok-owner",
				TermsVersion: v.Value.TermsVersion, TermsDigest: v.Value.TermsDigest,
				Revision: v.Value.Revision, DecidedAt: now,
			}, domain.TrainingTermsV1, "digest-v1")
			if v.Valid && err != nil {
				t.Fatalf("%s must construct: %v", v.ID, err)
			}
			if !v.Valid && err == nil {
				t.Fatalf("%s must refuse", v.ID)
			}
			if v.Valid && (c.Purpose != v.Value.Purpose || c.State != v.Value.State) {
				t.Fatalf("%s shape mismatch: %+v", v.ID, c)
			}
		case "consent-status-granted", "consent-status-off":
			if v.Value.Granted != (v.ID == "consent-status-granted") {
				t.Fatalf("%s granted flag mismatch", v.ID)
			}
			if v.Value.NeedsRegrant {
				t.Fatalf("%s must not need re-grant", v.ID)
			}
			if v.ID == "consent-status-off" && (v.Value.State != "" || v.Value.Revision != 0) {
				t.Fatalf("%s off shape must stay empty: %+v", v.ID, v.Value)
			}
		default:
			t.Fatalf("unknown vector %s", v.ID)
		}
	}
}
