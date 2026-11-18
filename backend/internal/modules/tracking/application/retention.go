package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Tracking retention purge job (EMP-GPS-011, BR-GPS-035..036). Deletes expired
// live positions (24h post-trip), raw segments past the organization retention
// window, and raw segments collected strictly after an effective trip end while
// preserving tracking summaries and purge receipts.
const RetentionKind = "org-tracking-retention"

type RetentionResult struct {
	ExpiredLivePositions int
	PurgedSessions       int
	DeletedSegments      int
	TrimmedSegments      int
}

type RetentionPayload struct {
	Version int    `json:"version"`
	OrgID   string `json:"org_id,omitempty"`
}

type RetentionStore interface {
	PurgeExpired(ctx context.Context, orgID *kernel.OrgID, now time.Time) (RetentionResult, error)
}

type RetentionHandler struct {
	Store RetentionStore
	Now   func() time.Time
}

var _ jobs.Handler = RetentionHandler{}

func (RetentionHandler) Kind() string { return RetentionKind }
func (RetentionHandler) Version() int { return 1 }

func (h RetentionHandler) Handle(ctx context.Context, job jobs.Job) error {
	if ctx == nil || absent(h.Store) {
		return ErrUnavailable
	}
	var scopeOrg *kernel.OrgID
	if len(job.Payload) > 0 {
		var p RetentionPayload
		if httpapi.DecodeJSON(job.Payload, &p) != nil || (p.Version != 0 && p.Version != 1) {
			return ErrMalformed
		}
		if p.OrgID != "" {
			id, err := kernel.ParseOrgID(p.OrgID)
			if err != nil || id.String() != p.OrgID {
				return ErrMalformed
			}
			scopeOrg = &id
		}
	}
	now := time.Now().UTC()
	if h.Now != nil {
		now = h.Now().UTC()
	}
	_, err := h.Store.PurgeExpired(ctx, scopeOrg, now)
	return err
}

func MarshalRetentionPayload(orgID string) (json.RawMessage, error) {
	if orgID != "" {
		id, err := kernel.ParseOrgID(orgID)
		if err != nil || id.String() != orgID {
			return nil, errors.New("tracking: invalid org id for retention payload")
		}
	}
	return json.Marshal(RetentionPayload{Version: 1, OrgID: orgID})
}
