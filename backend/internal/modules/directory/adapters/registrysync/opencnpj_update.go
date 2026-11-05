package registrysync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const OpenCNPJSourceCap = int64(64 << 30)
const sourceOwner = "abastevo-opencnpj-source-v1\n"

// CheckOpenCNPJDisk stops the updater before it consumes the shared host's
// reserve. No symlink is followed and unrelated paths are never traversed.
func CheckOpenCNPJDisk(root string) error {
	var size int64
	entries := 0
	return filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("OpenCNPJ disk telemetry unavailable")
		}
		entries++
		if entries > 100000 || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe OpenCNPJ source tree")
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("OpenCNPJ disk telemetry unavailable")
		}
		if info.Mode().IsRegular() {
			size += info.Size()
		}
		if size > OpenCNPJSourceCap {
			return fmt.Errorf("OpenCNPJ private disk budget exceeded")
		}
		return nil
	})
}

// SourceUpdater keeps OpenCNPJ's mutable build workspace separate from the
// immutable snapshots read by the daily registry job. Corrected bytes for an
// existing month can never overwrite the last published snapshot's inode.
type SourceUpdater struct {
	Root, Binary string
	Now          func() time.Time
	Invoke       func(context.Context, string, string, []string) error
	Info         func(context.Context, string) ([]byte, error)
}

