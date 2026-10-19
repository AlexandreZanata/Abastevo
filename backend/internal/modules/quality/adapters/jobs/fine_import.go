package jobs

import (
	"context"
	"encoding/hex"
	"errors"
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

const NativeFineImportKind = "quality-sanctions-import"

type FineFileImport struct {
	Pool    *pgxpool.Pool
	Publish *adapters.Publisher
	Fetch   func(context.Context, string) (FileArtifact, error)
	Prepare func(context.Context, ImportPayload, FileArtifact) (prepared.FineBatch, error)
	Serial  func(context.Context, func(context.Context) error) error
}

func (FineFileImport) Kind() string     { return NativeFineImportKind }
func (FineFileImport) Version() int     { return 1 }
func fineImportError(code string) error { return errors.New("quality fine import: " + code) }
func decodeFinePayload(raw []byte) (ImportPayload, error) {
	var p ImportPayload
	if len(raw) > 4096 || httpapi.DecodeJSON(raw, &p) != nil || p.Version != 1 || p.Source != "sanctions" || p.Stream != "sanction-v1" || p.Member != "" || len(p.SHA256) != 64 || p.SHA256 != strings.ToLower(p.SHA256) || native.ValidateFineSelection(p.URL, p.Edition) != nil {
		return p, fineImportError("invalid source payload")
	}
	if _, e := hex.DecodeString(p.SHA256); e != nil {
		return p, fineImportError("invalid source hash")
	}
	return p, nil
}
func (im FineFileImport) Handle(ctx context.Context, job platformjobs.Job) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	payload, e := decodeFinePayload(job.Payload)
	if e != nil {
		return e
	}
	if im.Pool == nil || im.Publish == nil || im.Fetch == nil || im.Prepare == nil || im.Serial == nil {
		return fineImportError("source ports unavailable")
	}
	return im.Serial(ctx, func(ctx context.Context) error { return im.importFine(ctx, payload) })
}
func (im FineFileImport) importFine(ctx context.Context, payload ImportPayload) (err error) {
	artifact, e := im.Fetch(ctx, payload.URL)
	closed := false
	closeArtifact := func() error {
		closed = true
		if artifact.Close == nil {
			return fineImportError("artifact ownership unavailable")
		}
		if artifact.Close() != nil {
			return fineImportError("artifact cleanup unavailable")
		}
		return nil
	}
	defer func() {
		if !closed && artifact.Close != nil {
			if closeErr := closeArtifact(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
	}()
	if e != nil {
		return fineImportError("download unavailable")
	}
	if artifact.Close == nil {
		return fineImportError("artifact ownership unavailable")
	}
	if e = verifyArtifact(ctx, artifact, payload.SHA256); e != nil {
		return e
	}
	batch, e := im.Prepare(ctx, payload, artifact)
	if e != nil {
		return fineImportError("native preparation unavailable")
	}
	if e = verifyArtifact(ctx, artifact, payload.SHA256); e != nil {
		return e
	}
	in := application.NativeFineInput{Edition: payload.Edition, Resources: []application.Resource{{URL: payload.URL, SHA256: payload.SHA256, Bytes: uint64(artifact.Identity.Bytes)}}, Clock: application.NativeFineClock{ReportYear: batch.ReportYear, ExtractionMonth: batch.ExtractionMonth}, Counts: batch.Counts, ManifestHash: batch.ManifestHash, Observations: batch.Observations}
	if in.Validate() != nil {
		return fineImportError("typed source contract refused")
	}
	// Raw source may contain names/CPF. Confirm cleanup before any publication;
	// only the verified minimized typed batch survives this point.
	if e = closeArtifact(); e != nil {
		return e
	}
	q := quality.New(im.Pool)
	if mark, e := q.GetWatermark(ctx, quality.GetWatermarkParams{Source: payload.Source, Stream: payload.Stream}); e == nil {
		in.Predecessor = mark.LastEdition
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return fineImportError("watermark unavailable")
	}
	revision, e := im.Publish.PublishNativeFines(ctx, in, platformjobs.NewUUIDv4)
	if errors.Is(e, application.ErrSupersededPublication) {
		return nil
	}
	if e != nil {
		return fineImportError("publication unavailable")
	}
	if e = q.SetWatermark(ctx, quality.SetWatermarkParams{Source: payload.Source, Stream: payload.Stream, LastEdition: payload.Edition, LastHash: batch.ManifestHash}); e != nil {
		return fineImportError("watermark acknowledgement unavailable")
	}
	if e = EnqueueRecompute(ctx, im.Pool, payload.Source, payload.Stream, revision); e != nil {
		return fineImportError("derived intent acknowledgement unavailable")
	}
	return nil
}
