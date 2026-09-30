//go:build integration

package adapters

import (
	"context"
	"testing"
	"time"
)

func insertDeviceKey(t *testing.T, r *Registrar, contributor, fingerprint, jwk string, revoked bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := r.pool.Exec(ctx, `INSERT INTO identity_contributors (id) VALUES ($1)`, contributor); err != nil {
		t.Fatalf("insert contributor: %v", err)
	}
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO identity_keys (id, contributor_id, algorithm, public_jwk, fingerprint, revoked_at)
		 VALUES (gen_random_uuid(), $1, 'ecdsa-p256-sha512', $2, $3, CASE WHEN $4 THEN now() ELSE NULL END)`,
		contributor, jwk, fingerprint, revoked); err != nil {
		t.Fatalf("insert key: %v", err)
	}
}

const testJWK = `{"crv":"P-256","kty":"EC","x":"MKBCTNIcKUSDii11yQNq5yaxomwxz0uNiU4NgzcDQE","y":"m4wFsz9YFyPxd5isoV9q_7dAuYUK51Siw2_l38L8M"}`

func TestFindDeviceKeyRoundTrip(t *testing.T) {
	r := freshRegistrar(t)
	ctx := context.Background()
	contributor := "11111111-1111-4111-8111-111111111111"
	insertDeviceKey(t, r, contributor, "fp:abc123", testJWK, false)

	got, err := r.FindDeviceKey(ctx, "fp:abc123")
	if err != nil {
		t.Fatalf("FindDeviceKey: %v", err)
	}
	if got.ContributorID != contributor || got.Fingerprint != "fp:abc123" || got.Revoked {
		t.Errorf("wrong key resolution: %+v", got)
	}
	if got.JWKX == "" || got.JWKY == "" {
		t.Errorf("JWK coordinates must resolve: %+v", got)
	}
}

func TestFindDeviceKeyUnknownAndRevoked(t *testing.T) {
	r := freshRegistrar(t)
	ctx := context.Background()

	if _, err := r.FindDeviceKey(ctx, "fp:missing"); err == nil {
		t.Error("unknown fingerprint must fail")
	}
	if _, err := r.FindDeviceKey(ctx, "  "); err == nil {
		t.Error("empty fingerprint must fail")
	}

	contributor := "22222222-2222-4222-8222-222222222222"
	insertDeviceKey(t, r, contributor, "fp:revoked1", testJWK, true)
	got, err := r.FindDeviceKey(ctx, "fp:revoked1")
	if err != nil {
		t.Fatalf("revoked key must still resolve: %v", err)
	}
	if !got.Revoked {
		t.Errorf("revocation must surface, got %+v", got)
	}

	contributor2 := "33333333-3333-4333-8333-333333333333"
	insertDeviceKey(t, r, contributor2, "fp:badjwk", `{"crv":"RSA"}`, false)
	if _, err := r.FindDeviceKey(ctx, "fp:badjwk"); err == nil {
		t.Error("malformed stored JWK must fail")
	}
}
