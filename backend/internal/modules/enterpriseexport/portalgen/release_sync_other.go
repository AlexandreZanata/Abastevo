//go:build !linux

package portalgen

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Without syncfs(2) every placed file is synced individually.
var releaseSyncOnce = false

func syncFilesystem(path string) error {
	return filepath.WalkDir(path, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		return file.Sync()
	})
}
