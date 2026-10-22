package adapters

import (
	"context"
	"errors"

	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/quality"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/domain"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrEnforcementAssociationInput = errors.New("quality: invalid enforcement company association plan")

// PlanEnforcementCompanyAssociations maps admitted enforcement source
// records to company observations and plans their associations under the
// current-company policy. Only records with an admitted company identifier
// are plannable; a record without one must stay source-global, so passing
// it here is refused instead of silently dropping it. Decisions remain
// company-only; a later read stage must fence live bindings in its own
// transaction. No Directory write occurs here.
func PlanEnforcementCompanyAssociations(ctx context.Context, records []application.EnforcementSourceRecord, decidedAt string, lookup application.CompanyIdentityLookup) ([]domain.CompanyObservationDecision, error) {
	observations := make([]domain.CompanyObservationRecord, 0, len(records))
	for _, record := range records {
		if record.SourceCNPJ == "" {
			return nil, ErrEnforcementAssociationInput
		}
		observations = append(observations, domain.CompanyObservationRecord{RecordKey: record.RecordKey, CNPJ: record.SourceCNPJ})
	}
	return application.PlanCompanyAssociations(ctx, observations, decidedAt, lookup)
}

// StageEnforcementCompanyAssociations stores one complete decision census
// revision-scoped. The revision must stage enforcement source facts; the
// stored count must equal the planned count or nothing is kept.
func StageEnforcementCompanyAssociations(ctx context.Context, q *quality.Queries, revision pgtype.UUID, decisionDay string, decisions []domain.CompanyObservationDecision) error {
	day, e := pgDate(decisionDay)
	if e != nil {
		return e
	}
	if _, e := q.RecordEnforcementCompanyAssociationContext(ctx, quality.RecordEnforcementCompanyAssociationContextParams{RevisionID: revision, PolicyVersion: domain.CompanyObservationPolicyVersion, AssociationScope: domain.CurrentCompanyOnly, DecisionDay: day, ObservationCount: int32(len(decisions))}); e != nil {
		return e
	}
	for _, d := range decisions {
		station, e := uuidOrNull(d.StationID)
		if e != nil {
			return e
		}
		if _, e := q.StageEnforcementCompanyAssociation(ctx, quality.StageEnforcementCompanyAssociationParams{RevisionID: revision, RecordKey: d.RecordKey, ResolutionKind: string(d.Kind), Reason: d.Reason, StationID: station}); e != nil {
			return e
		}
	}
	counts, e := q.EnforcementCompanyAssociationCounts(ctx, revision)
	if e != nil {
		return e
	}
	if counts.Total != int64(len(decisions)) {
		return errors.New("quality: staged enforcement census diverged")
	}
	return nil
}

// EnforcementAssociationDivergence flags a stored company decision that no
// longer matches live Directory evidence. Action is "unbind" when a bound
// link lost its current binding, and "review" when an unbound record became
// bindable (a rebound candidate that must not auto-publish under a frozen
// key). Anything else is still unbound and needs no action.
type EnforcementAssociationDivergence struct {
	RecordKey, OldKind, NewKind, Action string
}

// RevalidateEnforcementCompanyAssociations compares stored decisions with
// fresh evidence without writing anything. It is pure and deterministic:
// the same inputs always yield the same flags, so a missed job or restart
// simply re-runs it. Retirement, succession and suspension surface as
// "unbind"; a rebound surfaces as "review", never as an automatic binding.
func RevalidateEnforcementCompanyAssociations(stored []domain.CompanyObservationDecision, evidence map[string][]domain.IdentityEvidence, cnpjs map[string]string, decidedAt string) []EnforcementAssociationDivergence {
	out := []EnforcementAssociationDivergence{}
	for _, old := range stored {
		cnpj := cnpjs[old.RecordKey]
		fresh := domain.ResolveCompanyObservation(
			domain.CompanyObservationRecord{RecordKey: old.RecordKey, CNPJ: cnpj},
			decidedAt, evidence[cnpj], false)
		oldBound := old.Kind == domain.ResolutionBound
		newBound := fresh.Kind == domain.ResolutionBound && fresh.StationID == old.StationID
		switch {
		case oldBound && newBound:
		case oldBound:
			out = append(out, EnforcementAssociationDivergence{RecordKey: old.RecordKey, OldKind: string(old.Kind), NewKind: string(fresh.Kind), Action: "unbind"})
		case fresh.Kind == domain.ResolutionBound:
			out = append(out, EnforcementAssociationDivergence{RecordKey: old.RecordKey, OldKind: string(old.Kind), NewKind: string(fresh.Kind), Action: "review"})
		}
	}
	return out
}
