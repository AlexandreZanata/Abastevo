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
	ErrUnauthorized       = errors.New("evidence: authentication required")
	ErrQuotaDenied        = errors.New("evidence: quota exceeded")
	ErrStorageUnavailable = errors.New("evidence: storage not configured")
	ErrConflict           = errors.New("evidence: same key, different intent")
	ErrSessionNotFound    = errors.New("evidence: unknown session")
	ErrReservationFailed  = errors.New("evidence: reservation failed")
)

// MaxReservesPerDay bounds photo reservations per contributor per UTC day
// (abuse-signal defaults): issuance quotas cap bandwidth abuse, and the
// worker verifies actual bytes afterward regardless of declared intent.
const MaxReservesPerDay = 10

// QuotaDeniedError carries the retry delay for 429 mapping.
type QuotaDeniedError struct {
	RetryAfter time.Duration
}

func (e *QuotaDeniedError) Error() string { return ErrQuotaDenied.Error() }

// Unwrap matches ErrQuotaDenied so handlers map by errors.Is.
func (e *QuotaDeniedError) Unwrap() error { return ErrQuotaDenied }

// Ports declares every collaborator with stdlib-shaped signatures. The
// store port converges retries on the contributor-scoped natural key and
// conflicts on divergent payloads; the presign port mints the short
// direct-upload authorization (fresh on every call, including replays,
// since URLs expire).
type Ports struct {
	Clock      func() time.Time
	NewID      func() (string, error)
	NewKey     func() (string, error)
	CheckQuota func(ctx context.Context, subject, operation string) (time.Duration, error)
	Presign    func(ctx context.Context, key, mime string, maxBytes int64) (url string, headers map[string]string, expires time.Time, err error)
	Store      Store
}

// Store is the owned persistence port for reservations: one atomic
// insert-or-converge plus the daily-budget counter. The SQL
// implementation converges identical retries and conflicts on divergent
// payloads inside the transaction; fakes mirror that contract.
type Store interface {
	ReserveSession(ctx context.Context, s domain.Session) (stored domain.Session, replayed bool, err error)
	CountSince(ctx context.Context, contributorRef string, since time.Time) (int, error)
}

// Intent carries the client-declared upload intent: submission ID, media
// description and hash claim. No contributor identity and no object key.
type Intent struct {
	ClientSessionID string
	MIME            string
	DeclaredBytes   int64
	ClaimedSHA256   string
}

// Result is the safe reservation acknowledgment: identifiers, bounds
// and one fresh short upload authorization. It carries no object key
// beyond what the presigned URL itself binds.
type Result struct {
	SessionID       string
	ExpiresAt       time.Time
	MaxBytes        int64
	URL             string
	URLExpiresAt    time.Time
	RequiredHeaders map[string]string
	Replayed        bool
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
	now := p.Clock()
	used, err := p.Store.CountSince(ctx, caller.Token, dayStart(now))
	if err != nil {
		return Result{}, err
	}
	if used >= MaxReservesPerDay {
		next := dayStart(now).Add(24 * time.Hour)
		return Result{}, &QuotaDeniedError{RetryAfter: next.Sub(now)}
	}
	id, err := p.NewID()
	if err != nil {
		return Result{}, err
	}
	key, err := p.NewKey()
	if err != nil {
		return Result{}, err
	}
	built, _, err := domain.NewSession(domain.Params{
		ID: id, ContributorRef: caller.Token,
		ClientSessionID: in.ClientSessionID, MIME: in.MIME,
		DeclaredBytes: in.DeclaredBytes, ClaimedSHA256: in.ClaimedSHA256,
		QuarantineKey: key, CreatedAt: now, PolicyVersion: domain.PolicyV1,
	})
	if err != nil {
		return Result{}, err
	}
	stored, replayed, err := p.Store.ReserveSession(ctx, built)
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return Result{}, ErrConflict
		}
		return Result{}, err
	}
	bundleURL, headers, urlExpires, err := p.Presign(ctx, stored.QuarantineKey, stored.MIME, stored.MaxBytes)
	if err != nil {
		return Result{}, err
	}
	return Result{
		SessionID: stored.ID, ExpiresAt: stored.ExpiresAt, MaxBytes: stored.MaxBytes,
		URL: bundleURL, URLExpiresAt: urlExpires,
		RequiredHeaders: headers, Replayed: replayed,
	}, nil
}

// dayStart truncates to the current UTC date for the daily photo budget.
func dayStart(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
