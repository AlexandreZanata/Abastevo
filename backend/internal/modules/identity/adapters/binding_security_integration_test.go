//go:build integration

package adapters

import (
	"context"
	"fmt"
	"testing"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
)

func TestRotationUsesStoredChallengeBindings(t *testing.T) {
	for _, field := range []string{"nonce", "purpose", "fingerprint"} {
		t.Run(field, func(t *testing.T) {
			pool := freshRotationPool(t)
			reg := NewRegistrar(pool)
			ctx := context.Background()
			oldKey, oldFP, _ := enrollKey(t, pool)
			newKey, x, y := testKey(t)
			fp := fingerprintOf(t, x, y)
			old := rotationProof(t, pool, oldKey, oldFP)
			next := rotationProof(t, pool, newKey, fp)
			switch field {
			case "nonce":
				next.Challenge.Nonce = next.Challenge.ID + ".00000000000000000000000000000000"
			case "purpose":
				ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeRegister)
				if err != nil {
					t.Fatal(err)
				}
				next.Challenge = ch
				next.Challenge.Purpose = domain.PurposeSign
			case "fingerprint":
				ch, err := reg.IssueChallenge(ctx, oldFP, domain.PurposeSign)
				if err != nil {
					t.Fatal(err)
				}
				next.Challenge = ch
				next.Challenge.Fingerprint = fp
			}
			next.BaseLines[7] = `"nonce": "` + next.Challenge.Nonce + `"`
			next.Signature = signLines(t, newKey, next.BaseLines)
			if _, err := reg.Rotate(ctx, domain.RotationRequest{Old: old, New: next, NewJWKX: x, NewJWKY: y}); err == nil {
				t.Fatal("client replaced stored " + field)
			}
			var consumed bool
			if err := pool.QueryRow(ctx, "SELECT consumed_at IS NOT NULL FROM identity_challenges WHERE id=$1", old.Challenge.ID).Scan(&consumed); err != nil {
				t.Fatal(err)
			}
			if consumed {
				t.Fatal("failed binding consumed old proof")
			}
		})
	}
}
func TestRegistrationSignatureMustCoverConsumedNonce(t *testing.T) {
	pool := freshRotationPool(t)
	reg := NewRegistrar(pool)
	ctx := context.Background()
	key, x, y := testKey(t)
	fp := fingerprintOf(t, x, y)
	ch, err := reg.IssueChallenge(ctx, fp, domain.PurposeRegister)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	lines := []string{`"@method": POST`, `"@authority": test.invalid`, `"@path": /v1/contributors`, `"@query": `, fmt.Sprintf(`"created": %d`, now.Unix()-1), fmt.Sprintf(`"expires": %d`, now.Unix()+240), `"keyid": "` + fp + `"`, `"nonce": "unrelated-proof"`}
	req := domain.RegistrationRequest{JWKX: x, JWKY: y, Challenge: ch, BaseLines: lines, Signature: signLines(t, key, lines)}
	if _, err := reg.Register(ctx, req); err == nil {
		t.Fatal("consumed nonce outside signed proof")
	}
}
