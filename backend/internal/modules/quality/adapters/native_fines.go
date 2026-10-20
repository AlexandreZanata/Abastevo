package adapters

import (
	"context"
	"errors"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5/pgtype"
)

// PublishNativeFines preserves native semantics through the existing atomic
// edition/revision/provenance/correction machinery. Optional company-only
// associations remain separate from dated station findings.
func (p *Publisher) PublishNativeFines(ctx context.Context, in application.NativeFineInput, newID func() (string, error)) (string, error) {
	if e := in.Validate(); e != nil {
		return "", e
	}
	var bindings []application.CompanyBinding
	if in.CompanyAssociations != nil {
		sources := make(map[string]string, len(in.Observations))
		for _, row := range in.Observations {
			sources[row.ObservationKey] = row.SourceCNPJ
		}
		var e error
		bindings, e = fineCompanyBindings(in.CompanyAssociations, sources, p.companyGuard != nil)
		if e != nil {
			return "", e
		}
	}

	req := editionRequest{CompanyAssociations: in.CompanyAssociations, CompanyBindings: bindings, Source: "sanctions", Stream: domain.StreamSanction, Edition: in.Edition, Predecessor: in.Predecessor, Resources: in.Resources, Counts: in.Counts, ManifestHash: in.ManifestHash, Periods: in.Periods, NativeFineClock: &in.Clock}
	stage := func(q *quality.Queries, _, revision pgtype.UUID) error {
		n, e := q.RecordNativeFineClock(ctx, quality.RecordNativeFineClockParams{RevisionID: revision, ReportYear: int32(in.Clock.ReportYear), ExtractionMonth: in.Clock.ExtractionMonth})
		if e != nil {
			return e
		}
		if n != 1 {
			return errors.New("quality: native revision clock was not staged")
		}
		for _, row := range in.Observations {
			id, e := nextID(newID)
			if e != nil {
				return e
			}
			due, e := pgDate(row.DueOn)
			if e != nil {
				return e
			}
			if _, e = q.StageNativeFineObservation(ctx, quality.StageNativeFineObservationParams{ID: id, RevisionID: revision, NativeKey: row.ObservationKey, ProceedingRef: row.ProceedingRef, PartyCnpj: row.SourceCNPJ, AmountMinor: row.AmountMinor, Status: row.Status, EvidenceRef: row.EvidenceRef, ExtractionMonth: row.ExtractionMonth, ReportYear: int32(row.ReportYear), DufRef: row.DUFRef, Department: row.Department, DueOn: due, AmountText: row.AmountText, PaidText: row.PaidText, PaidMinor: row.PaidMinor}); e != nil {
				return e
			}
		}
		n, e = q.CountRevisionSanctions(ctx, revision)
		if e != nil {
			return e
		}
		if n != int64(len(in.Observations)) {
			return errors.New("quality: native staged count diverged")
		}
		return stageCompanyAssociations(ctx, q, revision, in.CompanyAssociations)
	}
	for attempt := 0; ; attempt++ {
		rev, retry, e := p.publishTx(ctx, req, newID, stage)
		if !retry || attempt >= 2 {
			return rev, e
		}
	}
}
