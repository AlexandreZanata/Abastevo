//go:build integration && qualitysource

package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
)

// Explicit manual live-source smoke only, excluded from deterministic CI.
// The immutable expected source identity belongs to the acceptance candidate;
// a changed remote resource requires fresh reconciliation, never a relaxed test.
func TestOfficialFineWorkerNetworkBoundary(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH")
	if binary == "" {
		t.Fatal("trusted fine CLI required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	refs, e := fetchFineListing(ctx)
	if e != nil {
		t.Fatal(e)
	}
	var selected native.FineRef
	for _, ref := range refs {
		if ref.Year == "2025" {
			selected = ref
		}
	}
	if selected.URL == "" {
		t.Fatal("verified 2025 href no longer listed")
	}
	root := t.TempDir()
	file, e := nativeFineFetch(root)(ctx, selected.URL)
	if e != nil {
		t.Fatal(e)
	}
	closed := false
	defer func() {
		if !closed && file.Close != nil {
			if e := file.Close(); e != nil {
				t.Error(e)
			}
		}
	}()
	if file.Identity.SHA256 != "277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96" || file.Identity.Bytes != 450504 {
		t.Fatal("remote fine source changed; reconcile a new source candidate")
	}
	batch, e := (native.FineRunner{Binary: binary, WorkspaceRoot: root}).Prepare(ctx, native.FineRequest{Path: file.Path, URL: selected.URL, Edition: selected.Year, Identity: file.Identity})
	if e != nil || batch.Counts.Input != 3395 || batch.Counts.Accepted != 3375 || batch.Counts.Duplicates != 0 || batch.Counts.Quarantined != 20 || batch.ExtractionMonth != "2026-01" {
		t.Fatal("live source reconciliation failed", e)
	}
	if e = file.Close(); e != nil {
		t.Fatal(e)
	}
	closed = true
	children, e := os.ReadDir(root)
	if e != nil || len(children) != 0 {
		t.Fatal("private live source retained", e)
	}
	t.Logf("listed annual CSVs=%d; immutable 2025 source reconciled: 3395=3375+0+20", len(refs))
}
