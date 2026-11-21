package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgconnector"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	confirmStatementAuditAction = "integ.card_statement.confirmed"
	statementAuditTarget        = "integration.card_statement"
)

func (s Store) ProcessStatement(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member string,
	action app.Action, statement app.Statement,
) (app.StatementResult, error) {
	if s.Pool == nil || s.Authority == nil || s.Audit == nil {
		return app.StatementResult{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !audit.ValidID(member) || statement.Validate(action) != nil {
		return app.StatementResult{}, domain.ErrInvalid
	}
	var result app.StatementResult
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckConnectionTx(ctx, tx, scope, actor, member, action); err != nil {
			return err
		}
		approval, err := s.Access.checkActiveTx(ctx, tx, scope, domain.FuelCardStatement, statement.ConnectionID, statement.SourceAccountID)
		if err != nil {
			return err
		}
		lines, err := domain.ReadFuelCardStatement(statement.CSV, approval.MaxBytes, approval.MaxItems)
		if err != nil {
			return err
		}
		rows, pending, err := reconcileStatement(ctx, db.New(tx), scope.OrgID().String(), member, approval, statement, lines)
		if err != nil {
			return err
		}
		payload := statement.PayloadDigest()
		proof, _ := json.Marshal(struct {
			Digest, Approval string
			Revision         int64
			Rows             []app.StatementOutcome
		}{payload, approval.ID, approval.Revision, rows})
		result.Rows, result.Proof = rows, app.Digest(proof)
		if action == app.PreviewStatement {
			return nil
		}
		if statement.PreviewProof != result.Proof {
			return domain.ErrConflict
		}
		event := audit.Event{OperationID: statement.OperationID, ActorID: actor.AccountID, Action: confirmStatementAuditAction,
			TargetType: statementAuditTarget, TargetID: statement.OperationID, RequestHash: payload}
		result.Replayed, err = s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			q := db.New(tx)
			for _, row := range pending {
				if err := q.StoreCardStatementLine(ctx, row); err != nil {
					return err
				}
			}
			return nil
		})
		return err
	})
	if err != nil {
		return app.StatementResult{}, err
	}
	return result, nil
}

func reconcileStatement(ctx context.Context, q *db.Queries, org, member string, approval domain.Approval,
	input app.Statement, lines []domain.StatementLine,
) ([]app.StatementOutcome, []db.StoreCardStatementLineParams, error) {
	outcomes := make([]app.StatementOutcome, 0, len(lines))
	var pending []db.StoreCardStatementLineParams
	for index, line := range lines {
		transaction := app.Digest([]byte(line.SourceID))
		fact, _ := json.Marshal(line)
		factHash := app.Digest(fact)
		prior, err := q.GetCardStatementLine(ctx, db.GetCardStatementLineParams{
			OrgID: org, ProviderCode: approval.Provider, SourceAccountID: approval.Resource, TransactionHash: transaction,
		})
		outcome := app.StatementOutcome{Row: index + 1, PaidCentavos: line.PaidCentavos, Status: "REVIEW_REQUIRED"}
		switch {
		case err == nil:
			if prior.FactHash != factHash {
				return nil, nil, domain.ErrConflict
			}
			if prior.Linked {
				outcome.Status = "LINKED"
			}
		case errors.Is(err, pgx.ErrNoRows):
			row := db.StoreCardStatementLineParams{
				OrgID: org, ConnectionID: input.ConnectionID, ProviderCode: approval.Provider, SourceAccountID: approval.Resource,
				TransactionHash: transaction, FactHash: factHash, PaidCentavos: line.PaidCentavos,
				OccurredAt:       pgtype.Timestamptz{Time: line.OccurredAt.Truncate(time.Microsecond), Valid: true},
				OccurredSubmicro: int16(line.OccurredAt.Nanosecond() % 1000),
				ConfirmedBy:      member,
				OperationID:      input.OperationID,
			}
			if line.FuelingID != "" {
				row.FuelingID, err = q.VerifiedCardFueling(ctx, db.VerifiedCardFuelingParams{
					OrgID: org, FuelingID: line.FuelingID, OccurredAt: row.OccurredAt,
					OccurredSubmicro: row.OccurredSubmicro, PaidCentavos: row.PaidCentavos,
				})
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return nil, nil, err
				}
				if row.FuelingID != "" {
					outcome.Status = "LINKED"
				}
			}
			pending = append(pending, row)
		default:
			return nil, nil, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, pending, nil
}