func (u SourceUpdater) Update(ctx context.Context) error {
	if u.Invoke == nil || u.Info == nil {
		return fmt.Errorf("OpenCNPJ updater ports missing")
	}
	unlock, err := LockOpenCNPJ(u.Root, true)
	if err != nil {
		return err
	}
	defer unlock()
	owner := filepath.Join(u.Root, "owner.txt")
	if raw, err := smallFile(owner, 128); os.IsNotExist(err) {
		entries, err := os.ReadDir(u.Root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.Name() != ".opencnpj.lock" {
				return fmt.Errorf("refusing unowned OpenCNPJ source root")
			}
		}
		if err = atomicFile(u.Root, "owner.txt", []byte(sourceOwner)); err != nil {
			return err
		}
	} else if err != nil || string(raw) != sourceOwner {
		return fmt.Errorf("unowned OpenCNPJ source root")
	}
	if err = cleanOpenCNPJPartialCopies(u.Root); err != nil {
		return err
	}
	if err = CheckOpenCNPJDisk(u.Root); err != nil {
		return err
	}
	work := filepath.Join(u.Root, "work")
	if err = os.MkdirAll(work, 0700); err != nil {
		return err
	}
	args := []string{"update", "--data-dir", work, "--profile", "balanced", "--verify-sources", "--parallel", "2", "--retries", "2", "--keep", "1"}
	if err = u.Invoke(ctx, "rfb_update", u.Binary, args); err != nil {
		return err
	}
	snapshot, hash, err := currentOpenCNPJ(work)
	if err != nil {
		return err
	}
	metadata, err := u.Info(ctx, snapshot)
	if err != nil {
		return err
	}
	var info struct {
		Competence string `json:"competence"`
	}
	if len(metadata) > 128<<10 || json.Unmarshal(metadata, &info) != nil {
		return fmt.Errorf("invalid OpenCNPJ source metadata")
	}
	observed, err := time.Parse("2006-01", info.Competence)
	now := time.Now().UTC()
	if u.Now != nil {
		now = u.Now().UTC()
	}
	if err != nil || observed.After(now) {
		return fmt.Errorf("invalid OpenCNPJ source competence")
	}
	previous := ""
	if raw, err := smallFile(filepath.Join(u.Root, "current"), 512); err == nil {
		previous = strings.TrimSpace(string(raw))
	} else if !os.IsNotExist(err) {
		return err
	}
	rel := "snapshots/" + hash + ".ocnpj"
	keepPrevious := previous
	if previous == rel {
		if _, published, err := currentOpenCNPJ(u.Root); err != nil || published != hash {
			return fmt.Errorf("published OpenCNPJ snapshot differs")
		}
		keepPrevious = ""
		if raw, err := smallFile(filepath.Join(u.Root, "previous"), 512); err == nil {
			keepPrevious = strings.TrimSpace(string(raw))
			if !strings.HasPrefix(keepPrevious, "snapshots/") || !strings.HasSuffix(keepPrevious, ".ocnpj") || !validHash(strings.TrimSuffix(strings.TrimPrefix(keepPrevious, "snapshots/"), ".ocnpj")) {
				return fmt.Errorf("unsafe previous snapshot pointer")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if previous != rel {
		// Explicit verification also covers an upstream CLI's unchanged fast
		// path after an earlier failed build in its mutable workspace.
		if err = u.Invoke(ctx, "rfb_verify", u.Binary, []string{"verify", snapshot, "--sources", filepath.Join(work, "raw", info.Competence)}); err != nil {
			return err
		}
		if _, verified, err := currentOpenCNPJ(work); err != nil || verified != hash {
			return fmt.Errorf("OpenCNPJ source changed during verification")
		}
		if err = os.MkdirAll(filepath.Join(u.Root, "snapshots"), 0700); err != nil {
			return err
		}
		if err = copyOpenCNPJ(ctx, snapshot, filepath.Join(u.Root, rel), hash); err != nil {
			return err
		}
		if previous != "" {
			if _, _, err := currentOpenCNPJ(u.Root); err != nil {
				return err
			}
			if err = atomicFile(u.Root, "previous", []byte(previous+"\n")); err != nil {
				return err
			}
		}
		if err = atomicFile(u.Root, "current", []byte(rel+"\n")); err != nil {
			return err
		}
	}
	receipt, _ := json.Marshal(map[string]any{"schema": "abastevo-opencnpj-refresh-v1", "checked_at": now.Format(time.RFC3339), "competence": info.Competence, "snapshot_sha256": hash, "changed": previous != rel})
	if err = atomicFile(u.Root, "source-state.json", receipt); err != nil {
		return err
	}
	if err = pruneOpenCNPJ(u.Root, rel, keepPrevious, info.Competence); err != nil {
		return err
	}
	return CheckOpenCNPJDisk(u.Root)
}

func cleanOpenCNPJPartialCopies(root string) error {
	dir := filepath.Join(root, "snapshots")
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe source snapshots directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	owned := regexp.MustCompile(`^\.snapshot-[0-9]+$`)
	for _, entry := range entries {
		if !owned.MatchString(entry.Name()) {
			continue
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsafe partial source copy")
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return syncDir(dir)
}

func copyOpenCNPJ(ctx context.Context, source, target, expected string) error {
	if _, err := os.Lstat(target); err == nil {
		actual, err := fileHash(target, 16<<30)
		if err != nil || actual != expected {
			return fmt.Errorf("existing immutable OpenCNPJ snapshot differs")
		}
		return atomicFile(filepath.Dir(target), filepath.Base(target)+".sha256", []byte(expected+"\n"))
	} else if !os.IsNotExist(err) {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(target), ".snapshot-")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	hash := sha256.New()
	writer := io.MultiWriter(out, hash)
	buffer := make([]byte, 1<<20)
	var total int64
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := in.Read(buffer)
		total += int64(n)
		if total > 16<<30 {
			return fmt.Errorf("OpenCNPJ snapshot budget exceeded")
		}
		if n > 0 {
			if _, err = writer.Write(buffer[:n]); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected {
		return fmt.Errorf("OpenCNPJ copied snapshot checksum differs")
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	if err = os.Rename(out.Name(), target); err != nil {
		return err
	}
	return atomicFile(filepath.Dir(target), filepath.Base(target)+".sha256", []byte(expected+"\n"))
}

func pruneOpenCNPJ(root, current, previous, competence string) error {
	// Only hash-named immutable files in this owned root can be retired.
	entries, err := os.ReadDir(filepath.Join(root, "snapshots"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".ocnpj") || !validHash(strings.TrimSuffix(name, ".ocnpj")) {
			continue
		}
		rel := "snapshots/" + name
		if rel == current || rel == previous {
			continue
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsafe immutable source artifact")
		}
		if err = os.Remove(filepath.Join(root, rel)); err != nil {
			return err
		}
		if err = os.Remove(filepath.Join(root, rel) + ".sha256"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	// Raw ZIPs are reproducible preparation inputs, not canonical DB history.
	// Keep current ZIPs for no-change checks. The independent published
	// snapshots retain rollback; older build inputs consume needed headroom.
	month, _ := time.Parse("2006-01", competence)
	rawRoot := filepath.Join(root, "work", "raw")
	entries, err = os.ReadDir(rawRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		stamp, err := time.Parse("2006-01", name)
		if err != nil || name == competence || !stamp.Before(month) {
			continue
		}
		path := filepath.Join(rawRoot, name)
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe raw source artifact")
		}
		manifest, err := smallFile(filepath.Join(path, "source-manifest.json"), 128<<10)
		if err != nil {
			return err
		}
		var provenance struct {
			Competence string `json:"competence"`
			BaseURL    string `json:"base_url"`
		}
		if json.Unmarshal(manifest, &provenance) != nil || provenance.Competence != name || provenance.BaseURL != "https://arquivos.receitafederal.gov.br/public.php/webdav" {
			return fmt.Errorf("unowned raw source artifact")
		}
		if err = os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}
