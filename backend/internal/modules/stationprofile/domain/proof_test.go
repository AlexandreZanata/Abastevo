package domain

import (
	"strings"
	"testing"
)

// P30-T04 private proof-byte validation (stdlib-only domain). PDF only
// by magic bytes, 5 MB cap, encrypted/active content refused,
// key-bundle material refused outright. Validation never fetches URLs
// and never rewrites bytes (rasterization would destroy evidence).

func TestProofFormatRules(t *testing.T) {
	pdf := append([]byte("%PDF-1.7\n"), bytesOf(100)...)
	if err := ValidateProofBytes("autorizacao.pdf", "application/pdf", pdf); err != nil {
		t.Fatalf("valid pdf: %v", err)
	}
	for _, tc := range []struct {
		name, filename, contentType string
		body                        []byte
	}{
		{"text file", "nota.txt", "text/plain", []byte("hello")},
		{"jpeg bytes", "foto.jpg", "image/jpeg", append([]byte("\xff\xd8\xff"), bytesOf(100)...)},
		{"pfx bundle", "cert.pfx", "application/x-pkcs12", append([]byte("pfx-magic"), bytesOf(100)...)},
		{"p12 bundle", "cert.p12", "application/pdf", append([]byte("%PDF-"), []byte("private-key-password")...)},
		{"encrypted", "doc.pdf", "application/pdf", append([]byte("%PDF-1.7 /Encrypt <<>>"), bytesOf(100)...)},
		{"javascript", "doc.pdf", "application/pdf", append([]byte("%PDF-1.7 /JavaScript (alert)"), bytesOf(100)...)},
		{"embedded", "doc.pdf", "application/pdf", append([]byte("%PDF-1.7 /EmbeddedFiles"), bytesOf(100)...)},
		{"empty", "doc.pdf", "application/pdf", nil},
		{"oversize", "doc.pdf", "application/pdf", append([]byte("%PDF-"), bytesOf(MaxProofBytes+1)...)},
	} {
		if err := ValidateProofBytes(tc.filename, tc.contentType, tc.body); err == nil {
			t.Fatalf("%s must be refused", tc.name)
		}
	}
}

func TestProofKindExpiry(t *testing.T) {
	if ExpiryForKind("authorization") <= 0 {
		t.Fatal("authorization needs retention")
	}
	if ExpiryForKind("scan") != ScanRetention {
		t.Fatal("scans obey the 24h all-copy cap")
	}
	if ExpiryForKind("mystery") != 0 {
		t.Fatal("unknown kinds must fail closed")
	}
}

func bytesOf(n int) []byte { return []byte(strings.Repeat("a", n)) }
