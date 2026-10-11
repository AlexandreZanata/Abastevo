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

// CS10A contract freeze: golden promotion vectors pin the wire shapes
// (future-only per-image scope, frozen verdicts, active receipt).
// Eligible vectors must construct through the domain; pre-grant and
// uncertain vectors must refuse. No pixels, GPS, keys or labels travel
// in these shapes.

type promotionVector struct {
	ID    string `json:"id"`
	Valid bool   `json:"valid"`
	Value struct {
		ImageID            string `json:"image_id"`
		CapturedAfterGrant bool   `json:"captured_after_grant"`
		PerImageAllowed    bool   `json:"per_image_allowed"`
		SourceSHA256       string `json:"source_sha256"`
		Verdict            string `json:"verdict"`
		State              string `json:"state"`
		ConsentRevision    uint64 `json:"consent_revision"`
		TermsVersion       string `json:"terms_version"`
	} `json:"value"`
}

func TestPromotionGoldenVectors(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "contracts", "testdata", "privacy-consent")
	matches, err := filepath.Glob(filepath.Join(dir, "promotion-*.json"))
	if err != nil || len(matches) != 4 {
		t.Fatalf("expected 4 promotion vectors, got %d (%v)", len(matches), err)
	}
	grantedAt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	consent, _, err := domain.GrantConsent(nil, domain.ConsentParams{
		ContributorID: "c1111111-0000-4000-8000-000000000001", ContributorRef: "tok-owner",
		TermsVersion: domain.TrainingTermsV1, TermsDigest: "digest-v1",
		Revision: 1, DecidedAt: grantedAt,
	}, domain.TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(path), err)
		}
		var v promotionVector
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("decode %s: %v", filepath.Base(path), err)
		}
		captured := grantedAt.Add(time.Hour)
		if !v.Value.CapturedAfterGrant {
			captured = grantedAt.Add(-time.Hour)
		}
		d, receipt, err := domain.ProposeDerivative(domain.PromotionCandidate{
			ImageID: v.Value.ImageID, ContributorID: consent.ContributorID,
			ContributorRef: consent.ContributorRef, CapturedAt: captured,
			PerImageAllowed: v.Value.PerImageAllowed, SourceSHA256: v.Value.SourceSHA256,
			QueuedAtConsentRevision: consent.Revision,
		}, &consent, domain.SanitizeVerdict(v.Value.Verdict), "derivative-hash",
			grantedAt.Add(2*time.Hour), domain.TrainingTermsV1, "digest-v1")
		if v.Valid && err != nil {
			t.Fatalf("%s must construct: %v", v.ID, err)
		}
		if !v.Valid && err == nil {
			t.Fatalf("%s must refuse", v.ID)
		}
		if !v.Valid {
			continue
		}
		if d.State != domain.DerivativeActive {
			t.Fatalf("%s state = %q, want ACTIVE", v.ID, d.State)
		}
		if v.ID == "promotion-derivative-receipt" &&
			(receipt.State != v.Value.State || receipt.ConsentRevision != v.Value.ConsentRevision ||
				d.TermsVersion != v.Value.TermsVersion) {
			t.Fatalf("%s receipt mismatch: %+v", v.ID, receipt)
		}
	}
}
