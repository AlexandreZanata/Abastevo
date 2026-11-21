package adapters

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgdata"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgdata/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrUsageConflict = errors.New("orgdata: request identity reused")
	ErrQuotaExceeded = errors.New("orgdata: current term quota exhausted")
	ErrNoUsagePolicy = errors.New("orgdata: usage policy unavailable")
)

type RateExceeded struct{ RetryAfter time.Duration }

func (RateExceeded) Error() string { return "orgdata: credential rate exhausted" }

// DataAllowanceSource uses the owning subscription ledger in the retained
// authorization transaction.
type DataAllowanceSource interface {
	DataAllowanceAt(context.Context, pgx.Tx, kernel.OrgScope, time.Time) (billing.DataAllowance, error)
}

type UsageCall struct {
	ID   string
	Hash [32]byte
}

type UsageMeter struct {
	Keys  KeyAccess
	Plans DataAllowanceSource
}

// Read binds a current scoped key and immutable query identity to a single
// data read. The reader must check current source admission in this transaction
// and publish its result only after Read returns successfully.
func (m UsageMeter) Read(ctx context.Context, scope kernel.OrgScope, bearer domain.Bearer, required domain.Scope, call UsageCall,
	read func(context.Context, pgx.Tx) error,
) error {
	if m.Plans == nil || read == nil {
		return app.ErrUnavailable
	}
	if !audit.ValidID(call.ID) || call.Hash == ([32]byte{}) {
		return domain.ErrInvalidDataset
	}
	return m.Keys.WithAuthorizedKey(ctx, scope, bearer, required, func(ctx context.Context, tx pgx.Tx, key domain.Key) error {
		if err := m.admit(ctx, tx, scope, key, required, call); err != nil {
			return err
		}
		return read(ctx, tx)
	})
}

func (m UsageMeter) admit(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, key domain.Key, required domain.Scope, call UsageCall) error {
	org := scope.OrgID().String()
	q := db.New(tx)
	if err := q.LockDataUsage(ctx, org); err != nil {
		return err
	}
	now, err := q.KeyClock(ctx)
	if err != nil || !now.Valid || now.Time.Unix() < 0 {
		return app.ErrUnavailable
	}
	if !key.LiveAt(now.Time, required) {
		return domain.ErrKeyDenied
	}
	allowance, err := m.Plans.DataAllowanceAt(ctx, tx, scope, now.Time)
	if errors.Is(err, billing.ErrLimitUndefined) || errors.Is(err, access.ErrEntitlementRequired) {
		return ErrNoUsagePolicy
	}
	if err != nil {
		return err
	}
	policy, err := q.ReadRatePolicy(ctx, db.ReadRatePolicyParams{ProductCode: allowance.Plan.Product,
		PlanCode: allowance.Plan.Plan, PlanRevision: allowance.Plan.Revision})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoUsagePolicy
	}
	if err != nil || policy.WindowSeconds <= 0 || allowance.Calls < 0 {
		return app.ErrUnavailable
	}
	family, err := q.ResolveKeyFamily(ctx, db.ResolveKeyFamilyParams{OrgID: org, KeyID: key.ID})
	if err != nil {
		return app.ErrUnavailable
	}
	seconds := int64(policy.WindowSeconds)
	start := time.Unix(now.Time.Unix()/seconds*seconds, 0).UTC()
	used, err := q.AddDataCallToWindow(ctx, db.AddDataCallToWindowParams{OrgID: org, FamilyID: family, WindowStart: usageTimestamp(start),
		WindowSeconds: policy.WindowSeconds, Ceiling: policy.WindowCalls})
	if err != nil {
		return err
	}
	if used != 1 {
		return RateExceeded{RetryAfter: start.Add(time.Duration(seconds) * time.Second).Sub(now.Time)}
	}
	return countDataCall(ctx, q, org, family, key.ID, call, allowance, now)
}

func countDataCall(ctx context.Context, q *db.Queries, org, family, key string, call UsageCall, allowance billing.DataAllowance, now pgtype.Timestamptz) error {
	digest := hex.EncodeToString(call.Hash[:])
	stored, err := q.ReadDataReceipt(ctx, db.ReadDataReceiptParams{OrgID: org, FamilyID: family, RequestID: call.ID})
	if err == nil {
		if stored != digest {
			return ErrUsageConflict
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	start := usageTimestamp(allowance.Term.Start)
	used, err := q.AddDataCallToTerm(ctx, db.AddDataCallToTermParams{OrgID: org, TermStart: start,
		TermEnd: usageTimestamp(allowance.Term.End), Ceiling: allowance.Calls})
	if err != nil {
		return err
	}
	if used != 1 {
		return ErrQuotaExceeded
	}
	return q.RecordDataReceipt(ctx, db.RecordDataReceiptParams{OrgID: org, FamilyID: family, KeyID: key, RequestID: call.ID,
		TermStart: start, AcceptedAt: now, RequestHash: digest})
}

func usageTimestamp(instant time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: instant, Valid: !instant.IsZero()}
}
