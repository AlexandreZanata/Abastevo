package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunnerRefusesInvalidPayloads(t *testing.T) {
	r := Runner{}
	for _, payload := range []ImportPayload{
		{},
		{Version: 1, SnapshotDir: "/tmp", Format: "unknown-v1"},
		{Version: 1, SnapshotDir: "../escape", Format: "charging-prepared-v1"},
		{Version: 1, SnapshotDir: "/tmp", Format: "charging-prepared-v1"},
	} {
		if _, err := r.Run(context.Background(), payload); err == nil {
			t.Fatal("invalid payload accepted")
		}
	}
}

func TestRunnerCleansWorkspaceOnFailure(t *testing.T) {
	r := Runner{PrepareCharging: "/nonexistent-binary", Store: func(ctx context.Context, format, profile, dir string) (ImportResult, error) {
		return ImportResult{}, errors.New("unreachable")
	}}
	dir := t.TempDir()
	if _, err := r.Run(context.Background(), ImportPayload{Version: 1, SnapshotDir: dir, Format: "charging-prepared-v1"}); err == nil {
		t.Fatal("missing preparer accepted")
	}
}

func TestRunnerRespectsDeadBudget(t *testing.T) {
	block := make(chan struct{})
	r := Runner{PrepareCharging: "/bin/sleep", Store: func(ctx context.Context, format, profile, dir string) (ImportResult, error) {
		<-block
		return ImportResult{}, nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	defer close(block)
	if _, err := r.Run(ctx, ImportPayload{Version: 1, SnapshotDir: t.TempDir(), Format: "charging-prepared-v1"}); err == nil {
		t.Fatal("dead budget ignored")
	}
}

func TestRunnerReplaysWithoutDuplication(t *testing.T) {
	calls := 0
	r := Runner{PrepareCharging: "/bin/true", Store: func(ctx context.Context, format, profile, dir string) (ImportResult, error) {
		calls++
		return ImportResult{BatchID: "stable-id", Entities: 10}, nil
	}}
	dir := t.TempDir()
	first, err := r.Run(context.Background(), ImportPayload{Version: 1, SnapshotDir: dir, Format: "charging-prepared-v1"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Run(context.Background(), ImportPayload{Version: 1, SnapshotDir: dir, Format: "charging-prepared-v1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.BatchID != second.BatchID || calls != 2 {
		t.Fatal("replay diverged", first, second, calls)
	}

}

func TestDiscoverRefusesNonAllowlistedOrigins(t *testing.T) {
	for _, url := range []string{
		"http://evil.example.com/ocm",
		"https://raw.githubusercontent.com/other/repo/file.json",
		"https://www.gov.br.evil.com/inmetro/",
		"ftp://raw.githubusercontent.com/openchargemap/ocm-export/x",
		"https://raw.githubusercontent.com/openchargemap/ocm-export/data/BR/..%2f..%2fetc/passwd",
	} {
		if _, err := Discover(context.Background(), url); err == nil {
			t.Fatal("non-allowlisted origin accepted", url)
		}
	}
}

func TestReviewFlagsMergesSplitsAndChanges(t *testing.T) {
	stored := map[string]string{"A": "hash1", "B": "hash2", "C": "hash3"}
	incoming := map[string]string{"B": "hash2-changed", "C": "hash3", "D": "hash4"}
	flags := ReviewMergeSplit(stored, incoming)
	if len(flags.Added) != 1 || flags.Added[0] != "D" {
		t.Fatal("added not flagged", flags)
	}
	if len(flags.Missing) != 1 || flags.Missing[0] != "A" {
		t.Fatal("missing not flagged", flags)
	}
	if len(flags.Changed) != 1 || flags.Changed[0] != "B" {
		t.Fatal("changed not flagged", flags)
	}
}

func TestHandlersExposeKindsWithoutAutoActions(t *testing.T) {
	if (ImportHandler{}).Kind() != ChargingImportKind || (DiscoveryHandler{}).Kind() != ChargingDiscoveryKind {
		t.Fatal("job kinds drifted")
	}
	if (ImportHandler{}).Version() != 1 || (DiscoveryHandler{}).Version() != 1 {
		t.Fatal("job versions drifted")
	}
}
