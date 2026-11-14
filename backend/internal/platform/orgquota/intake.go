package orgquota

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TransportDefinition meters delivery attempts, never grants a permission.
// Every deployment/feature registration must supply a reviewed policy explicitly.
type TransportDefinition struct {
	Class                         string
	Organization, Account, Device identity.OperationQuota
	Rationale                     string
}

func (TransportDefinition) String() string   { return "orgquota: transport policy redacted" }
func (TransportDefinition) GoString() string { return "orgquota: transport policy redacted" }
func (TransportDefinition) LogValue() slog.Value {
	return slog.StringValue("orgquota: transport policy redacted")
}
func (TransportDefinition) MarshalJSON() ([]byte, error) {
	return json.Marshal("orgquota: transport policy redacted")
}

type IntakeBudget struct {
	guard  *Guard
	policy entry
}

func (IntakeBudget) String() string   { return "orgquota: intake budget redacted" }
func (IntakeBudget) GoString() string { return "orgquota: intake budget redacted" }
func (IntakeBudget) LogValue() slog.Value {
	return slog.StringValue("orgquota: intake budget redacted")
}

func NewIntakeBudget(pool *pgxpool.Pool, key []byte, d TransportDefinition) (*IntakeBudget, error) {
	if pool == nil || len(key) < 32 || len(d.Class) > 64 || !classPattern.MatchString(d.Class) || strings.TrimSpace(d.Rationale) == "" || len(d.Rationale) > 512 || !validBudget(d.Organization) || !validBudget(d.Account) || !validBudget(d.Device) {
		return nil, errPolicy
	}
	b := &IntakeBudget{guard: &Guard{pool: pool, key: append([]byte(nil), key...)}, policy: entry{class: d.Class}}
	for i, q := range []identity.OperationQuota{d.Organization, d.Account, d.Device} {
		b.policy.axes[i] = identitypg.NewLimiter(pool, identity.QuotaPolicy{Version: 1, Ops: map[string]identity.OperationQuota{identity.OperationWrite: q}})
	}
	return b, nil
}

// CheckBatch reserves every attempted operation (including replays) atomically.
// Empty bodies spend one transport unit. There is no permission-shaped fallback.
func (b *IntakeBudget) CheckBatch(ctx context.Context, p orgapi.Principal, count int) error {
	if b == nil || b.guard == nil || b.guard.pool == nil || count < 0 || count > 50 || p.Channel != "android" || !validPrincipal(p) {
		return orgapi.Unavailable
	}
	units := max(1, count)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	subjects := b.guard.subjects(b.policy.class, p)
	err := database.WithOrgTx(ctx, b.guard.pool, p.Scope, func(tx pgx.Tx) error {
		var wait time.Duration
		// Same aggregate→account→device lock order as ordinary write admission.
		// Every counter update rolls back if any unit on any axis is denied.
		for i, limiter := range b.policy.axes {
			for range units {
				retry, e := limiter.CheckTx(ctx, tx, subjects[i], identity.OperationWrite)
				if errors.Is(e, identity.ErrQuotaExceeded) && retry > 0 {
					wait = max(wait, retry)
					break
				}
				if e != nil {
					return orgapi.Unavailable
				}
			}
		}
		if wait > 0 {
			return &orgapi.RateLimitError{RetryAfter: wait}
		}
		return nil
	})
	var limited *orgapi.RateLimitError
	if errors.As(err, &limited) && ctx.Err() == nil {
		return limited
	}
	if err != nil || ctx.Err() != nil {
		return orgapi.Unavailable
	}
	return nil
}
