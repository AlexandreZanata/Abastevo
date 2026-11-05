package registrysync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openCNPJFixture(t *testing.T) (OpenCNPJNames, string, *int) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "snapshots"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "snapshots", "synthetic.ocnpj")
	raw := []byte("synthetic snapshot")
	sum := sha256.Sum256(raw)
	for path, data := range map[string][]byte{path: raw, path + ".sha256": []byte(hex.EncodeToString(sum[:])), filepath.Join(root, "current"): []byte("snapshots/synthetic.ocnpj\n")} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	unlock, err := LockOpenCNPJ(root, true)
	if err != nil {
		t.Fatal(err)
	}
	unlock()
	calls := 0
	p := OpenCNPJNames{DataRoot: root, CacheRoot: t.TempDir(), Now: func() time.Time { return time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC) }}
	p.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		calls++
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if args[0] == "info" {
			return []byte(`{"competence":"2026-10"}`), nil
		}
		var rows []string
		for _, id := range strings.Split(strings.TrimPrefix(args[3], "cnpj="), ",") {
			row, _ := json.Marshal(map[string]any{"cnpj": id, "nome_fantasia": "[P25-TEST] Trade " + id[len(id)-2:], "situacao_cadastral": "02", "socios": "PRIVATE", "correio_eletronico": "PRIVATE"})
			rows = append(rows, string(row))
		}
		return []byte(strings.Join(rows, "\n")), nil
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n00123456000100;Synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return p, dir, &calls
}

func TestOpenCNPJNamesAutomaticallyCoversNewANPIdentitiesAndCachesUnchangedPairs(t *testing.T) {
	p, dir, calls := openCNPJFixture(t)
	first, err := p.Generate(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Generate(context.Background(), dir); err != nil || *calls != 2 {
		t.Fatal("unchanged source pair re-exported", err, *calls)
	}
	if err = os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n00123456000100;Synthetic\n00123456000290;New branch\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := p.Generate(context.Background(), dir)
	if err != nil || first == second {
		t.Fatal("new ANP identity not enriched", err)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(second), "rfb-names.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE") || !strings.Contains(string(raw), "00123456000290") {
		t.Fatal("minimization or exact branch matching failed")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var row map[string]any
		if json.Unmarshal([]byte(line), &row) != nil || len(row) != 5 {
			t.Fatal("unexpected exported fields")
		}
	}
	if err := validateNameCache(filepath.Dir(filepath.Dir(second))); err != nil {
		t.Fatal(err)
	}
}

func TestRFBProjectionReusesRowsAcrossANPChangesAndQueriesOnlyNewCNPJs(t *testing.T) {
	p, dir, calls := openCNPJFixture(t)
	first, err := p.Generate(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	firstData, err := os.ReadFile(filepath.Join(filepath.Dir(first), "rfb-names.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n00123456000100;Changed legal name\n"), 0600)
	second, err := p.Generate(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 3 {
		t.Fatal("unchanged RFB identity was exported again", *calls)
	}
	secondData, _ := os.ReadFile(filepath.Join(filepath.Dir(second), "rfb-names.jsonl"))
	if string(firstData) != string(secondData) {
		t.Fatal("ANP legal change changed RFB projection")
	}
	_ = os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n00123456000100;Changed legal name\n00123456000290;New branch\n"), 0600)
	run := p.Run
	p.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		if args[0] == "export" && args[3] != "cnpj=00123456000290" {
			t.Fatal("previous identities queried", args[3])
		}
		return run(ctx, args...)
	}
	third, err := p.Generate(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	fresh, freshDir, _ := openCNPJFixture(t)
	raw, _ := os.ReadFile(filepath.Join(dir, "registry.csv"))
	_ = os.WriteFile(filepath.Join(freshDir, "registry.csv"), raw, 0600)
	full, err := fresh.Generate(context.Background(), freshDir)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(filepath.Dir(third), "rfb-names.jsonl"))
	want, _ := os.ReadFile(filepath.Join(filepath.Dir(full), "rfb-names.jsonl"))
	if string(got) != string(want) {
		t.Fatal("cache-dependent output ordering")
	}
}

func TestOpenCNPJSourceLockCorruptionAndForeignRowsFailClosed(t *testing.T) {
	for _, mode := range []string{"busy", "cache_busy", "checksum", "traversal", "foreign", "duplicate", "status", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			p, dir, _ := openCNPJFixture(t)
			ctx := context.Background()
			switch mode {
			case "cache_busy":
				unlock, err := lock(p.CacheRoot)
				if err != nil {
					t.Fatal(err)
				}
				defer unlock()
			case "busy":
				unlock, err := LockOpenCNPJ(p.DataRoot, true)
				if err != nil {
					t.Fatal(err)
				}
				defer unlock()
			case "checksum":
				_ = os.WriteFile(filepath.Join(p.DataRoot, "snapshots", "synthetic.ocnpj"), []byte("changed"), 0600)
			case "traversal":
				_ = os.WriteFile(filepath.Join(p.DataRoot, "current"), []byte("../../outside.ocnpj"), 0600)
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			default:
				original := p.Run
				p.Run = func(ctx context.Context, args ...string) ([]byte, error) {
					if args[0] == "info" {
						return original(ctx, args...)
					}
					row := `{"cnpj":"00123456000100","nome_fantasia":"Synthetic","situacao_cadastral":"02"}`
					if mode == "foreign" {
						row = strings.Replace(row, "00123456000100", "00123456000290", 1)
					}
					if mode == "duplicate" {
						row += "\n" + row
					}
					if mode == "status" {
						row = strings.Replace(row, `"02"`, `"UNKNOWN"`, 1)
					}
					return []byte(row), nil
				}
			}
			if _, err := p.Generate(ctx, dir); err == nil {
				t.Fatal("unsafe source accepted")
			}
			entries, _ := os.ReadDir(p.CacheRoot)
			for _, entry := range entries {
				if validHash(entry.Name()) {
					t.Fatal("partial cache published")
				}
			}
		})
	}
}

func TestNewMonthlySnapshotRefreshesUnchangedANPAndCorruptCacheIsNeverReused(t *testing.T) {
	p, dir, calls := openCNPJFixture(t)
	first, err := p.Generate(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(p.DataRoot, "snapshots", "synthetic.ocnpj")
	for _, edition := range []string{"next complete month", "corrected complete month"} {
		raw := []byte(edition)
		sum := sha256.Sum256(raw)
		_ = os.WriteFile(snapshot, raw, 0600)
		_ = os.WriteFile(snapshot+".sha256", []byte(hex.EncodeToString(sum[:])), 0600)
		path, err := p.Generate(context.Background(), dir)
		if err != nil || path == first {
			t.Fatal("new monthly bytes reused old names", err)
		}
	}
	if *calls != 6 {
		t.Fatal("monthly source did not reexport", *calls)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("old cache exceeded retention")
	}
	path, err := p.Generate(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(path), "rfb-names.jsonl"), []byte("tampered"), 0600)
	if _, err := p.Generate(context.Background(), dir); err == nil || *calls != 6 {
		t.Fatal("corrupt frozen cache accepted or rebuilt silently", err)
	}
}
