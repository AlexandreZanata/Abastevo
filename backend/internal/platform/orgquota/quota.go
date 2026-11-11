// Package orgquota composes private write admission on the existing DB limiter.
package orgquota

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	identitypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identity "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Definition is reviewed source configuration, never user input. No production
// default is selected: each class must carry measured caps and their rationale.
type Definition struct {
	Class                         string
	Permissions                   []string
	Organization, Account, Device identity.OperationQuota
	Rationale                     string
}

func (Definition) String() string       { return "orgquota: policy redacted" }
func (Definition) GoString() string     { return "orgquota: policy redacted" }
func (Definition) LogValue() slog.Value { return slog.StringValue("orgquota: policy redacted") }

type entry struct {
	class string
	axes  [3]*identitypg.Limiter
}
type Guard struct {
	pool        *pgxpool.Pool
	key         []byte
	permissions map[string]entry
}

func (Guard) String() string       { return "orgquota: guard redacted" }
func (Guard) GoString() string     { return "orgquota: guard redacted" }
func (Guard) LogValue() slog.Value { return slog.StringValue("orgquota: guard redacted") }

var _ orgapi.WriteAdmission = (*Guard)(nil)
var classPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
var permissionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
var errPolicy = errors.New("orgquota: invalid source policy")

func validBudget(q identity.OperationQuota) bool {
	// Inherit the <=24h quota retention inventory and whole-second retry
	// protocol. The existing limiter derives fixed windows from the application
	// clock; only its expired-window cleanup uses the database clock.
	return q.Limit > 0 && q.Window >= time.Second && q.Window <= 24*time.Hour && q.Window%time.Second == 0
}

func New(pool *pgxpool.Pool, key []byte, definitions []Definition) (*Guard, error) {
	if pool == nil || len(key) < 32 || len(definitions) == 0 {
		return nil, errPolicy
	}
	g := &Guard{pool: pool, key: append([]byte(nil), key...), permissions: map[string]entry{}}
	classes := map[string]bool{}
	for _, d := range definitions {
		if len(d.Class) > 64 || !classPattern.MatchString(d.Class) || classes[d.Class] || len(d.Permissions) == 0 ||
			len(strings.TrimSpace(d.Rationale)) == 0 || len(d.Rationale) > 512 ||
			!validBudget(d.Organization) || !validBudget(d.Account) || !validBudget(d.Device) {
			return nil, errPolicy
		}
		classes[d.Class] = true
		e := entry{class: d.Class}
		for i, q := range []identity.OperationQuota{d.Organization, d.Account, d.Device} {
			e.axes[i] = identitypg.NewLimiter(pool, identity.QuotaPolicy{Version: 1, Ops: map[string]identity.OperationQuota{identity.OperationWrite: q}})
		}
		for _, permission := range d.Permissions {
			if len(permission) > 128 || !permissionPattern.MatchString(permission) {
				return nil, errPolicy
			}
			if _, exists := g.permissions[permission]; exists {
				return nil, errPolicy
			}
			g.permissions[permission] = e
		}
	}
	return g, nil
}

func (g *Guard) Validate(rule orgapi.Rule) error {
	if g == nil {
		return orgapi.Unavailable
	}
	if !rule.Write {
		return nil
	}
	if _, ok := g.permissions[rule.Permission]; !ok {
		return orgapi.Unavailable
	}
	return nil
}
func validPrincipal(p orgapi.Principal) bool {
	if p.Scope.Validate() != nil || !audit.ValidID(p.AccountID) || !audit.ValidID(p.MembershipID) {
		return false
	}
	switch p.Channel {
	case "android":
		return audit.ValidID(p.DeviceKeyID) && audit.ValidID(p.ContributorID)
	case "web":
		return p.DeviceKeyID == "" && p.ContributorID == ""
	default:
		return false
	}
}

func (g *Guard) subjects(class string, p orgapi.Principal) [3]string {
	var out [3]string
	device := p.DeviceKeyID
	if p.Channel == "web" {
		device = "web"
	}
	for i, axis := range []string{"organization", "account", "device"} {
		owner, key := "", ""
		if i > 0 {
			owner = p.AccountID
		}
		if i == 2 {
			key = device
		}
		raw, _ := json.Marshal([]string{"orgquota.v1", class, axis, p.Scope.OrgID().String(), owner, key})
		mac := hmac.New(sha256.New, g.key)
		_, _ = mac.Write(raw)
		out[i] = "orgquota:v1:" + hex.EncodeToString(mac.Sum(nil))
	}
	return out
}

// Check consumes admission, not successful business writes. All axes commit
// together before module work; quota denial and DB failure roll them all back.
func (g *Guard) Check(ctx context.Context, p orgapi.Principal, rule orgapi.Rule) error {
	if g.Validate(rule) != nil {
		return orgapi.Unavailable
	}
	if !rule.Write {
		return nil
	}
	if !validPrincipal(p) {
		return orgapi.Unavailable
	}
	e := g.permissions[rule.Permission]
	subjects := g.subjects(e.class, p)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := database.WithOrgTx(ctx, g.pool, p.Scope, func(tx pgx.Tx) error {
		// Common aggregate first, then account and device: every request uses
		// the same lock order, with no external service call under these locks.
		// After a denial the remaining axes are still read, inside the same
		// rolled-back transaction, so Retry-After covers every exhausted window.
		var wait time.Duration
		for i, limiter := range e.axes {
			retry, err := limiter.CheckTx(ctx, tx, subjects[i], identity.OperationWrite)
			if errors.Is(err, identity.ErrQuotaExceeded) && retry > 0 {
				wait = max(wait, retry)
				continue
			}
			if err != nil {
				return orgapi.Unavailable
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
