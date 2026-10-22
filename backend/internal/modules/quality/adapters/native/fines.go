package native

import (
	"context"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

const FineResourcePrefix = "/anp/pt-br/centrais-de-conteudo/dados-abertos/arquivos/mav/"

var fineName = regexp.MustCompile(`^multas-aplicadas-([0-9]{4})\.csv$`)

func FineURLYear(raw string) (int, error) {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "www.gov.br" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.RawPath != "" || !strings.HasPrefix(u.Path, FineResourcePrefix) {
		return 0, refused("fine resource URL")
	}
	parts := fineName.FindStringSubmatch(strings.TrimPrefix(u.Path, FineResourcePrefix))
	if parts == nil {
		return 0, refused("fine resource name")
	}
	year, e := strconv.Atoi(parts[1])
	if e != nil || year < 2016 {
		return 0, refused("fine resource year")
	}
	return year, nil
}
func ValidateFineSelection(raw, edition string) error {
	year, e := FineURLYear(raw)
	if e != nil || strconv.Itoa(year) != edition {
		return refused("fine source selection")
	}
	return nil
}

// FineResourcePeriod admits only actually verified source profiles. Filename
// suffixes do not guess an encoding: the2021 .xlsx resource is verified CSV.
func FineResourcePeriod(raw string) (string, error) {
	if raw == "https://www.gov.br"+FineResourcePrefix+"multas-aplicadas-2025.csv" {
		return "2025", nil
	}
	for _, period := range []string{"2016-2019", "2020", "2021", "2022", "2023", "2024"} {
		p, _ := application.HistoricalFineProfile(period)
		if raw == "https://www.gov.br"+FineResourcePrefix+p.SourceFile {
			return period, nil
		}
	}
	return "", refused("fine resource profile")
}
func ValidateFineResourceSelection(raw, period string) error {
	known, e := FineResourcePeriod(raw)
	if e != nil || known != period {
		return refused("fine resource period")
	}
	return nil
}

type FineRunner struct {
	Binary, WorkspaceRoot string
	Timeout               time.Duration
}
type FineRequest struct {
	Path, URL, Edition string
	Identity           prepared.InputIdentity
}

// Prepare pins a private source descriptor and an independently verified private
// process snapshot. Only the operator selects Binary; no job path enters exec.
func (r FineRunner) Prepare(ctx context.Context, req FineRequest) (prepared.FineBatch, error) {
	return prepareFineBatch(r, ctx, req, ValidateFineSelection, func(ctx context.Context, raw []byte, open prepared.OpenStream, identity prepared.InputIdentity) (prepared.FineBatch, error) {
		year, _ := FineURLYear(req.URL)
		return prepared.LoadFines(ctx, raw, open, identity, year)
	})
}

type FineHistoryRunner FineRunner

func (r FineHistoryRunner) Prepare(ctx context.Context, req FineRequest) (prepared.FineHistoryBatch, error) {
	selection := func(raw, period string) error {
		if period == "2025" {
			return refused("historical profile")
		}
		return ValidateFineResourceSelection(raw, period)
	}
	return prepareFineBatch(FineRunner(r), ctx, req, selection, func(ctx context.Context, raw []byte, open prepared.OpenStream, identity prepared.InputIdentity) (prepared.FineHistoryBatch, error) {
		return prepared.LoadFineHistory(ctx, raw, open, identity, req.Edition)
	})
}

func prepareFineBatch[T any](r FineRunner, ctx context.Context, req FineRequest, selection func(string, string) error, load func(context.Context, []byte, prepared.OpenStream, prepared.InputIdentity) (T, error)) (result T, err error) {
	return prepareFileBatch(r, ctx, req, selection, load, func(snapshot, edition, out string) []string { return []string{snapshot, edition, out} }, prepared.MaxBytes, false)
}

func prepareFileBatch[T any](r FineRunner, ctx context.Context, req FineRequest, selection func(string, string) error, load func(context.Context, []byte, prepared.OpenStream, prepared.InputIdentity) (T, error), arguments func(string, string, string) []string, artifactMax int64, privateArtifacts bool) (result T, err error) {
	if e := ctx.Err(); e != nil {
		return result, e
	}
	if e := (Runner{Binary: r.Binary, Timeout: r.Timeout}).Validate(); e != nil {
		return result, e
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if selection(req.URL, req.Edition) != nil || !filepath.IsAbs(req.Path) {
		return result, refused("fine input selection")
	}
	info, e := os.Lstat(req.Path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != req.Identity.Bytes {
		return result, refused("fine input file")
	}
	source, e := os.Open(req.Path)
	if e != nil {
		return result, refused("fine input unavailable")
	}
	defer source.Close()
	current, e := source.Stat()
	if e != nil || !os.SameFile(info, current) || !current.Mode().IsRegular() {
		return result, refused("fine input replacement")
	}
	if e = verify(ctx, source, req.Identity); e != nil {
		return result, e
	}
	workspace, e := os.MkdirTemp(r.WorkspaceRoot, "quality-fine-native-")
	if e != nil {
		return result, refused("fine workspace")
	}
	defer func() {
		if e := os.RemoveAll(workspace); e != nil {
			var zero T
			result = zero
			err = refused("fine workspace cleanup")
		}
	}()
	if e = os.Chmod(workspace, 0700); e != nil {
		return result, refused("fine workspace permissions")
	}
	parsed, _ := url.Parse(req.URL)
	snapshot := filepath.Join(workspace, path.Base(parsed.Path))
	file, e := os.OpenFile(snapshot, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return result, refused("fine snapshot")
	}
	defer file.Close()
	n, e := io.Copy(file, contextReader{ctx, io.NewSectionReader(source, 0, req.Identity.Bytes+1)})
	if e != nil || n != req.Identity.Bytes {
		return result, refused("fine snapshot copy")
	}
	if e = file.Sync(); e != nil {
		return result, refused("fine snapshot sync")
	}
	if e = verify(ctx, file, req.Identity); e != nil {
		return result, e
	}
	if e = verify(ctx, source, req.Identity); e != nil {
		return result, e
	}
	out := filepath.Join(workspace, "prepared")
	command := exec.CommandContext(ctx, r.Binary, arguments(snapshot, req.Edition, out)...)
	command.Dir = workspace
	command.Env = []string{"LANG=C.UTF-8", "HOME=" + workspace, "TMPDIR=" + workspace}
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if e = command.Run(); e != nil {
		return result, refused("fine process refused or cancelled")
	}
	if e = verify(ctx, file, req.Identity); e != nil {
		return result, e
	}
	if e = verify(ctx, source, req.Identity); e != nil {
		return result, e
	}
	artifactOpen := openArtifact
	if privateArtifacts {
		artifactOpen = openPrivateArtifact
	}
	manifest, e := artifactOpen(out, "manifest.json", 1<<20)
	if e != nil {
		return result, e
	}
	raw, e := io.ReadAll(io.LimitReader(manifest, (1<<20)+1))
	closed := manifest.Close()
	if e != nil || closed != nil || len(raw) > 1<<20 {
		return result, refused("fine manifest read")
	}
	batch, e := load(ctx, raw, func(name string) (io.ReadCloser, error) { return artifactOpen(out, name, artifactMax) }, req.Identity)
	if e != nil {
		return result, refused("fine artifacts refused")
	}
	return batch, nil
}
