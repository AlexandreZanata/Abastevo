package adapters

import (
	"context"
	"errors"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/membership"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EmployeeStore struct {
	Pool      *pgxpool.Pool
	Authority app.EmployeeAuthority
	Audit     auditing.TransactionRunner
}

var _ app.EmployeeRegister = (*EmployeeStore)(nil)
var _ app.EmployeeLookup = (*EmployeeStore)(nil)

func EmployeeAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: "idn.employee.created", TargetType: "employee", Reasons: []string{"registered", "duplicate_reviewed"}, RequireReason: true},
		{Action: "idn.employee.changed", TargetType: "employee", Reasons: []string{"profile_completed", "metadata", "duplicate_reviewed"}, RequireReason: true},
	}
}
func (s *EmployeeStore) ready() bool {
	return s != nil && s.Pool != nil && s.Authority != nil && s.Audit != nil
}
func employeeRegisterError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{d.ErrInvalid, d.ErrScope, d.ErrConflict, d.ErrRevision, d.ErrMerged, d.ErrTransition, app.ErrRegisterUnavailable, app.ErrEmployeeNotFound, app.ErrEmployeeUnauthenticated, app.ErrRegistrationDuplicate, app.ErrDuplicateReview, app.ErrEmployeeOperationReused, access.ErrNoMembership, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrUnavailable, access.ErrEntitlementRequired} {
		if errors.Is(err, known) {
			return known
		}
	}
	var denied *permissions.PermissionDenial
	if errors.As(err, &denied) {
		return err
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrEmployeeNotFound
	}
	if errors.Is(err, audit.ErrReused) {
		return app.ErrEmployeeOperationReused
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "P7901":
			return app.ErrRegistrationDuplicate
		case "P7902":
			return d.ErrInvalid
		case "23505":
			if pe.ConstraintName == "org_employee_registration_unique" {
				return app.ErrRegistrationDuplicate
			}
		case "23503":
			return app.ErrEmployeeNotFound
		case "23514", "22P02":
			return d.ErrInvalid
		case "42501":
			return d.ErrScope
		case "55000":
			return d.ErrConflict
		}
	}
	return app.ErrRegisterUnavailable
}
func employeeRow(scope kernel.OrgScope, row q.GetEmployeeRecordRow) (d.EmployeeRecord, error) {
	e, err := d.RehydrateEmployee(scope, d.EmployeeSnapshot{OrgID: scope.OrgID(), ID: row.ID, State: d.EmployeeState(row.State), Revision: row.IdentityRevision, MergedInto: row.MergedInto})
	if err != nil {
		return d.EmployeeRecord{}, app.ErrRegisterUnavailable
	}
	result, err := d.RehydrateEmployeeRecord(scope, d.EmployeeRecordSnapshot{Employee: e, ProfileRevision: row.ProfileRevision, Profile: d.EmployeeProfile{DisplayName: row.DisplayName, RegistrationNumber: row.RegistrationNumber, WorkEmail: row.WorkEmail, WorkPhone: row.WorkPhone, JobTitle: row.JobTitle, HomeUnitID: row.HomeUnitID}})
	if err != nil {
		return d.EmployeeRecord{}, app.ErrRegisterUnavailable
	}
	return result, nil
}
func loadEmployeeRecord(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (d.EmployeeRecord, error) {
	r, e := q.New(tx).GetEmployeeRecord(ctx, q.GetEmployeeRecordParams{OrgID: scope.OrgID().String(), ID: id})
	if e != nil {
		return d.EmployeeRecord{}, e
	}
	return employeeRow(scope, r)
}
func employeeCall(scope kernel.OrgScope, a app.EmployeeActor, member string) error {
	if scope.Validate() != nil {
		return d.ErrScope
	}
	if a.Validate() != nil {
		return d.ErrInvalid
	}
	_, e := id(scope, member)
	return e
}
func (s *EmployeeStore) FindEmployee(ctx context.Context, scope kernel.OrgScope, employeeID string) (d.EmployeeRecord, error) {
	if _, e := id(scope, employeeID); e != nil {
		return d.EmployeeRecord{}, e
	}
	if s == nil || s.Pool == nil {
		return d.EmployeeRecord{}, app.ErrRegisterUnavailable
	}
	var result d.EmployeeRecord
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var e error
		result, e = loadEmployeeRecord(ctx, tx, scope, employeeID)
		return e
	})
	if err != nil {
		return d.EmployeeRecord{}, employeeRegisterError(ctx, err)
	}
	return result, nil
}
func (s *EmployeeStore) WriteEmployee(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, member, newID string, c app.EmployeeWrite) (app.EmployeeResult, bool, error) {
	if e := employeeCall(scope, a, member); e != nil {
		return app.EmployeeResult{}, false, e
	}
	v, err := c.Snapshot()
	if err != nil {
		return app.EmployeeResult{}, false, err
	}
	if v.Kind == app.CreateEmployee {
		if _, err := id(scope, newID); err != nil {
			return app.EmployeeResult{}, false, err
		}
	} else if newID != "" {
		return app.EmployeeResult{}, false, d.ErrInvalid
	}
	if !s.ready() {
		return app.EmployeeResult{}, false, app.ErrRegisterUnavailable
	}
	target := v.EmployeeID
	if v.Kind == app.CreateEmployee {
		target = scope.OrgID().String()
	}
	digest := c.Digest(scope)
	var result app.EmployeeResult
	replay, readyToCommit := false, false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.Authority.CheckEmployeeTx(ctx, tx, scope, a, member, target, app.EmployeeManage); e != nil {
			return e
		}
		queries := q.New(tx)
		prior, e := queries.EmployeeCommandReceipt(ctx, q.EmployeeCommandReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID})
		if e == nil {
			if prior.ActorID != a.AccountID || prior.Kind != string(v.Kind) || prior.RequestHash != digest {
				return app.ErrEmployeeOperationReused
			}
			if _, e := id(scope, prior.EmployeeID); e != nil || prior.ResultProfileRevision < 1 || prior.ResultIdentityRevision < 1 {
				return app.ErrRegisterUnavailable
			}
			state := d.EmployeeState(prior.ResultState)
			if state != d.EmployeeActive && state != d.EmployeeInactive && state != d.EmployeeTerminated {
				return app.ErrRegisterUnavailable
			}
			result = app.EmployeeResult{EmployeeID: prior.EmployeeID, State: state, ProfileRevision: prior.ResultProfileRevision, IdentityRevision: prior.ResultIdentityRevision}
			replay = true
			readyToCommit = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		employeeID, action, reason := v.EmployeeID, "idn.employee.changed", "metadata"
		if v.Kind == app.CreateEmployee {
			employeeID, action, reason = newID, "idn.employee.created", "registered"
		} else if v.Kind == app.CompleteEmployee {
			reason = "profile_completed"
		}
		if v.AllowPossibleDuplicate {
			reason = "duplicate_reviewed"
		}
		repeated, e := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: v.OperationID, ActorID: a.AccountID, Action: action, TargetType: "employee", TargetID: employeeID, RequestHash: digest, ReasonCode: reason}, func(ctx context.Context, tx pgx.Tx) error {
			var record d.EmployeeRecord
			var e error
			if v.Kind == app.CreateEmployee {
				employee, e := d.NewEmployee(scope, newID)
				if e != nil {
					return e
				}
				record, e = d.RehydrateEmployeeRecord(scope, d.EmployeeRecordSnapshot{Employee: employee})
				if e != nil {
					return e
				}
				record, e = record.CompleteProfile(scope, 1, v.Profile)
				if e != nil {
					return e
				}
			} else {
				record, e = loadEmployeeRecord(ctx, tx, scope, employeeID)
				if e != nil {
					return e
				}
				if v.Kind == app.CompleteEmployee {
					record, e = record.CompleteProfile(scope, v.ExpectedRevision, v.Profile)
				} else {
					record, e = record.ChangeProfile(scope, v.ExpectedRevision, v.Profile)
				}
				if e != nil {
					return e
				}
			}
			duplicate, e := queries.EmployeeRegistrationExists(ctx, q.EmployeeRegistrationExistsParams{OrgID: scope.OrgID().String(), ExcludeID: employeeID, RegistrationNumber: v.Profile.RegistrationNumber})
			if e != nil {
				return e
			}
			if duplicate {
				return app.ErrRegistrationDuplicate
			}
			matches, e := queries.MatchEmployeeRecords(ctx, q.MatchEmployeeRecordsParams{OrgID: scope.OrgID().String(), ExcludeID: employeeID, DisplayName: v.Profile.DisplayName, WorkEmail: v.Profile.WorkEmail, WorkPhone: v.Profile.WorkPhone, RowLimit: 1})
			if e != nil {
				return e
			}
			if len(matches) > 0 && !v.AllowPossibleDuplicate {
				return app.ErrDuplicateReview
			}
			snap, e := record.Snapshot(scope)
			if e != nil {
				return e
			}
			identity, e := snap.Employee.Snapshot(scope)
			if e != nil {
				return e
			}
			p := snap.Profile
			if v.Kind == app.CreateEmployee {
				n, e := queries.InsertEmployee(ctx, q.InsertEmployeeParams{OrgID: scope.OrgID().String(), ID: employeeID})
				if e != nil {
					return e
				}
				if n != 1 {
					return app.ErrRegisterUnavailable
				}
			}
			if v.Kind == app.ChangeEmployee {
				n, e := queries.ChangeEmployeeProfile(ctx, q.ChangeEmployeeProfileParams{OrgID: scope.OrgID().String(), EmployeeID: employeeID, DisplayName: p.DisplayName, RegistrationNumber: p.RegistrationNumber, WorkEmail: p.WorkEmail, WorkPhone: p.WorkPhone, JobTitle: p.JobTitle, HomeUnitID: p.HomeUnitID, ExpectedRevision: v.ExpectedRevision})
				if e != nil {
					return e
				}
				if n != 1 {
					return d.ErrConflict
				}
			} else if e := queries.InsertEmployeeProfile(ctx, q.InsertEmployeeProfileParams{OrgID: scope.OrgID().String(), EmployeeID: employeeID, DisplayName: p.DisplayName, RegistrationNumber: p.RegistrationNumber, WorkEmail: p.WorkEmail, WorkPhone: p.WorkPhone, JobTitle: p.JobTitle, HomeUnitID: p.HomeUnitID}); e != nil {
				return e
			}
			result = app.EmployeeResult{EmployeeID: employeeID, State: identity.State, IdentityRevision: identity.Revision, ProfileRevision: snap.ProfileRevision}
			return queries.InsertEmployeeReceipt(ctx, q.InsertEmployeeReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID, ActorID: a.AccountID, EmployeeID: employeeID, Kind: string(v.Kind), RequestHash: digest, ResultProfileRevision: snap.ProfileRevision, ResultIdentityRevision: identity.Revision, ResultState: string(identity.State), DuplicateReviewed: v.AllowPossibleDuplicate})
		})
		if e != nil {
			return e
		}
		if repeated {
			return app.ErrRegisterUnavailable
		}
		readyToCommit = true
		return nil
	})
	if err != nil {
		if readyToCommit && ctx.Err() == nil {
			return app.EmployeeResult{}, false, app.ErrRegisterUnavailable
		}
		return app.EmployeeResult{}, false, employeeRegisterError(ctx, err)
	}
	return result, replay, nil
}
func (s *EmployeeStore) ReadEmployee(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, member, employeeID string) (d.EmployeeRecord, error) {
	if e := employeeCall(scope, a, member); e != nil {
		return d.EmployeeRecord{}, e
	}
	if _, e := id(scope, employeeID); e != nil {
		return d.EmployeeRecord{}, e
	}
	if !s.ready() {
		return d.EmployeeRecord{}, app.ErrRegisterUnavailable
	}
	var result d.EmployeeRecord
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.Authority.CheckEmployeeTx(ctx, tx, scope, a, member, employeeID, app.EmployeeRead); e != nil {
			return e
		}
		var e error
		result, e = loadEmployeeRecord(ctx, tx, scope, employeeID)
		return e
	})
	if err != nil {
		return d.EmployeeRecord{}, employeeRegisterError(ctx, err)
	}
	return result, nil
}
func (s *EmployeeStore) ListEmployees(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, member string, f app.EmployeeFilter) (app.EmployeePage, error) {
	if e := employeeCall(scope, a, member); e != nil {
		return app.EmployeePage{}, e
	}
	if e := f.Validate(); e != nil {
		return app.EmployeePage{}, e
	}
	if !s.ready() {
		return app.EmployeePage{}, app.ErrRegisterUnavailable
	}
	result := app.EmployeePage{Employees: []d.EmployeeRecord{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.Authority.CheckEmployeeTx(ctx, tx, scope, a, member, scope.OrgID().String(), app.EmployeeRead); e != nil {
			return e
		}
		rows, e := q.New(tx).ListEmployeeRecords(ctx, q.ListEmployeeRecordsParams{OrgID: scope.OrgID().String(), StateFilter: string(f.State), AfterID: f.AfterID, RowLimit: int32(f.Limit + 1)})
		if e != nil {
			return e
		}
		for i, row := range rows {
			if i == f.Limit {
				previous, _ := result.Employees[i-1].Snapshot(scope)
				identity, _ := previous.Employee.Snapshot(scope)
				result.NextID = identity.ID
				break
			}
			v, e := employeeRow(scope, q.GetEmployeeRecordRow(row))
			if e != nil {
				return e
			}
			result.Employees = append(result.Employees, v)
		}
		return nil
	})
	if err != nil {
		return app.EmployeePage{}, employeeRegisterError(ctx, err)
	}
	return result, nil
}
func (s *EmployeeStore) MatchEmployees(ctx context.Context, scope kernel.OrgScope, a app.EmployeeActor, member string, f app.EmployeeMatch) (app.EmployeePage, error) {
	if e := employeeCall(scope, a, member); e != nil {
		return app.EmployeePage{}, e
	}
	if e := f.Validate(); e != nil {
		return app.EmployeePage{}, e
	}
	if !s.ready() {
		return app.EmployeePage{}, app.ErrRegisterUnavailable
	}
	exclude := f.ExcludeID
	if exclude == "" {
		exclude = "00000000-0000-0000-0000-000000000000"
	}
	result := app.EmployeePage{Employees: []d.EmployeeRecord{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if e := s.Authority.CheckEmployeeTx(ctx, tx, scope, a, member, scope.OrgID().String(), app.EmployeeReview); e != nil {
			return e
		}
		if f.ExcludeID != "" {
			if _, e := loadEmployeeRecord(ctx, tx, scope, f.ExcludeID); e != nil {
				return e
			}
		}
		rows, e := q.New(tx).MatchEmployeeRecords(ctx, q.MatchEmployeeRecordsParams{OrgID: scope.OrgID().String(), ExcludeID: exclude, DisplayName: f.Profile.DisplayName, WorkEmail: f.Profile.WorkEmail, WorkPhone: f.Profile.WorkPhone, AfterID: f.AfterID, RowLimit: int32(f.Limit + 1)})
		if e != nil {
			return e
		}
		for i, row := range rows {
			if i == f.Limit {
				previous, _ := result.Employees[i-1].Snapshot(scope)
				identity, _ := previous.Employee.Snapshot(scope)
				result.NextID = identity.ID
				break
			}
			v, e := employeeRow(scope, q.GetEmployeeRecordRow(row))
			if e != nil {
				return e
			}
			result.Employees = append(result.Employees, v)
		}
		return nil
	})
	if err != nil {
		return app.EmployeePage{}, employeeRegisterError(ctx, err)
	}
	return result, nil
}
