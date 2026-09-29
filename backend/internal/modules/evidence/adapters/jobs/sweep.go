package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Sweep handles one evidence-sweep job: a single retention pass with an
// observable report. The run converges on repeat; database failures
// retry with backoff until the cap, storage failures stay counted
// inside the report for the next pass.
type Sweep struct {
	Run func(ctx context.Context) (evidenceapp.SweepReport, error)
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
	if v.Run == nil {
		return fmt.Errorf("sweep: no sweep configured")
	}
	rep, err := v.Run(ctx)
	if err != nil {
		return err
	}
	// The report is the observable cleanup outcome: every expired or
	// deleted unit counted, storage failures counted for the next pass.
	slog.Info("evidence.sweep",
		"expired_sessions", rep.ExpiredSessions,
		"skipped_active", rep.SkippedActive,
		"quarantines_deleted", rep.QuarantinesDeleted,
		"finals_deleted", rep.FinalsDeleted,
		"objects_purged", rep.ObjectsPurged,
		"storage_errors", rep.StorageErrors,
	)
	return nil
}
