package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	parent "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters"
	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// SessionStore is the persistence port for verification outcomes. The
// concrete *adapters.Store satisfies it; fakes prove the orchestration.
type SessionStore interface {
	Session(ctx context.Context, id string) (domain.Session, error)
	RecordVerified(ctx context.Context, sessionID string, obj parent.ObjectData) (string, error)
	RecordRejected(ctx context.Context, sessionID string, reasons []string) error
}

// Verify handles one verify-evidence job: exactly one VERIFYING session
// runs through the application orchestration, and its terminal outcome
// persists with the object row. Terminal replays converge without work;
// a job for a never-completed session fails toward DEAD with cause
// instead of succeeding silently.
type Verify struct {
	Store  SessionStore
	Verify func(ctx context.Context, session domain.Session) (evidenceapp.Outcome, error)
	NewID  func() (string, error)
}

// Kind implements jobs.Handler.
func (Verify) Kind() string { return "verify-evidence" }

// Version implements jobs.Handler.
func (Verify) Version() int { return 1 }

// Handle parses the versioned envelope and drives one session to its
// terminal state. Permanent media refusals complete the job after
// recording; anything else retries with backoff until the cap.
func (v Verify) Handle(ctx context.Context, job jobs.Job) error {
	var payload struct {
		Version  int    `json:"version"`
		UploadID string `json:"upload_id"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("verify: bad payload: %w", err)
	}
	if payload.Version != 1 || strings.TrimSpace(payload.UploadID) == "" {
		return fmt.Errorf("verify: bad payload")
	}
	if v.Store == nil || v.Verify == nil || v.NewID == nil {
		return fmt.Errorf("verify: no store, orchestration or ID source configured")
	}
	sess, err := v.Store.Session(ctx, payload.UploadID)
	if err != nil {
		return err
	}
	switch sess.Status {
	case domain.StateReady, domain.StateRejected, domain.StateExpired:
		return nil
	case domain.StateVerifying:
		out, err := v.Verify(ctx, sess)
		if err != nil {
			var rej *evidenceapp.RejectionError
			if errors.As(err, &rej) {
				return v.Store.RecordRejected(ctx, sess.ID, rej.Reasons)
			}
			return err
		}
		id, err := v.NewID()
		if err != nil {
			return err
		}
		_, err = v.Store.RecordVerified(ctx, sess.ID, parent.ObjectData{
			ID: id, FinalKey: out.FinalKey, SourceSHA256: out.SourceSHA256,
			SanitizedSHA256: out.SanitizedSHA256,
			Width:           out.Width, Height: out.Height, DHash: out.DHash,
			ReceivedAt: sess.UpdatedAt,
		})
		return err
	default:
		return fmt.Errorf("verify: session %s not yet completed", sess.ID)
	}
}
