package registrysync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCNPJUpdatePublishesIndependentSnapshotsRetainsRollbackAndFailedWork(t *testing.T) {
	root := t.TempDir()
	version := "first"
	fail := false
	verifies := 0
	u := SourceUpdater{Root: root, Binary: "synthetic", Info: func(context.Context, string) ([]byte, error) { return []byte(`{"competence":"2026-10"}`), nil }}
	u.Invoke = func(_ context.Context, stage, _ string, args []string) error {
		if stage == "rfb_verify" {
			verifies++
			if fail {
				return errors.New("synthetic verification failure")
			}
			return nil
		}
		work := filepath.Join(root, "work")
		_ = os.MkdirAll(filepath.Join(work, "snapshots"), 0700)
		path := filepath.Join(work, "snapshots", "2026-10-balanced.ocnpj")
		raw := []byte(version)
		sum := sha256.Sum256(raw)
		for p, data := range map[string][]byte{path: raw, path + ".sha256": []byte(hex.EncodeToString(sum[:])), filepath.Join(work, "current"): []byte("snapshots/2026-10-balanced.ocnpj\n")} {
			if err := os.WriteFile(p, data, 0600); err != nil {
				return err
			}
		}
		return nil
	}
	if err := u.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, _, err := currentOpenCNPJ(root)
	if err != nil {
		t.Fatal(err)
	}
	version = "corrected same month"
	fail = true
	if u.Update(context.Background()) == nil {
		t.Fatal("failed build published")
	}
	if path, _, err := currentOpenCNPJ(root); err != nil || path != first {
		t.Fatal("failed workspace replaced published source", err)
	}
	if raw, _ := os.ReadFile(first); string(raw) != "first" {
		t.Fatal("published source shared workspace inode")
	}
	fail = false
	if err := u.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	second, _, err := currentOpenCNPJ(root)
	if err != nil || second == first {
		t.Fatal("corrected month did not publish", err)
	}
	if err := u.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal("unchanged check deleted rollback snapshot", err)
	}
	if verifies != 3 {
		t.Fatal("unchanged source was reverified or changed source skipped verification", verifies)
	}
	version = "third"
	if err := u.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("retention kept more than current and previous")
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatal("immediate rollback source missing")
	}
}

func TestOpenCNPJSourceBudgetUnownedRootAndCopyCancellation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "foreign"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	u := SourceUpdater{Root: root, Invoke: func(context.Context, string, string, []string) error { t.Fatal("unowned root mutated"); return nil }, Info: func(context.Context, string) ([]byte, error) { return nil, nil }}
	if u.Update(context.Background()) == nil {
		t.Fatal("unowned root accepted")
	}
	sparse := filepath.Join(t.TempDir(), "synthetic.sparse")
	f, err := os.Create(sparse)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(OpenCNPJSourceCap + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if CheckOpenCNPJDisk(filepath.Dir(sparse)) == nil {
		t.Fatal("source exceeded disk budget")
	}
	source := filepath.Join(t.TempDir(), "synthetic")
	_ = os.WriteFile(source, []byte("fixture"), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	target := filepath.Join(t.TempDir(), "snapshot.ocnpj")
	if copyOpenCNPJ(ctx, source, target, "invalid") == nil {
		t.Fatal("cancelled source copied")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("partial snapshot visible")
	}
}

func TestInterruptedSnapshotCopyRecoveryPreservesNeighborsAndRefusesLinks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "snapshots")
	_ = os.Mkdir(dir, 0700)
	for _, name := range []string{".snapshot-123", "manual-note"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte("synthetic"), 0600)
	}
	if err := cleanOpenCNPJPartialCopies(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".snapshot-123")); !os.IsNotExist(err) {
		t.Fatal("interrupted copy retained")
	}
	if _, err := os.Stat(filepath.Join(dir, "manual-note")); err != nil {
		t.Fatal("neighbor deleted")
	}
	_ = os.Symlink(filepath.Join(dir, "manual-note"), filepath.Join(dir, ".snapshot-456"))
	if cleanOpenCNPJPartialCopies(root) == nil {
		t.Fatal("unsafe partial copy accepted")
	}
}

func TestSourceRetentionRemovesOnlyProvenOldBuildInputs(t *testing.T) {
	root := t.TempDir()
	_ = os.Mkdir(filepath.Join(root, "snapshots"), 0700)
	for _, month := range []string{"2026-09", "2026-10"} {
		dir := filepath.Join(root, "work", "raw", month)
		_ = os.MkdirAll(dir, 0700)
		raw := []byte(`{"competence":"` + month + `","base_url":"https://arquivos.receitafederal.gov.br/public.php/webdav"}`)
		_ = os.WriteFile(filepath.Join(dir, "source-manifest.json"), raw, 0600)
	}
	manual := filepath.Join(root, "work", "raw", "manual")
	_ = os.Mkdir(manual, 0700)
	if err := pruneOpenCNPJ(root, "", "", "2026-10"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "work", "raw", "2026-09")); !os.IsNotExist(err) {
		t.Fatal("old reproducible build inputs retained")
	}
	for _, name := range []string{"2026-10", "manual"} {
		if _, err := os.Stat(filepath.Join(root, "work", "raw", name)); err != nil {
			t.Fatal("current or manual data deleted", err)
		}
	}
	dir := filepath.Join(root, "work", "raw", "2026-08")
	_ = os.Mkdir(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "source-manifest.json"), []byte(`{"competence":"2026-08","base_url":"https://unowned.example"}`), 0600)
	if pruneOpenCNPJ(root, "", "", "2026-10") == nil {
		t.Fatal("unowned historical inputs removed")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatal("unowned directory deleted")
	}
}
