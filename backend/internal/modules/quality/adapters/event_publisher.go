package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
)

func pgDateOrNull(layout string) (pgtype.Date, error) {
	if layout == "" {
		return pgtype.Date{}, nil
	}
	return pgDate(layout)
}

func pgInt8OrNull(v *int64) pgtype.Int8 {
	if v == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *v, Valid: true}
}

// PublishEvents validates one enforcement/sanction manifest plus staged
// rows and publishes exactly one revision through the shared edition
// machinery: editions, manifests, revisions and the read lease behave
// exactly like assay publications, scoped per source/stream. Procedural
// scope/status stay source-verbatim; classification belongs to FQ-06/FQ-07.
func (p *Publisher) PublishEvents(ctx context.Context, in application.EventsInput, newID func() (string, error)) (string, error) {
	if err := in.Validate(); err != nil {
		return "", err
	}
	req := editionRequest{
		Source: in.Source, Stream: in.Stream, Edition: in.Edition,
		Predecessor: in.Predecessor, Resources: in.Resources,
		Counts: in.Counts, ManifestHash: in.ManifestHash, Periods: in.Periods,
	}
	stage := func(q *quality.Queries, editionID, revisionID pgtype.UUID) error {
		for _, e := range in.Enforcement {
			occurred, derr := pgDateOrNull(e.OccurredOn)
			if derr != nil {
				return derr
			}
			asOf, derr := pgDate(e.AsOf)
			if derr != nil {
				return derr
			}
			rowID, rerr := nextID(newID)
			if rerr != nil {
				return rerr
			}
			if _, serr := q.StageEnforcementEvent(ctx, quality.StageEnforcementEventParams{
				ID: rowID, RevisionID: revisionID, NativeKey: e.NativeKey,
				Scope: e.Scope, Status: e.Status, SubjectCnpj: e.SubjectCNPJ,
				OccurredOn: occurred, AsOf: asOf, EvidenceRef: e.EvidenceRef,
			}); serr != nil {
				return serr
			}
		}
		staged, cerr := q.CountRevisionEnforcement(ctx, revisionID)
		if cerr != nil {
			return cerr
		}
		if staged != int64(len(in.Enforcement)) {
			return errors.New("adapters: staged enforcement count diverged; rolled back")
		}
		for _, s := range in.Sanctions {
			decided, derr := pgDateOrNull(s.DecidedOn)
			if derr != nil {
				return derr
			}
			asOf, derr := pgDate(s.AsOf)
			if derr != nil {
				return derr
			}
			rowID, rerr := nextID(newID)
			if rerr != nil {
				return rerr
			}
			if _, serr := q.StageSanctionEvent(ctx, quality.StageSanctionEventParams{
				ID: rowID, RevisionID: revisionID, NativeKey: s.NativeKey,
				ProceedingRef: s.ProceedingRef, PartyCnpj: s.PartyCNPJ,
				AmountMinor: pgInt8OrNull(s.AmountMinor), Status: s.Status,
				DecidedOn: decided, AsOf: asOf, EvidenceRef: s.EvidenceRef,
			}); serr != nil {
				return serr
			}
		}
		staged, cerr = q.CountRevisionSanctions(ctx, revisionID)
		if cerr != nil {
			return cerr
		}
		if staged != int64(len(in.Sanctions)) {
			return errors.New("adapters: staged sanction count diverged; rolled back")
		}
		return nil
	}
	for attempt := 0; ; attempt++ {
		rev, retry, err := p.publishTx(ctx, req, newID, stage)
		if !retry || attempt >= 2 {
			return rev, err
		}
	}
}
