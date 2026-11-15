package adapters

import (
	"context"
	"errors"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestChecklistAuditActionsAreOnePerCommandKind(t *testing.T) {
	defs := ChecklistAuditDefinitions()
	if _, err := audit.NewCatalog(defs); err != nil || len(defs) != 4 {
		t.Fatal("audit catalog", err)
	}
	for _, d := range defs {
		if d.TargetType != TemplateTargetType {
			t.Fatal("audit target type", d.Action)
		}
	}
}

// The deferred exclusivity backstop maps to CONFLICT; provider details stay
// private; a missing row is not found.
func TestChecklistStoreErrorsKeepOnlyRegisteredRefusals(t *testing.T) {
	ctx := context.Background()
	for err, want := range map[error]error{
		&pgconn.PgError{Code: "23505", ConstraintName: "org_checklist_templates_target_taken"}: app.ErrConflict,
		&pgconn.PgError{Code: "23505", ConstraintName: "other"}:                                app.ErrUnavailable,
		audit.ErrReused:      app.ErrOperationReused,
		pgx.ErrNoRows:        app.ErrNotFound,
		checklist.ErrInvalid: checklist.ErrInvalid,
		app.ErrConflict:      app.ErrConflict,
	} {
		if got := storeError(ctx, err); !errors.Is(got, want) {
			t.Fatal("store error", err, got)
		}
	}
	if storeError(ctx, nil) != nil {
		t.Fatal("nil error")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if !errors.Is(storeError(cancelled, errors.New("x")), context.Canceled) {
		t.Fatal("cancellation")
	}
	var s *ChecklistStore
	if s.ready(false) {
		t.Fatal("nil store ready")
	}
}
