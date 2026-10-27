//go:build linux

package portalgen

import (
	"os"
	"runtime"
	"syscall"
)

// syncfsTrap is syncfs(2) for the architectures we deploy; the standard
// syscall package does not export it. Zero selects the per-file fallback.
var syncfsTrap = map[string]uintptr{"amd64": 306, "arm64": 267}[runtime.GOARCH]

// releaseSyncOnce reports that one syncfs after placing a release replaces
// the per-file fsync: a national portal has ~52k files, and a journal
// commit per file dominated promotion time.
var releaseSyncOnce = syncfsTrap != 0

// syncFilesystem flushes the filesystem holding path.
func syncFilesystem(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if _, _, errno := syscall.Syscall(syncfsTrap, dir.Fd(), 0, 0); errno != 0 {
		return errno
	}
	return nil
}
