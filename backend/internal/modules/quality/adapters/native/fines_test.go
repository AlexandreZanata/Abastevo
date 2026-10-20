package native

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
)

const fineResource = "https://www.gov.br/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/multas-aplicadas-2025.csv"

func TestFineSelectionExactResourceAndYearBeforeIO(t *testing.T) {
	if ValidateFineSelection(fineResource, "2025") != nil {
		t.Fatal("official selection refused")
	}
	for _, bad := range []string{strings.Replace(fineResource, "https:", "http:", 1), fineResource + "?token=x", fineResource + "#x", strings.Replace(fineResource, "www.gov.br", "www.gov.br.evil", 1), strings.Replace(fineResource, "www.gov.br", "user@www.gov.br", 1), strings.Replace(fineResource, "www.gov.br", "www.gov.br:443", 1), strings.Replace(fineResource, "2025.csv", "2024.csv", 1), strings.Replace(fineResource, "mav/", "mav/%2e%2e/", 1), strings.Replace(fineResource, "mav/", "mav/../", 1)} {
		if ValidateFineSelection(bad, "2025") == nil {
			t.Fatal("unsafe fine resource accepted")
		}
	}
	for _, year := range []string{"2024", "02025", "2025-01", "2015", ""} {
		if ValidateFineSelection(fineResource, year) == nil {
			t.Fatal("wrong fine selection year")
		}
	}
}

func TestFineProcessDeadlineAndModifiedSnapshotNeverYieldData(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.csv")
	raw := []byte("synthetic private source\n")
	if e := os.WriteFile(source, raw, 0600); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	request := FineRequest{Path: source, URL: fineResource, Edition: "2025", Identity: prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}}
	binary := filepath.Join(root, "preparer")
	for _, tc := range []struct {
		name, script string
		timeout      time.Duration
	}{
		{"deadline", "#!/bin/sh\nexec sleep 60\n", 30 * time.Millisecond},
		{"snapshot mutation", "#!/bin/sh\nprintf altered > \"$1\"\nexit 0\n", time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := os.WriteFile(binary, []byte(tc.script), 0755); e != nil {
				t.Fatal(e)
			}
			result, e := (FineRunner{Binary: binary, WorkspaceRoot: root, Timeout: tc.timeout}).Prepare(context.Background(), request)
			if e == nil || len(result.Observations) != 0 {
				t.Fatal("unsafe process yielded data")
			}
			if tc.name == "snapshot mutation" && !strings.Contains(e.Error(), "integrity") {
				t.Fatal("modified snapshot was not checked before artifacts", e)
			}
			original, e := os.ReadFile(source)
			if e != nil || string(original) != string(raw) {
				t.Fatal("process altered the original source")
			}
			children, e := os.ReadDir(root)
			if e != nil || len(children) != 2 {
				t.Fatal("failed native process leaked workspace")
			}
		})
	}
}
func TestFineRunnerTrustBudgetsCancellationAndRedaction(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "preparer")
	source := filepath.Join(root, "private.csv")
	raw := []byte("private source bytes\n")
	sum := sha256.Sum256(raw)
	if e := os.WriteFile(source, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(binary, []byte("#!/bin/sh\necho private-source-body >&2\nexit 1\n"), 0755); e != nil {
		t.Fatal(e)
	}
	request := FineRequest{Path: source, URL: fineResource, Edition: "2025", Identity: prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}}
	runner := FineRunner{Binary: binary, WorkspaceRoot: root}
	before, _ := os.ReadDir(root)
	result, e := runner.Prepare(context.Background(), request)
	if e == nil || len(result.Observations) != 0 || strings.Contains(e.Error(), "private-source-body") {
		t.Fatal("failed process yielded data/raw output")
	}
	after, _ := os.ReadDir(root)
	if len(before) != len(after) {
		t.Fatal("native fine workspace leaked")
	}
	if e = os.Chmod(binary, 0777); e != nil {
		t.Fatal(e)
	}
	if _, e = runner.Prepare(context.Background(), request); e == nil {
		t.Fatal("writable binary trusted")
	}
	_ = os.Chmod(binary, 0755)
	wrong := request
	wrong.Identity.SHA256 = strings.Repeat("a", 64)
	if _, e = runner.Prepare(context.Background(), wrong); e == nil {
		t.Fatal("wrong original identity trusted")
	}
	_ = os.Chmod(source, 0644)
	if _, e = runner.Prepare(context.Background(), request); e == nil {
		t.Fatal("publicly readable source trusted")
	}
	_ = os.Chmod(source, 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = runner.Prepare(ctx, request); e == nil {
		t.Fatal("cancelled native work accepted")
	}
	symlink := filepath.Join(root, "link.csv")
	_ = os.Symlink(source, symlink)
	wrong = request
	wrong.Path = symlink
	if _, e = runner.Prepare(context.Background(), wrong); e == nil {
		t.Fatal("symlink source trusted")
	}
}
