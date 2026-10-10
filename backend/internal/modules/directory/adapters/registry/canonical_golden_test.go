package registry

// RST-07 cross-implementation pin: the canonical checksum recipe must
// produce the exact hex station-prep emits for the same typed values.
// The SIMP-0001 vector below is frozen by the Rust golden test; any
// recipe drift on either side breaks this test on purpose.

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestCanonicalChecksumMatchesRustGolden(t *testing.T) {
	fields := []string{
		"48151623000191", "SIMP-0001", "PRC-2024-0001",
		"[RST01-TEST] POSTO ALFA LTDA", "AV PAULISTA 1000", "",
		"BELA VISTA", "01310100", "SP", "SÃO PAULO", "BRANCA",
		"2024-03-01", "2024-03-10",
	}
	sum := sha256.Sum256([]byte(strings.Join(fields, "\x1f")))
	if got := hex.EncodeToString(sum[:]); got != "6f15da0b2f8e63778bd78b04e9955e208ee504cc195c686888f543a24e75fe23" {
		t.Fatalf("canonical checksum = %s, want the frozen Rust vector", got)
	}
}
