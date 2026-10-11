package http

import (
	"sync"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
)

// BreakerError denies an operation at the deliberate global ceiling
// with the retry delay for 503 mapping. Handlers translate it to
// 503 plus Retry-After; nothing else in the error carries request
// content, and per-subject fairness budgets are untouched.
type BreakerError struct {
	RetryAfter time.Duration
}

func (e *BreakerError) Error() string { return "identity: global overload" }

// GlobalBreaker is the deliberate process-local hourly ceiling per
// operation (SEC02-C), separate from the per-client/key fairness
// budgets enforced by CheckQuota. A zero limit disables that
// operation (no global cap; fairness still enforced). Counts reset
// on the hourly window; Check is mutex-guarded so concurrent
// requests cannot overshoot. Multi-replica deployments size the
// edge layer separately; this breaker sheds single-process
// overload before per-subject accounting is consumed.
type GlobalBreaker struct {
	mu     sync.Mutex
	limits map[string]int
	counts map[string]int
	window time.Time
	now    func() time.Time
}

// NewGlobalBreaker wires hourly ceilings for challenge, register and
// write operations. Non-positive values disable that operation.
func NewGlobalBreaker(challenge, register, write int) *GlobalBreaker {
	return &GlobalBreaker{
		limits: map[string]int{
			domain.OperationChallenge: challenge,
			domain.OperationRegister:  register,
			domain.OperationWrite:     write,
		},
		counts: map[string]int{},
		now:    time.Now,
	}
}

// Check consumes one unit of the global budget. The returned
// RetryAfter is positive exactly when the call is denied.
func (b *GlobalBreaker) Check(op string, now time.Time) (time.Duration, error) {
	if b == nil {
		return 0, nil
	}
	limit, ok := b.limits[op]
	if !ok || limit <= 0 {
		return 0, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	current := now.UTC().Truncate(time.Hour)
	if b.window.IsZero() || !b.window.Equal(current) {
		b.window = current
		b.counts = map[string]int{}
	}
	if b.counts[op] >= limit {
		wait := current.Add(time.Hour).Sub(now)
		return wait, &BreakerError{RetryAfter: wait}
	}
	b.counts[op]++
	return 0, nil
}
