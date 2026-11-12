// Package adapters persists organization switches through scoped transactions.
package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	qcap "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgcapability"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	auditapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
)

// AuditDefinitions is copied by the audit catalog at composition.
func AuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: "capability.enabled", TargetType: "organization", Reasons: []string{"operator.request"}, RequireReason: true},
		{Action: "capability.disabled", TargetType: "organization", Reasons: []string{"operator.request"}, RequireReason: true},
	}
}

type Store struct {
	pool    *pgxpool.Pool
	catalog domain.Catalog
	audit   auditapp.Runner
}

var _ application.Repository = (*Store)(nil)

func NewStore(pool *pgxpool.Pool, catalog domain.Catalog, runner auditapp.Runner) *Store {
	return &Store{pool: pool, catalog: catalog, audit: runner}
}

func (store *Store) validate(scope kernel.OrgScope, code string) error {
	if store == nil {
		return domain.ErrUnavailable
	}
	if err := scope.Validate(); err != nil {
		return err
	}
	return store.catalog.Validate(code)
}

func (store *Store) Set(ctx context.Context, scope kernel.OrgScope, actor domain.Actor, change domain.Change) (bool, error) {
	if err := store.validate(scope, change.Capability); err != nil {
		return false, err
	}
	if err := actor.Validate(); err != nil {
		return false, err
	}
	if err := change.Validate(store.catalog); err != nil {
		return false, err
	}
	if store.audit == nil {
		return false, domain.ErrUnavailable
	}
	digest, err := change.Digest(scope)
	if err != nil {
		return false, err
	}
	action := "capability.disabled"
	if change.Enabled {
		action = "capability.enabled"
	}
	event := audit.Event{
		OperationID: change.OperationID, ActorID: actor.AccountID, DelegationID: actor.DelegationID,
		Action: action, TargetType: "organization", TargetID: scope.OrgID().String(),
		ReasonCode: "operator.request", RequestHash: digest,
	}
	replay, err := store.audit.RecordChange(ctx, scope, event, func(ctx context.Context, tx pgx.Tx) error {
		queries := qcap.New(tx)
		var rows int64
		var err error
		if change.ExpectedRevision == 0 {
			rows, err = queries.CreateState(ctx, qcap.CreateStateParams{OrgID: scope.OrgID().String(), Capability: change.Capability, Enabled: change.Enabled})
		} else {
			rows, err = queries.AdvanceState(ctx, qcap.AdvanceStateParams{OrgID: scope.OrgID().String(), Capability: change.Capability, Enabled: change.Enabled, ExpectedRevision: change.ExpectedRevision})
		}
		if err != nil {
			return domain.ErrUnavailable
		}
		if rows != 1 {
			return domain.ErrConflict
		}
		return nil
	})
	if errors.Is(err, audit.ErrReused) {
		return false, domain.ErrReused
	}
	if err != nil && !errors.Is(err, domain.ErrConflict) {
		return false, domain.ErrUnavailable
	}
	return replay, err
}

func (store *Store) State(ctx context.Context, scope kernel.OrgScope, code string) (domain.State, error) {
	if err := store.validate(scope, code); err != nil {
		return domain.State{}, err
	}
	if store.pool == nil {
		return domain.State{}, domain.ErrUnavailable
	}
	var state domain.State
	err := database.WithOrgTx(ctx, store.pool, scope, func(tx pgx.Tx) error {
		row, err := qcap.New(tx).GetState(ctx, qcap.GetStateParams{OrgID: scope.OrgID().String(), Capability: code})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		state = domain.State{Enabled: row.Enabled, Revision: row.Revision}
		return nil
	})
	if err != nil {
		return domain.State{}, domain.ErrUnavailable
	}
	return state, nil
}

// GuardWrite composes inside an existing scoped audit/business transaction.
// The caller checks current authority before entering its audit runner. It must
// not open a nested transaction or use another connection for the business write.
func (store *Store) GuardWrite(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, code string) error {
	if err := store.validate(scope, code); err != nil {
		return err
	}
	if tx == nil {
		return domain.ErrInvalid
	}
	row, err := qcap.New(tx).LockStateForWrite(ctx, qcap.LockStateForWriteParams{OrgID: scope.OrgID().String(), Capability: code})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrEntitlementRequired
	}
	if err != nil {
		return domain.ErrUnavailable
	}
	return (domain.State{Enabled: row.Enabled, Revision: row.Revision}).CheckWrite()
}

func (store *Store) Run(ctx context.Context, scope kernel.OrgScope, code string, intent domain.Intent, work func(context.Context, pgx.Tx) error) error {
	if err := store.validate(scope, code); err != nil {
		return err
	}
	if work == nil || (intent != domain.Read && intent != domain.Export && intent != domain.Write) {
		return domain.ErrInvalid
	}
	if store.pool == nil {
		return domain.ErrUnavailable
	}
	var callbackErr error
	err := database.WithOrgTx(ctx, store.pool, scope, func(tx pgx.Tx) error {
		if intent == domain.Write {
			if err := store.GuardWrite(ctx, tx, scope, code); err != nil {
				callbackErr = err
				return err
			}
		}
		callbackErr = work(ctx, tx)
		return callbackErr
	})
	if callbackErr != nil {
		return callbackErr
	}
	if err != nil {
		return domain.ErrUnavailable
	}
	return nil
}
