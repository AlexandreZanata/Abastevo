package jobs

import (
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/native"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/prepared"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const EnforcementImportKind = "quality-enforcement-import"

func enforcementImportError(code string) error {
	return errors.New("quality enforcement import: " + code)
}

type EnforcementFileImport struct {
	Pool    *pgxpool.Pool
	Publish *adapters.Publisher
	Fetch   func(context.Context, string) (FileArtifact, error)
	Prepare func(context.Context, ImportPayload, FileArtifact) (prepared.EnforcementBatch, error)
	Serial  func(context.Context, func(context.Context) error) error
}

func (EnforcementFileImport) Kind() string { return EnforcementImportKind }
func (EnforcementFileImport) Version() int { return 1 }
func decodeEnforcementPayload(raw []byte) (ImportPayload, error) {
	var p ImportPayload
	if len(raw) > 4096 || httpapi.DecodeJSON(raw, &p) != nil || p.Version != 1 || p.Source != "enforcement" || p.Stream != "enforcement-v1" || p.Member != "" || len(p.SHA256) != 64 || p.SHA256 != strings.ToLower(p.SHA256) || native.ValidateEnforcementSelection(p.URL, p.Edition) != nil {
		return p, enforcementImportError("invalid source payload")
	}
	if _, e := hex.DecodeString(p.SHA256); e != nil {
		return p, enforcementImportError("invalid source hash")
	}
	return p, nil
}
func (im EnforcementFileImport) Handle(ctx context.Context, job platformjobs.Job) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	p, e := decodeEnforcementPayload(job.Payload)
	if e != nil {
		return e
	}
	if im.Pool == nil || im.Publish == nil || im.Fetch == nil || im.Prepare == nil || im.Serial == nil {
		return enforcementImportError("source ports unavailable")
	}
	return im.Serial(ctx, func(ctx context.Context) error { return im.importSource(ctx, p) })
}
func (im EnforcementFileImport) importSource(ctx context.Context, p ImportPayload) (err error) {
	f, e := im.Fetch(ctx, p.URL)
	closed := false
	closeArtifact := func() error {
		closed = true
		if f.Close == nil || f.Close() != nil {
			return enforcementImportError("artifact cleanup unavailable")
		}
		if _, e := os.Lstat(f.Path); !errors.Is(e, os.ErrNotExist) {
			return enforcementImportError("raw artifact retained")
		}
		return nil
	}
	defer func() {
		if !closed && f.Close != nil {
			if e := closeArtifact(); e != nil && err == nil {
				err = e
			}
		}
	}()
	if e != nil {
		return enforcementImportError("download unavailable")
	}
	if f.Close == nil {
		return enforcementImportError("artifact ownership unavailable")
	}
	if e = verifyArtifact(ctx, f, p.SHA256); e != nil {
		return e
	}
	b, e := im.Prepare(ctx, p, f)
	if e != nil {
		return enforcementImportError("native preparation unavailable")
	}
	if e = verifyArtifact(ctx, f, p.SHA256); e != nil {
		return e
	}
	in := application.EnforcementSourceInput{Context: b.Context, Resources: []application.Resource{{URL: p.URL, SHA256: p.SHA256, Bytes: uint64(f.Identity.Bytes)}}, Counts: b.Counts, ManifestHash: b.ManifestHash, Records: b.Records}
	if b.SourceProfile != application.EnforcementSourceProfile || b.SourceFile != native.EnforcementSourceFile || in.Context.SourceFile != b.SourceFile || in.Validate() != nil {
		return enforcementImportError("typed source contract refused")
	}
	if e = closeArtifact(); e != nil {
		return e
	}
	q := quality.New(im.Pool)
	if mark, e := q.GetWatermark(ctx, quality.GetWatermarkParams{Source: p.Source, Stream: p.Stream}); e == nil {
		in.Predecessor = mark.LastEdition
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return enforcementImportError("watermark unavailable")
	}
	_, e = im.Publish.PublishEnforcementSource(ctx, in, platformjobs.NewUUIDv4)
	if errors.Is(e, application.ErrSupersededPublication) {
		return nil
	}
	if e != nil {
		return enforcementImportError("publication unavailable")
	}
	if e = q.SetWatermark(ctx, quality.SetWatermarkParams{Source: p.Source, Stream: p.Stream, LastEdition: p.Edition, LastHash: b.ManifestHash}); e != nil {
		return enforcementImportError("watermark acknowledgement unavailable")
	}
	return nil
}
