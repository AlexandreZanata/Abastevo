package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Sweep handles one evidence-sweep job: a single retention pass with an
// observable report. The run converges on repeat; database failures
// retry with backoff until the cap. Storage failures, unknown health and
// overdue copies are observable partial failures, never successful runs.
type Sweep struct {
	Run               func(ctx context.Context) (evidenceapp.SweepReport, error)
	BeginObservation  func(context.Context) (int64, error)
	FinishObservation func(context.Context, int64, bool) error
}

// Kind implements jobs.Handler.
func (Sweep) Kind() string { return "evidence-sweep" }

// Version implements jobs.Handler.
func (Sweep) Version() int { return 1 }

// Handle parses the versioned envelope and runs one retention pass.
func (v Sweep) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("sweep: bad payload: %w", err)
	}
	if payload.Version != 1 {
		return fmt.Errorf("sweep: bad payload")
	}
	if v.Run == nil || v.BeginObservation == nil || v.FinishObservation == nil {
		return fmt.Errorf("sweep: sweep/observation ports required")
	}
	beginCtx, stop := context.WithTimeout(ctx, 3*time.Second)
	epoch, err := v.BeginObservation(beginCtx)
	stop()
	if err != nil || epoch < 1 {
		return errors.Join(err, errors.New("evidence sweep: observation could not start"))
	}
	rep, err := v.Run(ctx)
	err = errors.Join(err, ctx.Err())
	healthy := err == nil && rep.HealthKnown && rep.OverdueKnown && rep.StorageErrors == 0 && !rep.Overdue.Unhealthy()
	ackCtx, finish := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	ackErr := v.FinishObservation(ackCtx, epoch, healthy)
	finish()
	err = errors.Join(err, ackErr)
	// The report is the observable cleanup outcome: every expired or
	// deleted unit counted, storage failures counted for the next pass.
	// Intake posture rides along as counts/bytes/timestamp only — never
	// keys, hashes or payloads. A remaining overdue backlog alarms at
	// warn level with the worst lag per namespace so the next minute
	// run's catch-up is watched, not silent.
	args := []any{
		"expired_sessions", rep.ExpiredSessions,
		"skipped_active", rep.SkippedActive,
		"quarantines_deleted", rep.QuarantinesDeleted,
		"finals_deleted", rep.FinalsDeleted,
		"objects_purged", rep.ObjectsPurged,
		"storage_errors", rep.StorageErrors,
		"health_known", rep.HealthKnown,
		"overdue_known", rep.OverdueKnown,
		"reserved_sessions", rep.Health.ReservedSessions,
		"reserved_bytes", rep.Health.ReservedBytes,
		"accepted_today", rep.Health.AcceptedToday,
		"accepted_bytes", rep.Health.AcceptedBytes,
		"rejected_today", rep.Health.RejectedToday,
		"rejected_bytes", rep.Health.RejectedBytes,
		"oldest_incomplete", rep.Health.OldestIncomplete.UTC().Format(time.RFC3339),
		"overdue_finals", rep.Overdue.Count,
		"overdue_originals", rep.Overdue.Originals,
	}
	if err != nil || !rep.HealthKnown || !rep.OverdueKnown || rep.StorageErrors > 0 || rep.Overdue.Unhealthy() {
		args = append(args,
			"oldest_overdue_final", rep.Overdue.Oldest.UTC().Format(time.RFC3339),
			"oldest_overdue_original", rep.Overdue.OldestOriginal.UTC().Format(time.RFC3339),
		)
		slog.Warn("evidence.sweep.incomplete", args...)
		return errors.Join(err, errors.New("evidence sweep: cleanup or health unresolved"))
	}
	slog.Info("evidence.sweep", args...)
	return nil
}
