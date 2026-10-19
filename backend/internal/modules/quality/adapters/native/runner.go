package native

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

// Binary is an operator-owned immutable image path, never a job/source field.
// The production worker bundles this exact preparer; injected paths are for tests.
type Runner struct {
	Binary, WorkspaceRoot string
	Timeout               time.Duration
}
type Request struct {
	Path, URL, Member, Edition string
	Identity                   prepared.InputIdentity
}
type Result struct {
	Batch    prepared.Batch
	Resource application.Resource
	Periods  []application.PeriodInput
}

func (r Runner) Validate() error {
	if !filepath.IsAbs(r.Binary) {
		return refused("preparer path")
	}
	st, e := os.Lstat(r.Binary)
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0111 == 0 || st.Mode().Perm()&0022 != 0 {
		return refused("preparer unavailable")
	}
	if r.Timeout < 0 || r.Timeout > 2*time.Minute {
		return refused("preparer timeout budget")
	}
	return nil
}
func (r Runner) Prepare(ctx context.Context, req Request, resolve prepared.ResolveSample) (result Result, err error) {
	if e := r.Validate(); e != nil {
		return result, e
	}
	if resolve == nil {
		return result, refused("resolver unavailable")
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	a, e := openArchive(ctx, req.Path, req.Identity, req.URL)
	if e != nil {
		return result, e
	}
	defer a.file.Close()
	workspace, e := os.MkdirTemp(r.WorkspaceRoot, "quality-native-")
	if e != nil {
		return result, refused("workspace unavailable")
	}
	defer func() {
		if e := os.RemoveAll(workspace); e != nil {
			result = Result{}
			err = refused("workspace cleanup")
		}
	}()
	if e = os.Chmod(workspace, 0700); e != nil {
		return result, refused("workspace permissions")
	}
	csv := filepath.Join(workspace, path.Base(req.Member))
	identity, e := a.extract(ctx, req.Member, req.Edition, csv)
	if e != nil {
		return result, e
	}
	// Recheck the same open parent descriptor after extraction. A concurrently
	// changed archive never gets published from a valid-looking selected member.
	if e = verify(ctx, a.file, req.Identity); e != nil {
		return result, e
	}
	out := filepath.Join(workspace, "prepared")
	cmd := exec.CommandContext(ctx, r.Binary, csv, out)
	cmd.Dir = workspace
	cmd.Env = []string{"LANG=C.UTF-8", "HOME=" + workspace, "TMPDIR=" + workspace}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if e = cmd.Run(); e != nil {
		return result, refused("preparer failed or cancelled")
	}
	manifest, e := openArtifact(out, "manifest.json", 1<<20)
	if e != nil {
		return result, e
	}
	raw, e := io.ReadAll(io.LimitReader(manifest, (1<<20)+1))
	closeErr := manifest.Close()
	if e != nil || closeErr != nil || len(raw) > 1<<20 {
		return result, refused("manifest read")
	}
	batch, e := prepared.LoadNative(ctx, raw, func(name string) (io.ReadCloser, error) { return openArtifact(out, name, prepared.MaxBytes) }, identity, resolve)
	if e != nil {
		return result, refused("prepared artifact refused")
	}
	start, e := time.Parse("2006-01-02", req.Edition+"-01")
	if e != nil || req.Edition != start.Format("2006-01") {
		return result, refused("edition period")
	}
	result.Batch = batch
	result.Resource = application.Resource{URL: req.URL, SHA256: req.Identity.SHA256, Bytes: uint64(req.Identity.Bytes), Member: req.Member, MemberSHA256: identity.SHA256, MemberBytes: uint64(identity.Bytes)}
	result.Periods = []application.PeriodInput{{Label: req.Edition, Start: start.Format("2006-01-02"), End: start.AddDate(0, 1, -1).Format("2006-01-02")}}
	return result, nil
}
func openArtifact(dir, name string, maxBytes int64) (*os.File, error) {
	switch name {
	case "manifest.json", "assays.jsonl", "sanctions.jsonl", "duplicates.jsonl", "quarantine.jsonl":
	default:
		return nil, refused("artifact name")
	}
	filename := filepath.Join(dir, name)
	info, e := os.Lstat(filename)
	if e != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, refused("artifact file")
	}
	f, e := os.Open(filename)
	if e != nil {
		return nil, refused("artifact unavailable")
	}
	current, e := f.Stat()
	if e != nil || !os.SameFile(info, current) || !current.Mode().IsRegular() || current.Size() > maxBytes {
		_ = f.Close()
		return nil, refused("artifact replacement")
	}
	return f, nil
}
