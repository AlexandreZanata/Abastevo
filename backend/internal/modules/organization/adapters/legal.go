package adapters

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

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

type LegalStore struct {
	Pool         *pgxpool.Pool
	Memberships  members.TransactionRepository
	Roles        access.TransactionRepository
	Audit        auditing.TransactionRunner
	Capabilities setupCapability
}

var _ app.LegalRepository = (*LegalStore)(nil)
var _ app.LegalLookup = (*LegalStore)(nil)

func LegalAuditDefinitions() []audit.Definition {
	return []audit.Definition{
		{Action: "org.legal_entity.declared", TargetType: "legal_entity"},
		{Action: "org.legal_entity.withdrawn", TargetType: "legal_entity"},
		{Action: "org.legal_entity.identifier_read", TargetType: "legal_entity"},
		{Action: "org.establishment.declared", TargetType: "establishment"},
		{Action: "org.establishment.changed", TargetType: "establishment"},
	}
}
func legalError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{d.ErrInvalidCPF, d.ErrInvalidCNPJ, app.ErrLegalDuplicate, access.ErrNoMembership, access.ErrOrgSuspended, access.ErrOrgArchived} {
		if errors.Is(err, known) {
			return known
		}
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "P7601", "23505":
			return app.ErrLegalDuplicate
		case "P7603":
			return d.ErrInvalid
		case "55000":
			return d.ErrConflict
		}
	}
	return onboardingError(ctx, err)
}
func legalIdentity(scope kernel.OrgScope, raw string) error {
	if scope.Validate() != nil {
		return d.ErrScope
	}
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return d.ErrInvalid
	}
	return nil
}
func (s *LegalStore) ready() bool {
	return s != nil && s.Pool != nil && s.Memberships != nil && s.Roles != nil && s.Audit != nil && s.Capabilities != nil
}
func legalEntity(scope kernel.OrgScope, row q.GetLegalEntityRow) (d.LegalEntity, error) {
	value, err := d.RehydrateLegalEntity(scope, d.LegalEntitySnapshot{OrgID: scope.OrgID(), ID: row.ID, Kind: d.LegalKind(row.Kind), LegalName: row.LegalName, Identifier: row.Identifier, Purpose: d.LegalPurpose(row.Purpose), State: d.LegalState(row.State), Revision: row.Revision, VerificationHistory: row.VerificationHistory})
	if err != nil {
		return d.LegalEntity{}, app.ErrUnavailable
	}
	return value, nil
}
func legalEstablishment(scope kernel.OrgScope, row q.GetEstablishmentRow) (d.Establishment, error) {
	value, err := d.RehydrateEstablishment(scope, d.EstablishmentSnapshot{OrgID: scope.OrgID(), ID: row.ID, EntityID: row.LegalEntityID, CNPJ: row.Cnpj, Role: d.EstablishmentRole(row.DeclaredRole), TradeName: row.TradeName, MunicipalityCode: row.MunicipalityCode, Revision: row.Revision})
	if err != nil {
		return d.Establishment{}, app.ErrUnavailable
	}
	return value, nil
}
func loadLegal(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (d.LegalEntity, error) {
	row, err := q.New(tx).GetLegalEntity(ctx, q.GetLegalEntityParams{OrgID: scope.OrgID().String(), ID: id})
	if err != nil {
		return d.LegalEntity{}, err
	}
	return legalEntity(scope, row)
}
func loadEstablishment(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (d.Establishment, error) {
	row, err := q.New(tx).GetEstablishment(ctx, q.GetEstablishmentParams{OrgID: scope.OrgID().String(), ID: id})
	if err != nil {
		return d.Establishment{}, err
	}
	return legalEstablishment(scope, row)
}
func (s *LegalStore) FindLegalEntity(ctx context.Context, scope kernel.OrgScope, id string) (d.LegalEntity, error) {
	if err := legalIdentity(scope, id); err != nil {
		return d.LegalEntity{}, err
	}
	if s == nil || s.Pool == nil {
		return d.LegalEntity{}, app.ErrUnavailable
	}
	var value d.LegalEntity
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error { var err error; value, err = loadLegal(ctx, tx, scope, id); return err })
	if err != nil {
		return d.LegalEntity{}, legalError(ctx, err)
	}
	return value, nil
}
func (s *LegalStore) FindEstablishment(ctx context.Context, scope kernel.OrgScope, id string) (d.Establishment, error) {
	if err := legalIdentity(scope, id); err != nil {
		return d.Establishment{}, err
	}
	if s == nil || s.Pool == nil {
		return d.Establishment{}, app.ErrUnavailable
	}
	var value d.Establishment
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error { var err error; value, err = loadEstablishment(ctx, tx, scope, id); return err })
	if err != nil {
		return d.Establishment{}, legalError(ctx, err)
	}
	return value, nil
}

