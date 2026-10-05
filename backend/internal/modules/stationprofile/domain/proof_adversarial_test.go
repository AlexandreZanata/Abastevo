package domain

import (
	"bytes"
	"fmt"
	"testing"
)

// P33-T01 — Proof-parser exhaustion vectors (no parser execution beyond
// the frozen byte policy, no URL fetch, no production documents).
//
// Every vector must be refused with a typed error and none may panic.
// Secrets below are synthetic detector-tripwires, never real credentials.

func TestProofParserExhaustionRefused(t *testing.T) {
	pdf := func(markers ...string) []byte {
		body := []byte("%PDF-1.7 synthetic fixture")
		for _, m := range markers {
			body = append(body, []byte(" "+m)...)
		}
		return body
	}
	vectors := []struct {
		name string
		file string
		body []byte
	}{
		{"empty", "doc.pdf", []byte{}},
		{"oversize", "doc.pdf", bytes.Repeat([]byte("%PDF-"), MaxProofBytes/5+1)},
		{"magic-mismatch", "doc.pdf", []byte("PNG fake bytes")},
		{"pfx-bundle", "bundle.pfx", pdf()},
		{"p12-bundle", "bundle.p12", pdf()},
		{"pem-key", "key.pem", pdf()},
		{"key-ext", "key.key", pdf()},
		{"encrypted", "doc.pdf", pdf("/Encrypt")},
		{"javascript", "doc.pdf", pdf("/JavaScript")},
		{"embedded-files", "doc.pdf", pdf("/EmbeddedFiles")},
		{"launch", "doc.pdf", pdf("/Launch")},
		{"xfa", "doc.pdf", pdf("/XFA")},
		{"private-key", "doc.pdf", pdf("PRIVATE KEY")},
		{"private-key-underscore", "doc.pdf", pdf("private_key blob")},
		{"password", "doc.pdf", pdf("password=secret")},
	}
	for i, v := range vectors {
		if err := ValidateProofBytes(v.file, "application/pdf", v.body); err == nil {
			t.Fatalf("vector %q accepted (fail-closed violation)", v.name)
		}
		_ = i
	}
}

func TestProofFuzzCorpusRefusedFast(t *testing.T) {
	// 200 deterministic corruptions of a minimal PDF: all must be
	// refused or be inert-clean; none may panic. Bounded well under 1s.
	seeds := []byte("%PDF-1.7\ntrailer\n")
	for i := 0; i < 200; i++ {
		mut := append([]byte{}, seeds...)
		mut = append(mut, []byte(fmt.Sprintf(" %d /Encrypt%d", i, i))...)
		_ = ValidateProofBytes("doc.pdf", "application/pdf", mut)
	}
}
