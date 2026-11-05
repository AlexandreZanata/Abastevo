package registrysync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func nameFixture(t *testing.T, dir, name string) string {
	t.Helper()
	data := []byte(name + "\n")
	hash := sha256.Sum256(data)
	if err := os.WriteFile(filepath.Join(dir, "rfb.jsonl"), data, 0600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"format_version": "registry-name-inputs-v1", "inputs": []any{map[string]any{
		"source": "rfb", "profile": "rfb-station-names-v1", "file": "rfb.jsonl", "reference": "https://example.invalid/rfb",
		"edition": "synthetic", "sha256": hex.EncodeToString(hash[:]), "bytes": len(data), "rows": 1,
	}}})
	path := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNameInputsAreFrozenResumedAndFingerprintNewEditions(t *testing.T) {
	r, fetches, applied, verified := fixture(t)
	dir := t.TempDir()
	r.NameInputs = nameFixture(t, dir, "synthetic first")
	prepare := r.Prepare
	var prepared string
	r.Prepare = func(ctx context.Context, frozen string, s Snapshot) error {
		if s.NameInputsHash == "" {
			t.Fatal("name input identity missing")
		}
		data, err := os.ReadFile(filepath.Join(frozen, "names", "rfb.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		prepared = string(data)
		return prepare(ctx, frozen, s)
	}
	r.Apply = func(context.Context, string) error { return errors.New("synthetic interruption") }
	if r.Run(context.Background()) == nil {
		t.Fatal("failure hidden")
	}
	before := *fetches
	nameFixture(t, dir, "synthetic next")
	r.Apply = func(context.Context, string) error { *applied++; return nil }
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if prepared != "synthetic first\n" || *fetches != before {
		t.Fatal("pending input was rebound")
	}
	r.Now = func() time.Time { return time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC) }
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *applied != 2 || prepared != "synthetic next\n" {
		t.Fatal("new names did not trigger preparation")
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *applied != 2 || *verified != 1 {
		t.Fatal("identical enrichment was not verified idempotently")
	}
	r.NameInputs = ""
	if r.Run(context.Background()) == nil {
		t.Fatal("configured names silently removed")
	}
}

func TestCorruptPendingNamesCannotPublish(t *testing.T) {
	r, _, _, _ := fixture(t)
	r.NameInputs = nameFixture(t, t.TempDir(), "synthetic")
	r.Apply = func(context.Context, string) error { return errors.New("synthetic interruption") }
	_ = r.Run(context.Background())
	s, err := readState(r.Root)
	if err != nil || s.Pending == nil {
		t.Fatal("pending snapshot missing", err)
	}
	if err := os.WriteFile(filepath.Join(r.Root, s.Pending.Key, "names", "rfb.jsonl"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	r.Apply = func(context.Context, string) error { t.Fatal("corrupt input published"); return nil }
	if r.Run(context.Background()) == nil {
		t.Fatal("corrupt names accepted")
	}
}

func TestUnsafeNameInputsFailBeforePublication(t *testing.T) {
	for _, mode := range []string{"checksum", "traversal", "symlink", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			r, _, applied, _ := fixture(t)
			dir := t.TempDir()
			r.NameInputs = nameFixture(t, dir, "synthetic")
			switch mode {
			case "checksum":
				_ = os.WriteFile(filepath.Join(dir, "rfb.jsonl"), []byte("different"), 0600)
			case "symlink":
				_ = os.Remove(filepath.Join(dir, "rfb.jsonl"))
				_ = os.Symlink("missing", filepath.Join(dir, "rfb.jsonl"))
			case "traversal", "oversize":
				var m map[string]any
				raw, _ := os.ReadFile(r.NameInputs)
				_ = json.Unmarshal(raw, &m)
				pin := m["inputs"].([]any)[0].(map[string]any)
				if mode == "traversal" {
					pin["file"] = "../outside"
				} else {
					pin["bytes"] = 65 << 20
				}
				raw, _ = json.Marshal(m)
				_ = os.WriteFile(r.NameInputs, raw, 0600)
			}
			if r.Run(context.Background()) == nil || *applied != 0 {
				t.Fatal("unsafe names published")
			}
		})
	}
}
