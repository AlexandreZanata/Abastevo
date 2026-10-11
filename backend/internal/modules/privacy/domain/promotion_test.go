package domain

import (
	"reflect"
	"testing"
	"time"
)

func promotionConsent(at time.Time) Consent {
	c, _, err := GrantConsent(nil, ConsentParams{
		ContributorID: "c1111111-0000-4000-8000-000000000001", ContributorRef: "tok-owner",
		TermsVersion: TrainingTermsV1, TermsDigest: "digest-v1",
		Revision: 1, DecidedAt: at,
	}, TrainingTermsV1, "digest-v1")
	if err != nil {
		panic(err)
	}
	return c
}

func promotionCandidate(c Consent, captured time.Time) PromotionCandidate {
	return PromotionCandidate{
		ImageID: "img-1", ContributorID: c.ContributorID, ContributorRef: c.ContributorRef,
		CapturedAt: captured, PerImageAllowed: true, SourceSHA256: "source-hash",
		QueuedAtConsentRevision: c.Revision,
	}
}

func TestProposeDerivativeHappyPath(t *testing.T) {
	grantedAt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	c := promotionConsent(grantedAt)
	d, receipt, err := ProposeDerivative(promotionCandidate(c, grantedAt.Add(time.Hour)), &c,
		VerdictPass, "derivative-hash", grantedAt.Add(2*time.Hour), TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatalf("propose = %v", err)
	}
	if d.State != DerivativeActive || d.ConsentRevision != 1 || d.TermsVersion != TrainingTermsV1 {
		t.Errorf("derivative = %+v", d)
	}
	if receipt.Replayed {
		t.Errorf("first proposal replayed: %+v", receipt)
	}
}

