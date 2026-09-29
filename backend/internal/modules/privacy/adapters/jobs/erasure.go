package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	privacydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Erasure handles one privacy-erasure job: exactly one owner's scope
// removal, executed through the application Erase use-case. Terminal
// (non-REQUESTED) requests replay as no-ops; failures retry with
// backoff until the cap, then park for audited replay. The payload
// carries the request ID and the reviewed reason; owner identity
// resolves server-side from the loaded request.
type Erasure struct {
	Load  func(ctx context.Context, requestID string) (privacydomain.Request, error)
	Erase func(ctx context.Context, contributorID string, dto privacyapp.EraseDTO) (privacyapp.ErasureReport, error)
}

// Kind implements jobs.Handler.
func (Erasure) Kind() string { return "privacy-erasure" }

// Version implements jobs.Handler.
func (Erasure) Version() int { return 1 }

// Handle parses the versioned envelope and delegates to one request.
func (e Erasure) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version   int    `json:"version"`
		RequestID string `json:"request_id"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("erasure: bad payload: %w", err)
	}
	if payload.Version != 1 || strings.TrimSpace(payload.RequestID) == "" {
		return fmt.Errorf("erasure: bad payload")
	}
	if e.Load == nil || e.Erase == nil {
		return fmt.Errorf("erasure: no erasure configured")
	}
	req, err := e.Load(ctx, strings.TrimSpace(payload.RequestID))
	if err != nil {
		return err
	}
	if req.Status != privacydomain.StatusRequested {
		return nil
	}
	reason := strings.TrimSpace(payload.Reason)
	if reason == "" {
		reason = "owner deletion request"
	}
	_, err = e.Erase(ctx, req.ContributorID, privacyapp.EraseDTO{
		ClientSubmissionID: req.ClientSubmissionID, Reason: reason,
	})
	return err
}
