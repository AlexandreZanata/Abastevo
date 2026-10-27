package main

import (
	"bytes"
	"strings"
	"testing"

	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
)

// TestAdminKeygen_ADM02 proves keygen prints a valid key/hash pair, never
// repeats a key, and writes only to the provided stdout.
func TestAdminKeygen_ADM02(t *testing.T) {
	run := func() (string, string) {
		var out bytes.Buffer
		if err := runAdmin([]string{"keygen"}, &out); err != nil {
			t.Fatal(err)
		}
		var key, hash string
		for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			switch {
			case strings.HasPrefix(line, "key="):
				key = strings.TrimPrefix(line, "key=")
			case strings.HasPrefix(line, "sha256="):
				hash = strings.TrimPrefix(line, "sha256=")
			}
		}
		return key, hash
	}
	firstKey, firstHash := run()
	secondKey, secondHash := run()
	if !admindomain.ValidKey(firstKey) || admindomain.HashKey(firstKey) != firstHash {
		t.Fatal("keygen output mismatch")
	}
	if !admindomain.ValidKey(secondKey) || admindomain.HashKey(secondKey) != secondHash {
		t.Fatal("second keygen output mismatch")
	}
	if firstKey == secondKey || firstHash == secondHash {
		t.Fatal("keygen repeated a key")
	}
	if err := runAdmin(nil, &bytes.Buffer{}); err == nil {
		t.Fatal("missing keygen command accepted")
	}
}
