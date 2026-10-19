//go:build integration && qualitysource

package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Explicit manual source acceptance: never downloads or prints private rows.
// The exact source watermark is pinned; drift needs a fresh source review.
func TestOfficialFineRustGoBoundary(t *testing.T) {
	binary := os.Getenv("ANPFUEL_TEST_SANCTIONS_PREPARER_PATH")
	source := os.Getenv("ANPFUEL_TEST_SANCTIONS_SOURCE_PATH")
	if !filepath.IsAbs(binary) || !filepath.IsAbs(source) {
		t.Fatal("absolute trusted preparer and private source paths are required")
	}
	st, e := os.Lstat(binary)
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0111 == 0 || st.Mode().Perm()&0022 != 0 {
		t.Fatal("trusted preparer unavailable")
	}
	raw, e := os.ReadFile(source)
	if e != nil {
		t.Fatal("private source unavailable")
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	if hash != "277f6cf5c59716581c42c5068fde47827c28eb693ee643c75d7841bda191fe96" || len(raw) != 450504 {
		t.Fatal("official2025 source watermark differs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out := filepath.Join(t.TempDir(), "prepared")
	command := exec.CommandContext(ctx, binary, source, "2025", out)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if command.Run() != nil {
		t.Fatal("native fine preparation refused")
	}
	manifest, e := os.ReadFile(filepath.Join(out, "manifest.json"))
	if e != nil {
		t.Fatal("native manifest unavailable")
	}
	batch, e := LoadFines(ctx, manifest, func(name string) (io.ReadCloser, error) { return os.Open(filepath.Join(out, name)) }, InputIdentity{SHA256: hash, Bytes: int64(len(raw))}, 2025)
	if e != nil {
		t.Fatal("real native artifact boundary", e)
	}
	if batch.Counts.Input != 3395 || batch.Counts.Accepted != 3375 || batch.Counts.Duplicates != 0 || batch.Counts.Quarantined != 20 || len(batch.Observations) != 3375 || batch.ExtractionMonth != "2026-01" {
		t.Fatal("real loss/precision reconciliation differs", batch.Counts)
	}
	for _, row := range batch.Observations {
		if len(row.SourceCNPJ) != 14 || len(row.ObservationKey) != 76 || row.ReportYear != 2025 || row.ExtractionMonth != "2026-01" || row.DueOn == "" {
			t.Fatal("source observation field mismatch")
		}
	}
	t.Log("3395 input = 3375 accepted + 0 duplicate + 20 CPF quarantine; Rust/Go artifact, money, dates and row census verified")
}
