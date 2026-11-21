package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	payments "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const adjustmentAction = "subs.adjustment.posted"

var (
	ErrAdjustmentUnavailable = errors.New("organization billing: reviewed adjustment unavailable")
	ErrAdjustmentPosted      = errors.New("organization billing: adjustment approval already posted")
)

func AdjustmentAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: adjustmentAction, TargetType: "subscription", RequireReason: true,
		Reasons: []string{"service_credit", "debit_correction"}}}
}

type AdjustmentReceipt struct {
	SubscriptionID       string
	SubscriptionRevision int64
	Kind                 domain.AdjustmentKind
	DeltaCentavos        int64
	PostedAt             time.Time
	Replayed             bool
}

type AdjustmentStore struct {
	Pool  *pgxpool.Pool
	Audit auditing.TransactionRunner
}

func (s AdjustmentStore) Apply(ctx context.Context, scope kernel.OrgScope, approval, operator, operation string) (AdjustmentReceipt, error) {
	if s.Pool == nil || s.Audit == nil {
		return AdjustmentReceipt{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || !audit.ValidID(approval) || !audit.ValidID(operator) || !audit.ValidID(operation) {
		return AdjustmentReceipt{}, ErrAdjustmentUnavailable
	}
	var result AdjustmentReceipt
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var err error
		result, err = s.applyTx(ctx, tx, scope, approval, operator, operation)
		return err
	})
	if err != nil {
		return AdjustmentReceipt{}, err
	}
	return result, nil
}

func (s AdjustmentStore) applyTx(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, approval, operator, operation string) (AdjustmentReceipt, error) {
	q := db.New(tx)
	org := scope.OrgID().String()
	record, err := q.ReadAdjustmentApproval(ctx, db.ReadAdjustmentApprovalParams{OrgID: org, ApprovalID: approval, OperatorID: operator})
	if errors.Is(err, pgx.ErrNoRows) {
		return AdjustmentReceipt{}, ErrAdjustmentUnavailable
	}
	if err != nil {
		return AdjustmentReceipt{}, err
	}
	kind := domain.AdjustmentKind(record.Kind)
	delta, err := domain.AdjustmentDelta(kind, payments.Money{Centavos: record.AmountCentavos, Currency: record.Currency})
	if err != nil || !record.ValidFrom.Valid || !record.ValidUntil.Valid {
		return AdjustmentReceipt{}, app.ErrUnavailable
	}
	body, err := json.Marshal([]any{1, org, approval, record.SubscriptionID, record.SubscriptionRevision, kind, delta})
	if err != nil {
		return AdjustmentReceipt{}, app.ErrUnavailable
	}
	hash := sha256.Sum256(body)
	event := audit.Event{OperationID: operation, ActorID: operator, Action: adjustmentAction, TargetType: "subscription",
		TargetID: record.SubscriptionID, ReasonCode: strings.ToLower(record.Kind), RequestHash: hex.EncodeToString(hash[:])}
	replay, err := s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
		now, err := q.OrganizationBillingClock(ctx)
		if err != nil || !now.Valid {
			return app.ErrUnavailable
		}
		if now.Time.Before(record.ValidFrom.Time) || !now.Time.Before(record.ValidUntil.Time) {
			return ErrAdjustmentUnavailable
		}
		count, err := q.RecordAdjustmentPosting(ctx, db.RecordAdjustmentPostingParams{
			OrgID: org, OperationID: operation, ApprovalID: approval, DeltaCentavos: delta,
		})
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrAdjustmentPosted
		}
		return nil
	})
	if err != nil {
		return AdjustmentReceipt{}, err
	}
	posting, err := q.ReadAdjustmentPosting(ctx, db.ReadAdjustmentPostingParams{OrgID: org, OperationID: operation, ApprovalID: approval})
	if err != nil || !posting.PostedAt.Valid || posting.DeltaCentavos != delta {
		return AdjustmentReceipt{}, app.ErrUnavailable
	}
	return AdjustmentReceipt{SubscriptionID: record.SubscriptionID, SubscriptionRevision: record.SubscriptionRevision,
		Kind: kind, DeltaCentavos: delta, PostedAt: posting.PostedAt.Time.UTC(), Replayed: replay}, nil
}
