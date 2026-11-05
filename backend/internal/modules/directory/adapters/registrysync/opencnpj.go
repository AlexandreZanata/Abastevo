package registrysync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// OpenCNPJNames is an offline preparation port, never a public search service.
// The owning Runner holds its state lock; the RFB updater owns the exclusive
// source lock. Full snapshot/CSV hashes identify a reusable minimized cache.
type OpenCNPJNames struct {
	Binary, DataRoot, CacheRoot string
	Now                         func() time.Time
	Run                         func(context.Context, ...string) ([]byte, error)
}

type minimizedRFBName struct {
	Schema   string `json:"schema_version"`
	CNPJ     string `json:"cnpj"`
	Trade    string `json:"trade_name"`
	Active   bool   `json:"active"`
	Observed string `json:"observed_at"`
}

func LockOpenCNPJ(root string, exclusive bool) (func(), error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("unsafe OpenCNPJ root")
	}
	flags := os.O_RDONLY
	if exclusive {
		flags = os.O_CREATE | os.O_RDWR
	}
	f, err := os.OpenFile(filepath.Join(root, ".opencnpj.lock"), flags|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("OpenCNPJ source lock unavailable")
	}
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX
	}
	if err = syscall.Flock(int(f.Fd()), mode|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("OpenCNPJ source busy")
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

type boundedOutput struct {
	bytes.Buffer
	Max int
}

func (w *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > w.Max-w.Len() {
		return 0, fmt.Errorf("OpenCNPJ output budget exceeded")
	}
	return w.Buffer.Write(p)
}

func (p OpenCNPJNames) command(ctx context.Context, args ...string) ([]byte, error) {
	if p.Run != nil {
		return p.Run(ctx, args...)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.Binary, args...)
	cmd.Env = []string{"LANG=C.UTF-8", "TZ=UTC", "MALLOC_ARENA_MAX=2"}
	cmd.WaitDelay = 2 * time.Second
	output := &boundedOutput{Max: 32 << 20}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("OpenCNPJ preparation process failed")
	}
	return output.Bytes(), nil
}

func (p OpenCNPJNames) Info(ctx context.Context, snapshot string) ([]byte, error) {
	return p.command(ctx, "info", snapshot)
}

func currentOpenCNPJ(root string) (string, string, error) {
	raw, err := smallFile(filepath.Join(root, "current"), 512)
	if err != nil {
		return "", "", fmt.Errorf("OpenCNPJ current snapshot unavailable")
	}
	rel := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(rel, "snapshots/") || filepath.Clean(rel) != rel || filepath.IsAbs(rel) || strings.Contains(rel, "..") || filepath.Ext(rel) != ".ocnpj" {
		return "", "", fmt.Errorf("unsafe OpenCNPJ snapshot pointer")
	}
	path := filepath.Join(root, rel)
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return "", "", fmt.Errorf("unsafe OpenCNPJ snapshot path")
	}
	checksum, err := smallFile(path+".sha256", 4096)
	if err != nil {
		return "", "", fmt.Errorf("OpenCNPJ snapshot checksum unavailable")
	}
	fields := strings.Fields(string(checksum))
	if len(fields) == 0 || !validHash(fields[0]) {
		return "", "", fmt.Errorf("invalid OpenCNPJ checksum")
	}
	actual, err := fileHash(path, 16<<30)
	if err != nil || actual != fields[0] {
		return "", "", fmt.Errorf("OpenCNPJ snapshot checksum mismatch")
	}
	return path, actual, nil
}

