package domain

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func consentParams(revision uint64) ConsentParams {
	return ConsentParams{
		ContributorID:  "c1111111-0000-4000-8000-000000000001",
		ContributorRef: "tok-owner",
		TermsVersion:   TrainingTermsV1,
		TermsDigest:    "digest-v1",
		Revision:       revision,
		DecidedAt:      time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC),
	}
}

func TestGrantFromAbsentEstablishesGranted(t *testing.T) {
	c, receipt, err := GrantConsent(nil, consentParams(1), TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatalf("grant = %v", err)
	}
	if c.State != StateGranted || c.Revision != 1 || c.Purpose != TrainingPurpose {
		t.Errorf("consent = %+v", c)
	}
	if !c.GrantedAt.Equal(consentParams(1).DecidedAt) || !c.DecidedAt.Equal(consentParams(1).DecidedAt) {
		t.Errorf("timestamps = %+v", c)
	}
	if receipt.Replayed || receipt.State != StateGranted || receipt.Revision != 1 {
		t.Errorf("receipt = %+v", receipt)
	}
}

func TestGrantRefusesInvalidInput(t *testing.T) {
	base := consentParams(1)
	for name, mutate := range map[string]func(*ConsentParams){
		"empty owner":   func(p *ConsentParams) { p.ContributorID = " " },
		"empty ref":     func(p *ConsentParams) { p.ContributorRef = "" },
		"zero revision": func(p *ConsentParams) { p.Revision = 0 },
		"zero time":     func(p *ConsentParams) { p.DecidedAt = time.Time{} },
		"empty version": func(p *ConsentParams) { p.TermsVersion = "" },
		"empty digest":  func(p *ConsentParams) { p.TermsDigest = "" },
	} {
		p := base
		mutate(&p)
		if _, _, err := GrantConsent(nil, p, TrainingTermsV1, "digest-v1"); err != ErrInvalidRequest {
			t.Errorf("%s = %v, want invalid request", name, err)
		}
	}
	if _, _, err := GrantConsent(nil, base, "", "digest-v1"); err != ErrInvalidRequest {
		t.Errorf("empty server version = %v", err)
	}
}

func TestGrantRefusesOldTerms(t *testing.T) {
	p := consentParams(1)
	if _, _, err := GrantConsent(nil, p, "training-terms-v2", "digest-v2"); err != ErrTermsOutdated {
		t.Errorf("rotated version grant = %v, want terms outdated", err)
	}
	p.TermsDigest = "stale-digest"
	if _, _, err := GrantConsent(nil, p, TrainingTermsV1, "digest-v1"); err != ErrTermsOutdated {
		t.Errorf("stale digest grant = %v, want terms outdated", err)
	}
	p = consentParams(1)
	p.TermsVersion = "training-terms-v0"
	if _, _, err := GrantConsent(nil, p, TrainingTermsV1, "digest-v1"); err != ErrTermsOutdated {
		t.Errorf("old version grant = %v, want terms outdated", err)
	}
}

func TestRevokeFromAbsentEstablishesRevokedBaseline(t *testing.T) {
	at := time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC)
	c, receipt, err := RevokeConsent(nil, "c1", "tok-c1", 1, at)
	if err != nil {
		t.Fatalf("revoke = %v", err)
	}
	if c.State != StateRevoked || c.Revision != 1 || !c.RevokedAt.Equal(at) {
		t.Errorf("consent = %+v", c)
	}
	if receipt.Replayed {
		t.Errorf("first revoke replayed: %+v", receipt)
	}
	// A later stale grant at an older-or-equal revision cannot resurrect it.
	if _, _, err := GrantConsent(&c, consentParams(1), TrainingTermsV1, "digest-v1"); err != ErrStaleConsent && err != ErrNotFound {
		t.Errorf("stale grant after baseline revoke = %v", err)
	}
}

