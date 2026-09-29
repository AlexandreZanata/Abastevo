package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/platform"
)

var (
	// ErrStaleLease means another claim already superseded this holder: the
	// effect must not be applied or acknowledged twice.
	ErrStaleLease = errors.New("jobs: stale lease")
	// ErrNoJob means nothing claimable right now, not a failure.
	ErrNoJob = errors.New("jobs: nothing claimable")
)

// Job is one unit of durable work.
type Job struct {
	ID          string
	Kind        string
	Payload     []byte
	DedupeKey   string
	Status      string
	Attempts    int32
	MaxAttempts int32
	LeaseToken  int64
	WorkerID    string
	LastError   string
	CreatedAt   time.Time
}

// Queue moves jobs through the owned tables.
type Queue struct {
	pool *pgxpool.Pool
}

// NewQueue wires the owned generated queries to a pool.
func NewQueue(pool *pgxpool.Pool) *Queue {
	return &Queue{pool: pool}
}

// NewUUIDv4 mints identifiers from crypto/rand.
func NewUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32], nil
}

func mustUUID(text string) (pgtype.UUID, error) {
	raw, err := hex.DecodeString(stripDashes(text))
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("jobs: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func stripDashes(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '-' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

// Enqueue stores one job, sharing the caller's transaction when given one
// (pass the pool for standalone enqueue). Payload must be valid JSON;
// dedupe constrains repeats only when non-empty. Re-enqueueing an existing
// dedupe key converges on the stored id instead of forking the work.
func Enqueue(ctx context.Context, db platform.DBTX, kind string, payload []byte, dedupeKey string, maxAttempts int32, notBefore time.Time) (string, error) {
	if kind == "" {
		return "", errors.New("jobs: empty kind")
	}
	if !json.Valid(payload) {
		return "", errors.New("jobs: payload must be JSON")
	}
	if maxAttempts < 1 {
		return "", errors.New("jobs: max attempts must be positive")
	}
	id, err := NewUUIDv4()
	if err != nil {
		return "", err
	}
	uid, err := mustUUID(id)
	if err != nil {
		return "", err
	}
	var dedupe pgtype.Text
	if dedupeKey != "" {
		dedupe = pgtype.Text{String: dedupeKey, Valid: true}
	}
	if notBefore.IsZero() {
		notBefore = time.Now()
	}
	returned, err := platform.New(db).EnqueueJob(ctx, platform.EnqueueJobParams{
		ID:          uid,
		Kind:        kind,
		Payload:     payload,
		DedupeKey:   dedupe,
		MaxAttempts: maxAttempts,
		NotBefore:   pgtype.Timestamptz{Time: notBefore, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existing, rerr := platform.New(db).GetJobByDedupe(ctx, dedupe)
			if rerr != nil {
				return "", rerr
			}
			return uuidString(existing), nil
		}
		return "", err
	}
	return uuidString(returned), nil
}

// Claim takes the oldest claimable job of a kind ("" for any), fencing it
// with a fresh lease token. Expired leases re-enter the pool automatically.
func (q *Queue) Claim(ctx context.Context, kind, workerID string, leaseTTL time.Duration) (Job, error) {
	if workerID == "" {
		return Job{}, errors.New("jobs: worker id required")
	}
	if leaseTTL <= 0 {
		return Job{}, errors.New("jobs: positive lease TTL required")
	}
	row, err := platform.New(q.pool).ClaimJob(ctx, platform.ClaimJobParams{
		Kind:     kind,
		WorkerID: workerID,
		LeaseTtl: fmt.Sprintf("%d microseconds", leaseTTL.Microseconds()),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, ErrNoJob
		}
		return Job{}, err
	}
	return mapJob(row.ID, row.Kind, row.Payload, row.DedupeKey, row.Status, row.Attempts, row.MaxAttempts, row.LeaseToken, row.WorkerID, row.LastError, row.CreatedAt), nil
}

// Complete acknowledges a held lease. A superseded token fails instead of
// overwriting the new holder.
func (q *Queue) Complete(ctx context.Context, id string, token int64) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	n, err := platform.New(q.pool).CompleteJob(ctx, platform.CompleteJobParams{ID: uid, LeaseToken: token})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrStaleLease
	}
	return nil
}

// Fail records an attempt; capped jobs park in DEAD with the reason kept.
// Stale tokens fail without touching the new holder.
func (q *Queue) Fail(ctx context.Context, id string, token int64, retryDelay time.Duration, reason string) (string, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return "", err
	}
	if retryDelay < 0 {
		retryDelay = 0
	}
	status, err := platform.New(q.pool).FailJob(ctx, platform.FailJobParams{
		ID:         uid,
		LeaseToken: token,
		RetryDelay: fmt.Sprintf("%d microseconds", retryDelay.Microseconds()),
		LastError:  reason,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrStaleLease
		}
		return "", err
	}
	return status, nil
}

// ReplayDead requeues a DEAD job with an audited reason and a fresh budget.
func (q *Queue) ReplayDead(ctx context.Context, id, reason string) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	if reason == "" {
		return errors.New("jobs: replay needs an audited reason")
	}
	n, err := platform.New(q.pool).ReplayDeadJob(ctx, platform.ReplayDeadJobParams{ID: uid, Reason: reason})
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("jobs: not a dead job")
	}
	return nil
}

func mapJob(id pgtype.UUID, kind string, payload []byte, dedupe pgtype.Text, status string, attempts, maxAttempts int32, token int64, worker, lastErr string, created pgtype.Timestamptz) Job {
	return Job{
		ID: idString(id), Kind: kind, Payload: payload,
		DedupeKey: dedupe.String, Status: status,
		Attempts: attempts, MaxAttempts: maxAttempts,
		LeaseToken: token, WorkerID: worker, LastError: lastErr,
		CreatedAt: created.Time,
	}
}

func idString(id pgtype.UUID) string { return uuidString(id) }
