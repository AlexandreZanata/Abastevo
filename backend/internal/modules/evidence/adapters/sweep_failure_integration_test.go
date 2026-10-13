//go:build integration

package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/jackc/pgx/v5/pgconn"
)

// Exercise actual SQL failures after committed cleanup. The synthetic byte
// sink isolates observability from provider protocol (covered separately).
type sweepSQLFault struct {
	*Store
	healthFault bool
}

func (s sweepSQLFault) IntakeHealth(ctx context.Context, since, now time.Time) (app.IntakeHealth, error) {
	if !s.healthFault {
		return s.Store.IntakeHealth(ctx, since, now)
	}
	var n int
	err := s.pool.QueryRow(ctx, "SELECT 1/0").Scan(&n)
	return app.IntakeHealth{}, err
}
func (s sweepSQLFault) OverdueFinals(ctx context.Context, cutoff time.Time, batch int) ([]domain.ObjectRef, error) {
	if s.healthFault {
		return s.Store.OverdueFinals(ctx, cutoff, batch)
	}
	var n int
	err := s.pool.QueryRow(ctx, "SELECT 1/0").Scan(&n)
	return nil, err
}

type sweepSuccessfulStorage struct{}

func (sweepSuccessfulStorage) DeleteQuarantine(context.Context, string) error { return nil }
func (sweepSuccessfulStorage) DeleteFinal(context.Context, string) error      { return nil }
func TestSweepSQLObservationFailurePreservesCleanupAndUnknown(t *testing.T) {
	for _, healthFault := range []bool{true, false} {
		t.Run(map[bool]string{true: "health", false: "overdue"}[healthFault], func(t *testing.T) {
			store, pool := freshStore(t)
			ctx := context.Background()
			at := time.Now()
			session := testSessionAt("e0000000-0000-4000-8000-000000000090", "sweep-failure", at.Add(-25*time.Hour))
			if _, _, err := store.ReserveSession(ctx, session, openBudget()); err != nil {
				t.Fatal(err)
			}
			rep, err := app.Sweep(ctx, app.SweepDeps{Clock: func() time.Time { return at }, Batch: 100, Store: sweepSQLFault{Store: store, healthFault: healthFault}, Jobs: hasNoLiveJobs{}, Storage: sweepSuccessfulStorage{}})
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "22012" {
				t.Fatal("SQL failure lost", err)
			}
			if rep.QuarantinesDeleted != 1 || rep.HealthKnown == healthFault || rep.OverdueKnown != healthFault {
				t.Fatal("partial effects/UNKNOWN lost", rep)
			}
			var deleted bool
			if err := pool.QueryRow(ctx, `SELECT quarantine_deleted_at IS NOT NULL FROM evidence_sessions WHERE id=$1`, session.ID).Scan(&deleted); err != nil || !deleted {
				t.Fatal("cleanup marker not retained", deleted, err)
			}
		})
	}
}
