package oidc

import (
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

func TestGoogleNativeVerifiesAuthorizedPartyIssuerAndEmailClaims(t *testing.T) {
	const audience = "test-web.apps.example.invalid"
	const party = "test-android.apps.example.invalid"
	v, issuer, nonces := testSetup(t, "RS256")
	v.GoogleNativeAudience = audience
	v.GoogleAuthorizedParties = map[string]bool{party: true}
	for _, tt := range []struct {
		name, issuer, aud, party string
		verified                 any
		wantErr                  error
		wantVerified             bool
	}{
		{"canonical issuer", domain.IssuerGoogle, audience, party, true, nil, true},
		{"legacy issuer spelling", "accounts.google.com", audience, party, "true", nil, true},
		{"different issuer", "https://accounts.example.invalid", audience, party, true, domain.ErrOIDCUnknownIssuer, false},
		{"legacy link audience", domain.IssuerGoogle, testAudience, party, true, domain.ErrOIDCWrongAudience, false},
		{"wrong presenter", domain.IssuerGoogle, audience, "other-client", true, domain.ErrOIDCWrongAudience, false},
		{"missing presenter", domain.IssuerGoogle, audience, "", true, domain.ErrOIDCWrongAudience, false},
		{"unverified", domain.IssuerGoogle, audience, party, false, nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			nonce := domain.NativeNonceClaim("native-test-nonce-" + tt.name)
			tok, err := issuer.TokenWithGoogleClaims("google-native-subject", "person@example.invalid", tt.aud, nonce, v.Clock(), v.Clock().Add(time.Hour), tt.issuer, GoogleClaims{EmailVerified: tt.verified, HostedDomain: "example.invalid", AuthorizedParty: tt.party})
			if err != nil {
				t.Fatal(err)
			}
			got, err := v.Verify(t.Context(), "google", tok, audience, nonce)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("want %v, got %v", tt.wantErr, err)
			}
			if err != nil {
				if !nonces.Consume(nonce) {
					t.Error("rejected proof consumed a nonce")
				}
				return
			}
			if got.Issuer != domain.IssuerGoogle || got.Subject != "google-native-subject" || got.EmailVerified != tt.wantVerified || got.HostedDomain != "example.invalid" {
				t.Fatalf("wrong verified proof: %+v", got)
			}
		})
	}
	legacy, err := issuer.Token("legacy-link-test-subject", "person@example.invalid", testAudience, testNonce, v.Clock(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(t.Context(), "google", legacy, testAudience, testNonce); err != nil {
		t.Fatalf("legacy linking must not have a native presenter policy: %v", err)
	}
}
