// Command map-sign creates one detached Ed25519 distribution envelope for a
// native-map manifest. The private key stays offline; the API only receives
// the public key.
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/mapdist"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "map-sign:", err)
		os.Exit(2)
	}
}

func run() error {
	manifestPath := flag.String("manifest", "", "native-map manifest JSON")
	keyPath := flag.String("key", "", "base64url raw Ed25519 private key (32-byte seed or 64-byte key)")
	keyID := flag.String("key-id", "", "pinned key identifier")
	out := flag.String("out", "", "distribution.json output path")
	replace := flag.Bool("replace", false, "atomically replace an existing envelope")
	flag.Parse()
	if *manifestPath == "" || *keyPath == "" || *keyID == "" || *out == "" {
		return errors.New("manifest, key, key-id and out are required")
	}
	manifest, err := os.ReadFile(*manifestPath)
	if err != nil {
		return err
	}
	rawKey, err := os.ReadFile(*keyPath)
	if err != nil {
		return err
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(rawKey)))
	if err != nil {
		return errors.New("key must be base64url raw Ed25519")
	}
	var privateKey ed25519.PrivateKey
	switch len(keyBytes) {
	case ed25519.SeedSize:
		privateKey = ed25519.NewKeyFromSeed(keyBytes)
	case ed25519.PrivateKeySize:
		privateKey = ed25519.PrivateKey(keyBytes)
	default:
		return errors.New("key must be a 32-byte seed or 64-byte private key")
	}
	envelope, err := mapdist.Sign(manifest, *keyID, privateKey)
	if err != nil {
		return err
	}
	if _, err := os.Stat(*out); err == nil && !*replace {
		return errors.New("output exists; pass --replace")
	}
	temp, err := os.CreateTemp(filepath.Dir(*out), ".map-sign-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(envelope); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(name, *out)
}
