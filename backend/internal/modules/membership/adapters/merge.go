package adapters

import (
	"context"
	"encoding/json"
	"errors"
	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EmployeeMergeStore struct {
	Pool      *pgxpool.Pool
	Authority app.EmployeeMergeAuthority
	Teams     app.EmployeeMergeTeams
	Audit     auditing.TransactionRunner
}

var _ app.EmployeeMergeRepository = (*EmployeeMergeStore)(nil)

func EmployeeMergeAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "idn.employee.merged", TargetType: "employee", Reasons: []string{"merged"}, RequireReason: true}}
}
func (s *EmployeeMergeStore) ready() bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.Teams != nil && s.Audit != nil
}
func mergeError(ctx context.Context, e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, d.ErrMergeConflict) {
		return e
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) && (pe.Code == "P9004" || pe.Code == "P8005") {
		return d.ErrMergeConflict
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return app.ErrEmployeeNotFound
	}
	return accessChangeError(ctx, e)
}
func (s *EmployeeMergeStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, a app.EmployeeActor, member string, write bool) error {
	queries := q.New(tx)
	var e error
	if write {
		e = queries.LockMembershipAccessAccount(ctx, a.AccountID)
	} else {
		e = queries.ShareMembershipAccessAccount(ctx, a.AccountID)
	}
	if e != nil {
		return e
	}
	return s.Authority.CheckEmployeeMergeTx(ctx, tx, scope, a, member, write)
}
func (s *EmployeeMergeStore) view(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, source, survivor string) (app.EmployeeMergeView, error) {
	a, e := loadEmployeeRecord(ctx, tx, scope, source)
	if e != nil {
		return app.EmployeeMergeView{}, e
	}
	b, e := loadEmployeeRecord(ctx, tx, scope, survivor)
	if e != nil {
		return app.EmployeeMergeView{}, e
	}
	queries := q.New(tx)
	al, e := queries.RehireLivePeriods(ctx, q.RehireLivePeriodsParams{OrgID: scope.OrgID().String(), EmployeeID: source})
	if e != nil {
		return app.EmployeeMergeView{}, e
	}
	bl, e := queries.RehireLivePeriods(ctx, q.RehireLivePeriodsParams{OrgID: scope.OrgID().String(), EmployeeID: survivor})
	if e != nil {
		return app.EmployeeMergeView{}, e
	}
	teams, e := s.Teams.PreviewEmployeeMergeTx(ctx, tx, scope, source, survivor)
	if e != nil {
		return app.EmployeeMergeView{}, e
	}
	for _, t := range teams {
		if !t.Admissible {
			return app.EmployeeMergeView{}, d.ErrMergeConflict
		}
	}
	av, e := a.Snapshot(scope)
	if e != nil {
		return app.EmployeeMergeView{}, e
	}
	bv, e := b.Snapshot(scope)
	if e != nil {
		return app.EmployeeMergeView{}, e
	}
	ae, e := av.Employee.Snapshot(scope)
	if e != nil {
		return app.EmployeeMergeView{}, e
	}
	be, e := bv.Employee.Snapshot(scope)
	if e != nil {
		return app.EmployeeMergeView{}, e
	}
	if _, e := d.PlanEmployeeMerge(scope, av.Employee, bv.Employee, ae.Revision, be.Revision, al, bl, int64(len(teams)), "Review eligibility"); e != nil {
		return app.EmployeeMergeView{}, e
	}
	out := app.EmployeeMergeView{Source: a, Survivor: b, SourceLive: al, SurvivorLive: bl, Teams: teams}
	out.Digest, e = app.MergeReviewDigest(scope, out)
	return out, e
}
func (s *EmployeeMergeStore) PreviewEmployeeMerge(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, member, source, survivor string) (app.EmployeeMergeView, error) {
	if e := employeeCall(scope, a, member); e != nil {
		return app.EmployeeMergeView{}, e
	}
	if !validUUID(source) || !validUUID(survivor) {
		return app.EmployeeMergeView{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.EmployeeMergeView{}, app.ErrRegisterUnavailable
	}
	var out app.EmployeeMergeView
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.authorize(ctx, tx, scope, a, member, false); e != nil {
			return e
		}
		var e error
		out, e = s.view(ctx, tx, scope, source, survivor)
		return e
	})
	if e != nil {
		return app.EmployeeMergeView{}, mergeError(ctx, e)
	}
	return out, nil
}
func (s *EmployeeMergeStore) MergeEmployees(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, member string, c app.EmployeeMergeCommand) (app.EmployeeMergeResult, error) {
	if e := employeeCall(scope, a, member); e != nil {
		return app.EmployeeMergeResult{}, e
	}
	if e := c.Validate(); e != nil {
		return app.EmployeeMergeResult{}, e
	}
	if !s.ready() {
		return app.EmployeeMergeResult{}, app.ErrRegisterUnavailable
	}
	var out app.EmployeeMergeResult
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.authorize(ctx, tx, scope, a, member, true); e != nil {
			return e
		}
		queries := q.New(tx)
		digest := c.Digest(scope)
		prior, e := queries.EmployeeMergeCommand(ctx, q.EmployeeMergeCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID})
		if e == nil {
			if prior.ActorID != a.AccountID || prior.SourceID != c.SourceID || prior.SurvivorID != c.SurvivorID || prior.RequestHash != digest || prior.ReviewDigest != c.ReviewDigest || prior.Reason != c.Reason {
				return app.ErrAccessOperationReused
			}
			// Stored JSONB is wrapped to keep the shared decoder object-only and strict.
			raw, e := json.Marshal(map[string]json.RawMessage{"teams": prior.TeamEffects})
			if e != nil {
				return e
			}
			var wire struct {
				Teams []app.MergeTeamEffect `json:"teams"`
			}
			if httpapi.DecodeJSON(raw, &wire) != nil {
				return app.ErrRegisterUnavailable
			}
			at, e := accessStamp(prior.OccurredAt)
			if e != nil {
				return e
			}
			out = app.EmployeeMergeResult{OperationID: c.OperationID, SourceID: prior.SourceID, SurvivorID: prior.SurvivorID, Reason: prior.Reason, SourceRevision: prior.SourceRevision, SurvivorRevision: prior.SurvivorRevision, SourceState: d.EmployeeTerminated, SurvivorState: d.EmployeeState(prior.SurvivorState), Teams: wire.Teams, OccurredAt: at, Replayed: true}
			return out.Validate(c)
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		v, e := s.view(ctx, tx, scope, c.SourceID, c.SurvivorID)
		if e != nil {
			return e
		}
		av, e := v.Source.Snapshot(scope)
		if e != nil {
			return e
		}
		bv, e := v.Survivor.Snapshot(scope)
		if e != nil {
			return e
		}
		be, e := bv.Employee.Snapshot(scope)
		if e != nil {
			return e
		}
		next, e := d.PlanEmployeeMerge(scope, av.Employee, bv.Employee, c.SourceRevision, c.SurvivorRevision, v.SourceLive, v.SurvivorLive, int64(len(v.Teams)), c.Reason)
		if e != nil {
			return e
		}
		if c.ReviewDigest != v.Digest {
			return d.ErrMergeConflict
		}
		state, e := next.Snapshot(scope)
		if e != nil {
			return e
		}
		replay, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: c.OperationID, ActorID: a.AccountID, Action: "idn.employee.merged", TargetType: "employee", TargetID: c.SourceID, ReasonCode: "merged", RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			effects, e := s.Teams.TransferEmployeeTeamsTx(ctx, tx, scope, c.SourceID, c.SurvivorID, c.OperationID, v.Teams)
			if e != nil {
				return e
			}
			n, e := queries.MergeEmployeeIdentity(ctx, q.MergeEmployeeIdentityParams{OrgID: scope.OrgID().String(), SourceID: c.SourceID, SurvivorID: c.SurvivorID, ExpectedRevision: c.SourceRevision})
			if e != nil {
				return e
			}
			if n != 1 {
				return d.ErrConflict
			}
			if e := s.Authority.CheckEmployeeMergeTx(ctx, tx, scope, a, member, true); e != nil {
				return e
			}
			raw, e := json.Marshal(effects)
			if e != nil {
				return e
			}
			stamp, e := queries.InsertEmployeeMergeCommand(ctx, q.InsertEmployeeMergeCommandParams{OrgID: scope.OrgID().String(), OperationID: c.OperationID, ActorID: a.AccountID, SourceID: c.SourceID, SurvivorID: c.SurvivorID, RequestHash: digest, ReviewDigest: c.ReviewDigest, Reason: c.Reason, SourceRevision: state.Revision, SurvivorRevision: be.Revision, SurvivorState: string(be.State), TeamEffects: raw})
			if e != nil {
				return e
			}
			at, e := accessStamp(stamp)
			if e != nil {
				return e
			}
			out = app.EmployeeMergeResult{OperationID: c.OperationID, SourceID: c.SourceID, SurvivorID: c.SurvivorID, Reason: c.Reason, SourceRevision: state.Revision, SurvivorRevision: be.Revision, SourceState: state.State, SurvivorState: be.State, Teams: effects, OccurredAt: at}
			return out.Validate(c)
		})
		if e != nil {
			return e
		}
		if replay {
			return app.ErrAccessOperationReused
		}
		return nil
	})
	if e != nil {
		return app.EmployeeMergeResult{}, mergeError(ctx, e)
	}
	return out, nil
}
func (s *EmployeeMergeStore) ResolveEmployee(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, member, id string) (app.EmployeeResolution, error) {
	if e := employeeCall(scope, a, member); e != nil {
		return app.EmployeeResolution{}, e
	}
	if !validUUID(id) {
		return app.EmployeeResolution{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.EmployeeResolution{}, app.ErrRegisterUnavailable
	}
	var out app.EmployeeResolution
	e := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.authorize(ctx, tx, scope, a, member, false); e != nil {
			return e
		}
		seen := map[string]bool{}
		current := id
		out.Chain = []string{}
		for hop := 0; hop <= d.MaxEmployeeMergeDepth; hop++ {
			if seen[current] {
				return app.ErrRegisterUnavailable
			}
			seen[current] = true
			record, e := loadEmployeeRecord(ctx, tx, scope, current)
			if e != nil {
				if hop > 0 {
					return app.ErrRegisterUnavailable
				}
				return e
			}
			v, e := record.Snapshot(scope)
			if e != nil {
				return e
			}
			employee, e := v.Employee.Snapshot(scope)
			if e != nil {
				return e
			}
			if hop == 0 {
				out.Source = record
			}
			out.Chain = append(out.Chain, employee.ID)
			if employee.MergedInto == "" {
				out.Canonical = record
				return s.Authority.CheckEmployeeMergeTx(ctx, tx, scope, a, member, false)
			}
			current = employee.MergedInto
		}
		return app.ErrRegisterUnavailable
	})
	if e != nil {
		return app.EmployeeResolution{}, mergeError(ctx, e)
	}
	return out, nil
}
