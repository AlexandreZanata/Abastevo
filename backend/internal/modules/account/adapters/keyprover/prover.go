// Package keyprover verifies contributor device-key proofs for the
// account binding ceremony (P13-T04C, B-BR-A04). The proof is
// "<fingerprint>.<expiry-unix>.<base64url-signature>" where the
// signature covers "anpfuel-bind.v1\n<accountID>\n<contributorID>\n
// <expiry>" with the contributor's registered P-256 device key.
// Binding the ceremony account keeps a proof minted for one account
// from authorizing another, on top of the stolen-ID ownership refusal.
// Every failure — unknown or revoked keys, contributor mismatch,
// expired or oversized windows, malformed proofs, bad signatures —
// denies uniformly with domain.ErrKeyProofDenied, so no oracle
// distinguishes failure classes. Keys resolve through the injected
// KeyLookup (owned by identity, wired in main); this package never
// imports another module's tables.
package keyprover

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
)

// ProofTTLSeconds bounds proof freshness: the expiry must lie within
// five minutes past now, mirroring the identity challenge windows.
const ProofTTLSeconds = 300

// BindMessage builds the signed binding statement for the ceremony.
func BindMessage(accountID, contributorID string, expiry int64) string {
	return "anpfuel-bind.v1\n" + accountID + "\n" + contributorID + "\n" + strconv.FormatInt(expiry, 10)
}

// KeyLookup resolves a fingerprint to its owner and JWK coordinates.
// Any error denies the proof uniformly.
type KeyLookup func(ctx context.Context, fingerprint string) (contributorID, jwkX, jwkY string, revoked bool, err error)

// Prover implements domain.KeyProver over device keys.
type Prover struct {
	Keys KeyLookup
	Now  func() time.Time
}

func (p Prover) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// VerifyKeyProof checks fingerprint shape, key ownership, revocation,
// expiry window and the P-256 signature, in that order.
func (p Prover) VerifyKeyProof(ctx context.Context, accountID, contributorID, proof string) (string, error) {
	deny := func() (string, error) { return "", domain.ErrKeyProofDenied }
	fingerprint, expiry, sig, err := splitProof(proof)
	if err != nil {
		return deny()
	}
	if p.Keys == nil {
		return deny()
	}
	owner, x, y, revoked, err := p.Keys(ctx, fingerprint)
	if err != nil {
		return deny()
	}
	if owner != contributorID || revoked {
		return deny()
	}
	now := p.now().Unix()
	if now >= expiry || expiry-now > ProofTTLSeconds {
		return deny()
	}
	pub, err := publicKey(x, y)
	if err != nil {
		return deny()
	}
	if len(sig) != 64 {
		return deny()
	}
	digest := sha256.Sum256([]byte(BindMessage(accountID, contributorID, expiry)))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return deny()
	}
	return fingerprint, nil
}

func splitProof(proof string) (fingerprint string, expiry int64, sig []byte, err error) {
	parts := strings.Split(proof, ".")
	if len(parts) != 3 {
		return "", 0, nil, fmt.Errorf("keyprover: malformed proof")
	}
	fingerprint = parts[0]
	if !validFingerprint(fingerprint) {
		return "", 0, nil, fmt.Errorf("keyprover: malformed fingerprint")
	}
	expiry, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil || expiry <= 0 {
		return "", 0, nil, fmt.Errorf("keyprover: malformed expiry")
	}
	sig, err = base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", 0, nil, fmt.Errorf("keyprover: malformed signature")
	}
	return fingerprint, expiry, sig, nil
}

func validFingerprint(fp string) bool {
	if !strings.HasPrefix(fp, "fp:") || len(fp) != 67 {
		return false
	}
	for _, r := range fp[3:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func publicKey(xb, yb string) (*ecdsa.PublicKey, error) {
	x, err := base64.RawURLEncoding.DecodeString(xb)
	if err != nil {
		return nil, err
	}
	y, err := base64.RawURLEncoding.DecodeString(yb)
	if err != nil {
		return nil, err
	}
	if len(x) != 32 || len(y) != 32 {
		return nil, fmt.Errorf("keyprover: JWK coordinates must be 32 bytes")
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), append([]byte{0x04}, append(x, y...)...))
	if err != nil {
		return nil, fmt.Errorf("keyprover: key off curve")
	}
	return pub, nil
}
