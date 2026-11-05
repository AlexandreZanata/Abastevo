package registrysync

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDailyProjectionRequiresMonthlyPreparationAndRejectsNewSnapshotUntilReady(t *testing.T) {
	p, dir, calls := openCNPJFixture(t)
	p.RequireSeed = true
	if _, err := p.Generate(context.Background(), dir); err == nil || *calls != 1 {
		t.Fatal("daily bulk export allowed without monthly seed", err, *calls)
	}
	p.RequireSeed = false
	if _, err := p.Generate(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	p.RequireSeed = true
	// Metadata changes must consume the same verified identities without export.
	if err := os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n00123456000100;Correction\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := *calls
	if _, err := p.Generate(context.Background(), dir); err != nil || *calls != before+1 {
		t.Fatal("daily reuse failed", err, *calls)
	}
	// A previously verified month cannot serve as the new month's projection.
	snapshot := filepath.Join(p.DataRoot, "snapshots", "synthetic.ocnpj")
	if err := os.WriteFile(snapshot, []byte("new bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	hash, err := fileHash(snapshot, 16<<30)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(snapshot+".sha256", []byte(hash), 0600); err != nil {
		t.Fatal(err)
	}
	before = *calls
	if _, err = p.Generate(context.Background(), dir); err == nil || *calls != before+1 {
		t.Fatal("unprepared monthly source bulk-exported", err, *calls)
	}
}

func TestDailyReadsMonthlyProjectionWithoutWritingToSourceAndHonorsItsLock(t *testing.T) {
	for _, busy := range []bool{false, true} {
		t.Run(map[bool]string{false: "read_only", true: "busy"}[busy], func(t *testing.T) {
			p, dir, calls := openCNPJFixture(t)
			p.CacheRoot = filepath.Join(p.DataRoot, "name-cache")
			if _, err := p.Generate(context.Background(), dir); err != nil {
				t.Fatal(err)
			}
			monthly := p.CacheRoot
			p.CacheRoot, p.RequireSeed = t.TempDir(), true
			_ = os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n00123456000100;Changed\n"), 0600)
			if busy {
				unlock, err := lock(monthly)
				if err != nil {
					t.Fatal(err)
				}
				defer unlock()
			} else {
				if err := os.Chmod(filepath.Join(monthly, ".lock"), 0400); err != nil {
					t.Fatal(err)
				}
			}
			_, err := p.Generate(context.Background(), dir)
			if busy && err == nil {
				t.Fatal("concurrent monthly cache writer accepted")
			}
			if !busy && err != nil {
				t.Fatal(err)
			}
			if *calls != 3 {
				t.Fatal("daily re-exported monthly identities", *calls)
			}
		})
	}
}

func TestConflictingVerifiedProjectionAndForgedOwnerFailBeforeExport(t *testing.T) {
	for _, mode := range []string{"conflict", "owner", "private_field", "accounting"} {
		t.Run(mode, func(t *testing.T) {
			p, dir, calls := openCNPJFixture(t)
			first, err := p.Generate(context.Background(), dir)
			if err != nil {
				t.Fatal(err)
			}
			cache := filepath.Dir(filepath.Dir(first))
			if mode == "conflict" {
				other, otherDir, _ := openCNPJFixture(t)
				run := other.Run
				other.Run = func(ctx context.Context, args ...string) ([]byte, error) {
					raw, err := run(ctx, args...)
					if args[0] == "export" {
						raw = []byte(strings.ReplaceAll(string(raw), "Trade", "Different"))
					}
					return raw, err
				}
				_ = os.WriteFile(filepath.Join(otherDir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n00123456000100;Other edition\n"), 0600)
				manifest, err := other.Generate(context.Background(), otherDir)
				if err != nil {
					t.Fatal(err)
				}
				source := filepath.Dir(filepath.Dir(manifest))
				if err = os.CopyFS(filepath.Join(p.CacheRoot, filepath.Base(source)), os.DirFS(source)); err != nil {
					t.Fatal(err)
				}
			} else if mode == "owner" {
				raw, _ := os.ReadFile(filepath.Join(cache, "owner.json"))
				raw = []byte(strings.ReplaceAll(string(raw), `"registry_sha256":"`, `"registry_sha256":"00`))
				_ = os.WriteFile(filepath.Join(cache, "owner.json"), raw, 0600)
			} else {
				// Rehashing a file cannot authorize private fields or false aggregate counts.
				path := filepath.Join(filepath.Dir(first), "rfb-names.jsonl")
				raw, _ := os.ReadFile(path)
				if mode == "private_field" {
					raw = []byte(strings.Replace(string(raw), `{"schema_version":`, `{"contact":"PRIVATE","schema_version":`, 1))
					_ = os.WriteFile(path, raw, 0600)
				}
				digest, _ := fileHash(path, nameFileCap)
				descriptor, pin, _ := readNameDescriptor(first)
				descriptor = []byte(strings.Replace(string(descriptor), pin.Inputs[0].SHA256, digest, 1))
				descriptor = []byte(strings.Replace(string(descriptor), `"bytes":`+strconv.FormatInt(pin.Inputs[0].Bytes, 10), `"bytes":`+strconv.FormatInt(int64(len(raw)), 10), 1))
				_ = os.WriteFile(first, descriptor, 0600)
				if mode == "accounting" {
					owner, _ := os.ReadFile(filepath.Join(cache, "owner.json"))
					owner = []byte(strings.Replace(string(owner), `"active":1`, `"active":0`, 1))
					_ = os.WriteFile(filepath.Join(cache, "owner.json"), owner, 0600)
				}
			}
			_ = os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n00123456000100;Another edition\n"), 0600)
			before := *calls
			if _, err := p.Generate(context.Background(), dir); err == nil || *calls != before+1 {
				t.Fatal("unsafe projection accepted or exported", err, *calls)
			}
		})
	}
}

func TestMonthlyPreparationDownloadCancellationAndSourceLock(t *testing.T) {
	for _, mode := range []string{"success", "download", "cancel", "source_busy"} {
		t.Run(mode, func(t *testing.T) {
			p, dir, calls := openCNPJFixture(t)
			registry, _ := os.ReadFile(filepath.Join(dir, "registry.csv"))
			requests := 0
			client := &http.Client{Transport: transport(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.String() != ANPURL {
					t.Fatal("unexpected monthly source")
				}
				if unlock, err := LockOpenCNPJ(p.DataRoot, true); err == nil {
					unlock()
					t.Fatal("monthly source read not locked")
				}
				status := 200
				if mode == "download" {
					status = 503
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(registry))), Header: make(http.Header)}, nil
			})}
			ctx := context.Background()
			if mode == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if mode == "source_busy" {
				unlock, err := LockOpenCNPJ(p.DataRoot, true)
				if err != nil {
					t.Fatal(err)
				}
				defer unlock()
			}
			err := p.PrepareMonthly(ctx, client)
			if mode == "success" {
				if err != nil || requests != 1 || *calls != 2 {
					t.Fatal("monthly projection not ready", err, requests, *calls)
				}
			} else if err == nil {
				t.Fatal("failed preparation accepted")
			}
			leftovers, _ := filepath.Glob(filepath.Join(p.DataRoot, ".name-source-*"))
			if len(leftovers) != 0 {
				t.Fatal("monthly input leaked")
			}
			if mode == "success" {
				p.RequireSeed = true
				run := p.Run
				p.Run = func(ctx context.Context, args ...string) ([]byte, error) {
					if args[0] == "export" {
						t.Fatal("daily exported monthly names")
					}
					return run(ctx, args...)
				}
				_ = os.WriteFile(filepath.Join(dir, "registry.csv"), []byte("CNPJ;RAZAOSOCIAL\n00123456000100;New legal name\n"), 0600)
				if _, err = p.Generate(context.Background(), dir); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestExactCacheHitRejectsForgedSourceProvenance(t *testing.T) {
	p, dir, calls := openCNPJFixture(t)
	manifest, err := p.Generate(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	owner := filepath.Join(filepath.Dir(filepath.Dir(manifest)), "owner.json")
	raw, _ := os.ReadFile(owner)
	raw = []byte(strings.ReplaceAll(string(raw), `"registry_sha256":"`, `"registry_sha256":"00`))
	if err = os.WriteFile(owner, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = p.Generate(context.Background(), dir); err == nil || *calls != 2 {
		t.Fatal("exact cache hit accepted forged source binding", err, *calls)
	}
}
