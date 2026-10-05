package adapters

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	stationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
)

// ReviewDecisions implements application.DecisionStore in one
// transaction: audited decision + optional narrow grant + claim-state
// transition commit together, so concurrent approvals converge on the
// open-state guard and the active-grant key instead of duplicating
// authority or forking state.
type ReviewDecisions struct {
	Pool *pgxpool.Pool
}

func (d ReviewDecisions) DecideAtomically(ctx context.Context, decision application.DecisionInput, grant *application.GrantInput, newState string) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := stationprofile.New(tx)
	decisionID, err := mustUUID(decision.ID)
	if err != nil {
		return err
	}
	claimID, err := mustUUID(decision.ClaimID)
	if err != nil {
		return err
	}
	decisionRow, err := q.CreateClaimDecision(ctx, stationprofile.CreateClaimDecisionParams{
		ID: decisionID, ClaimID: claimID,
		Reviewer: decision.Reviewer, Decision: decision.Decision, Reason: decision.Reason,
		PolicyVersion: decision.PolicyVersion, ProofVersion: 0,
		OperatorCnpj: decision.OperatorCNPJ, Scopes: joinScopes(decision.Scopes),
	})
	if err != nil {
		return err
	}
	if grant != nil {
		grantID, err := mustUUID(grant.ID)
		if err != nil {
			return err
		}
		accountID, err := mustUUID(grant.AccountID)
		if err != nil {
			return err
		}
		stationID, err := mustUUID(grant.StationID)
		if err != nil {
			return err
		}
		if _, err := q.CreateGrant(ctx, stationprofile.CreateGrantParams{
			ID: grantID, AccountID: accountID, StationID: stationID,
			OperatorCnpj: grant.OperatorCNPJ, Role: grant.Role,
			Scopes: joinScopes(grant.Scopes), Version: 1,
			ClaimID: claimID, DecisionID: decisionRow.ID,
		}); err != nil {
			// ON CONFLICT DO NOTHING returns no rows when the active
			// grant already exists: convergence, not failure. The
			// open-state guard below still arbitrates the winner.
			if !isNoRows(err) {
				return err
			}
		}
	}
	affected, err := q.SetClaimReviewState(ctx, stationprofile.SetClaimReviewStateParams{
		ID: claimID, State: newState,
	})
	if err != nil {
		return err
	}
	if affected == 0 {
		return application.ErrVerifyClosed
	}
	return tx.Commit(ctx)
}

func joinScopes(scopes []string) string {
	return strings.Join(scopes, ",")
}

func (d ReviewDecisions) ActiveGrant(ctx context.Context, accountID, stationID string) (application.GrantRow, bool, error) {
	accountUID, err := mustUUID(accountID)
	if err != nil {
		return application.GrantRow{}, false, err
	}
	stationUID, err := mustUUID(stationID)
	if err != nil {
		return application.GrantRow{}, false, err
	}
	row, err := stationprofile.New(d.Pool).ActiveGrant(ctx, stationprofile.ActiveGrantParams{
		AccountID: accountUID, StationID: stationUID,
	})
	if err != nil {
		if isNoRows(err) {
			return application.GrantRow{}, false, nil
		}
		return application.GrantRow{}, false, err
	}
	return application.GrantRow{
		ID: uuidString(row.ID), AccountID: uuidString(row.AccountID),
		StationID: uuidString(row.StationID), OperatorCNPJ: row.OperatorCnpj,
		Role: row.Role, Scopes: splitScopes(row.Scopes),
		Status: row.Status, Version: int(row.Version),
	}, true, nil
}

func splitScopes(scopes string) []string {
	if scopes == "" {
		return nil
	}
	return strings.Split(scopes, ",")
}
