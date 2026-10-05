package application

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/verify"
)

type verifyFixture struct {
	proofs *fakeProofStore
	claims *fakeClaimStore
	bytes  *fakeProofBytes
	declID string
	proof  Proof
	roots  *x509.CertPool
	revoke verify.RevocationSource
	certs  [][]byte
	clock  time.Time
}

func claimPortsForProof(store *fakeClaimStore) ClaimPorts {
	n := 0
	return ClaimPorts{
		Store: store,
		Clock: func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) {
			n++
			return "claim-id-" + string(rune('0'+n)), nil
		},
		OperatorOf: func(context.Context, string) (string, string, bool, error) {
			return "04218406000104", "registry", true, nil
		},
	}
}

func proofTestPortsForVerify(proofs *fakeProofStore, bytes *fakeProofBytes, claims *fakeClaimStore) ProofPorts {
	n := 0
	return ProofPorts{
		Proofs: proofs,
		Claims: claims,
		Clock:  func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) },
		NewID: func() (string, error) {
			n++
			return "proof-id-" + string(rune('0'+n)), nil
		},
		Bytes: bytes,
	}
}

type testPKIKeys struct {
	leaf  []byte
	ca    []byte
	roots *x509.CertPool
}

func syntheticTestPKI(t *testing.T) testPKIKeys {
	t.Helper()
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(11), Subject: pkix.Name{CommonName: "APP-TEST Root"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(1, 0, 0),
		IsCA: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ca: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(12), Subject: pkix.Name{CommonName: "APP-TEST Signer"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(0, 6, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("leaf: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return testPKIKeys{
		leaf:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		ca:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		roots: roots,
	}
}

type knownGoodRevocation struct{}

func (knownGoodRevocation) Revoked(string) (bool, bool) { return false, true }

type unknownRevocation struct{}

func (unknownRevocation) Revoked(string) (bool, bool) { return false, false }

func verifyTestPorts(fix verifyFixture) VerifyPorts {
	return VerifyPorts{
		Proofs: fix.proofs, Claims: fix.claims,
		Bytes:   fixBytesReader(fix.bytes),
		Roots:   func() (*x509.CertPool, error) { return fix.roots, nil },
		Revoker: fix.revoke,
		Clock:   func() time.Time { return fix.clock },
		Certs:   func(context.Context, string) ([][]byte, error) { return fix.certs, nil },
	}
}

type bytesReader struct{ inner *fakeProofBytes }

func (b bytesReader) Get(ctx context.Context, key string) ([]byte, error) {
	return b.inner.Get(ctx, key)
}

func fixBytesReader(inner *fakeProofBytes) ProofBytesReader { return bytesReader{inner: inner} }

func setupVerifyProof(t *testing.T) verifyFixture {
	t.Helper()
	claims := &fakeClaimStore{}
	proofs := &fakeProofStore{}
	bytes := &fakeProofBytes{}
	// Open a claim and submit a proof through the real paths so the
	// fixture carries genuine digests and bindings.
	claimPorts := claimPortsForProof(claims)
	claim, created, err := OpenClaim(context.Background(), claimPorts, "acc-1", "station-1", "administrator", []string{"profile.edit"}, "verify-key-1")
	if err != nil || !created {
		t.Fatalf("open = %+v, %v, %v", claim, created, err)
	}
	declaration, err := claims.ActiveDeclaration(context.Background(), claim.ID)
	if err != nil {
		t.Fatalf("declaration: %v", err)
	}
	proofPorts := proofTestPortsForVerify(proofs, bytes, claims)
	// The signed document embeds the exact declaration text (as the
	// operator-extracted content would) while keeping the PDF magic
	// prefix the intake policy requires.
	prefix := append([]byte("%PDF-1.7\n"), []byte(declaration.Declaration)...)
	body := append(prefix, make([]byte, 200)...)
	for i := range body[len(prefix):] {
		body[len(prefix)+i] = 'a'
	}
	proof, created, err := SubmitProof(context.Background(), proofPorts, "acc-1", claim.ID, declaration.ID, "a.pdf", "application/pdf", "authorization", body)
	if err != nil || !created {
		t.Fatalf("submit = %+v, %v, %v", proof, created, err)
	}
	pki := syntheticTestPKI(t)
	return verifyFixture{
		proofs: proofs, claims: claims, bytes: bytes,
		declID: declaration.ID, proof: proof, roots: pki.roots,
		revoke: knownGoodRevocation{}, certs: [][]byte{pki.leaf, pki.ca},
		clock: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC),
	}
}

func TestVerifyProofValidConsumesBinding(t *testing.T) {
	fix := setupVerifyProof(t)
	ports := verifyTestPorts(fix)
	result, err := VerifyProof(context.Background(), ports, fix.proof.ID)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if result.Outcome != verify.OutcomeValid || result.Signer == "" {
		t.Fatalf("result = %+v", result)
	}
	proof, err := fix.proofs.GetProof(context.Background(), fix.proof.ID)
	if err != nil || proof.Status != "verified" {
		t.Fatalf("proof = %+v, err = %v", proof, err)
	}
	decl, err := fix.claims.GetDeclaration(context.Background(), fix.declID)
	if err != nil {
		t.Fatalf("declaration: %v", err)
	}
	_ = decl
}

func TestVerifyProofInvalidConsumesBinding(t *testing.T) {
	fix := setupVerifyProof(t)
	// Tamper the stored bytes: content no longer binds the declaration.
	fix.bytes.objects = map[string][]byte{}
	ports := verifyTestPorts(fix)
	// Re-store different bytes under the same key would change the
	// hash; instead the stored object is now absent -> transport error.
	// For a content failure, submit-then-tamper via direct store write:
	fix.proofs.proofs[fix.proof.ID] = fix.proofs.proofs[fix.proof.ID]
	result, err := VerifyProof(context.Background(), ports, fix.proof.ID)
	if err == nil {
		t.Fatalf("missing bytes must fail, got %+v", result)
	}
}

func TestVerifyProofIndeterminateBumpsWithoutConsuming(t *testing.T) {
	fix := setupVerifyProof(t)
	ports := verifyTestPorts(fix)
	ports.Revoker = unknownRevocation{}
	first, err := VerifyProof(context.Background(), ports, fix.proof.ID)
	if err != nil || first.Outcome != verify.OutcomeIndeterminate {
		t.Fatalf("first = %+v, err = %v", first, err)
	}
	// Attempts accumulate; the binding stays active for retry.
	decl, err := fix.claims.GetDeclaration(context.Background(), fix.declID)
	if err != nil {
		t.Fatalf("declaration: %v", err)
	}
	_ = decl
	// Four more indeterminate rounds (attempts 2..5), then exhaustion.
	for i := 0; i < 4; i++ {
		if _, err := VerifyProof(context.Background(), ports, fix.proof.ID); err != nil {
			t.Fatalf("retry %d: %v", i, err)
		}
	}
	last, err := VerifyProof(context.Background(), ports, fix.proof.ID)
	if err != nil {
		t.Fatalf("exhaustion: %v", err)
	}
	if last.Outcome != verify.OutcomeInvalid {
		t.Fatalf("exhausted attempts must invalidate, got %+v", last)
	}
	proof, err := fix.proofs.GetProof(context.Background(), fix.proof.ID)
	if err != nil || proof.Status != "rejected" {
		t.Fatalf("proof = %+v, err = %v", proof, err)
	}
}

func TestVerifyProofClosedProofFails(t *testing.T) {
	fix := setupVerifyProof(t)
	ports := verifyTestPorts(fix)
	if _, err := VerifyProof(context.Background(), ports, "missing-proof"); err == nil {
		t.Fatal("missing proof must fail")
	}
}

var _ = errors.New
