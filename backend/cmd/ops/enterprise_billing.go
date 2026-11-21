package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	queries "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbillingops"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

const chargeBoundAction = "subs.billing.charge.bound"

var (
	errBillingBindUsage        = errors.New("usage: ops enterprise billing bind --org <uuid> --approval-id <uuid> --operation-id <uuid> [--operator <uuid>]")
	errChargeReviewUnavailable = errors.New("enterprise billing charge review unavailable")
	errChargeBindingConflict   = errors.New("enterprise billing charge binding conflict")
)

type chargeBindingReceipt struct {
	subscription string
	replayed     bool
}

func runEnterpriseBillingBind(args []string, getenv func(string) string, out io.Writer) error {
	in, err := parseReviewedBillingArgs(args, getenv, "enterprise billing bind", "bind", errBillingBindUsage)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, cfg, err := openPool(ctx)
	if err != nil {
		return errors.New("enterprise database unavailable")
	}
	defer pool.Close()
	if !cfg.EnterpriseEnabled || !cfg.EnterpriseBillingEnabled {
		return errors.New("enterprise billing is disabled")
	}
	receipt, err := executeBillingBind(ctx, pool.Underlying(), in)
	switch {
	case errors.Is(err, database.ErrUnsafeOrgRole):
		return errUnsafeLogin
	case errors.Is(err, errChargeReviewUnavailable), errors.Is(err, errChargeBindingConflict), errors.Is(err, audit.ErrReused):
		return err
	case err != nil:
		return errors.New("enterprise billing charge binding unavailable")
	}
	_, err = fmt.Fprintf(out, "organization %s subscription %s operation=%s replayed=%t\n",
		in.scope.OrgID().String(), receipt.subscription, in.operation, receipt.replayed)
	return err
}

func executeBillingBind(ctx context.Context, pool *pgxpool.Pool, in reviewedBillingArgs) (chargeBindingReceipt, error) {
	catalog, err := audit.NewCatalog([]audit.Definition{{Action: chargeBoundAction, TargetType: "subscription"}})
	if err != nil {
		return chargeBindingReceipt{}, err
	}
	store := auditpg.NewStore(pool, catalog)
	org := in.scope.OrgID().String()
	var receipt chargeBindingReceipt
	err = database.WithOrgTx(ctx, pool, in.scope, func(tx pgx.Tx) error {
		q := queries.New(tx)
		approval, err := q.ReadChargeApproval(ctx, queries.ReadChargeApprovalParams{
			OrgID: org, ApprovalID: in.approval, OperatorID: in.operator,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return errChargeReviewUnavailable
		}
		if err != nil {
			return err
		}
		// The immutable review ID identifies the exact reviewed fields. Keep
		// this versioned digest in sync with the database proof constraint.
		hash := sha256.Sum256([]byte(chargeBoundAction + "/v1/" + org + "/" + in.approval))
		event := audit.Event{OperationID: in.operation, ActorID: in.operator, Action: chargeBoundAction,
			TargetType: "subscription", TargetID: approval.SubscriptionID, RequestHash: hex.EncodeToString(hash[:])}
		replay, err := store.RecordChangeTx(ctx, tx, in.scope, event, func(ctx context.Context, tx pgx.Tx) error {
			if !approval.CanBind {
				return errChargeReviewUnavailable
			}
			count, err := q.RecordReviewedChargeBinding(ctx, queries.RecordReviewedChargeBindingParams{
				OrgID: org, ApprovalID: in.approval, OperationID: in.operation,
			})
			if err != nil {
				var cause *pgconn.PgError
				if errors.As(err, &cause) && cause.Code == "23514" &&
					(cause.ConstraintName == "org_billing_charge_binding_review" || cause.ConstraintName == "org_billing_asaas_reference_scope") {
					return errChargeReviewUnavailable
				}
				return err
			}
			if count != 1 {
				return errChargeBindingConflict
			}
			return nil
		})
		if err != nil {
			return err
		}
		if replay {
			bound, err := q.HasReviewedChargeBinding(ctx, queries.HasReviewedChargeBindingParams{
				OrgID: org, ApprovalID: in.approval, OperationID: in.operation,
			})
			if err != nil {
				return err
			}
			if !bound {
				return errChargeBindingConflict
			}
		}
		receipt = chargeBindingReceipt{subscription: approval.SubscriptionID, replayed: replay}
		return nil
	})
	if err != nil {
		return chargeBindingReceipt{}, err
	}
	return receipt, nil
}
