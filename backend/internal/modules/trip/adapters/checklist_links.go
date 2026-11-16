package adapters

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// EMP-TRIP-019 checklist links and gate reasons (B-BR-EMP-513..515,
// EDR-334, migration 000255), written on the borrowed intake transaction
// under the caller's root and asset locks. The database re-proves a link
// against the stored trip and a BLOCKING or EXPIRED reason against the run's
// history; the trip stanza does not include the checklist tables, so these
// are raw parametrized statements (the EMP-TRIP-023 precedent).

func checklistContext(kind string) bool {
	return kind == app.ChecklistDeparture || kind == app.ChecklistReturn
}

func gateReason(reason string) bool {
	switch reason {
	case app.ChecklistRequired, app.ChecklistBlocking, app.ChecklistExpired, app.OrphanReference:
		return true
	}
	return false
}

// borrowed validates the borrowed intake transaction of the scope.
func (s *Store) borrowed(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope) (syncapp.DBTX, bool) {
	db = syncapp.Borrow(db)
	return db, s != nil && db != nil && database.ValidateOrgDBTX(ctx, db, scope) == nil
}

// ChecklistLinkedTx answers whether a trip is linked to the run.
func (s *Store) ChecklistLinkedTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, run string) (bool, error) {
	db, ok := s.borrowed(ctx, db, scope)
	if !ok || !canonicalID(run) {
		return false, app.ErrUnavailable
	}
	var linked bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM org_trip_checklist_links WHERE org_id=$1::uuid AND run_id=$2::uuid)`, scope.OrgID().String(), run).Scan(&linked); err != nil {
		return false, storeError(err)
	}
	return linked, nil
}

// ChecklistLinkTx reads the run linked to the trip for the context.
func (s *Store) ChecklistLinkTx(ctx context.Context, db syncapp.DBTX, scope kernel.OrgScope, trip, kind string) (string, error) {
	db, ok := s.borrowed(ctx, db, scope)
	if !ok || !canonicalID(trip) || !checklistContext(kind) {
		return "", app.ErrUnavailable
	}
	var run string
	if err := db.QueryRow(ctx, `SELECT COALESCE((SELECT run_id::text FROM org_trip_checklist_links WHERE org_id=$1::uuid AND trip_id=$2::uuid AND context=$3),'')`,
		scope.OrgID().String(), trip, kind).Scan(&run); err != nil {
		return "", storeError(err)
	}
	return run, nil
}

// LinkChecklistTx stores the departure or return link of the trip.
func (s *Store) LinkChecklistTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, trip, kind, run, operation string) error {
	db, ok := s.borrowed(ctx, db, source.Scope)
	if !ok || source.Validate() != nil || !canonicalID(trip) || !checklistContext(kind) || !canonicalID(run) || !canonicalID(operation) {
		return app.ErrUnavailable
	}
	if _, err := db.Exec(ctx, `INSERT INTO org_trip_checklist_links(org_id,trip_id,context,run_id,operation_id) VALUES($1::uuid,$2::uuid,$3,$4::uuid,$5::uuid)`,
		source.Scope.OrgID().String(), trip, kind, run, operation); err != nil {
		return storeError(err)
	}
	return nil
}

// HoldChecklistTx stores the gate reason beside a held start.
func (s *Store) HoldChecklistTx(ctx context.Context, db syncapp.DBTX, source syncapp.Source, operation, review, reason, run string) error {
	db, ok := s.borrowed(ctx, db, source.Scope)
	if !ok || source.Validate() != nil || !canonicalID(operation) || !canonicalID(review) || !gateReason(reason) || run != "" && !canonicalID(run) {
		return app.ErrUnavailable
	}
	if _, err := db.Exec(ctx, `INSERT INTO org_trip_checklist_holds(org_id,operation_id,review_item_id,reason,run_id) VALUES($1::uuid,$2::uuid,$3::uuid,$4,NULLIF($5,'')::uuid)`,
		source.Scope.OrgID().String(), operation, review, reason, run); err != nil {
		return storeError(err)
	}
	return nil
}
