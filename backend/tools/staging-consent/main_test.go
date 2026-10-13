package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProofUsesFreshKeyAndExactBody(t *testing.T) {
	a, err := newOwner()
	if err != nil {
		t.Fatal(err)
	}
	b, err := newOwner()
	if err != nil {
		t.Fatal(err)
	}
	if a.fingerprint == b.fingerprint {
		t.Fatal("owner keys reused")
	}
	req := request(context.Background(), "POST", consentPath+"/revoke", []byte(`{"revision":2}`))
	ls, _, _ := lines(req, a, challenge{Nonce: "synthetic-nonce"}, []byte(`{"revision":2}`))
	sig, err := sign(a, ls)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha512.Sum512([]byte(strings.Join(ls, "\n")))
	if !ecdsa.Verify(&a.key.PublicKey, digest[:], newInt(raw[:32]), newInt(raw[32:])) {
		t.Fatal("invalid proof")
	}
	if ecdsa.Verify(&b.key.PublicKey, digest[:], newInt(raw[:32]), newInt(raw[32:])) {
		t.Fatal("foreign owner accepted")
	}
	tampered := strings.Join(ls, "\n") + " "
	d := sha512.Sum512([]byte(tampered))
	if ecdsa.Verify(&a.key.PublicKey, d[:], newInt(raw[:32]), newInt(raw[32:])) {
		t.Fatal("tamper accepted")
	}
}
func TestBoundedResponseAndRequestBudget(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat("x", maxResponse+1))) }))
	defer s.Close()
	p := probe{client: s.Client()}
	r, _ := http.NewRequest("GET", s.URL, nil)
	if _, err := p.send(r); err == nil {
		t.Fatal("oversize accepted")
	}
	p.requests = 64
	if _, err := p.send(r); err == nil {
		t.Fatal("request ceiling ignored")
	}
}
func TestNoStoreAndExactDecisionRequired(t *testing.T) {
	h := http.Header{"Cache-Control": []string{"no-store"}}
	if _, err := decode(result{200, h, []byte(`{"state":"REVOKED","revision":3}`)}, 200, "REVOKED", 2); err == nil {
		t.Fatal("wrong revision accepted")
	}
	if err := require(result{200, http.Header{}, nil}, 200); err == nil {
		t.Fatal("cacheable accepted")
	}
	p := probe{}
	if err := p.check("negative", errors.New("failed")); err == nil || len(p.checks) != 0 {
		t.Fatal("failure marked pass")
	}
}

func newInt(b []byte) *big.Int { return new(big.Int).SetBytes(b) }
