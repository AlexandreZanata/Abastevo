package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

const unitFileURL = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/pmqc/pmqc_2026_09.zip"

func unitFilePayload(t *testing.T, change func(*ImportPayload)) []byte {
	t.Helper()
	p := ImportPayload{Version: 1, Source: "pmqc", Stream: "pmqc-assay-v1", Edition: "2026-09", URL: unitFileURL, SHA256: strings.Repeat("a", 64), Member: "PMQC_2026_09.csv"}
	if change != nil {
		change(&p)
	}
	raw, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestFileImportPayloadRefusedBeforePorts(t *testing.T) {
	for _, change := range []func(*ImportPayload){func(p *ImportPayload) { p.Source = "sanctions" }, func(p *ImportPayload) { p.Stream = "enforcement-v1" }, func(p *ImportPayload) { p.Member = "../PMQC_2026_09.csv" }, func(p *ImportPayload) { p.Member = "PMQC_2026_09.json" }, func(p *ImportPayload) { p.Edition = "2026-08" }, func(p *ImportPayload) { p.URL += "?private=source-token" }, func(p *ImportPayload) { p.SHA256 = strings.Repeat("A", 64) }, func(p *ImportPayload) { p.Version = 2 }} {
		raw := unitFilePayload(t, change)
		if _, e := decodeFilePayload(raw); e == nil || strings.Contains(e.Error(), "source-token") {
			t.Fatal("payload accepted or leaked", e)
		}
	}
	raw := unitFilePayload(t, nil)
	for _, bad := range []string{string(raw[:len(raw)-1]) + `,"version":1}`, string(raw[:len(raw)-1]) + `,"unknown":"private"}`, `{"version":1,"Version":1}`, strings.Repeat("a", 4097)} {
		if _, e := decodeFilePayload([]byte(bad)); e == nil {
			t.Fatal("ambiguous/unknown payload accepted")
		}
	}
	if _, e := decodeFilePayload(raw); e != nil {
		t.Fatal(e)
	}
	calls := 0
	im := FileImport{Fetch: func(context.Context, string) (FileArtifact, error) { calls++; return FileArtifact{}, nil }}
	if e := im.Handle(context.Background(), platformjobs.Job{Payload: raw}); e == nil || calls != 0 {
		t.Fatal("missing ports fetched source", e, calls)
	}
}
func TestFileArtifactVerifierRefusesBadIdentityAndFile(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "archive.zip")
	raw := []byte("synthetic file")
	if e := os.WriteFile(p, raw, 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	id := prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}
	file := FileArtifact{Path: p, Identity: id}
	if e := verifyArtifact(context.Background(), file, id.SHA256); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*FileArtifact){func(f *FileArtifact) { f.Identity.Bytes++ }, func(f *FileArtifact) { f.Identity.Bytes = 101 << 20 }, func(f *FileArtifact) { f.Identity.SHA256 = strings.Repeat("b", 64) }, func(f *FileArtifact) { f.Path = "relative.zip" }, func(f *FileArtifact) { f.Path = root }} {
		bad := file
		change(&bad)
		if e := verifyArtifact(context.Background(), bad, id.SHA256); e == nil {
			t.Fatal("bad artifact accepted")
		}
	}
	link := filepath.Join(root, "link.zip")
	if e := os.Symlink(p, link); e != nil {
		t.Fatal(e)
	}
	bad := file
	bad.Path = link
	if e := verifyArtifact(context.Background(), bad, id.SHA256); e == nil {
		t.Fatal("symlink accepted")
	}
	if e := os.Chmod(p, 0644); e != nil {
		t.Fatal(e)
	}
	if e := verifyArtifact(context.Background(), file, id.SHA256); e == nil {
		t.Fatal("nonprivate archive accepted")
	}
}
func TestFileImportFailureCleanupAndRedaction(t *testing.T) {
	for _, mode := range []string{"fetch", "prepare", "tamper", "guard", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			filename := filepath.Join(root, "input.zip")
			raw := []byte("synthetic file")
			sum := sha256.Sum256(raw)
			sha := hex.EncodeToString(sum[:])
			closed := 0
			im := FileImport{Pool: &pgxpool.Pool{}, Publish: adapters.NewPublisher(nil), Serial: func(ctx context.Context, run func(context.Context) error) error { return run(ctx) }, Fetch: func(context.Context, string) (FileArtifact, error) {
				if e := os.WriteFile(filename, raw, 0600); e != nil {
					t.Fatal(e)
				}
				f := FileArtifact{Path: filename, Identity: prepared.InputIdentity{SHA256: sha, Bytes: int64(len(raw))}, Close: func() error { closed++; return os.Remove(filename) }}
				if mode == "fetch" {
					return f, errors.New("private fetch error")
				}
				return f, nil
			}, Prepare: func(context.Context, ImportPayload, FileArtifact) (PreparedBatch, error) {
				if mode == "prepare" {
					return PreparedBatch{}, errors.New("private source body")
				}
				if mode == "tamper" {
					if e := os.WriteFile(filename, []byte("different file"), 0600); e != nil {
						t.Fatal(e)
					}
				}
				return PreparedBatch{}, nil
			}}
			ctx := context.Background()
			if mode == "cancel" {
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			payload := unitFilePayload(t, func(p *ImportPayload) { p.SHA256 = sha })
			e := im.Handle(ctx, platformjobs.Job{Payload: payload})
			if e == nil || strings.Contains(e.Error(), "private") || strings.Contains(e.Error(), filename) || closed != 1 {
				t.Fatal("failure leaked/retained artifact", e, closed)
			}
			if _, e = os.Stat(filename); !os.IsNotExist(e) {
				t.Fatal("failed file remained", e)
			}
		})
	}
}
