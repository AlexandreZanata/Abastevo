package native

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnforcementExactSelectionListingAndTrust(t *testing.T) {
	if ValidateEnforcementSelection(EnforcementResourceURL, application.EnforcementSourceEdition) != nil {
		t.Fatal("reviewed resource refused")
	}
	for _, raw := range []string{strings.Replace(EnforcementResourceURL, "https:", "http:", 1), EnforcementResourceURL + "?token=private", EnforcementResourceURL + "#fragment", strings.Replace(EnforcementResourceURL, "www.gov.br", "www.gov.br.evil", 1), strings.Replace(EnforcementResourceURL, "2019.xlsx", "2020.xlsx", 1), strings.Replace(EnforcementResourceURL, "www.gov.br", "user@www.gov.br", 1), strings.Replace(EnforcementResourceURL, "www.gov.br", "www.gov.br:443", 1)} {
		if ValidateEnforcementSelection(raw, application.EnforcementSourceEdition) == nil {
			t.Fatal("unsafe selection admitted")
		}
	}
	if ValidateEnforcementSelection(EnforcementResourceURL, "2019+") == nil {
		t.Fatal("invented source edition admitted")
	}
	if ParseEnforcementListing(EnforcementListingPage, []byte(`<a href="`+EnforcementResourceURL+`">untrusted label</a>`)) != nil {
		t.Fatal("reviewed href refused")
	}
	for _, body := range []string{`<a href="https://evil.invalid/source.xlsx">source</a>`, strings.Repeat("x", MaxListingBytes+1), ""} {
		if ParseEnforcementListing(EnforcementListingPage, []byte(body)) == nil {
			t.Fatal("unsafe listing admitted")
		}
	}
	root := t.TempDir()
	binary := filepath.Join(root, "preparer")
	if e := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0775); e != nil {
		t.Fatal(e)
	}
	if _, e := (EnforcementRunner{Binary: binary}).Prepare(context.Background(), FineRequest{}); e == nil {
		t.Fatal("writable preparer admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := (EnforcementRunner{Binary: binary}).Prepare(ctx, FineRequest{}); e != context.Canceled {
		t.Fatal("cancellation not preserved", e)
	}
}

func TestEnforcementArtifactsRequirePrivateRegularFiles(t *testing.T) {
	root := t.TempDir()
	name := "enforcement-source-records.jsonl"
	p := filepath.Join(root, name)
	if e := os.WriteFile(p, []byte("synthetic"), 0600); e != nil {
		t.Fatal(e)
	}
	f, e := openPrivateArtifact(root, name, 200<<20)
	if e != nil {
		t.Fatal(e)
	}
	f.Close()
	if e = os.Chmod(p, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = openPrivateArtifact(root, name, 200<<20); e == nil {
		t.Fatal("public artifact admitted")
	}
	if _, e = openPrivateArtifact(root, "../"+name, 200<<20); e == nil {
		t.Fatal("path traversal admitted")
	}
}
