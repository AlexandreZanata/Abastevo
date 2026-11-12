package adapters

import (
	"context"
	"errors"

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

type CostCentreStore struct {
	Pool         *pgxpool.Pool
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Audit        auditing.TransactionRunner
	Capabilities setupCapability
}

var _ app.CostCentreRepository = (*CostCentreStore)(nil)
var _ app.CostCentreLookup = (*CostCentreStore)(nil)

func CostCentreAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "org.cost_centre.changed", TargetType: "cost_centre", Reasons: []string{"created", "metadata", "archived", "restored"}, RequireReason: true}}
}
func (s *CostCentreStore) ready() bool {
	return s != nil && s.Pool != nil && s.Memberships != nil && s.Roles != nil && s.Audit != nil && s.Capabilities != nil
}
func (s *CostCentreStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, member, target string, write bool) error {
	return (companyAuthority{Memberships: s.Memberships, Roles: s.Roles, Capabilities: s.Capabilities}).check(ctx, tx, scope, actor, member, target, app.StructurePolicy(write))
}
func costCentreError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, app.ErrCostCentreCodeReserved) || errors.Is(err, d.ErrStructureInUse) {
		return err
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "P7801":
			return app.ErrCostCentreCodeReserved
		case "P7803":
			return d.ErrStructureInUse
		case "P7804":
			return app.ErrUnavailable
		case "23505":
			if pe.ConstraintName == "org_cost_centre_codes_pkey" {
				return app.ErrCostCentreCodeReserved
			}
			return app.ErrUnavailable
		}
	}
	return legalError(ctx, err)
}
func costCentre(scope kernel.OrgScope, row q.GetCostCentreRow) (d.CostCentre, error) {
	c, err := d.RehydrateCostCentre(scope, d.CostCentreSnapshot{OrgID: scope.OrgID(), ID: row.ID, CostCentreProfile: d.CostCentreProfile{Name: row.Name, Code: row.Code}, State: d.CostCentreState(row.State), Revision: row.Revision})
	if err != nil {
		return d.CostCentre{}, app.ErrUnavailable
	}
	return c, nil
}
func loadCostCentre(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (d.CostCentre, error) {
	row, err := q.New(tx).GetCostCentre(ctx, q.GetCostCentreParams{OrgID: scope.OrgID().String(), ID: id})
	if err != nil {
		return d.CostCentre{}, err
	}
	return costCentre(scope, row)
}
func (s *CostCentreStore) FindCostCentre(ctx context.Context, scope kernel.OrgScope, id string) (d.CostCentre, error) {
	if err := legalIdentity(scope, id); err != nil {
		return d.CostCentre{}, err
	}
	if s == nil || s.Pool == nil {
		return d.CostCentre{}, app.ErrUnavailable
	}
	var result d.CostCentre
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error { var err error; result, err = loadCostCentre(ctx, tx, scope, id); return err })
	if err != nil {
		return d.CostCentre{}, costCentreError(ctx, err)
	}
	return result, nil
}
func (s *CostCentreStore) WriteCostCentre(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, newID string, command app.CostCentreWrite) (app.CostCentreResult, bool, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.CostCentreResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.CostCentreResult{}, false, err
	}
	if v.Kind == app.CreateCostCentre && legalIdentity(scope, newID) != nil || v.Kind != app.CreateCostCentre && newID != "" {
		return app.CostCentreResult{}, false, d.ErrInvalid
	}
	if !s.ready() {
		return app.CostCentreResult{}, false, app.ErrUnavailable
	}
	target := v.CentreID
	if v.Kind == app.CreateCostCentre {
		target = scope.OrgID().String()
	}
	digest := command.Digest(scope)
	var result app.CostCentreResult
	replay := false
	readyToCommit := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, target, true); err != nil {
			return err
		}
		queries := q.New(tx)
		prior, err := queries.CostCentreCommandReceipt(ctx, q.CostCentreCommandReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(v.Kind) || prior.RequestHash != digest {
				return app.ErrOperationReused
			}
			if legalIdentity(scope, prior.CentreID) != nil || prior.ResultRevision < 1 || prior.ResultState != "ACTIVE" && prior.ResultState != "ARCHIVED" {
				return app.ErrUnavailable
			}
			result = app.CostCentreResult{CentreID: prior.CentreID, State: d.CostCentreState(prior.ResultState), Revision: prior.ResultRevision}
			replay = true
			readyToCommit = true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		eventTarget, reason := v.CentreID, "metadata"
		switch v.Kind {
		case app.CreateCostCentre:
			eventTarget, reason = newID, "created"
		case app.ArchiveCostCentre:
			reason = "archived"
		case app.RestoreCostCentre:
			reason = "restored"
		}
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: "org.cost_centre.changed", TargetType: "cost_centre", TargetID: eventTarget, RequestHash: digest, ReasonCode: reason}, func(ctx context.Context, tx pgx.Tx) error {
			var centre d.CostCentre
			var err error
			if v.Kind == app.CreateCostCentre {
				centre, err = d.NewCostCentre(scope, newID, v.Profile)
			} else {
				current, readErr := loadCostCentre(ctx, tx, scope, v.CentreID)
				if readErr != nil {
					return readErr
				}
				before, readErr := current.Snapshot(scope)
				if readErr != nil {
					return readErr
				}
				if before.Revision != v.ExpectedRevision {
					return d.ErrConflict
				}
				switch v.Kind {
				case app.ChangeCostCentre:
					centre, err = current.Change(scope, v.ExpectedRevision, v.Profile)
				case app.RestoreCostCentre:
					centre, err = current.Restore(scope, v.ExpectedRevision)
				case app.ArchiveCostCentre:
					if before.State != d.CostCentreActive {
						return d.ErrTransition
					}
					raw, readErr := queries.CostCentreCurrentUsage(ctx, q.CostCentreCurrentUsageParams{OrgID: scope.OrgID().String(), ID: v.CentreID})
					if readErr != nil {
						return readErr
					}
					usage, readErr := decodeUnitUsage(raw)
					if readErr != nil {
						return readErr
					}
					centre, err = current.Archive(scope, v.ExpectedRevision, usage)
					if errors.Is(err, d.ErrStructureInUse) {
						return app.StructureInUse(usage)
					}
				default:
					return d.ErrInvalid
				}
			}
			if err != nil {
				return err
			}
			snapshot, err := centre.Snapshot(scope)
			if err != nil {
				return err
			}
			if v.Kind == app.CreateCostCentre {
				err = queries.InsertCostCentre(ctx, q.InsertCostCentreParams{OrgID: scope.OrgID().String(), ID: snapshot.ID, Name: snapshot.Name, Code: snapshot.Code})
			} else {
				changed, writeErr := queries.ChangeCostCentre(ctx, q.ChangeCostCentreParams{OrgID: scope.OrgID().String(), ID: snapshot.ID, ExpectedRevision: v.ExpectedRevision, Name: snapshot.Name, Code: snapshot.Code, State: string(snapshot.State)})
				err = writeErr
				if err == nil && changed != 1 {
					return d.ErrConflict
				}
			}
			if err != nil {
				return err
			}
			result = app.CostCentreResult{CentreID: snapshot.ID, State: snapshot.State, Revision: snapshot.Revision}
			return queries.InsertCostCentreReceipt(ctx, q.InsertCostCentreReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID, ActorID: actor.AccountID, CentreID: snapshot.ID, Kind: string(v.Kind), RequestHash: digest, ResultState: string(snapshot.State), ResultRevision: snapshot.Revision})
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		readyToCommit = true
		return nil
	})
	if err != nil {
		if readyToCommit && ctx.Err() == nil {
			return app.CostCentreResult{}, false, app.ErrUnavailable
		}
		return app.CostCentreResult{}, false, costCentreError(ctx, err)
	}
	return result, replay, nil
}
func (s *CostCentreStore) ReadCostCentre(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, id string) (d.CostCentre, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return d.CostCentre{}, err
	}
	if err := legalIdentity(scope, id); err != nil {
		return d.CostCentre{}, err
	}
	if !s.ready() {
		return d.CostCentre{}, app.ErrUnavailable
	}
	var result d.CostCentre
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, id, false); err != nil {
			return err
		}
		var err error
		result, err = loadCostCentre(ctx, tx, scope, id)
		return err
	})
	if err != nil {
		return d.CostCentre{}, costCentreError(ctx, err)
	}
	return result, nil
}
func (s *CostCentreStore) ReadCostCentreCode(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, raw string) (d.CostCentre, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return d.CostCentre{}, err
	}
	code, err := d.CanonicalCostCentreCode(raw)
	if err != nil {
		return d.CostCentre{}, err
	}
	if !s.ready() {
		return d.CostCentre{}, app.ErrUnavailable
	}
	var result d.CostCentre
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, scope.OrgID().String(), false); err != nil {
			return err
		}
		row, err := q.New(tx).GetCostCentreByHistoricalCode(ctx, q.GetCostCentreByHistoricalCodeParams{OrgID: scope.OrgID().String(), Code: code})
		if err != nil {
			return err
		}
		result, err = costCentre(scope, q.GetCostCentreRow(row))
		return err
	})
	if err != nil {
		return d.CostCentre{}, costCentreError(ctx, err)
	}
	return result, nil
}
func (s *CostCentreStore) ListCostCentres(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, filter app.CostCentreFilter) (app.CostCentrePage, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.CostCentrePage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.CostCentrePage{}, err
	}
	if !s.ready() {
		return app.CostCentrePage{}, app.ErrUnavailable
	}
	result := app.CostCentrePage{Centres: []d.CostCentre{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, scope.OrgID().String(), false); err != nil {
			return err
		}
		rows, err := q.New(tx).ListCostCentres(ctx, q.ListCostCentresParams{OrgID: scope.OrgID().String(), AfterID: filter.AfterID, StateFilter: string(filter.State), RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		for i, row := range rows {
			if i == filter.Limit {
				last, err := result.Centres[len(result.Centres)-1].Snapshot(scope)
				if err != nil {
					return err
				}
				result.NextID = last.ID
				break
			}
			value, err := costCentre(scope, q.GetCostCentreRow(row))
			if err != nil {
				return err
			}
			result.Centres = append(result.Centres, value)
		}
		return nil
	})
	if err != nil {
		return app.CostCentrePage{}, costCentreError(ctx, err)
	}
	return result, nil
}
