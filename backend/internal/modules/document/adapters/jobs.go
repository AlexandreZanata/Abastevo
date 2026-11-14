package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// SafetyKind is the document safety job (EMP-DOC-014, EDR-170). It runs on
// its own serial worker slot and is never claimed by the general dispatcher.
const (
	SafetyKind = "org-document-safety"
	// SafetyAttempts bounds one queued convergence; its final attempt
	// settles an incomplete check as CHECK_ATTEMPTS_EXHAUSTED.
	SafetyAttempts int32 = 10
)

// Purposes of the two per-document safety jobs.
const (
	CheckPurpose  = "check"
	ExpiryPurpose = "expiry"
)

// errSafetyPayload parks a malformed envelope: it can never become valid.
var errSafetyPayload = errors.New("document: unsupported safety payload")

// SafetyPayload is the reference-only versioned envelope (BR-PRIV-003).
type SafetyPayload struct {
	Version    int    `json:"version"`
	OrgID      string `json:"org_id"`
	DocumentID string `json:"document_id"`
}

// SafetyDedupe returns the queue de-duplication key of one purpose.
func SafetyDedupe(purpose string, scope kernel.OrgScope, id string) string {
	if purpose != CheckPurpose && purpose != ExpiryPurpose || scope.Validate() != nil || !validID(id) {
		return ""
	}
	return SafetyKind + ":" + purpose + ":" + scope.OrgID().String() + ":" + id
}

// ParseSafetyPayload accepts only the exact version-1 envelope.
func ParseSafetyPayload(raw []byte) (kernel.OrgScope, string, error) {
	var p SafetyPayload
	if httpapi.DecodeJSON(raw, &p) != nil || p.Version != 1 || !validID(p.OrgID) || !validID(p.DocumentID) {
		return kernel.OrgScope{}, "", errSafetyPayload
	}
	id, err := kernel.ParseOrgID(p.OrgID)
	if err != nil {
		return kernel.OrgScope{}, "", errSafetyPayload
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return kernel.OrgScope{}, "", errSafetyPayload
	}
	return scope, p.DocumentID, nil
}

// enqueueSafety queues one reference-only safety job in the caller's
// transaction; a repeat converges on the stored job (EDR-170).
func enqueueSafety(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id, purpose string, notBefore time.Time) error {
	dedupe := SafetyDedupe(purpose, scope, id)
	if dedupe == "" {
		return app.ErrUnavailable
	}
	payload, err := json.Marshal(SafetyPayload{Version: 1, OrgID: scope.OrgID().String(), DocumentID: id})
	if err != nil {
		return err
	}
	_, err = jobs.Enqueue(ctx, tx, SafetyKind, payload, dedupe, SafetyAttempts, notBefore)
	return err
}

// SafetyHandler runs Converge for one queued job.
type SafetyHandler struct{ Safety app.Safety }

var _ jobs.Handler = SafetyHandler{}

func (SafetyHandler) Kind() string { return SafetyKind }
func (SafetyHandler) Version() int { return 1 }

// Handle reports only static module errors to the queue, never provider text.
// A job outside its exact envelope never runs; the last attempt, also one
// reclaimed after its lease expired, settles an incomplete check.
func (h SafetyHandler) Handle(ctx context.Context, job jobs.Job) error {
	if h.Safety == nil {
		return app.ErrUnavailable
	}
	if job.Kind != SafetyKind {
		return errSafetyPayload
	}
	scope, id, err := ParseSafetyPayload(job.Payload)
	if err != nil || job.DedupeKey == "" ||
		job.DedupeKey != SafetyDedupe(CheckPurpose, scope, id) && job.DedupeKey != SafetyDedupe(ExpiryPurpose, scope, id) {
		return errSafetyPayload
	}
	final := job.MaxAttempts > 0 && job.Attempts >= job.MaxAttempts
	_, err = h.Safety.Converge(ctx, scope, id, final)
	switch {
	case err == nil, errors.Is(err, app.ErrNotFound):
		// A missing or another organization's document has nothing to converge.
		return nil
	case errors.Is(err, app.ErrNotDue):
		return app.ErrNotDue
	case errors.Is(err, app.ErrRetry):
		return app.ErrRetry
	}
	return app.ErrUnavailable
}