func TestProposeRefusesWithoutValidConsent(t *testing.T) {
	grantedAt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	c := promotionConsent(grantedAt)
	// Absent consent denies.
	if _, _, err := ProposeDerivative(promotionCandidate(c, grantedAt.Add(time.Hour)), nil,
		VerdictPass, "h", grantedAt.Add(2*time.Hour), TrainingTermsV1, "digest-v1"); err != ErrConsentRequired {
		t.Errorf("absent consent = %v", err)
	}
	// Revoked consent denies, even for images captured while granted.
	revoked, _, err := RevokeConsent(&c, c.ContributorID, c.ContributorRef, 2, grantedAt.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := ProposeDerivative(promotionCandidate(c, grantedAt.Add(time.Hour)), &revoked,
		VerdictPass, "h", grantedAt.Add(4*time.Hour), TrainingTermsV1, "digest-v1"); err != ErrConsentRequired {
		t.Errorf("revoked consent = %v", err)
	}
	// Superseded terms deny until a fresh grant lands.
	if _, _, err := ProposeDerivative(promotionCandidate(c, grantedAt.Add(time.Hour)), &c,
		VerdictPass, "h", grantedAt.Add(2*time.Hour), "training-terms-v2", "digest-v2"); err != ErrTermsOutdated {
		t.Errorf("rotated terms = %v", err)
	}
}

func TestProposeEnforcesFutureOnlyAndPerImageScope(t *testing.T) {
	grantedAt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	c := promotionConsent(grantedAt)
	pre := promotionCandidate(c, grantedAt.Add(-time.Hour))
	if _, _, err := ProposeDerivative(pre, &c, VerdictPass, "h", grantedAt.Add(2*time.Hour),
		TrainingTermsV1, "digest-v1"); err != ErrImageNotCovered {
		t.Errorf("pre-grant capture = %v, want not covered", err)
	}
	optOut := promotionCandidate(c, grantedAt.Add(time.Hour))
	optOut.PerImageAllowed = false
	if _, _, err := ProposeDerivative(optOut, &c, VerdictPass, "h", grantedAt.Add(2*time.Hour),
		TrainingTermsV1, "digest-v1"); err != ErrImageNotCovered {
		t.Errorf("opt-out capture = %v, want not covered", err)
	}
}

func TestProposeFailsClosedOnVerdict(t *testing.T) {
	grantedAt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	c := promotionConsent(grantedAt)
	if _, _, err := ProposeDerivative(promotionCandidate(c, grantedAt.Add(time.Hour)), &c,
		VerdictFail, "h", grantedAt.Add(2*time.Hour), TrainingTermsV1, "digest-v1"); err != ErrNotSanitizable {
		t.Errorf("failed sanitization = %v", err)
	}
	// Uncertain sanitization refuses exactly like failure: no promotion
	// on doubt, no original-photo fallback.
	if _, _, err := ProposeDerivative(promotionCandidate(c, grantedAt.Add(time.Hour)), &c,
		VerdictUncertain, "h", grantedAt.Add(2*time.Hour), TrainingTermsV1, "digest-v1"); err != ErrNotSanitizable {
		t.Errorf("uncertain sanitization = %v", err)
	}
	if _, _, err := ProposeDerivative(promotionCandidate(c, grantedAt.Add(time.Hour)), &c,
		SanitizeVerdict("BLESSED"), "h", grantedAt.Add(2*time.Hour), TrainingTermsV1, "digest-v1"); err != ErrInvalidRequest {
		t.Errorf("unknown verdict = %v", err)
	}
}

func TestProposeRejectsMalformedInput(t *testing.T) {
	grantedAt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	c := promotionConsent(grantedAt)
	base := promotionCandidate(c, grantedAt.Add(time.Hour))
	for name, mutate := range map[string]func(*PromotionCandidate){
		"empty image":  func(p *PromotionCandidate) { p.ImageID = " " },
		"empty owner":  func(p *PromotionCandidate) { p.ContributorID = "" },
		"empty hash":   func(p *PromotionCandidate) { p.SourceSHA256 = "" },
		"zero capture": func(p *PromotionCandidate) { p.CapturedAt = time.Time{} },
	} {
		p := base
		mutate(&p)
		if _, _, err := ProposeDerivative(p, &c, VerdictPass, "h", grantedAt.Add(2*time.Hour),
			TrainingTermsV1, "digest-v1"); err != ErrInvalidRequest {
			t.Errorf("%s = %v, want invalid request", name, err)
		}
	}
	if _, _, err := ProposeDerivative(base, &c, VerdictPass, " ", grantedAt.Add(2*time.Hour),
		TrainingTermsV1, "digest-v1"); err != ErrInvalidRequest {
		t.Errorf("empty derivative hash = %v", err)
	}
	foreign := base
	foreign.ContributorID = "someone-else"
	if _, _, err := ProposeDerivative(foreign, &c, VerdictPass, "h", grantedAt.Add(2*time.Hour),
		TrainingTermsV1, "digest-v1"); err != ErrNotFound {
		t.Errorf("foreign image = %v, want not found", err)
	}
}

func TestRevokeDerivativesIsIdempotent(t *testing.T) {
	grantedAt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	c := promotionConsent(grantedAt)
	d, _, err := ProposeDerivative(promotionCandidate(c, grantedAt.Add(time.Hour)), &c,
		VerdictPass, "h", grantedAt.Add(2*time.Hour), TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	revoked, changed, err := RevokeDerivative(d, grantedAt.Add(3*time.Hour))
	if err != nil || !changed || revoked.State != DerivativeRevoked {
		t.Fatalf("revoke = %+v, %v, %v", revoked, changed, err)
	}
	again, changed, err := RevokeDerivative(revoked, grantedAt.Add(4*time.Hour))
	if err != nil || changed || again.State != DerivativeRevoked {
		t.Errorf("repeat revoke = %+v, %v, %v", again, changed, err)
	}
}

func TestSelectableForTrainingFiltersRestore(t *testing.T) {
	grantedAt := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	c := promotionConsent(grantedAt)
	d, _, err := ProposeDerivative(promotionCandidate(c, grantedAt.Add(time.Hour)), &c,
		VerdictPass, "h", grantedAt.Add(2*time.Hour), TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	if SelectableForTraining(d, &c, TrainingTermsV1, "digest-v1") {
		t.Error("pending output selectable")
	}
	d.OutputState = OutputReady
	if !SelectableForTraining(d, &c, TrainingTermsV1, "digest-v1") {
		t.Error("active derivative not selectable")
	}
	// Revoked consent stops selection without deleting the receipt.
	revokedConsent, _, err := RevokeConsent(&c, c.ContributorID, c.ContributorRef, 2, grantedAt.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if SelectableForTraining(d, &revokedConsent, TrainingTermsV1, "digest-v1") {
		t.Error("revoked consent still selects")
	}
	// Revoked derivatives never reselect, even under a fresh grant.
	revoked, _, err := RevokeDerivative(d, grantedAt.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	fresh, _, err := GrantConsent(&revokedConsent, ConsentParams{
		ContributorID: c.ContributorID, ContributorRef: c.ContributorRef,
		TermsVersion: TrainingTermsV1, TermsDigest: "digest-v1",
		Revision: 3, DecidedAt: grantedAt.Add(4 * time.Hour),
	}, TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	if SelectableForTraining(revoked, &fresh, TrainingTermsV1, "digest-v1") {
		t.Error("revoked derivative resurrected by fresh grant")
	}
	// A forged future capture cannot make an earlier grant revision valid
	// under a later grant. Terms on the derivative must match too.
	future := d
	future.CapturedAt = fresh.GrantedAt.Add(time.Hour)
	if SelectableForTraining(future, &fresh, TrainingTermsV1, "digest-v1") {
		t.Error("old revision reselected under later grant")
	}
	altered := d
	altered.TermsDigest = "other-digest"
	if SelectableForTraining(altered, &c, TrainingTermsV1, "digest-v1") {
		t.Error("derivative terms mismatch selected")
	}
	// Absent consent denies without error (default OFF everywhere).
	if SelectableForTraining(d, nil, TrainingTermsV1, "digest-v1") {
		t.Error("absent consent selects")
	}
}

func TestPromotionCarriesIdentifiersOnly(t *testing.T) {
	// Derivative rows must stay identifier-only: references and hashes
	// that bind bytes, never pixels, GPS, keys or labels.
	allowed := map[string]bool{
		"ImageID": true, "ContributorID": true, "ContributorRef": true,
		"SourceSHA256": true, "DerivativeSHA256": true, "CapturedAt": true,
		"TermsVersion": true, "TermsDigest": true, "State": true, "ObjectKey": true, "OutputState": true, "OutputLeaseUntil": true,
		"ConsentRevision": true, "PromotedAt": true, "RevokedAt": true,
	}
	typ := reflect.TypeOf(Derivative{})
	for i := 0; i < typ.NumField(); i++ {
		if !allowed[typ.Field(i).Name] {
			t.Errorf("unexpected Derivative field %q", typ.Field(i).Name)
		}
	}
	allowedCandidate := map[string]bool{
		"ImageID": true, "ContributorID": true, "ContributorRef": true,
		"CapturedAt": true, "PerImageAllowed": true, "SourceSHA256": true,
		"QueuedAtConsentRevision": true,
	}
	typ = reflect.TypeOf(PromotionCandidate{})
	for i := 0; i < typ.NumField(); i++ {
		if !allowedCandidate[typ.Field(i).Name] {
			t.Errorf("unexpected PromotionCandidate field %q", typ.Field(i).Name)
		}
	}
}
