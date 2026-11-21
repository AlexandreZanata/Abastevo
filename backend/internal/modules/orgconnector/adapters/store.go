package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgconnector"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const connectionTarget = "integration.connection"

var actions = map[app.Action]string{
	app.Configure: "integ.connection.configured",
	app.Activate:  "integ.connection.activated",
	app.Disable:   "integ.connection.disabled",
}

func AuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: actions[app.Configure], TargetType: connectionTarget},
		{Action: actions[app.Activate], TargetType: connectionTarget},
		{Action: actions[app.Disable], TargetType: connectionTarget},
		{Action: confirmStatementAuditAction, TargetType: statementAuditTarget},
	}
}

type Store struct {
	Pool      *pgxpool.Pool
	Authority app.Authority
	Audit     auditing.TransactionRunner
	Access    Access
}

func (s Store) Change(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member string, change app.Change) (app.Result, error) {
	if s.Pool == nil || s.Audit == nil || s.Authority == nil {
		return app.Result{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !audit.ValidID(member) || change.Validate() != nil {
		return app.Result{}, domain.ErrInvalid
	}
	org := scope.OrgID().String()
	id := change.TargetID
	if change.Action == app.Configure {
		id = change.OperationID
	}
	event := audit.Event{OperationID: change.OperationID, ActorID: actor.AccountID, Action: actions[change.Action],
		TargetType: connectionTarget, TargetID: id, RequestHash: change.Digest(scope)}
	var result app.Result
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckConnectionTx(ctx, tx, scope, actor, member, change.Action); err != nil {
			return err
		}
		q := db.New(tx)
		now, err := q.ReadConnectionClock(ctx)
		if err != nil || !now.Valid {
			return app.ErrUnavailable
		}
		result.Replayed, err = s.Audit.RecordChangeTx(ctx, tx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
			return apply(ctx, db.New(tx), org, id, change, now)
		})
		if err != nil {
			return err
		}
		row, err := q.GetConnection(ctx, db.GetConnectionParams{OrgID: org, ID: id})
		if err != nil {
			return err
		}
		result.Connection = connection(row)
		return nil
	})
	if err != nil {
		return app.Result{}, err
	}
	return result, nil
}

func apply(ctx context.Context, q *db.Queries, org, id string, change app.Change, now pgtype.Timestamptz) error {
	if change.Action == app.Configure {
		a, err := currentApproval(ctx, q, org, change.ApprovalID)
		if err != nil {
			return err
		}
		if a.Revision != change.ApprovalRevision || a.Check(a.Kind, a.Resource, now.Time, false) != nil {
			return domain.ErrDenied
		}
		changed, err := q.ConfigureConnection(ctx, db.ConfigureConnectionParams{OrgID: org, ID: id, ApprovalID: a.ID,
			ApprovalRevision: a.Revision, Label: change.Label, ChangedAt: now})
		return requireOne(changed, err)
	}
	row, err := q.GetConnection(ctx, db.GetConnectionParams{OrgID: org, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrDenied
	}
	if err != nil {
		return err
	}
	next := domain.Disabled
	if change.Action == app.Activate {
		next = domain.Active
		a, err := currentApproval(ctx, q, org, row.ApprovalID)
		if err != nil {
			return err
		}
		if a.Revision != row.ApprovalRevision {
			return domain.ErrDenied
		}
		if err := a.Check(domain.Kind(row.Kind), a.Resource, now.Time, true); err != nil {
			return err
		}
		if a.Kind == domain.AccountingExport && (a.Resource == "" || a.Provider != accountingConnector) {
			return domain.ErrDenied
		}
		if a.Kind == domain.OutboundWebhook {
			if a.Resource == "" {
				return domain.ErrDenied
			}
			if _, err := readWebhookEndpoint(ctx, q, org, a); err != nil {
				return err
			}
		}
	}
	if row.Revision != change.ExpectedRevision || !domain.State(row.State).Allows(next) {
		return domain.ErrConflict
	}
	changed, err := q.ChangeConnectionState(ctx, db.ChangeConnectionStateParams{
		OrgID: org, ID: id, ExpectedRevision: change.ExpectedRevision, State: string(next), ChangedAt: now,
	})
	return requireOne(changed, err)
}

func requireOne(changed int64, err error) error {
	if err != nil {
		return err
	}
	if changed != 1 {
		return domain.ErrConflict
	}
	return nil
}

func currentApproval(ctx context.Context, q *db.Queries, org, id string) (domain.Approval, error) {
	if err := q.LockConnectorApproval(ctx, db.LockConnectorApprovalParams{OrgID: org, ID: id}); err != nil {
		return domain.Approval{}, err
	}
	row, err := q.ReadConnectorApproval(ctx, db.ReadConnectorApprovalParams{OrgID: org, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Approval{}, domain.ErrDenied
	}
	if err != nil {
		return domain.Approval{}, err
	}
	ref, err := domain.NewCredentialRef(row.CredentialRef)
	if err != nil {
		return domain.Approval{}, err
	}
	return domain.Approval{ID: row.ID, Kind: domain.Kind(row.Kind), Provider: row.ProviderCode, Revision: row.Revision,
		Decision: row.Decision, Resource: row.ResourceID, Credential: ref,
		MaxBytes: row.MaximumBytes, MaxItems: row.MaximumItems, ReadyAt: row.ReadyAt.Time.UTC()}, nil
}

func connection(row db.GetConnectionRow) domain.Connection {
	return domain.Connection{ID: row.ID, Kind: domain.Kind(row.Kind), Provider: row.ProviderCode, Label: row.Label,
		State: domain.State(row.State), Revision: row.Revision, ChangedAt: row.ChangedAt.Time.UTC(), ApprovalCurrent: row.ApprovalCurrent}
}

func (s Store) List(ctx context.Context, scope kernel.OrgScope, actor organization.CompanyActor, member, after string, limit int) (app.Page, error) {
	if s.Pool == nil || s.Authority == nil {
		return app.Page{}, app.ErrUnavailable
	}
	if scope.Validate() != nil || actor.Validate() != nil || !audit.ValidID(member) ||
		(after != "" && !audit.ValidID(after)) || limit < 1 || limit > 100 {
		return app.Page{}, domain.ErrInvalid
	}
	result := app.Page{Connections: make([]domain.Connection, 0, limit)}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.Authority.CheckConnectionTx(ctx, tx, scope, actor, member, app.List); err != nil {
			return err
		}
		rows, err := db.New(tx).ListConnections(ctx, db.ListConnectionsParams{OrgID: scope.OrgID().String(), AfterID: after, RowLimit: int32(limit + 1)})
		if err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			result.After = rows[limit-1].ID
		}
		for _, row := range rows {
			result.Connections = append(result.Connections, connection(db.GetConnectionRow{
				ID: row.ID, Kind: row.Kind, ProviderCode: row.ProviderCode, Label: row.Label, State: row.State,
				Revision: row.Revision, ChangedAt: row.ChangedAt, ApprovalCurrent: row.ApprovalCurrent}))
		}
		return nil
	})
	if err != nil {
		return app.Page{}, err
	}
	return result, nil
}