func (s *LegalStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.CompanyActor, member, target string, write, fresh bool) error {
	return (companyAuthority{Memberships: s.Memberships, Roles: s.Roles, Capabilities: s.Capabilities}).check(ctx, tx, scope, actor, member, target, app.LegalPolicy(write, fresh))
}

func legalCall(scope kernel.OrgScope, actor app.CompanyActor, member string) error {
	if scope.Validate() != nil {
		return d.ErrScope
	}
	if actor.Validate() != nil {
		return app.ErrUnauthenticated
	}
	if err := legalIdentity(scope, member); err != nil {
		return app.ErrUnauthenticated
	}
	return nil
}
func legalReceipt(row q.LegalCommandReceiptRow) (app.LegalResult, error) {
	id, err := kernel.ParseOrgID(row.LegalEntityID)
	if err != nil || id.String() != row.LegalEntityID || row.ResultRevision < 1 {
		return app.LegalResult{}, app.ErrUnavailable
	}
	if row.EstablishmentID != "" {
		id, err := kernel.ParseOrgID(row.EstablishmentID)
		if err != nil || id.String() != row.EstablishmentID {
			return app.LegalResult{}, app.ErrUnavailable
		}
	}
	if row.ResultState != "DECLARED" && row.ResultState != "REVOKED" && row.ResultState != "REMOVED" {
		return app.LegalResult{}, app.ErrUnavailable
	}
	return app.LegalResult{EntityID: row.LegalEntityID, EstablishmentID: row.EstablishmentID, State: row.ResultState, Revision: row.ResultRevision}, nil
}
func insertLegalEstablishment(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, value d.Establishment) error {
	v, err := value.Snapshot(scope)
	if err != nil {
		return err
	}
	return q.New(tx).InsertLegalEstablishment(ctx, q.InsertLegalEstablishmentParams{OrgID: scope.OrgID().String(), ID: v.ID, LegalEntityID: v.EntityID, Cnpj: v.CNPJ, DeclaredRole: string(v.Role), TradeName: v.TradeName, MunicipalityCode: v.MunicipalityCode})
}
func (s *LegalStore) WriteLegal(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, plan app.LegalPlan, command app.LegalWrite) (app.LegalResult, bool, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.LegalResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.LegalResult{}, false, err
	}
	if !s.ready() {
		return app.LegalResult{}, false, app.ErrUnavailable
	}
	target := v.EntityID
	action, targetType := "", "legal_entity"
	switch v.Kind {
	case app.DeclareLegal:
		target = scope.OrgID().String()
		action = "org.legal_entity.declared"
		if legalIdentity(scope, plan.EntityID) != nil {
			return app.LegalResult{}, false, d.ErrInvalid
		}
	case app.AddLegalEstablishment:
		action = "org.establishment.declared"
		targetType = "establishment"
		if legalIdentity(scope, plan.EstablishmentID) != nil {
			return app.LegalResult{}, false, d.ErrInvalid
		}
	case app.ChangeLegalEstablishment:
		target = v.EstablishmentID
		action = "org.establishment.changed"
		targetType = "establishment"
	case app.WithdrawLegal:
		action = "org.legal_entity.withdrawn"
	default:
		return app.LegalResult{}, false, d.ErrInvalid
	}
	digest := command.Digest(scope)
	var result app.LegalResult
	var replay bool
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		queries := q.New(tx)
		if err := s.authorize(ctx, tx, scope, actor, member, target, true, true); err != nil {
			return err
		}
		prior, err := queries.LegalCommandReceipt(ctx, q.LegalCommandReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(v.Kind) || prior.RequestHash != digest {
				return app.ErrOperationReused
			}
			result, err = legalReceipt(prior)
			replay = err == nil
			return err
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		eventTarget := v.EntityID
		if v.Kind == app.DeclareLegal {
			eventTarget = plan.EntityID
		} else if v.Kind == app.AddLegalEstablishment {
			eventTarget = plan.EstablishmentID
		} else if v.Kind == app.ChangeLegalEstablishment {
			eventTarget = v.EstablishmentID
		}
		auditReplay, err := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: action, TargetType: targetType, TargetID: eventTarget, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			switch v.Kind {
			case app.DeclareLegal:
				entity, establishment, err := v.Declaration.Create(scope, plan.EntityID, plan.EstablishmentID)
				if err != nil {
					return err
				}
				snap, err := entity.Snapshot(scope)
				if err != nil {
					return err
				}
				if err := queries.InsertLegalEntity(ctx, q.InsertLegalEntityParams{OrgID: scope.OrgID().String(), ID: snap.ID, Kind: string(snap.Kind), LegalName: snap.LegalName, Identifier: snap.Identifier, Purpose: string(snap.Purpose)}); err != nil {
					return err
				}
				if snap.Kind == d.Company {
					if err := insertLegalEstablishment(ctx, tx, scope, establishment); err != nil {
						return err
					}
				}
				result = app.LegalResult{EntityID: snap.ID, EstablishmentID: plan.EstablishmentID, State: string(d.LegalDeclared), Revision: 1}
			case app.AddLegalEstablishment:
				entity, err := loadLegal(ctx, tx, scope, v.EntityID)
				if err != nil {
					return err
				}
				parent, err := entity.Snapshot(scope)
				if err != nil {
					return err
				}
				if parent.Revision != v.ExpectedRevision {
					return d.ErrConflict
				}
				establishment, err := d.NewEstablishment(scope, plan.EstablishmentID, entity, v.Establishment)
				if err != nil {
					return err
				}
				if err := insertLegalEstablishment(ctx, tx, scope, establishment); err != nil {
					return err
				}
				result = app.LegalResult{EntityID: v.EntityID, EstablishmentID: plan.EstablishmentID, State: string(d.LegalDeclared), Revision: 1}
			case app.ChangeLegalEstablishment:
				entity, err := loadLegal(ctx, tx, scope, v.EntityID)
				if err != nil {
					return err
				}
				parent, err := entity.Snapshot(scope)
				if err != nil {
					return err
				}
				if parent.State != d.LegalDeclared {
					return d.ErrTransition
				}
				current, err := loadEstablishment(ctx, tx, scope, v.EstablishmentID)
				if err != nil {
					return err
				}
				before, err := current.Snapshot(scope)
				if err != nil {
					return err
				}
				if before.EntityID != v.EntityID {
					return app.ErrNotFound
				}
				next, err := current.ChangeMetadata(scope, v.ExpectedRevision, v.Metadata)
				if err != nil {
					return err
				}
				after, err := next.Snapshot(scope)
				if err != nil {
					return err
				}
				n, err := queries.ChangeLegalEstablishment(ctx, q.ChangeLegalEstablishmentParams{OrgID: scope.OrgID().String(), ID: after.ID, DeclaredRole: string(after.Role), TradeName: after.TradeName, MunicipalityCode: after.MunicipalityCode, ExpectedRevision: v.ExpectedRevision})
				if err != nil {
					return err
				}
				if n != 1 {
					return d.ErrConflict
				}
				result = app.LegalResult{EntityID: v.EntityID, EstablishmentID: v.EstablishmentID, State: string(d.LegalDeclared), Revision: after.Revision}
			case app.WithdrawLegal:
				entity, err := loadLegal(ctx, tx, scope, v.EntityID)
				if err != nil {
					return err
				}
				result, err = withdrawLegal(ctx, tx, scope, v.EntityID, v.ExpectedRevision, entity)
				if err != nil {
					return err
				}
			}
			return queries.InsertLegalCommandReceipt(ctx, q.InsertLegalCommandReceiptParams{OrgID: scope.OrgID().String(), OperationID: v.OperationID, ActorID: actor.AccountID, Kind: string(v.Kind), RequestHash: digest, LegalEntityID: result.EntityID, EstablishmentID: result.EstablishmentID, ResultState: result.State, ResultRevision: result.Revision})
		})
		if err != nil {
			return err
		}
		if auditReplay {
			return app.ErrUnavailable
		}
		return nil
	})
	if err != nil {
		return app.LegalResult{}, false, legalError(ctx, err)
	}
	return result, replay, nil
}
func withdrawLegal(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string, expected int64, entity d.LegalEntity) (app.LegalResult, error) {
	decision, err := entity.Withdraw(scope, expected, d.LegalReferences{})
	if err != nil {
		return app.LegalResult{}, err
	}
	if decision.Remove {
		// Real FKs are the reference authority, including consumers added later.
		// Flush deferred constraints inside the savepoint too; a refused deletion
		// rolls back the cascade and is replaced by revocation in the same command.
		savepoint, err := tx.Begin(ctx)
		if err != nil {
			return app.LegalResult{}, err
		}
		n, removeErr := q.New(savepoint).DeleteLegalDeclaration(ctx, q.DeleteLegalDeclarationParams{OrgID: scope.OrgID().String(), ID: id, ExpectedRevision: expected})
		if removeErr == nil {
			_, removeErr = savepoint.Exec(ctx, `SET CONSTRAINTS ALL IMMEDIATE`)
		}
		if removeErr == nil && n != 1 {
			removeErr = d.ErrConflict
		}
		if removeErr == nil {
			if err := savepoint.Commit(ctx); err != nil {
				return app.LegalResult{}, err
			}
			return app.LegalResult{EntityID: id, State: "REMOVED", Revision: expected}, nil
		}
		if err := savepoint.Rollback(ctx); err != nil {
			return app.LegalResult{}, err
		}
		var failure *pgconn.PgError
		if !errors.As(removeErr, &failure) || (failure.Code != "23503" && failure.Code != "P7602") {
			return app.LegalResult{}, removeErr
		}
		decision, err = entity.Withdraw(scope, expected, d.LegalReferences{Current: 1})
		if err != nil {
			return app.LegalResult{}, err
		}
	}
	snap, err := decision.Entity.Snapshot(scope)
	if err != nil {
		return app.LegalResult{}, err
	}
	n, err := q.New(tx).RevokeLegalDeclaration(ctx, q.RevokeLegalDeclarationParams{OrgID: scope.OrgID().String(), ID: id, ExpectedRevision: expected})
	if err != nil {
		return app.LegalResult{}, err
	}
	if n != 1 {
		return app.LegalResult{}, d.ErrConflict
	}
	return app.LegalResult{EntityID: id, State: string(snap.State), Revision: snap.Revision}, nil
}
func (s *LegalStore) ReadLegal(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, id string, reveal bool, operation string) (app.LegalReadResult, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.LegalReadResult{}, err
	}
	if err := legalIdentity(scope, id); err != nil {
		return app.LegalReadResult{}, err
	}
	if reveal {
		if err := legalIdentity(scope, operation); err != nil {
			return app.LegalReadResult{}, err
		}
	} else if operation != "" {
		return app.LegalReadResult{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.LegalReadResult{}, app.ErrUnavailable
	}
	var result app.LegalReadResult
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, id, false, reveal); err != nil {
			return err
		}
		entity, err := loadLegal(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		result = app.LegalReadResult{Entity: entity, Revealed: reveal}
		if reveal {
			raw := fmt.Sprintf("org.legal.identifier_read.v1:%s:%s:%s", scope.OrgID().String(), id, operation)
			digest := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
			replay, err := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: operation, ActorID: actor.AccountID, Action: "org.legal_entity.identifier_read", TargetType: "legal_entity", TargetID: id, RequestHash: digest}, func(context.Context, pgx.Tx) error { return nil })
			if err != nil {
				return err
			}
			if replay {
				return app.ErrOperationReused
			}
			result.AuditOperationID = operation
		}
		return nil
	})
	if err != nil {
		return app.LegalReadResult{}, legalError(ctx, err)
	}
	return result, nil
}
func (s *LegalStore) ListLegal(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member string, filter app.LegalFilter) (app.LegalPage, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.LegalPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.LegalPage{}, err
	}
	if !s.ready() {
		return app.LegalPage{}, app.ErrUnavailable
	}
	result := app.LegalPage{Entities: []d.LegalEntity{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, scope.OrgID().String(), false, false); err != nil {
			return err
		}
		rows, err := q.New(tx).ListLegalEntities(ctx, q.ListLegalEntitiesParams{OrgID: scope.OrgID().String(), StateFilter: string(filter.State), AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		for i, row := range rows {
			if i == filter.Limit {
				last, err := result.Entities[len(result.Entities)-1].Snapshot(scope)
				if err != nil {
					return err
				}
				result.NextID = last.ID
				break
			}
			entity, err := legalEntity(scope, q.GetLegalEntityRow(row))
			if err != nil {
				return err
			}
			result.Entities = append(result.Entities, entity)
		}
		return nil
	})
	if err != nil {
		return app.LegalPage{}, legalError(ctx, err)
	}
	return result, nil
}
func (s *LegalStore) ListEstablishments(ctx context.Context, scope kernel.OrgScope, actor app.CompanyActor, member, entityID string, filter app.LegalFilter) (app.EstablishmentPage, error) {
	if err := legalCall(scope, actor, member); err != nil {
		return app.EstablishmentPage{}, err
	}
	if legalIdentity(scope, entityID) != nil || filter.Validate() != nil || filter.State != "" {
		return app.EstablishmentPage{}, d.ErrInvalid
	}
	if !s.ready() {
		return app.EstablishmentPage{}, app.ErrUnavailable
	}
	result := app.EstablishmentPage{Establishments: []d.Establishment{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, entityID, false, false); err != nil {
			return err
		}
		if _, err := loadLegal(ctx, tx, scope, entityID); err != nil {
			return err
		}
		rows, err := q.New(tx).ListLegalEstablishments(ctx, q.ListLegalEstablishmentsParams{OrgID: scope.OrgID().String(), LegalEntityID: entityID, AfterID: filter.AfterID, RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		for i, row := range rows {
			if i == filter.Limit {
				last, err := result.Establishments[len(result.Establishments)-1].Snapshot(scope)
				if err != nil {
					return err
				}
				result.NextID = last.ID
				break
			}
			establishment, err := legalEstablishment(scope, q.GetEstablishmentRow(row))
			if err != nil {
				return err
			}
			result.Establishments = append(result.Establishments, establishment)
		}
		return nil
	})
	if err != nil {
		return app.EstablishmentPage{}, legalError(ctx, err)
	}
	return result, nil
}

// A removed declaration retains only its immutable creation acknowledgement.
// Resolving this reference lets an authorized command replay its original result;
// content reads still query the live row and return not-found after removal.
func (s *LegalStore) LegalTargetExists(ctx context.Context, scope kernel.OrgScope, entity, establishment string) (bool, error) {
	if err := legalIdentity(scope, entity); err != nil {
		return false, err
	}
	if establishment != "" {
		if err := legalIdentity(scope, establishment); err != nil {
			return false, err
		}
	}
	if s == nil || s.Pool == nil {
		return false, app.ErrUnavailable
	}
	var found bool
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		var err error
		found, err = q.New(tx).LegalTargetExists(ctx, q.LegalTargetExistsParams{OrgID: scope.OrgID().String(), LegalEntityID: entity, EstablishmentID: establishment})
		return err
	})
	if err != nil {
		return false, legalError(ctx, err)
	}
	return found, nil
}
