package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Sweep runs one revoked-copy purge pass (CS10C, AT05): revoked
// derivatives past the 24-hour deadline lose their namespace bytes
// while receipts stay for the restore filter. Failures report
// through the report and fail the handler for bounded queue retry. Partial
// cleanup must never be acknowledged as a successful completed sweep.
type Sweep struct {
	Run func(ctx context.Context) (privacyapp.SweepReport, error)
}

// Kind implements jobs.Handler.
func (Sweep) Kind() string { return "privacy-training-sweep" }

// Version implements jobs.Handler.
func (Sweep) Version() int { return 1 }

// Handle parses the versioned envelope and runs one pass.
func (s Sweep) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("sweep: bad payload: %w", err)
	}
	if payload.Version != 1 {
		return fmt.Errorf("sweep: bad payload")
	}
	if s.Run == nil {
		return fmt.Errorf("sweep: no sweep configured")
	}
	rep, err := s.Run(ctx)
	if err != nil {
		return err
	}
	if len(rep.Failures) > 0 {
		return fmt.Errorf("training sweep: %d unresolved copies", len(rep.Failures))
	}
	return nil
}
