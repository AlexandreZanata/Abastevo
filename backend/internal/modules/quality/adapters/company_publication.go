package adapters

import (
	"context"
	"errors"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5/pgtype"
)

func fineCompanyBindings(in *application.CompanyAssociationInput, sources map[string]string, guardAvailable bool) ([]application.CompanyBinding, error) {
	bindings, e := in.Bindings(sources)
	if e != nil {
		return nil, e
	}
	if len(bindings) > 0 && !guardAvailable {
		return nil, errors.New("quality: live company identity guard unavailable")
	}
	return bindings, nil
}

func stageCompanyAssociations(ctx context.Context, q *quality.Queries, revision pgtype.UUID, in *application.CompanyAssociationInput) error {
	if in == nil {
		return nil
	}
	day, e := pgDate(in.DecisionDay)
	if e != nil {
		return e
	}
	if _, e := q.RecordCompanyAssociationContext(ctx, quality.RecordCompanyAssociationContextParams{RevisionID: revision, PolicyVersion: in.PolicyVersion, AssociationScope: domain.CurrentCompanyOnly, DecisionDay: day, ObservationCount: int32(len(in.Decisions))}); e != nil {
		return e
	}
	for _, d := range in.Decisions {
		station, e := uuidOrNull(d.StationID)
		if e != nil {
			return e
		}
		if _, e := q.StageCompanyAssociation(ctx, quality.StageCompanyAssociationParams{RevisionID: revision, RecordKey: d.RecordKey, ResolutionKind: string(d.Kind), Reason: d.Reason, StationID: station}); e != nil {
			return e
		}
	}
	counts, e := q.CompanyAssociationCounts(ctx, revision)
	if e != nil {
		return e
	}
	if counts.Total != int64(len(in.Decisions)) {
		return errors.New("quality: staged company census diverged")
	}
	return nil
}