func (p OpenCNPJNames) Generate(ctx context.Context, dir string) (string, error) {
	unlock, err := LockOpenCNPJ(p.DataRoot, false)
	if err != nil {
		return "", err
	}
	defer unlock()
	snapshot, snapshotHash, err := currentOpenCNPJ(p.DataRoot)
	if err != nil {
		return "", err
	}
	registryHash, err := fileHash(filepath.Join(dir, "registry.csv"), SourceCap)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte("opencnpj-minimized-v1\n" + registryHash + "\n" + snapshotHash))
	key := hex.EncodeToString(sum[:])
	if err = os.MkdirAll(p.CacheRoot, 0700); err != nil {
		return "", err
	}
	cacheInfo, err := os.Lstat(p.CacheRoot)
	if err != nil || !cacheInfo.IsDir() || cacheInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("unsafe name cache root")
	}
	unlockCache, err := lock(p.CacheRoot)
	if err != nil {
		return "", err
	}
	defer unlockCache()
	// Serialize the offline CLI path as well as Runner cache writers. Recover only interrupted
	// private build directories; canonical state and unknown neighbors survive.
	if err = cleanTemporary(p.CacheRoot, `^\.name-build-[0-9]+$`); err != nil {
		return "", err
	}
	cache := filepath.Join(p.CacheRoot, key)
	if _, err := os.Lstat(cache); err == nil {
		if err = validateNameCache(cache); err != nil {
			return "", err
		}
		return filepath.Join(cache, "names", "manifest.json"), nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	metadata, err := p.command(ctx, "info", snapshot)
	if err != nil {
		return "", err
	}
	var info struct {
		Competence string `json:"competence"`
	}
	if len(metadata) > 128<<10 || json.Unmarshal(metadata, &info) != nil {
		return "", fmt.Errorf("invalid OpenCNPJ metadata")
	}
	observed, err := time.Parse("2006-01", info.Competence)
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	if err != nil || observed.After(now.UTC()) {
		return "", fmt.Errorf("invalid OpenCNPJ competence")
	}
	membership, err := registryMembership(filepath.Join(dir, "registry.csv"))
	if err != nil {
		return "", err
	}
	ids := make([]string, 0, len(membership))
	for id := range membership {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	temp, err := os.MkdirTemp(p.CacheRoot, ".name-build-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	if err = os.Mkdir(filepath.Join(temp, "names"), 0700); err != nil {
		return "", err
	}
	out, err := os.OpenFile(filepath.Join(temp, "names", "rfb-names.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer out.Close()
	hash := sha256.New()
	encoder := json.NewEncoder(io.MultiWriter(out, hash))
	seen := map[string]bool{}
	active, nonblank := 0, 0
	for start := 0; start < len(ids); start += 100 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		batch := map[string]bool{}
		for _, id := range ids[start:end] {
			batch[id] = true
		}
		raw, err := p.command(ctx, "export", snapshot, "--filter", "cnpj="+strings.Join(ids[start:end], ","), "--max", fmt.Sprint(end-start+1))
		if err != nil {
			return "", err
		}
		if len(raw) > 32<<20 {
			return "", fmt.Errorf("OpenCNPJ batch budget exceeded")
		}
		projected := []minimizedRFBName{}
		for _, line := range bytes.Split(raw, []byte{'\n'}) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			if len(line) > 256<<10 {
				return "", fmt.Errorf("OpenCNPJ row budget exceeded")
			}
			var row struct {
				CNPJ   string  `json:"cnpj"`
				Name   *string `json:"nome_fantasia"`
				Status string  `json:"situacao_cadastral"`
			}
			if json.Unmarshal(line, &row) != nil || !batch[row.CNPJ] || seen[row.CNPJ] {
				return "", fmt.Errorf("unexpected OpenCNPJ identity")
			}
			name := ""
			if row.Name != nil {
				name = *row.Name
			}
			if len(name) > 2048 || !strings.Contains(" 01 02 03 04 08 ", " "+row.Status+" ") || row.Status == "" {
				return "", fmt.Errorf("invalid OpenCNPJ establishment fields")
			}
			seen[row.CNPJ] = true
			if row.Status == "02" {
				active++
				if strings.TrimSpace(name) != "" {
					nonblank++
				}
			}
			projected = append(projected, minimizedRFBName{"rfb-station-names-v1", row.CNPJ, name, row.Status == "02", observed.Format("2006-01-02")})
		}
		sort.Slice(projected, func(i, j int) bool { return projected[i].CNPJ < projected[j].CNPJ })
		for _, row := range projected {
			if err := encoder.Encode(row); err != nil {
				return "", err
			}
		}
		if info, err := out.Stat(); err != nil || info.Size() > nameFileCap {
			return "", fmt.Errorf("minimized name budget exceeded")
		}
	}
	if err = out.Sync(); err != nil {
		return "", err
	}
	stat, err := out.Stat()
	if err != nil || stat.Size() > nameFileCap {
		return "", fmt.Errorf("minimized name budget exceeded")
	}
	if err = out.Close(); err != nil {
		return "", err
	}
	descriptor := nameDescriptor{Format: "registry-name-inputs-v1", Inputs: []namePin{{Source: "rfb", Profile: "rfb-station-names-v1", File: "rfb-names.jsonl", Reference: "https://arquivos.receitafederal.gov.br/dados/cnpj/dados_abertos_cnpj/" + info.Competence + "/", Edition: "opencnpj-" + info.Competence + "-sha256:" + snapshotHash, SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: stat.Size(), Rows: len(seen)}}}
	raw, _ := json.Marshal(descriptor)
	if err = atomicFile(filepath.Join(temp, "names"), "manifest.json", raw); err != nil {
		return "", err
	}
	owner, _ := json.Marshal(map[string]any{"schema": "abastevo-name-cache-v1", "key": key, "registry_sha256": registryHash, "snapshot_sha256": snapshotHash, "competence": info.Competence, "matched": len(seen), "not_found": len(ids) - len(seen), "active": active, "active_nonblank": nonblank})
	if err = atomicFile(temp, "owner.json", owner); err != nil {
		return "", err
	}
	if err = os.Rename(temp, cache); err != nil {
		return "", err
	}
	if err = syncDir(p.CacheRoot); err != nil {
		return "", err
	}
	if err = pruneNameCache(p.CacheRoot, key); err != nil {
		return "", err
	}
	return filepath.Join(cache, "names", "manifest.json"), nil
}

func validateNameCache(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe name cache")
	}
	raw, err := smallFile(filepath.Join(dir, "owner.json"), 4096)
	if err != nil {
		return err
	}
	var owner struct{ Schema, Key string }
	if json.Unmarshal(raw, &owner) != nil || owner.Schema != "abastevo-name-cache-v1" || owner.Key != filepath.Base(dir) {
		return fmt.Errorf("unowned name cache")
	}
	raw, _, err = readNameDescriptor(filepath.Join(dir, "names", "manifest.json"))
	if err != nil {
		return err
	}
	hash := sha256.Sum256(raw)
	return verifyNameInputs(dir, hex.EncodeToString(hash[:]))
}

func pruneNameCache(root, current string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type owned struct {
		name  string
		stamp time.Time
	}
	old := []owned{}
	for _, entry := range entries {
		if entry.Name() == current || !validHash(entry.Name()) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if err := validateNameCache(dir); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		old = append(old, owned{entry.Name(), info.ModTime()})
	}
	sort.Slice(old, func(i, j int) bool { return old[i].stamp.After(old[j].stamp) })
	for _, entry := range old[min(1, len(old)):] {
		if err := os.RemoveAll(filepath.Join(root, entry.name)); err != nil {
			return err
		}
	}
	return nil
}
