package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
)

const adminUsage = "usage: ops admin keygen"

// runAdmin owns the offline key ceremony. keygen prints the raw key and its
// SHA-256 once to stdout and writes nothing to disk (ADM-02).
func runAdmin(args []string, out io.Writer) error {
	if len(args) != 1 || args[0] != "keygen" {
		return errors.New(adminUsage)
	}
	raw := make([]byte, admindomain.KeyRandom)
	if _, err := rand.Read(raw); err != nil {
		return errors.New("admin key generation failed")
	}
	key := admindomain.KeyPrefix + base64.RawURLEncoding.EncodeToString(raw)
	if !admindomain.ValidKey(key) {
		return errors.New("admin key generation failed")
	}
	if _, err := fmt.Fprintf(out, "key=%s\nsha256=%s\n", key, admindomain.HashKey(key)); err != nil {
		return errors.New("admin key output failed")
	}
	return nil
}
