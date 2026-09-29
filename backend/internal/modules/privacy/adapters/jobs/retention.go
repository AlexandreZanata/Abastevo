package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Retention handles one privacy-retention job: every retention
// category purges bounded oldest-first, and the per-category report
// (purged counts plus oldest overdue instants) goes to the structured
// log for the operator metrics surface. A category failure fails the
// job after all categories ran, so the next tick resumes converged
// categories with zero work.
type Retention struct {
	Run func(ctx context.Context) (privacyapp.RetentionReport, error)
	Log func(msg string, args ...any)
}

// Kind implements jobs.Handler.
func (Retention) Kind() string { return "privacy-retention" }

// Version implements jobs.Handler.
func (Retention) Version() int { return 1 }

// Handle parses the versioned envelope and delegates to one sweep.
func (r Retention) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("retention: bad payload: %w", err)
	}
	if payload.Version != 1 {
		return fmt.Errorf("retention: bad payload")
	}
	if r.Run == nil {
		return fmt.Errorf("retention: no sweep configured")
	}
	report, err := r.Run(ctx)
	if r.Log != nil {
		args := make([]any, 0, 2+2*len(report.Categories))
		args = append(args, "generated_at", report.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z"))
		for _, c := range report.Categories {
			overdue := ""
			if !c.OldestOverdue.IsZero() {
				overdue = c.OldestOverdue.UTC().Format("2006-01-02T15:04:05Z")
			}
			args = append(args, c.Category+"/purged", c.Purged, c.Category+"/oldest_overdue", overdue)
		}
		r.Log("retention sweep", args...)
	}
	return err
}
