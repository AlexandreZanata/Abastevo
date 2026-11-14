package adapters

import (
	"context"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RehireStore struct {
	Pool      *pgxpool.Pool
	Authority app.RehireAuthority
	Employees app.TransactionRepository
	Audit     auditing.TransactionRunner
}

var _ app.RehireRepository = (*RehireStore)(nil)

func RehireAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "idn.employee.rehired", TargetType: "employee", Reasons: []string{"rehired"}, RequireReason: true}}
}
func (s *RehireStore) ready() bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.Employees != nil && s.Audit != nil
}
func (s *RehireStore) Rehire(ctx context.Context, scope kernel.OrgScope, actor app.EmployeeActor, member string, c app.RehireCommand) (app.RehireResult, error) {
	if e := employeeCall(scope, actor, member); e != nil {
		return app.RehireResult{}, e
	}
	if e := c.Validate(); e != nil {
		return app.RehireResult{}, e
	}
	if !s.ready() {
		return app.RehireResult{}, app.ErrRegisterUnavailable
	}
	var out app.RehireResult
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		// This operation never reads or locks former account bindings.
		if e := queries.LockMembershipAccessAccount(ctx, actor.AccountID); e != nil {
			return e
		}
		if e := s.Authority.CheckRehireTx(ctx, tx, scope, actor, member); e != nil {
			return e
		}
		digest := c.Digest(scope)
		prior, e := queries.RehireCommand(ctx, q.RehireCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID})
		if e == nil {
			if prior.ActorID != actor.AccountID || prior.EmployeeID != c.EmployeeID || prior.RequestHash != digest || prior.Reason != c.Reason || prior.PreviousRevision != c.ExpectedRevision {
				return app.ErrAccessOperationReused
			}
			at, e := accessStamp(prior.OccurredAt)
			if e != nil {
				return e
			}
			out = app.RehireResult{EmployeeID: prior.EmployeeID, OperationID: c.OperationID, Reason: prior.Reason, State: d.EmployeeState(prior.ResultState), Revision: prior.ResultRevision, OccurredAt: at, Replayed: true}
			return out.Validate(c)
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		employee, e := s.Employees.GetEmployeeTx(ctx, tx, scope, c.EmployeeID)
		if e != nil {
			return e
		}
		count, e := queries.RehireLivePeriods(ctx, q.RehireLivePeriodsParams{OrgID: scope.OrgID().String(), EmployeeID: c.EmployeeID})
		if e != nil {
			return e
		}
		next, e := d.PlanRehire(scope, employee, c.ExpectedRevision, count, c.Reason)
		if e != nil {
			return e
		}
		v, e := next.Snapshot(scope)
		if e != nil {
			return e
		}
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: c.OperationID, ActorID: actor.AccountID, Action: "idn.employee.rehired", TargetType: "employee", TargetID: c.EmployeeID, ReasonCode: "rehired", RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			changed, e := queries.ReopenTerminatedEmployee(ctx, q.ReopenTerminatedEmployeeParams{OrgID: scope.OrgID().String(), EmployeeID: c.EmployeeID, ExpectedRevision: c.ExpectedRevision})
			if e != nil {
				return e
			}
			if changed != 1 {
				return d.ErrConflict
			}
			if e := s.Authority.CheckRehireTx(ctx, tx, scope, actor, member); e != nil {
				return e
			}
			stamp, e := queries.InsertRehireCommand(ctx, q.InsertRehireCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID, ActorID: actor.AccountID, EmployeeID: c.EmployeeID, RequestHash: digest, Reason: c.Reason, PreviousRevision: c.ExpectedRevision, ResultRevision: v.Revision})
			if e != nil {
				return e
			}
			at, e := accessStamp(stamp)
			if e != nil {
				return e
			}
			out = app.RehireResult{EmployeeID: v.ID, OperationID: c.OperationID, Reason: c.Reason, State: v.State, Revision: v.Revision, OccurredAt: at}
			return out.Validate(c)
		})
		if e != nil {
			return e
		}
		if repeated {
			return app.ErrAccessOperationReused
		}
		return nil
	})
	if e != nil {
		return app.RehireResult{}, accessChangeError(ctx, e)
	}
	if e := out.Validate(c); e != nil {
		return app.RehireResult{}, e
	}
	return out, nil
}
