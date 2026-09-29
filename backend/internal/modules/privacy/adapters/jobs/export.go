package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// ExportBuild handles one privacy-export-build job: exactly one bounded
// owner archive, assembled through the application Build use-case. A
// nil error completes it (including terminal replays); anything else
// retries with backoff until the cap, then parks for audited replay.
type ExportBuild struct {
	Build func(ctx context.Context, requestID string) (string, bool, error)
}

// Kind implements jobs.Handler.
func (ExportBuild) Kind() string { return "privacy-export-build" }

// Version implements jobs.Handler.
func (ExportBuild) Version() int { return 1 }

// Handle parses the versioned envelope and delegates to one request.
func (e ExportBuild) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version   int    `json:"version"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("export: bad payload: %w", err)
	}
	if payload.Version != 1 || strings.TrimSpace(payload.RequestID) == "" {
		return fmt.Errorf("export: bad payload")
	}
	if e.Build == nil {
		return fmt.Errorf("export: no build configured")
	}
	_, _, err := e.Build(ctx, strings.TrimSpace(payload.RequestID))
	return err
}
