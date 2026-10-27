package domain

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNeutrality_CTB14_NoTrustRankingImports keeps contributor status out of
// trust, moderation and community ranking code.
func TestNeutrality_CTB14_NoTrustRankingImports(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	modules := filepath.Join(filepath.Dir(file), "..", "..")
	for _, owner := range []string{"trust", "moderation", "community"} {
		root := filepath.Join(modules, owner)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(raw), "modules/contributor") {
				t.Errorf("%s imports contributor", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