func TestStaleGrantCannotUndoRevocation(t *testing.T) {
	p := consentParams(1)
	c, _, err := GrantConsent(nil, p, TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	revoked, _, err := RevokeConsent(&c, p.ContributorID, p.ContributorRef, 2, p.DecidedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	stale := consentParams(2)
	if _, _, err := GrantConsent(&revoked, stale, TrainingTermsV1, "digest-v1"); err != ErrStaleConsent {
		t.Errorf("equal-revision grant after revoke = %v, want stale", err)
	}
	older := consentParams(1)
	if _, _, err := GrantConsent(&revoked, older, TrainingTermsV1, "digest-v1"); err != ErrStaleConsent {
		t.Errorf("older-revision grant after revoke = %v, want stale", err)
	}
	if revoked.State != StateRevoked {
		t.Errorf("revoked state flipped: %+v", revoked)
	}
}

func TestFreshGrantAfterRevocationReenables(t *testing.T) {
	p := consentParams(1)
	c, _, err := GrantConsent(nil, p, TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	revoked, _, err := RevokeConsent(&c, p.ContributorID, p.ContributorRef, 2, p.DecidedAt.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	fresh := consentParams(3)
	fresh.DecidedAt = p.DecidedAt.Add(2 * time.Hour)
	reganted, receipt, err := GrantConsent(&revoked, fresh, TrainingTermsV1, "digest-v1")
	if err != nil || receipt.Replayed {
		t.Fatalf("fresh grant = %+v, %v", receipt, err)
	}
	if reganted.State != StateGranted || reganted.Revision != 3 {
		t.Errorf("regranted = %+v", reganted)
	}
}

func TestSameRevisionReplayAndConflict(t *testing.T) {
	p := consentParams(1)
	c, _, err := GrantConsent(nil, p, TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	same, receipt, err := GrantConsent(&c, p, TrainingTermsV1, "digest-v1")
	if err != nil || !receipt.Replayed || same.Revision != 1 {
		t.Errorf("identical retry = %+v, %v", receipt, err)
	}
	// Same revision under a live clock stamps a new attempt instant but
	// stays the same decision: lost-response retries still converge.
	later := p
	later.DecidedAt = p.DecidedAt.Add(time.Minute)
	if _, receipt, err := GrantConsent(&c, later, TrainingTermsV1, "digest-v1"); err != nil || !receipt.Replayed {
		t.Errorf("live-clock retry = %+v, %v", receipt, err)
	}
	divergent := p
	divergent.TermsDigest = "digest-v1-changed"
	// Same revision but different terms is a conflicting retry. (The
	// current-terms gate runs first, so exercise it directly here.)
	if _, _, err := GrantConsent(&c, divergent, TrainingTermsV1, "digest-v1-changed"); err != ErrConflict {
		t.Errorf("divergent retry = %v, want conflict", err)
	}
}

func TestOwnerMismatchIsNotFound(t *testing.T) {
	p := consentParams(1)
	c, _, err := GrantConsent(nil, p, TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	foreign := p
	foreign.ContributorID = "c9999999-0000-4000-8000-000000000009"
	foreign.Revision = 2
	if _, _, err := GrantConsent(&c, foreign, TrainingTermsV1, "digest-v1"); err != ErrNotFound {
		t.Errorf("foreign owner grant = %v, want not found", err)
	}
	if _, _, err := RevokeConsent(&c, "c9999999-0000-4000-8000-000000000009", "tok-owner", 2, p.DecidedAt); err != ErrNotFound {
		t.Errorf("foreign owner revoke = %v, want not found", err)
	}
}

func TestAuthorizesImageMatrix(t *testing.T) {
	p := consentParams(1)
	c, _, err := GrantConsent(nil, p, TrainingTermsV1, "digest-v1")
	if err != nil {
		t.Fatal(err)
	}
	img := func(captured time.Time, allowed bool) ImageSelection {
		return ImageSelection{
			ImageID: "img-1", ContributorID: p.ContributorID, ContributorRef: p.ContributorRef,
			CapturedAt: captured, PerImageAllowed: allowed,
		}
	}
	// Absent consent denies without error (default OFF).
	if ok, err := AuthorizesImage(nil, img(p.DecidedAt.Add(time.Hour), true), TrainingTermsV1, "digest-v1"); err != nil || ok {
		t.Errorf("absent consent = %v, %v", ok, err)
	}
	// Pre-grant capture is never retroactively authorized.
	if ok, _ := AuthorizesImage(&c, img(p.DecidedAt.Add(-time.Hour), true), TrainingTermsV1, "digest-v1"); ok {
		t.Error("pre-grant capture authorized")
	}
	// Post-grant capture without the per-image opt-in is denied.
	if ok, _ := AuthorizesImage(&c, img(p.DecidedAt.Add(time.Hour), false), TrainingTermsV1, "digest-v1"); ok {
		t.Error("opt-out capture authorized")
	}
	// Post-grant capture with the per-image opt-in is authorized.
	if ok, err := AuthorizesImage(&c, img(p.DecidedAt.Add(time.Hour), true), TrainingTermsV1, "digest-v1"); err != nil || !ok {
		t.Errorf("eligible capture = %v, %v", ok, err)
	}
	// Terms rotation invalidates the old grant until a fresh one lands.
	if ok, _ := AuthorizesImage(&c, img(p.DecidedAt.Add(time.Hour), true), "training-terms-v2", "digest-v2"); ok {
		t.Error("rotated terms still authorize")
	}
	// Revocation stops selection atomically.
	revoked, _, err := RevokeConsent(&c, p.ContributorID, p.ContributorRef, 2, p.DecidedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := AuthorizesImage(&revoked, img(p.DecidedAt.Add(3*time.Hour), true), TrainingTermsV1, "digest-v1"); ok {
		t.Error("revoked consent still authorizes")
	}
	// Foreign image reads as not found, never a silent denial oracle.
	foreign := img(p.DecidedAt.Add(time.Hour), true)
	foreign.ContributorID = "someone-else"
	if _, err := AuthorizesImage(&c, foreign, TrainingTermsV1, "digest-v1"); err != ErrNotFound {
		t.Errorf("foreign image = %v, want not found", err)
	}
	bad := img(time.Time{}, true)
	if _, err := AuthorizesImage(&c, bad, TrainingTermsV1, "digest-v1"); err != ErrInvalidRequest {
		t.Errorf("zero capture time = %v, want invalid request", err)
	}
}

func TestConsentCarriesIdentifiersOnly(t *testing.T) {
	// The consent row must stay identifier-only: no pixels, GPS, keys
	// or tokens beyond the attribution reference. Additions fail loudly.
	allowed := map[string]bool{
		"ContributorID": true, "ContributorRef": true, "Purpose": true,
		"TermsVersion": true, "TermsDigest": true, "State": true,
		"Revision": true, "GrantedAt": true, "RevokedAt": true, "DecidedAt": true,
	}
	typ := reflect.TypeOf(Consent{})
	for i := 0; i < typ.NumField(); i++ {
		if !allowed[typ.Field(i).Name] {
			t.Errorf("unexpected Consent field %q", typ.Field(i).Name)
		}
	}
	allowedImg := map[string]bool{
		"ImageID": true, "ContributorID": true, "ContributorRef": true,
		"CapturedAt": true, "PerImageAllowed": true,
	}
	typ = reflect.TypeOf(ImageSelection{})
	for i := 0; i < typ.NumField(); i++ {
		if !allowedImg[typ.Field(i).Name] {
			t.Errorf("unexpected ImageSelection field %q", typ.Field(i).Name)
		}
	}
	if strings.Contains(reflect.TypeOf(ConsentReceipt{}).String(), "GPS") {
		t.Error("receipt must never carry location")
	}
}
