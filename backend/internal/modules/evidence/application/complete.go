package application

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// CompleteStore is the persistence port for completion intents and owner
// reads. Missing and foreign sessions share one not-found error so reads
// never distinguish them (B-BR-011).
type CompleteStore interface {
	Session(ctx context.Context, id string) (domain.Session, error)
	CompleteSession(ctx context.Context, id string, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) error
	ObjectIDBySession(ctx context.Context, sessionID string) (string, error)
}

// CompletePorts wires completion through the store and the durable job
// enqueue threaded into the claim transaction.
type CompletePorts struct {
	Store      CompleteStore
	EnqueueJob func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error
}

// StatusView is the owner-safe session projection: state and deadline,
// the evidence reference only once READY, never keys or URLs.
type StatusView struct {
	SessionID  string
	State      string
	ExpiresAt  time.Time
	UpdatedAt  time.Time
	EvidenceID string
}

// Complete records one completion intent: ISSUED claims VERIFYING with
// its verify job atomically, VERIFYING replays idempotently, and
// terminal states report themselves with no new work.
func Complete(ctx context.Context, p CompletePorts, caller Caller, sessionID string) (StatusView, error) {
	if caller.ContributorID == "" || caller.Token == "" {
		return StatusView{}, ErrUnauthorized
	}
	sess, err := p.Store.Session(ctx, sessionID)
	if err != nil {
		return StatusView{}, ErrSessionNotFound
	}
	if sess.ContributorRef != caller.Token {
		return StatusView{}, ErrSessionNotFound
	}
	if sess.PhotoCaptureID != "" && !time.Now().Before(sess.ExpiresAt) {
		return view(ctx, p.Store, sess)
	}
	switch sess.Status {
	case domain.StateIssued, domain.StateVerifying:
		if err := p.Store.CompleteSession(ctx, sess.ID, func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
		}); err != nil {
			return StatusView{}, err
		}
		updated, err := p.Store.Session(ctx, sess.ID)
		if err != nil {
			return StatusView{}, err
		}
		return view(ctx, p.Store, updated)
	default:
		return view(ctx, p.Store, sess)
	}
}

// Status returns one owned session projection. Missing and foreign
// sessions share one 404 without distinction.
func Status(ctx context.Context, store CompleteStore, caller Caller, sessionID string) (StatusView, error) {
	if caller.Token == "" {
		return StatusView{}, ErrUnauthorized
	}
	sess, err := store.Session(ctx, sessionID)
	if err != nil {
		return StatusView{}, ErrSessionNotFound
	}
	if sess.ContributorRef != caller.Token {
		return StatusView{}, ErrSessionNotFound
	}
	return view(ctx, store, sess)
}

func view(ctx context.Context, store CompleteStore, sess domain.Session) (StatusView, error) {
	v := StatusView{
		SessionID: sess.ID, State: sess.Status,
		ExpiresAt: sess.ExpiresAt, UpdatedAt: sess.UpdatedAt,
	}
	if sess.PhotoCaptureID != "" && !time.Now().Before(sess.ExpiresAt) {
		v.State = domain.StateExpired
		return v, nil
	}
	if sess.Status != domain.StateReady {
		return v, nil
	}
	evidenceID, err := store.ObjectIDBySession(ctx, sess.ID)
	if err != nil {
		return StatusView{}, err
	}
	v.EvidenceID = evidenceID
	return v, nil
}
