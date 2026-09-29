package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// ExportStore is the owned persistence port for export requests:
// idempotent intents plus guarded outcome writes and owner-scoped
// reads. Get is the unscoped worker-side read for the durable build
// job; all owner-facing paths use GetForOwner.
type ExportStore interface {
	RequestExport(ctx context.Context, r domain.Request, enqueue func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error) (id string, replayed bool, err error)
	CompleteExport(ctx context.Context, id string, archive []byte, sha string, readyAt time.Time) error
	FailExport(ctx context.Context, id string, at time.Time) error
	Get(ctx context.Context, id string) (domain.Request, error)
	GetForOwner(ctx context.Context, contributorID, id string) (domain.Request, []byte, error)
}

// Ports wires export requests through server clock and IDs, owner
// attribution, the atomic store, the inventory reader and the durable
// build enqueue.
type Ports struct {
	Clock       func() time.Time
	NewID       func() (string, error)
	Attribution func(ctx context.Context, contributorID string) (string, error)
	Store       ExportStore
	Inventory   InventoryReader
	EnqueueJob  func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error
}

// Caller carries the proof-derived owner identity: contributor ID from
// the verified key (never a body-supplied ID) — B-BR-004.
type Caller struct {
	ContributorID string
}

// RequestDTO carries one export intent with its client operation
// identity for safe retries.
type RequestDTO struct {
	ClientSubmissionID string
}

// RequestResult is the safe acknowledgment: the request identity and
// whether the call converged on an existing intent.
type RequestResult struct {
	RequestID string
	Replayed  bool
}

// Request validates and persists one export intent with its durable
// build job. Same contributor/key/body converges; a different payload
// conflicts (B-BR-005).
func Request(ctx context.Context, p Ports, caller Caller, dto RequestDTO) (RequestResult, error) {
	if strings.TrimSpace(caller.ContributorID) == "" {
		return RequestResult{}, ErrUnauthorized
	}
	if strings.TrimSpace(dto.ClientSubmissionID) == "" {
		return RequestResult{}, domain.ErrInvalidRequest
	}
	token, err := p.Attribution(ctx, caller.ContributorID)
	if err != nil {
		return RequestResult{}, err
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	id, err := p.NewID()
	if err != nil {
		return RequestResult{}, err
	}
	req, err := domain.NewRequest(domain.RequestParams{
		ID: id, ContributorID: caller.ContributorID, ContributorRef: token,
		ClientSubmissionID: dto.ClientSubmissionID, Type: domain.TypeExport,
		RequestedAt: now,
	})
	if err != nil {
		return RequestResult{}, err
	}
	storedID, replayed, err := p.Store.RequestExport(ctx, req,
		func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			return p.EnqueueJob(ctx, tx, kind, payload, dedupe)
		})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return RequestResult{}, ErrConflict
		}
		return RequestResult{}, err
	}
	return RequestResult{RequestID: storedID, Replayed: replayed}, nil
}

// StatusResult is the owner-safe poll view: lifecycle state plus the
// download window once READY. No archive bytes travel here.
type StatusResult struct {
	RequestID string
	Status    string
	ReadyAt   time.Time
	ExpiresAt time.Time
	Expired   bool
}

// Status returns one request's poll view to its owner only. Missing
// and foreign requests share one 404 shape (no ownership oracle).
func Status(ctx context.Context, p Ports, caller Caller, id string) (StatusResult, error) {
	if strings.TrimSpace(caller.ContributorID) == "" {
		return StatusResult{}, ErrUnauthorized
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	req, _, err := p.Store.GetForOwner(ctx, caller.ContributorID, strings.TrimSpace(id))
	if err != nil {
		return StatusResult{}, err
	}
	return StatusResult{
		RequestID: req.ID, Status: req.Status,
		ReadyAt: req.ReadyAt, ExpiresAt: req.ExpiresAt,
		Expired: domain.Expired(req, now),
	}, nil
}

// DownloadResult carries one READY archive with the hash to verify it.
type DownloadResult struct {
	Archive []byte
	SHA256  string
}

// Download returns one READY archive to its owner inside the 24 h
// window. Expired archives refuse (request a fresh export); missing,
// foreign and non-ready requests share safe shapes.
func Download(ctx context.Context, p Ports, caller Caller, id string) (DownloadResult, error) {
	if strings.TrimSpace(caller.ContributorID) == "" {
		return DownloadResult{}, ErrUnauthorized
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	req, archive, err := p.Store.GetForOwner(ctx, caller.ContributorID, strings.TrimSpace(id))
	if err != nil {
		return DownloadResult{}, err
	}
	if req.Status != domain.StatusReady {
		return DownloadResult{}, domain.ErrBadState
	}
	if domain.Expired(req, now) {
		return DownloadResult{}, domain.ErrExpired
	}
	return DownloadResult{Archive: archive, SHA256: req.ArchiveSHA256}, nil
}
