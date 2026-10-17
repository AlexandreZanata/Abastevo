package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestNativeQualityFetchPreflightAndCancelledCleanup(t *testing.T) {
	root := t.TempDir()
	fetch := nativeQualityFetch(root)
	for _, url := range []string{"https://example.invalid/pmqc_2026_09.zip", "https://www.gov.br/private?source-secret=token", "file:///tmp/private"} {
		if _, e := fetch(context.Background(), url); e == nil || strings.Contains(e.Error(), "source-secret") {
			t.Fatal("foreign download accepted or leaked", e)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := fetch(ctx, "https://www.gov.br"+qualityArchivePrefix+"pmqc_2026_09.zip"); e == nil {
		t.Fatal("cancelled download accepted")
	}
	children, e := os.ReadDir(root)
	if e != nil || len(children) != 0 {
		t.Fatal("fetch workspace retained", children, e)
	}
}
