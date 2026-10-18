package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// RecomputeKind is the durable recomputation handler for derived official
// dimensions. It is reserved from general claims and composed with the
// native source handlers.
const RecomputeKind = "quality-recompute"

// RecomputePayload is the versioned recomputation envelope: one published
// revision of one stream, resolved through the current-pointer fence.
type RecomputePayload struct {
	Version    int    `json:"version"`
	Source     string `json:"source"`
	Stream     string `json:"stream"`
	RevisionID string `json:"revision_id"`
}

// RecomputeProjections is the store capability the handler needs.
type RecomputeProjections interface {
	RecomputeRevision(ctx context.Context, source, stream, revisionID string) (adapters.ProjectionReport, error)
}

// Recompute judges and projects every active linked sample of one published
// revision. A basis that is no longer the current published revision is a
// completed no-op, so a stale worker can never overwrite newer derived rows
// and a retry never needs a manual reset.
type Recompute struct {
	Store RecomputeProjections
}

func (Recompute) Kind() string { return RecomputeKind }

// Version implements jobs.Handler.
func (Recompute) Version() int { return 1 }

// EnqueueRecompute records the recomputation intent for one published
// revision. The dedupe identity is the revision itself: replays and retries
// converge instead of forking, and a correction revision always gets its own.
func EnqueueRecompute(ctx context.Context, pool *pgxpool.Pool, source, stream, revisionID string) error {
	payload, err := json.Marshal(RecomputePayload{Version: 1, Source: source, Stream: stream, RevisionID: revisionID})
	if err != nil {
		return err
	}
	_, err = platformjobs.Enqueue(ctx, pool, RecomputeKind, payload, "quality-recompute:"+revisionID, 5, time.Time{})
	return err
}

// Handle validates the envelope and runs one recomputation.
func (r Recompute) Handle(ctx context.Context, job platformjobs.Job) error {
	if r.Store == nil {
		return errors.New("quality recompute: ports unavailable")
	}
	var payload RecomputePayload
	if len(job.Payload) > 4096 || httpapi.DecodeJSON(job.Payload, &payload) != nil ||
		payload.Version != 1 || payload.Source == "" || payload.Stream == "" ||
		len(payload.RevisionID) != 36 {
		return errors.New("quality recompute: invalid payload")
	}
	if _, err := r.Store.RecomputeRevision(ctx, payload.Source, payload.Stream, payload.RevisionID); err != nil {
		if errors.Is(err, adapters.ErrProjectionBasisStale) {
			return nil
		}
		return err
	}
	return nil
}
