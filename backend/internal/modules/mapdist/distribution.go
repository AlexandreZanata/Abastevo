// Package mapdist verifies and serves signed native-map distributions.
// It never signs with a private key at request time and never serves private
// media or arbitrary filesystem paths.
package mapdist

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/mapcontract"
)

const DistributionFormat = "abastevo.map-distribution-v1"

var (
	ErrInvalidEnvelope = errors.New("mapdist: invalid envelope")
	ErrUntrustedKey    = errors.New("mapdist: untrusted key")
	ErrBadSignature    = errors.New("mapdist: bad signature")
	ErrBadManifest     = errors.New("mapdist: bad manifest")
	ErrBadPackage      = errors.New("mapdist: bad package")
)

type Envelope struct {
	Format         string `json:"format"`
	Version        int    `json:"version"`
	KeyID          string `json:"key_id"`
	Algorithm      string `json:"algorithm"`
	Manifest       string `json:"manifest"`
	ManifestSHA256 string `json:"manifest_sha256"`
	Signature      string `json:"signature"`
}

type Package struct {
	ID     string
	Role   string
	Path   string
	Bytes  int64
	SHA256 string
}

type Distribution struct {
	EnvelopeBytes []byte
	ManifestBytes []byte
	Manifest      *mapcontract.NativeManifest
	Packages      map[string]Package
	Files         map[string]string
}

// Sign creates the detached distribution envelope. The private key is only
// used by the build-time signer.
func Sign(manifest []byte, keyID string, privateKey ed25519.PrivateKey) ([]byte, error) {
	if len(privateKey) != ed25519.PrivateKeySize || strings.TrimSpace(keyID) == "" {
		return nil, ErrInvalidEnvelope
	}
	if _, err := mapcontract.ParseNativeManifest(manifest); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}
	sum := sha256.Sum256(manifest)
	envelope := Envelope{
		Format:         DistributionFormat,
		Version:        1,
		KeyID:          keyID,
		Algorithm:      "Ed25519",
		Manifest:       base64.RawURLEncoding.EncodeToString(manifest),
		ManifestSHA256: hex.EncodeToString(sum[:]),
		Signature:      base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, manifest)),
	}
	return json.MarshalIndent(envelope, "", "  ")
}

// Verify checks the envelope, key id, hash and signature and returns the exact
// manifest bytes.
func Verify(envelopeBytes []byte, keys map[string]ed25519.PublicKey) ([]byte, *mapcontract.NativeManifest, error) {
	var envelope Envelope
	dec := json.NewDecoder(bytes.NewReader(envelopeBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&envelope); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	if envelope.Format != DistributionFormat || envelope.Version != 1 || envelope.Algorithm != "Ed25519" ||
		strings.TrimSpace(envelope.KeyID) == "" {
		return nil, nil, ErrInvalidEnvelope
	}
	publicKey, ok := keys[envelope.KeyID]
	if !ok || len(publicKey) != ed25519.PublicKeySize {
		return nil, nil, ErrUntrustedKey
	}
	manifest, err := base64.RawURLEncoding.DecodeString(envelope.Manifest)
	if err != nil {
		return nil, nil, ErrInvalidEnvelope
	}
	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, nil, ErrInvalidEnvelope
	}
	sum := sha256.Sum256(manifest)
	if hex.EncodeToString(sum[:]) != envelope.ManifestSHA256 {
		return nil, nil, ErrBadManifest
	}
	if !ed25519.Verify(publicKey, manifest, signature) {
		return nil, nil, ErrBadSignature
	}
	parsed, err := mapcontract.ParseNativeManifest(manifest)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}
	return manifest, parsed, nil
}

// Load verifies one distribution directory and all package bytes before any
// request is served. Missing, oversized or digest-mismatched packages fail
// closed.
func Load(dir string, keys map[string]ed25519.PublicKey) (*Distribution, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	envelopeBytes, err := os.ReadFile(filepath.Join(dir, "distribution.json"))
	if err != nil {
		return nil, err
	}
	manifestBytes, manifest, err := Verify(envelopeBytes, keys)
	if err != nil {
		return nil, err
	}
	dist := &Distribution{
		EnvelopeBytes: envelopeBytes,
		ManifestBytes: manifestBytes,
		Manifest:      manifest,
		Packages:      map[string]Package{},
		Files:         map[string]string{},
	}
	for _, pkg := range manifest.Packages {
		path, err := safeJoin(dir, pkg.Path)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: %s", ErrBadPackage, pkg.ID)
		}
		if info.Size() != pkg.Bytes {
			return nil, fmt.Errorf("%w: %s size", ErrBadPackage, pkg.ID)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrBadPackage, pkg.ID)
		}
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != pkg.SHA256 {
			return nil, fmt.Errorf("%w: %s digest", ErrBadPackage, pkg.ID)
		}
		dist.Packages[pkg.ID] = Package{ID: pkg.ID, Role: pkg.Role, Path: pkg.Path, Bytes: pkg.Bytes, SHA256: pkg.SHA256}
		dist.Files[pkg.ID] = path
	}
	return dist, nil
}

func safeJoin(root, name string) (string, error) {
	if filepath.IsAbs(name) || strings.Contains(name, "..") {
		return "", fmt.Errorf("%w: unsafe path", ErrBadPackage)
	}
	clean := filepath.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("%w: unsafe path", ErrBadPackage)
	}
	full := filepath.Join(root, clean)
	relative, err := filepath.Rel(root, full)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: unsafe path", ErrBadPackage)
	}
	return full, nil
}

// ParsePublicKeys parses keyid:base64url,keyid:base64url.
func ParsePublicKeys(raw string) (map[string]ed25519.PublicKey, error) {
	keys := map[string]ed25519.PublicKey{}
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
			return nil, ErrInvalidEnvelope
		}
		rawKey, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(parts[1]))
		if err != nil || len(rawKey) != ed25519.PublicKeySize {
			return nil, ErrInvalidEnvelope
		}
		keys[parts[0]] = ed25519.PublicKey(rawKey)
	}
	if len(keys) == 0 {
		return nil, ErrInvalidEnvelope
	}
	return keys, nil
}
