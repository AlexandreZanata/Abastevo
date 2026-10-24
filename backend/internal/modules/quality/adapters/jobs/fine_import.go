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
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const NativeFineImportKind = "quality-sanctions-import"

type FineFileImport struct {
	Associate      func(context.Context, []domain.CompanyObservationRecord) (*application.CompanyAssociationInput, error)
	Pool           *pgxpool.Pool
	Publish        *adapters.Publisher
	Fetch          func(context.Context, string) (FileArtifact, error)
	Prepare        func(context.Context, ImportPayload, FileArtifact) (prepared.FineBatch, error)
	PrepareHistory func(context.Context, ImportPayload, FileArtifact) (prepared.FineHistoryBatch, error)
	Serial         func(context.Context, func(context.Context) error) error
	// AfterPublish is the optional durable export intent. It runs after the
	// watermark/recompute acknowledgement; a failure is visible to the job and
	// recovered by periodic export/portal reconciliation, never by rolling back
	// the published revision.
	AfterPublish func(context.Context, string) error
}

func (FineFileImport) Kind() string     { return NativeFineImportKind }
func (FineFileImport) Version() int     { return 1 }
func fineImportError(code string) error { return errors.New("quality fine import: " + code) }
func decodeFinePayload(raw []byte) (ImportPayload, error) {
	var p ImportPayload
	if len(raw) > 4096 || httpapi.DecodeJSON(raw, &p) != nil || p.Version != 1 || p.Source != "sanctions" || p.Stream != "sanction-v1" || p.Member != "" || len(p.SHA256) != 64 || p.SHA256 != strings.ToLower(p.SHA256) || native.ValidateFineResourceSelection(p.URL, p.Edition) != nil {
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
	if im.Pool == nil || im.Publish == nil || im.Fetch == nil || im.Serial == nil || (payload.Edition == "2025" && im.Prepare == nil) || (payload.Edition != "2025" && im.PrepareHistory == nil) {
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
	publish, manifestHash, e := im.preparedFinePublication(ctx, payload, artifact)
	if e != nil {
		return e
	}
	if e = verifyArtifact(ctx, artifact, payload.SHA256); e != nil {
		return e
	}
	// Raw source may contain names/CPF. Confirm cleanup before any publication;
	// only the verified minimized typed batch survives this point.
	if e = closeArtifact(); e != nil {
		return e
	}
	q := quality.New(im.Pool)
	predecessor := ""
	if mark, e := q.GetWatermark(ctx, quality.GetWatermarkParams{Source: payload.Source, Stream: payload.Stream}); e == nil {
		predecessor = mark.LastEdition
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return fineImportError("watermark unavailable")
	}
	revision, e := publish(ctx, predecessor)
	if errors.Is(e, application.ErrSupersededPublication) {
		return nil
	}
	if e != nil {
		return fineImportError("publication unavailable")
	}
	if e = q.SetWatermark(ctx, quality.SetWatermarkParams{Source: payload.Source, Stream: payload.Stream, LastEdition: payload.Edition, LastHash: manifestHash}); e != nil {
		return fineImportError("watermark acknowledgement unavailable")
	}
	if e = EnqueueRecompute(ctx, im.Pool, payload.Source, payload.Stream, revision); e != nil {
		return fineImportError("derived intent acknowledgement unavailable")
	}
	if im.AfterPublish != nil {
		if e := im.AfterPublish(ctx, revision); e != nil {
			return fineImportError("export intent acknowledgement unavailable")
		}
	}
	return nil
}

func (im FineFileImport) preparedFinePublication(ctx context.Context, payload ImportPayload, artifact FileArtifact) (func(context.Context, string) (string, error), string, error) {
	resources := []application.Resource{{URL: payload.URL, SHA256: payload.SHA256, Bytes: uint64(artifact.Identity.Bytes)}}
	if payload.Edition == "2025" {
		batch, e := im.Prepare(ctx, payload, artifact)
		if e != nil {
			return nil, "", fineImportError("native preparation unavailable")
		}
		in := application.NativeFineInput{Edition: payload.Edition, Resources: resources, Clock: application.NativeFineClock{ReportYear: batch.ReportYear, ExtractionMonth: batch.ExtractionMonth}, Counts: batch.Counts, ManifestHash: batch.ManifestHash, Observations: batch.Observations}
		if in.Validate() != nil {
			return nil, "", fineImportError("typed source contract refused")
		}
		if im.Associate != nil {
			records := make([]domain.CompanyObservationRecord, 0, len(batch.Observations))
			for _, row := range batch.Observations {
				records = append(records, domain.CompanyObservationRecord{RecordKey: row.ObservationKey, CNPJ: row.SourceCNPJ})
			}
			in.CompanyAssociations, e = im.companyPlan(ctx, records)
			if e != nil {
				return nil, "", e
			}
		}
		return func(ctx context.Context, predecessor string) (string, error) {
			in.Predecessor = predecessor
			return im.Publish.PublishNativeFines(ctx, in, platformjobs.NewUUIDv4)
		}, batch.ManifestHash, nil
	}
	batch, e := im.PrepareHistory(ctx, payload, artifact)
	if e != nil {
		return nil, "", fineImportError("historical preparation unavailable")
	}
	in := application.HistoricalFineInput{Context: application.FineHistoryContext{InputContract: batch.InputContract, SourcePeriod: batch.SourcePeriod, SourceFile: batch.SourceFile, HeaderRecord: batch.HeaderRecord, Clock: batch.Clock, SourceRecordCount: batch.Counts.Input, MetadataRecords: batch.MetadataRecords}, Resources: resources, Counts: batch.Counts, ManifestHash: batch.ManifestHash, Observations: batch.Observations, Notes: batch.Notes}
	if batch.SourcePeriod != payload.Edition || in.Validate() != nil {
		return nil, "", fineImportError("typed historical contract refused")
	}
	if im.Associate != nil {
		records := make([]domain.CompanyObservationRecord, 0, len(batch.Observations))
		for _, row := range batch.Observations {
			records = append(records, domain.CompanyObservationRecord{RecordKey: row.ObservationKey, CNPJ: row.SourceCNPJ})
		}
		in.CompanyAssociations, e = im.companyPlan(ctx, records)
		if e != nil {
			return nil, "", e
		}
	}
	return func(ctx context.Context, predecessor string) (string, error) {
		in.Predecessor = predecessor
		return im.Publish.PublishHistoricalFines(ctx, in, platformjobs.NewUUIDv4)
	}, batch.ManifestHash, nil
}

func (im FineFileImport) companyPlan(ctx context.Context, records []domain.CompanyObservationRecord) (*application.CompanyAssociationInput, error) {
	plan, e := im.Associate(ctx, records)
	if e != nil || plan == nil {
		return nil, fineImportError("company identity planning unavailable")
	}
	sources := make(map[string]string, len(records))
	for _, r := range records {
		sources[r.RecordKey] = r.CNPJ
	}
	if _, e := plan.Bindings(sources); e != nil {
		return nil, fineImportError("company identity plan refused")
	}
	return plan, nil
}
