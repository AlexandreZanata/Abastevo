package adapters

import (
	"context"
	"errors"
	"time"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/maintenance"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

// EMP-TRIP-019 (EDR-334): the trip owners' checklist gate over the CHECK-001
// templates and the CHECK-002 runs, on the borrowed intake transaction.

// TripChecklistGate answers the departure requirement of an asset and a run's
// facts at an instant. It takes no row lock: the caller holds the
// organization root lock that every run insert and release takes first.
type TripChecklistGate struct {
	// PreGateImage is set only by rehearsals that populate a schema image
	// before migration 000255, where no trip can link a run or store a gate
	// reason: the gate then requires nothing and finds no run. Production
	// leaves it false.
	PreGateImage bool
}

var _ tripapp.ChecklistGate = TripChecklistGate{}

func gateDB(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (syncapp.DBTX, bool) {
	db = syncapp.Borrow(db)
	return db, db != nil && database.ValidateOrgDBTX(ctx, db, scope) == nil
}

// DepartureRequiredDBTX applies BR-CHECK-002 to the asset's current
// DEPARTURE templates (B-BR-EMP-512).
func (g TripChecklistGate) DepartureRequiredDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, asset string) (bool, error) {
	db, ok := gateDB(ctx, db, scope)
	if !ok || !checklist.CanonicalID(asset) {
		return false, tripapp.ErrUnavailable
	}
	if g.PreGateImage {
		return false, nil
	}
	resolved, err := applicable(ctx, q.New(db), scope.OrgID().String(), asset, checklist.Departure)
	if err != nil {
		return false, tripapp.ErrUnavailable
	}
	return resolved.Match != checklist.MatchNone, nil
}

// ChecklistRunDBTX reads a run of the organization at the instant: whether
// its server outcome was BLOCKING then (EDR-285: from its declared completion
// until its release was recorded) and the validity of its template's policy
// revision in force then by the server's history (the latest revision
// effective at or before it, else revision 1).
func (g TripChecklistGate) ChecklistRunDBTX(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, run string, at time.Time) (tripapp.ChecklistRun, error) {
	db, ok := gateDB(ctx, db, scope)
	if !ok || !checklist.CanonicalID(run) || at.IsZero() {
		return tripapp.ChecklistRun{}, tripapp.ErrUnavailable
	}
	if g.PreGateImage {
		return tripapp.ChecklistRun{}, tripapp.ErrNotFound
	}
	org := scope.OrgID().String()
	queries := q.New(db)
	row, err := queries.ChecklistRunGateFacts(ctx, q.ChecklistRunGateFactsParams{OrgID: org, ID: run})
	if errors.Is(err, pgx.ErrNoRows) {
		return tripapp.ChecklistRun{}, tripapp.ErrNotFound
	}
	if err != nil {
		return tripapp.ChecklistRun{}, tripapp.ErrUnavailable
	}
	completed, ok := runInstant(row.OccurredAt)
	if !ok || row.OccurredSubmicro < 0 || row.OccurredSubmicro > 999 || completed.Nanosecond()%1000 != 0 {
		return tripapp.ChecklistRun{}, tripapp.ErrUnavailable
	}
	completed = completed.Add(time.Duration(row.OccurredSubmicro))
	history, err := queries.ChecklistPolicyHistory(ctx, q.ChecklistPolicyHistoryParams{OrgID: org, TemplateID: row.TemplateID})
	if err != nil || len(history) == 0 || history[0].Revision != 1 {
		return tripapp.ChecklistRun{}, tripapp.ErrUnavailable
	}
	validity := history[0].ValiditySeconds
	for _, p := range history {
		effective, ok := runInstant(p.EffectiveAt)
		if !ok {
			return tripapp.ChecklistRun{}, tripapp.ErrUnavailable
		}
		if !effective.After(at) {
			validity = p.ValiditySeconds
		}
	}
	blocking := row.RecheckOutcome == string(checklist.Blocking) && !at.Before(completed)
	if blocking && row.ReleasedAt.Valid {
		released, ok := runInstant(row.ReleasedAt)
		if !ok {
			return tripapp.ChecklistRun{}, tripapp.ErrUnavailable
		}
		blocking = at.Before(released)
	}
	return tripapp.ChecklistRun{ID: row.ID, AssetID: row.AssetID, Context: row.Context, AuthorMembershipID: row.AuthorMembershipID, CompletedAt: completed,
		BlockingAt: blocking, ValiditySeconds: int64(validity)}, nil
}
