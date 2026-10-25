package mapdist

// Guards the committed Go-signed fixture consumed by Kotlin MAP-15 tests
// (contracts/testdata/map/dist-kotlin-v1). The envelope signature is real
// Go crypto/ed25519 output; package bytes are deterministic synthetic
// content, not real map data.

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func committedFixtureDir(t *testing.T) string {
	t.Helper()
	// Backend module dir -> repo root -> contracts fixture.
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	// internal/modules/mapdist -> backend; repo root is two levels above backend.
	dir := filepath.Join(root, "..", "..", "contracts", "testdata", "map", "dist-kotlin-v1")
	if _, err := os.Stat(filepath.Join(dir, "distribution.json")); err != nil {
		t.Skipf("fixture not checked out: %v", err)
	}
	return dir
}

func TestCommittedKotlinFixture(t *testing.T) {
	dir := committedFixtureDir(t)
	rawKey, err := os.ReadFile(filepath.Join(dir, "test-key.pub"))
	if err != nil {
		t.Fatal(err)
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(string(rawKey))
	if err != nil || len(keyBytes) != ed25519.PublicKeySize {
		t.Fatalf("bad test public key: %v", err)
	}
	keys := map[string]ed25519.PublicKey{"test-key": ed25519.PublicKey(keyBytes)}
	dist, err := Load(dir, keys)
	if err != nil {
		t.Fatalf("Load committed fixture: %v", err)
	}
	if dist.Manifest.Generation != "test-20261025.1" {
		t.Fatalf("unexpected generation %q", dist.Manifest.Generation)
	}
	if len(dist.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(dist.Packages))
	}
	if _, ok := dist.Files["test-rect"]; !ok {
		t.Fatalf("missing test-rect package file")
	}

	other := make([]byte, ed25519.PublicKeySize)
	if _, _, err := Verify(dist.EnvelopeBytes, map[string]ed25519.PublicKey{"other-key": ed25519.PublicKey(keyBytes)}); err != ErrUntrustedKey {
		t.Fatalf("unknown key id must be refused with ErrUntrustedKey, got %v", err)
	}
	if _, _, err := Verify(dist.EnvelopeBytes, map[string]ed25519.PublicKey{"test-key": ed25519.PublicKey(other)}); err != ErrBadSignature {
		t.Fatalf("wrong key bytes must be refused with ErrBadSignature, got %v", err)
	}
}
