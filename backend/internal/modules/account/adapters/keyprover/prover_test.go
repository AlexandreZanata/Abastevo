package keyprover

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type deviceKey struct {
	contributor string
	fingerprint string
	x, y        string
	priv        *ecdsa.PrivateKey
	revoked     bool
}

func mintKey(t *testing.T, contributor string) *deviceKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	x := base64.RawURLEncoding.EncodeToString(priv.X.Bytes())
	y := base64.RawURLEncoding.EncodeToString(priv.Y.Bytes())
	// Pad to 32 bytes: Bytes() strips leading zeroes.
	xb, _ := base64.RawURLEncoding.DecodeString(x)
	yb, _ := base64.RawURLEncoding.DecodeString(y)
	xp := make([]byte, 32)
	yp := make([]byte, 32)
	copy(xp[32-len(xb):], xb)
	copy(yp[32-len(yb):], yb)
	x = base64.RawURLEncoding.EncodeToString(xp)
	y = base64.RawURLEncoding.EncodeToString(yp)
	thumb := `{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`
	sum := sha256.Sum256([]byte(thumb))
	return &deviceKey{
		contributor: contributor,
		fingerprint: fmt.Sprintf("fp:%x", sum),
		x:           x, y: y, priv: priv,
	}
}

func (k *deviceKey) sign(t *testing.T, accountID, contributor string, expiry int64) string {
	t.Helper()
	digest := sha256.Sum256([]byte(BindMessage(accountID, contributor, expiry)))
	r, s, err := ecdsa.Sign(rand.Reader, k.priv, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	rb, sb := r.Bytes(), s.Bytes()
	raw := make([]byte, 64)
	copy(raw[32-len(rb):32], rb)
	copy(raw[64-len(sb):], sb)
	return k.fingerprint + "." + fmt.Sprintf("%d", expiry) + "." + base64.RawURLEncoding.EncodeToString(raw)
}

type memKeys struct {
	mu   sync.Mutex
	keys map[string]*deviceKey
}

func (m *memKeys) lookup(_ context.Context, fingerprint string) (string, string, string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.keys[fingerprint]
	if !ok {
		return "", "", "", false, errors.New("keyprover: unknown key")
	}
	return k.contributor, k.x, k.y, k.revoked, nil
}

func testProver(now int64, keys ...*deviceKey) (Prover, *memKeys) {
	src := &memKeys{keys: map[string]*deviceKey{}}
	for _, k := range keys {
		src.keys[k.fingerprint] = k
	}
	return Prover{Keys: src.lookup, Now: func() time.Time { return time.Unix(now, 0) }}, src
}

const (
	provNow     = 1_800_000_000
	provAccount = "aaaaaaaa-1111-4111-8111-111111111111"
	provContrib = "contrib-1"
)

func TestValidProofAccepted(t *testing.T) {
	key := mintKey(t, provContrib)
	p, _ := testProver(provNow, key)
	proof := key.sign(t, provAccount, provContrib, provNow+60)
	got, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, proof)
	if err != nil {
		t.Fatalf("valid proof rejected: %v", err)
	}
	if got != key.fingerprint {
		t.Errorf("must return the proven fingerprint, got %q", got)
	}
}

func TestWrongKeyAndAccountMismatch(t *testing.T) {
	key := mintKey(t, provContrib)
	other := mintKey(t, "contrib-2")
	p, _ := testProver(provNow, key, other)

	// Signature by another key over the same statement.
	proof := other.sign(t, provAccount, provContrib, provNow+60)
	if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, proof); err == nil {
		t.Error("wrong-key signature must fail")
	}
	// Proof minted for another account cannot authorize this one.
	proof = key.sign(t, "bbbbbbbb-2222-4222-8222-222222222222", provContrib, provNow+60)
	if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, proof); err == nil {
		t.Error("cross-account proof must fail")
	}
	// Key registered to another contributor cannot bind here.
	proof = other.sign(t, provAccount, "contrib-2", provNow+60)
	if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, proof); err == nil {
		t.Error("contributor mismatch must fail")
	}
	// Tampered statement byte fails.
	proof = key.sign(t, provAccount, provContrib, provNow+60)
	parts := strings.Split(proof, ".")
	parts[2] = parts[2][:len(parts[2])-2] + "AA"
	if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, strings.Join(parts, ".")); err == nil {
		t.Error("tampered signature must fail")
	}
}

func TestExpiryWindowEnforced(t *testing.T) {
	key := mintKey(t, provContrib)
	p, _ := testProver(provNow, key)

	if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, key.sign(t, provAccount, provContrib, provNow-1)); err == nil {
		t.Error("expired proof must fail")
	}
	if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, key.sign(t, provAccount, provContrib, provNow+301)); err == nil {
		t.Error("oversized window must fail")
	}
	if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, key.sign(t, provAccount, provContrib, provNow+300)); err != nil {
		t.Errorf("300s edge must pass, got %v", err)
	}
}

func TestUnknownRevokedMalformedDenied(t *testing.T) {
	key := mintKey(t, provContrib)
	p, src := testProver(provNow, key)

	other := mintKey(t, "ghost-1")
	ghost := other.sign(t, provAccount, "ghost-1", provNow+60)
	if _, err := p.VerifyKeyProof(context.Background(), provAccount, "ghost-1", ghost); err == nil {
		t.Error("unknown fingerprint must fail")
	}
	key.revoked = true
	proof := key.sign(t, provAccount, provContrib, provNow+60)
	if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, proof); err == nil {
		t.Error("revoked key must fail")
	}
	key.revoked = false
	_ = src
	for _, bad := range []string{
		"", "a.b", "fp:xyz.123.sig", "fp:" + strings.Repeat("g", 64) + ".123.sig",
		key.fingerprint + ".notanumber.sig", key.fingerprint + ".123.!!!",
	} {
		if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, bad); err == nil {
			t.Errorf("malformed proof %q must fail", bad)
		}
	}
	p.Keys = nil
	if _, err := p.VerifyKeyProof(context.Background(), provAccount, provContrib, proof); err == nil {
		t.Error("missing source must fail closed")
	}
}
