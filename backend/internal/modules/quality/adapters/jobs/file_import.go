package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// FileArtifact transfers one private verified download and its cleanup capability.
// The path is returned by the trusted fetch port, never accepted from job input.
type FileArtifact struct {
	Path     string
	Identity prepared.InputIdentity
	Close    func() error
}

// FileImport composes native PMQC without archive-sized byte slices. Serial must
// acquire the cross-worker source lease before invoking run; Prepare returns the
// live per-attempt Directory transaction guard, never cached globally.
type FileImport struct {
	Pool    *pgxpool.Pool
	Publish *adapters.Publisher
	Fetch   func(context.Context, string) (FileArtifact, error)
	Prepare func(context.Context, ImportPayload, FileArtifact) (PreparedBatch, error)
	Serial  func(context.Context, func(context.Context) error) error
}

func (FileImport) Kind() string         { return "quality-import" }
func (FileImport) Version() int         { return 1 }
func fileImportError(code string) error { return errors.New("quality import: " + code) }

func decodeFilePayload(raw []byte) (ImportPayload, error) {
	var payload ImportPayload
	if len(raw) > 4096 || httpapi.DecodeJSON(raw, &payload) != nil || payload.Version != 1 || payload.Source != "pmqc" || payload.Stream != "pmqc-assay-v1" || len(payload.SHA256) != 64 || payload.SHA256 != strings.ToLower(payload.SHA256) {
		return payload, fileImportError("invalid native payload")
	}
	if _, e := hex.DecodeString(payload.SHA256); e != nil {
		return payload, fileImportError("invalid native payload")
	}
	if native.ValidateSelection(payload.URL, payload.Member, payload.Edition) != nil {
		return payload, fileImportError("invalid source selection")
	}
	return payload, nil
}
func (im FileImport) Handle(ctx context.Context, job platformjobs.Job) error {
	payload, e := decodeFilePayload(job.Payload)
	if e != nil {
		return e
	}
	if im.Pool == nil || im.Publish == nil || im.Fetch == nil || im.Prepare == nil || im.Serial == nil {
		return fileImportError("native ports unavailable")
	}
	return im.Serial(ctx, func(ctx context.Context) error { return im.importFile(ctx, payload) })
}
func (im FileImport) importFile(ctx context.Context, payload ImportPayload) (err error) {
	file, e := im.Fetch(ctx, payload.URL)
	if file.Close != nil {
		defer func() {
			if e := file.Close(); e != nil && err == nil {
				err = fileImportError("artifact cleanup unavailable")
			}
		}()
	}
	if e != nil {
		return fileImportError("download unavailable")
	}
	if file.Close == nil {
		return fileImportError("artifact ownership unavailable")
	}
	if e = verifyArtifact(ctx, file, payload.SHA256); e != nil {
		return e
	}
	batch, e := im.Prepare(ctx, payload, file)
	if e != nil {
		return fileImportError("preparation unavailable")
	}
	if e = verifyArtifact(ctx, file, payload.SHA256); e != nil {
		return e
	}
	if batch.Guard == nil {
		return fileImportError("live identity guard unavailable")
	}
	return (Import{Pool: im.Pool, Publish: im.Publish}).publishPrepared(ctx, payload, batch, uint64(file.Identity.Bytes))
}

type fileContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r fileContextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(b)
}
func verifyArtifact(ctx context.Context, file FileArtifact, expected string) error {
	id := file.Identity
	if !filepath.IsAbs(file.Path) || id.Bytes <= 0 || id.Bytes > native.MaxArchiveBytes || id.SHA256 != expected {
		return fileImportError("artifact identity refused")
	}
	info, e := os.Lstat(file.Path)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() != id.Bytes {
		return fileImportError("artifact file refused")
	}
	f, e := os.Open(file.Path)
	if e != nil {
		return fileImportError("artifact unavailable")
	}
	defer f.Close()
	current, e := f.Stat()
	if e != nil || !os.SameFile(info, current) || !current.Mode().IsRegular() || current.Size() != id.Bytes {
		return fileImportError("artifact replacement refused")
	}
	hash := sha256.New()
	n, e := io.Copy(hash, fileContextReader{ctx, io.LimitReader(f, id.Bytes+1)})
	if e != nil || n != id.Bytes || hex.EncodeToString(hash.Sum(nil)) != expected {
		return fileImportError("artifact integrity refused")
	}
	return nil
}
