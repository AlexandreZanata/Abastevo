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

func TestSevenFineResourcesUseExactVerifiedPeriods(t *testing.T) {
	for _, tc := range []struct{ period, file string }{{"2016-2019", "multas-aplicadas-2016a2019.csv"}, {"2020", "multas-aplicadas-2020.csv"}, {"2021", "multas-2021-junho22.xlsx"}, {"2022", "multas-aplicadas-2022.csv"}, {"2023", "multas-aplicadas-2023.csv"}, {"2024", "multas-aplicadas-2024.csv"}, {"2025", "multas-aplicadas-2025.csv"}} {
		raw := "https://www.gov.br" + FineResourcePrefix + tc.file
		if got, e := FineResourcePeriod(raw); e != nil || got != tc.period || ValidateFineResourceSelection(raw, tc.period) != nil {
			t.Fatal("exact resource refused", tc.period, e)
		}
		for _, bad := range []string{raw + "?x", raw + "#x", raw + "/extra"} {
			if _, e := FineResourcePeriod(bad); e == nil {
				t.Fatal("unsafe resource accepted")
			}
		}
	}
	for _, file := range []string{"multas-aplicadas-2016.csv", "multas-aplicadas-2017.csv", "multas-aplicadas-2021.csv", "multas-aplicadas-2024-excel.xlsx", "multas-aplicadas-2026.csv"} {
		if _, e := FineResourcePeriod("https://www.gov.br" + FineResourcePrefix + file); e == nil {
			t.Fatal("invented/unverified profile accepted")
		}
	}
}

func TestHistoricalFineProcessTrustFailureCancellationAndCleanup(t *testing.T) {
	for _, mode := range []string{"timeout", "snapshot", "process", "permission", "symlink", "identity", "cancel", "profile", "schema"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "private.csv")
			binary := filepath.Join(root, "preparer")
			raw := []byte("private synthetic source\n")
			sum := sha256.Sum256(raw)
			if e := os.WriteFile(source, raw, 0600); e != nil {
				t.Fatal(e)
			}
			script := "#!/bin/sh\necho private-body >&2\nexit 1\n"
			timeout := time.Second
			switch mode {
			case "timeout":
				script = "#!/bin/sh\nexec sleep 60\n"
				timeout = 30 * time.Millisecond
			case "snapshot":
				script = "#!/bin/sh\nprintf altered > \"$1\"\nexit 0\n"
			case "schema":
				script = "#!/bin/sh\nmkdir \"$3\"\nprintf '{}' > \"$3/manifest.json\"\nexit 0\n"
			}
			if e := os.WriteFile(binary, []byte(script), 0755); e != nil {
				t.Fatal(e)
			}
			req := FineRequest{Path: source, URL: "https://www.gov.br" + FineResourcePrefix + "multas-aplicadas-2024.csv", Edition: "2024", Identity: prepared.InputIdentity{SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(raw))}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "permission":
				if e := os.Chmod(binary, 0777); e != nil {
					t.Fatal(e)
				}
			case "symlink":
				link := filepath.Join(root, "link")
				if e := os.Symlink(source, link); e != nil {
					t.Fatal(e)
				}
				req.Path = link
			case "identity":
				req.Identity.SHA256 = strings.Repeat("a", 64)
			case "cancel":
				cancel()
			case "profile":
				req.Edition = "2025"
			}
			batch, e := (FineHistoryRunner{Binary: binary, WorkspaceRoot: root, Timeout: timeout}).Prepare(ctx, req)
			if e == nil || len(batch.Observations) != 0 || batch.ManifestHash != "" || strings.Contains(e.Error(), "private-body") {
				t.Fatal("unsafe history process yielded facts/raw output", e)
			}
			if mode == "snapshot" && !strings.Contains(e.Error(), "integrity") {
				t.Fatal("modified snapshot unchecked", e)
			}
			original, e := os.ReadFile(source)
			if e != nil || string(original) != string(raw) {
				t.Fatal("original source changed")
			}
			files, e := os.ReadDir(root)
			want := 2
			if mode == "symlink" {
				want = 3
			}
			if e != nil || len(files) != want {
				t.Fatal("historical process retained workspace", e)
			}
		})
	}
}
