package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

// Caller is the server-resolved contributor: identity plus the attribution
// token that owns sessions. The client never supplies either.
type Caller struct {
	ContributorID string
	Fingerprint   string
	Token         string
}

var (
	ErrUnauthorized      = errors.New("evidence: authentication required")
	ErrQuotaDenied       = errors.New("evidence: quota exceeded")
	ErrConflict          = errors.New("evidence: same key, different intent")
	ErrSessionNotFound   = errors.New("evidence: unknown session")
	ErrReservationFailed = errors.New("evidence: reservation failed")
)

// QuotaDeniedError carries the retry delay for 429 mapping.
type QuotaDeniedError struct {
	RetryAfter time.Duration
}

func (e *QuotaDeniedError) Error() string { return ErrQuotaDenied.Error() }

// Unwrap matches ErrQuotaDenied so handlers map by errors.Is.
func (e *QuotaDeniedError) Unwrap() error { return ErrQuotaDenied }

// Ports declares every collaborator with stdlib-shaped signatures. The
// store port converges retries on the contributor-scoped natural key and
// conflicts on divergent payloads; real SQL uniqueness lands in T04.
type Ports struct {
	Clock      func() time.Time
	NewID      func() (string, error)
	NewKey     func() (string, error)
	CheckQuota func(ctx context.Context, subject, operation string) (time.Duration, error)
	Store      Store
}

// Store is the owned persistence port for reservations.
type Store interface {
	ByNaturalKey(ctx context.Context, contributorRef, clientSessionID string) (domain.Session, error)
	Save(ctx context.Context, s domain.Session) error
}

// Intent carries the client-declared upload intent: submission ID, media
// description and hash claim. No contributor identity and no object key.
type Intent struct {
	ClientSessionID string
	MIME            string
	DeclaredBytes   int64
	ClaimedSHA256   string
}

// Result is the safe reservation acknowledgment: identifiers and bounds,
// never a signed URL (presigned issuance lands in T02).
type Result struct {
	SessionID string
	ExpiresAt time.Time
	MaxBytes  int64
	Replayed  bool
}

// Reserve checks authentication and quota first — exhausted quota grants
// no reservation — then converges identical retries on the natural key and
// persists one owned session with a server-generated quarantine key.
func Reserve(ctx context.Context, p Ports, caller Caller, in Intent) (Result, error) {
	if caller.ContributorID == "" || caller.Token == "" {
		return Result{}, ErrUnauthorized
	}
	if strings.TrimSpace(in.ClientSessionID) == "" {
		return Result{}, domain.ErrInvalidSession
	}
	if _, err := p.CheckQuota(ctx, caller.ContributorID, "write"); err != nil {
		return Result{}, err
	}
	if prev, err := p.Store.ByNaturalKey(ctx, caller.Token, in.ClientSessionID); err == nil {
		if prev.MIME != in.MIME || prev.DeclaredBytes != in.DeclaredBytes ||
			!strings.EqualFold(prev.ClaimedSHA256, in.ClaimedSHA256) {
			return Result{}, ErrConflict
		}
		return Result{SessionID: prev.ID, ExpiresAt: prev.ExpiresAt, MaxBytes: prev.MaxBytes, Replayed: true}, nil
	} else if !errors.Is(err, ErrSessionNotFound) {
		return Result{}, err
	}
	now := p.Clock()
	id, err := p.NewID()
	if err != nil {
		return Result{}, err
	}
	key, err := p.NewKey()
	if err != nil {
		return Result{}, err
	}
	s, _, err := domain.NewSession(domain.Params{
		ID: id, ContributorRef: caller.Token,
		ClientSessionID: in.ClientSessionID, MIME: in.MIME,
		DeclaredBytes: in.DeclaredBytes, ClaimedSHA256: in.ClaimedSHA256,
		QuarantineKey: key, CreatedAt: now, PolicyVersion: domain.PolicyV1,
	})
	if err != nil {
		return Result{}, err
	}
	if err := p.Store.Save(ctx, s); err != nil {
		return Result{}, err
	}
	return Result{SessionID: s.ID, ExpiresAt: s.ExpiresAt, MaxBytes: s.MaxBytes}, nil
}
