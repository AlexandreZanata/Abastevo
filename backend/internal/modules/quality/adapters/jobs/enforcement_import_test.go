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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

func enforcementPayload(t *testing.T, change func(*ImportPayload)) []byte {
	t.Helper()
	p := ImportPayload{Version: 1, Source: "enforcement", Stream: "enforcement-v1", Edition: application.EnforcementSourceEdition, URL: native.EnforcementResourceURL, SHA256: strings.Repeat("a", 64)}
	if change != nil {
		change(&p)
	}
	raw, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestEnforcementPayloadAndMissingPortsRefuseBeforeIO(t *testing.T) {
	for _, change := range []func(*ImportPayload){func(p *ImportPayload) { p.Source = "sanctions" }, func(p *ImportPayload) { p.Stream = "sanction-v1" }, func(p *ImportPayload) { p.Edition = "2019+" }, func(p *ImportPayload) { p.Member = "../file.xlsx" }, func(p *ImportPayload) { p.URL += "?secret=private" }, func(p *ImportPayload) { p.SHA256 = strings.Repeat("A", 64) }, func(p *ImportPayload) { p.Version = 2 }} {
		if _, e := decodeEnforcementPayload(enforcementPayload(t, change)); e == nil || strings.Contains(e.Error(), "private") {
			t.Fatal("unsafe payload admitted or leaked")
		}
	}
	raw := enforcementPayload(t, nil)
	for _, bad := range []string{string(raw[:len(raw)-1]) + `,"version":1}`, string(raw[:len(raw)-1]) + `,"path":"private"}`, strings.Repeat("x", 4097)} {
		if _, e := decodeEnforcementPayload([]byte(bad)); e == nil {
			t.Fatal("ambiguous payload admitted")
		}
	}
	calls := 0
	im := EnforcementFileImport{Fetch: func(context.Context, string) (FileArtifact, error) { calls++; return FileArtifact{}, nil }}
	if e := im.Handle(context.Background(), platformjobs.Job{Payload: raw}); e == nil || calls != 0 {
		t.Fatal("unwired source performed I/O")
	}
}
func TestEnforcementPrepublicationFailureCleansAndRedacts(t *testing.T) {
	for _, mode := range []string{"fetch", "prepare", "tamper", "cancel", "cleanup", "retained", "typed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := filepath.Join(t.TempDir(), "private.xlsx")
			raw := []byte("private synthetic source")
			sum := sha256.Sum256(raw)
			hash := hex.EncodeToString(sum[:])
			closed := 0
			im := EnforcementFileImport{Pool: &pgxpool.Pool{}, Publish: &adapters.Publisher{}, Serial: func(ctx context.Context, run func(context.Context) error) error { return run(ctx) }}
			im.Fetch = func(context.Context, string) (FileArtifact, error) {
				if e := os.WriteFile(path, raw, 0600); e != nil {
					t.Fatal(e)
				}
				f := FileArtifact{Path: path, Identity: prepared.InputIdentity{SHA256: hash, Bytes: int64(len(raw))}, Close: func() error {
					closed++
					if mode == "retained" {
						return nil
					}
					e := os.Remove(path)
					if mode == "cleanup" {
						return errors.New("private body")
					}
					return e
				}}
				if mode == "fetch" {
					return f, errors.New("private body")
				}
				return f, nil
			}
			im.Prepare = func(context.Context, ImportPayload, FileArtifact) (prepared.EnforcementBatch, error) {
				b := prepared.EnforcementBatch{SourceFile: native.EnforcementSourceFile, SourceProfile: application.EnforcementSourceProfile, ManifestHash: strings.Repeat("b", 64), Context: application.EnforcementSourceContext{Profile: application.EnforcementSourceProfile, SourceFile: native.EnforcementSourceFile, DateSystem: "EXCEL_1900"}}
				switch mode {
				case "prepare":
					return b, errors.New("private body")
				case "tamper":
					_ = os.WriteFile(path, []byte("changed"), 0600)
				case "cancel":
					cancel()
				case "typed":
					b.Context.Profile = "unknown"
				}
				return b, nil
			}
			job := platformjobs.Job{Payload: enforcementPayload(t, func(p *ImportPayload) { p.SHA256 = hash })}
			if e := im.Handle(ctx, job); e == nil || strings.Contains(e.Error(), "private") {
				t.Fatal("failure published or leaked", e)
			}
			if closed != 1 {
				t.Fatal("cleanup not owned exactly once", closed)
			}
			if mode != "retained" {
				if _, e := os.Stat(path); !errors.Is(e, os.ErrNotExist) {
					t.Fatal("raw retained")
				}
			}
		})
	}
}
