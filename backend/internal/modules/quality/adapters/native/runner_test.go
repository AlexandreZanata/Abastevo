package native

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
)

func unbound(context.Context, domain.SourceRecord) (domain.Decision, error) {
	return domain.Decision{Kind: domain.ResolutionUnmatched, Reason: domain.ReasonUnmatchedID}, nil
}
func TestRunnerOutageCleanupAndRedaction(t *testing.T) {
	p, id := archiveFixture(t, "PMQC_2025_01.csv")
	root := t.TempDir()
	for _, body := range []string{"#!/bin/sh\necho private-source-output >&2\nexit 2\n", "#!/bin/sh\nexec /bin/sleep 60\n"} {
		binary := filepath.Join(t.TempDir(), "synthetic-preparer")
		if e := os.WriteFile(binary, []byte(body), 0700); e != nil {
			t.Fatal(e)
		}
		// The timeout spans the archive read too; 30 ms expired there under
		// load and surfaced "archive read" instead of exercising the child.
		runner := Runner{Binary: binary, WorkspaceRoot: root, Timeout: time.Second}
		_, e := runner.Prepare(context.Background(), Request{Path: p, Identity: id, URL: annualURL, Member: "PMQC_2025_01.csv", Edition: "2025-01"}, unbound)
		if e == nil || !strings.Contains(e.Error(), "preparer failed or cancelled") || strings.Contains(e.Error(), "private-source-output") || strings.Contains(e.Error(), p) {
			t.Fatal("failure leaked or accepted", e)
		}
		children, e := os.ReadDir(root)
		if e != nil || len(children) != 0 {
			t.Fatal("workspace leaked", children, e)
		}
	}
}
func TestRunnerRejectsUntrustedBinaryAndMember(t *testing.T) {
	p, id := archiveFixture(t, "PMQC_2025_01.csv")
	root := t.TempDir()
	req := Request{Path: p, Identity: id, URL: annualURL, Member: "PMQC_2025_01.csv", Edition: "2025-02"}
	for _, binary := range []string{"prepare_quality", "/nonexistent/prepare_quality"} {
		if _, e := (Runner{Binary: binary, WorkspaceRoot: root}).Prepare(context.Background(), req, unbound); e == nil {
			t.Fatal("bad preparer accepted")
		}
	}
	binary := filepath.Join(t.TempDir(), "preparer")
	if e := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := (Runner{Binary: binary, WorkspaceRoot: root}).Prepare(context.Background(), req, unbound); e == nil {
		t.Fatal("cross-edition member accepted")
	}
	link := filepath.Join(t.TempDir(), "link")
	if e := os.Symlink(binary, link); e != nil {
		t.Fatal(e)
	}
	req.Edition = "2025-01"
	if _, e := (Runner{Binary: link, WorkspaceRoot: root}).Prepare(context.Background(), req, unbound); e == nil {
		t.Fatal("symlink binary accepted")
	}
}
