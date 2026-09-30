package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/identity"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
)

// QuotaError denies an operation with the retry delay for 429 mapping.
// Handlers translate it to 429 plus Retry-After; nothing else in the error
// carries request content.
type QuotaError struct {
	RetryAfter time.Duration
}

func (e *QuotaError) Error() string { return domain.ErrQuotaExceeded.Error() }
func (e *QuotaError) Unwrap() error { return domain.ErrQuotaExceeded }

// Limiter enforces versioned budgets with one atomic database counter per
// subject, operation and window. No successful operation can exceed quota
// under concurrency: the capped upsert admits at most Limit acceptances,
// and anything beyond fails here before expensive work starts (B-BR-015).
type Limiter struct {
	pool   *pgxpool.Pool
	policy domain.QuotaPolicy
	now    func() time.Time
}

// NewLimiter wires the owned generated queries to a pool under a policy.
func NewLimiter(pool *pgxpool.Pool, policy domain.QuotaPolicy) *Limiter {
	return &Limiter{pool: pool, policy: policy, now: time.Now}
}

// Check consumes one unit of budget. The returned RetryAfter is positive
// exactly when the call is denied.
func (l *Limiter) Check(ctx context.Context, subject, operation string) (time.Duration, error) {
	q, err := l.policy.Lookup(operation)
	if err != nil {
		return 0, err
	}
	if subject == "" {
		return 0, errors.New("adapters: empty quota subject")
	}
	now := l.now()
	start := domain.WindowStart(now, q.Window)
	row, err := identity.New(l.pool).ConsumeQuota(ctx, identity.ConsumeQuotaParams{
		SubjectDigest: subject,
		Operation:     operation,
		WindowStart:   pgtype.Timestamptz{Time: start, Valid: true},
		ExpiresAt:     pgtype.Timestamptz{Time: start.Add(q.Window), Valid: true},
		Cap:           q.Limit,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return start.Add(q.Window).Sub(now), &QuotaError{RetryAfter: start.Add(q.Window).Sub(now)}
		}
		return 0, err
	}
	_ = row
	return 0, nil
}

// Cleanup removes expired windows. Evidence retention follows the
// documented inventory; this table keeps no history beyond live windows.
func (l *Limiter) Cleanup(ctx context.Context) (int64, error) {
	return identity.New(l.pool).CleanupExpiredWindows(ctx)
}
