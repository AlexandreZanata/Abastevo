package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/organization"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UnitStore struct {
	Pool         *pgxpool.Pool
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Audit        auditing.TransactionRunner
	Capabilities setupCapability
}

var _ app.UnitRepository = (*UnitStore)(nil)
var _ app.UnitLookup = (*UnitStore)(nil)

func UnitAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: "org.unit.created", TargetType: "unit"}, {Action: "org.unit.changed", TargetType: "unit", Reasons: []string{"metadata", "restored"}, RequireReason: true}, {Action: "org.unit.archived", TargetType: "unit"},
	}
}
func (s *UnitStore) ready() bool {
	return s != nil && s.Pool != nil && s.Memberships != nil && s.Roles != nil && s.Audit != nil && s.Capabilities != nil
}
func (s *UnitStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, member, target string, write bool) error {
	return (companyAuthority{Memberships: s.Memberships, Roles: s.Roles, Capabilities: s.Capabilities}).check(ctx, tx, scope, actor, member, target, app.UnitPolicy(write))
}
func unitError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{d.ErrLastUnit, d.ErrStructureInUse, app.ErrUnitDuplicate} {
		if errors.Is(err, known) {
			return err
		}
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "P7506":
			return d.ErrLastUnit
		case "P7701":
			return app.DuplicateUnit("name")
		case "P7702":
			return app.DuplicateUnit("code")
		case "P7703":
			return d.ErrStructureInUse
		case "P7704":
			return app.ErrUnavailable
		case "23505":
			if pe.ConstraintName == "org_units_active_name" {
				return app.DuplicateUnit("name")
			}
			if pe.ConstraintName == "org_units_active_code" {
				return app.DuplicateUnit("code")
			}
			return app.ErrUnavailable
		}
	}
	return legalError(ctx, err)
}
func operatingUnit(scope kernel.OrgScope, row q.GetOperatingUnitRow) (d.Unit, error) {
	unit, err := d.RehydrateUnit(scope, d.UnitSnapshot{OrgID: scope.OrgID(), ID: row.ID, UnitProfile: d.UnitProfile{Name: row.Name, Code: row.Code, TimeZone: row.TimeZone, EstablishmentID: row.EstablishmentID, MunicipalityCode: row.MunicipalityCode}, State: d.UnitState(row.State), Revision: row.Revision})
	if err != nil {
		return d.Unit{}, app.ErrUnavailable
	}
	return unit, nil
}
func loadOperatingUnit(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (d.Unit, error) {
	row, err := q.New(tx).GetOperatingUnit(ctx, q.GetOperatingUnitParams{OrgID: scope.OrgID().String(), ID: id})
	if err != nil {
		return d.Unit{}, err
	}
	return operatingUnit(scope, row)
}
func (s *UnitStore) FindUnit(ctx context.Context, scope kernel.OrgScope, id string) (d.Unit, error) {
	if err := legalIdentity(scope, id); err != nil {
		return d.Unit{}, err
	}
	if s == nil || s.Pool == nil {
		return d.Unit{}, app.ErrUnavailable
	}
	var unit d.Unit
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error { var err error; unit, err = loadOperatingUnit(ctx, tx, scope, id); return err })
	if err != nil {
		return d.Unit{}, unitError(ctx, err)
	}
	return unit, nil
}
func decodeUnitUsage(raw []byte) (d.UnitUsage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 {
		return d.UnitUsage{}, app.ErrUnavailable
	}
	var usage d.UnitUsage
	for key, target := range map[string]*int64{"assets": &usage.Assets, "employees": &usage.Employees, "budgets": &usage.Budgets, "operations": &usage.Operations} {
		value, err := strconv.ParseInt(string(fields[key]), 10, 64)
		if err != nil || value < 0 {
			return d.UnitUsage{}, app.ErrUnavailable
		}
		*target = value
	}
	return usage, nil
}
func (s *UnitStore) WriteUnit(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, newID string, command app.UnitWrite) (app.UnitResult, bool, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.UnitResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.UnitResult{}, false, err
	}
	if v.Kind == app.CreateUnit && legalIdentity(scope, newID) != nil || v.Kind != app.CreateUnit && newID != "" {
		return app.UnitResult{}, false, d.ErrInvalid
	}
	if !s.ready() {
		return app.UnitResult{}, false, app.ErrUnavailable
	}
	target := v.UnitID
	if v.Kind == app.CreateUnit {
		target = scope.OrgID().String()
	}
	digest := command.Digest(scope)
	var result app.UnitResult
	replay := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, target, true); err != nil {
			return err
		}
		queries := q.New(tx)
		prior, err := queries.OperatingUnitCommandReceipt(ctx, q.OperatingUnitCommandReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(v.Kind) || prior.RequestHash != digest {
				return app.ErrOperationReused
			}
			if legalIdentity(scope, prior.UnitID) != nil || prior.ResultRevision < 1 || prior.ResultState != "ACTIVE" && prior.ResultState != "ARCHIVED" {
				return app.ErrUnavailable
			}
			result = app.UnitResult{UnitID: prior.UnitID, State: d.UnitState(prior.ResultState), Revision: prior.ResultRevision}
			replay = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		eventTarget, action, reason := v.UnitID, "org.unit.changed", "metadata"
		switch v.Kind {
		case app.CreateUnit:
			eventTarget, action, reason = newID, "org.unit.created", ""
		case app.ArchiveUnit:
			action, reason = "org.unit.archived", ""
		case app.RestoreUnit:
			reason = "restored"
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: action, TargetType: "unit", TargetID: eventTarget, RequestHash: digest, ReasonCode: reason}, func(ctx context.Context, tx pgx.Tx) error {
			var unit d.Unit
			zone, err := queries.OperatingUnitDefaults(ctx, scope.OrgID().String())
			if err != nil {
				return err
			}
			if !zone.Valid {
				return app.ErrUnavailable
			}
			if v.Kind == app.CreateUnit {
				unit, err = d.NewUnit(scope, newID, v.Profile, zone.String)
			} else {
				current, loadErr := loadOperatingUnit(ctx, tx, scope, v.UnitID)
				if loadErr != nil {
					return loadErr
				}
				before, snapshotErr := current.Snapshot(scope)
				if snapshotErr != nil {
					return snapshotErr
				}
				if before.Revision != v.ExpectedRevision {
					return d.ErrConflict
				}
				if v.Kind == app.ArchiveUnit && before.State != d.UnitActive {
					return d.ErrTransition
				}
				switch v.Kind {
				case app.ChangeUnit:
					unit, err = current.Change(scope, v.ExpectedRevision, v.Profile, zone.String)
				case app.RestoreUnit:
					unit, err = current.Restore(scope, v.ExpectedRevision)
				case app.ArchiveUnit:
					facts, readErr := queries.OperatingUnitArchiveFacts(ctx, q.OperatingUnitArchiveFactsParams{OrgID: scope.OrgID().String(), ID: v.UnitID})
					if readErr != nil {
						return readErr
					}
					usage, readErr := decodeUnitUsage(facts.Usage)
					if readErr != nil {
						return readErr
					}
					unit, err = current.Archive(scope, v.ExpectedRevision, d.UnitArchiveFacts{ActiveUnits: facts.ActiveUnits, Usage: usage})
					if errors.Is(err, d.ErrStructureInUse) {
						return app.UnitInUse(usage)
					}
				default:
					return d.ErrInvalid
				}
			}
			if err != nil {
				return err
			}
			snap, err := unit.Snapshot(scope)
			if err != nil {
				return err
			}
			if v.Kind == app.CreateUnit {
				err = queries.InsertOperatingUnit(ctx, q.InsertOperatingUnitParams{OrgID: scope.OrgID().String(), ID: snap.ID, Name: snap.Name, Code: snap.Code, TimeZone: snap.TimeZone, EstablishmentID: snap.EstablishmentID, MunicipalityCode: snap.MunicipalityCode})
			} else {
				changed, writeErr := queries.ChangeOperatingUnit(ctx, q.ChangeOperatingUnitParams{OrgID: scope.OrgID().String(), ID: snap.ID, ExpectedRevision: v.ExpectedRevision, Name: snap.Name, Code: snap.Code, TimeZone: snap.TimeZone, EstablishmentID: snap.EstablishmentID, MunicipalityCode: snap.MunicipalityCode, State: string(snap.State)})
				err = writeErr
				if err == nil && changed != 1 {
					return d.ErrConflict
				}
			}
			if err != nil {
				return err
			}
			result = app.UnitResult{UnitID: snap.ID, State: snap.State, Revision: snap.Revision}
			return queries.InsertOperatingUnitReceipt(ctx, q.InsertOperatingUnitReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID, ActorID: actor.AccountID, UnitID: snap.ID, Kind: string(v.Kind), RequestHash: digest, ResultState: string(snap.State), ResultRevision: snap.Revision})
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.UnitResult{}, false, unitError(ctx, err)
	}
	return result, replay, nil
}
func (s *UnitStore) ReadUnit(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, id string) (d.Unit, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return d.Unit{}, err
	}
	if err := legalIdentity(scope, id); err != nil {
		return d.Unit{}, err
	}
	if !s.ready() {
		return d.Unit{}, app.ErrUnavailable
	}
	var result d.Unit
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, id, false); err != nil {
			return err
		}
		var err error
		result, err = loadOperatingUnit(ctx, tx, scope, id)
		return err
	})
	if err != nil {
		return d.Unit{}, unitError(ctx, err)
	}
	return result, nil
}
func (s *UnitStore) ListUnits(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, filter app.UnitFilter) (app.UnitPage, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.UnitPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.UnitPage{}, err
	}
	if !s.ready() {
		return app.UnitPage{}, app.ErrUnavailable
	}
	result := app.UnitPage{Units: []d.Unit{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, scope.OrgID().String(), false); err != nil {
			return err
		}
		rows, err := q.New(tx).ListOperatingUnits(ctx, q.ListOperatingUnitsParams{OrgID: scope.OrgID().String(), AfterID: filter.AfterID, StateFilter: string(filter.State), RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		for i, row := range rows {
			if i == filter.Limit {
				last, err := result.Units[len(result.Units)-1].Snapshot(scope)
				if err != nil {
					return err
				}
				result.NextID = last.ID
				break
			}
			unit, err := operatingUnit(scope, q.GetOperatingUnitRow(row))
			if err != nil {
				return err
			}
			result.Units = append(result.Units, unit)
		}
		return nil
	})
	if err != nil {
		return app.UnitPage{}, unitError(ctx, err)
	}
	return result, nil
}
