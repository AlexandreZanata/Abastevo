package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Validate handles one validate-observation job. Run is the application
// orchestration bound at the composition root; the job ID travels as the
// persisted command proof so RECEIVED→VALIDATING is worker-owned, never
// claimed from an HTTP handler.
type Validate struct {
	Run func(ctx context.Context, observationID, commandRef string) (string, error)
}

// Kind implements jobs.Handler.
func (Validate) Kind() string { return "validate-observation" }

// Version implements jobs.Handler.
func (Validate) Version() int { return 1 }

// Handle parses the versioned envelope and delegates to the orchestration.
// A nil result error completes the job (validated, rejected or already
// terminal); any error retries with backoff until the cap, then parks for
// audited replay. Malformed envelopes fail toward DEAD with cause.
func (v Validate) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version       int    `json:"version"`
		ObservationID string `json:"observation_id"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("validate: bad payload: %w", err)
	}
	if payload.Version != 1 || strings.TrimSpace(payload.ObservationID) == "" {
		return fmt.Errorf("validate: bad payload")
	}
	if v.Run == nil {
		return fmt.Errorf("validate: no orchestration configured")
	}
	_, err := v.Run(ctx, payload.ObservationID, job.ID)
	return err
}
