// Package adapters persists private fleet records through ordinary scoped transactions.
package adapters

import (
	"context"
	"errors"
	"math"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	auditing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AssetStore struct {
	Pool      *pgxpool.Pool
	Authority app.FleetAuthority
	Audit     auditing.TransactionRunner
	NewID     func() (string, error)
}

var _ app.AssetRepository = (*AssetStore)(nil)
var _ app.AssetLookup = (*AssetStore)(nil)

func AssetAuditDefinitions() []audit.Definition {
	return []audit.Definition{{Action: "fleet.asset.created", TargetType: "asset"}, {Action: "fleet.asset.changed", TargetType: "asset"}, {Action: "fleet.asset.discarded", TargetType: "asset"}, {Action: "fleet.asset.activated", TargetType: "asset"}}
}
func validID(raw string) bool { id, e := kernel.ParseOrgID(raw); return e == nil && id.String() == raw }
func validCall(scope kernel.OrgScope, actor app.FleetActor, member string) error {
	if scope.Validate() != nil {
		return fleet.ErrScope
	}
	if actor.Validate() != nil {
		return app.ErrUnauthenticated
	}
	if !validID(member) {
		return fleet.ErrInvalid
	}
	return nil
}
func storeError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, known := range []error{access.ErrNoMembership, access.ErrMembershipSuspended, access.ErrOrgSuspended, access.ErrOrgArchived, access.ErrFreshAuthRequired, access.ErrEntitlementRequired, access.ErrUnauthenticated, permission.ErrPermissionDenied, permission.ErrResourceNotFound, app.ErrConflict, app.ErrOperationReused, app.ErrIdentifierTaken, app.ErrNotFound, app.ErrUnavailable, app.ErrUnauthenticated, fleet.ErrInvalid, fleet.ErrScope, fleet.ErrKindLocked, fleet.ErrIncomplete} {
		if errors.Is(err, known) {
			return err
		}
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505":
			if pe.ConstraintName == "org_assets_live_fleet_key" || pe.ConstraintName == "org_assets_live_plate_key" {
				return app.ErrIdentifierTaken
			}
		case "P7903", "55000":
			return app.ErrConflict
		case "23503":
			return app.ErrNotFound
		case "23514":
			return fleet.ErrInvalid
		}
	}
	return app.ErrUnavailable
}
func (s *AssetStore) authorize(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor app.FleetActor, member, target string, action app.FleetAction) error {
	if err := s.Authority.CheckFleetTx(ctx, tx, scope, actor, member, target, action); err != nil {
		return err
	}
	return nil
}
func (s *AssetStore) category(ctx context.Context, queries *q.Queries, scope kernel.OrgScope, choice app.CategoryChoice) (string, string, error) {
	org := scope.OrgID().String()
	if choice.ExistingID != "" {
		row, err := queries.AssetCategoryByID(ctx, q.AssetCategoryByIDParams{OrgID: org, ID: choice.ExistingID})
		return row.ID, row.Label, err
	}
	row, err := queries.AssetCategoryByLabel(ctx, q.AssetCategoryByLabelParams{OrgID: org, Label: choice.NewLabel})
	if err == nil {
		return row.ID, row.Label, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	if s.NewID == nil {
		return "", "", app.ErrUnavailable
	}
	id, err := s.NewID()
	if err != nil || !validID(id) {
		return "", "", app.ErrUnavailable
	}
	err = queries.InsertAssetCategory(ctx, q.InsertAssetCategoryParams{OrgID: org, ID: id, Label: choice.NewLabel})
	return id, choice.NewLabel, err
}
func (s *AssetStore) WriteAsset(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, newID string, command app.AssetWrite) (app.AssetResult, bool, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AssetResult{}, false, err
	}
	v, err := command.Snapshot()
	if err != nil {
		return app.AssetResult{}, false, err
	}
	if (v.Kind == app.CreateAsset && !validID(newID)) || (v.Kind != app.CreateAsset && newID != "") {
		return app.AssetResult{}, false, fleet.ErrInvalid
	}
	if s == nil || s.Pool == nil || s.Authority == nil || s.Audit == nil {
		return app.AssetResult{}, false, app.ErrUnavailable
	}
	org := scope.OrgID().String()
	target := v.AssetID
	if v.Kind == app.CreateAsset {
		target = org
	}
	digest := command.Digest(scope)
	var result app.AssetResult
	replayed := false
	err = database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, target, app.FleetManage); err != nil {
			return err
		}
		queries := q.New(tx)
		if _, err := queries.AssetRootLock(ctx, org); err != nil {
			return err
		}
		prior, err := queries.AssetCommandReceipt(ctx, q.AssetCommandReceiptParams{OrgID: org, OperationID: v.OperationID})
		if err == nil {
			if prior.ActorID != actor.AccountID || prior.Kind != string(v.Kind) || prior.RequestHash != digest {
				return app.ErrOperationReused
			}
			if !validID(prior.AssetID) || prior.ResultRevision < 1 || prior.Discarded != (prior.Kind == string(app.DiscardAsset)) || prior.Discarded && prior.ResultState != "" || !prior.Discarded && prior.ResultState != "DRAFT" && prior.ResultState != "ACTIVE" {
				return app.ErrUnavailable
			}
			result = app.AssetResult{AssetID: prior.AssetID, State: fleet.State(prior.ResultState), Revision: prior.ResultRevision, Discarded: prior.Discarded}
			replayed = true
			return s.authorize(ctx, tx, scope, actor, member, target, app.FleetManage)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		id := v.AssetID
		if v.Kind == app.CreateAsset {
			id = newID
		}
		action := map[app.AssetWriteKind]string{app.CreateAsset: "fleet.asset.created", app.ChangeAsset: "fleet.asset.changed", app.DiscardAsset: "fleet.asset.discarded", app.ActivateAsset: "fleet.asset.activated"}[v.Kind]
		repeated, err := s.Audit.RecordChangeTx(ctx, tx, scope, audit.Event{OperationID: v.OperationID, ActorID: actor.AccountID, Action: action, TargetType: "asset", TargetID: id, RequestHash: digest}, func(ctx context.Context, tx pgx.Tx) error {
			queries = q.New(tx)
			var current app.AssetView
			if v.Kind != app.CreateAsset {
				var err error
				current, err = loadAsset(ctx, tx, scope, id)
				if err != nil {
					return err
				}
				if current.Revision != v.ExpectedRevision || current.Revision == math.MaxInt64 {
					return app.ErrConflict
				}
			}
			result = app.AssetResult{AssetID: id, State: fleet.Draft, Revision: 1}
			if v.Kind == app.DiscardAsset {
				if current.State != fleet.Draft {
					return app.ErrConflict
				}
				n, err := queries.DeleteAsset(ctx, q.DeleteAssetParams{OrgID: org, ID: id, ExpectedRevision: v.ExpectedRevision})
				if err != nil {
					return err
				}
				if n != 1 {
					return app.ErrConflict
				}
				result = app.AssetResult{AssetID: id, Revision: current.Revision + 1, Discarded: true}
			} else {
				var view app.AssetView
				var checked fleet.Snapshot
				if v.Kind == app.ActivateAsset {
					if current.State != fleet.Draft {
						return app.ErrConflict
					}
					if current.Profile.Kind != fleet.Implement {
						return app.ErrUnavailable
					}
					if current.UnitID == "" {
						pure, err := fleet.NewDraft(current.Ref, current.Profile, ownedCatalog{categoryID: current.Profile.Category})
						if err != nil {
							return err
						}
						return pure.CheckActivation(scope, fleet.ActivationFacts{})
					}
					if err := queries.AssetAdmitUnit(ctx, q.AssetAdmitUnitParams{OrgID: org, UnitID: current.UnitID}); err != nil {
						return err
					}
					var err error
					checked, err = validated(scope, id, current.Profile)
					if err != nil {
						return err
					}
					unit, err := fleet.NewUnitRef(scope, current.UnitID)
					if err != nil {
						return err
					}
					pure, err := fleet.NewDraft(checked.Ref, current.Profile, ownedCatalog{categoryID: current.Profile.Category})
					if err != nil {
						return err
					}
					if err := pure.CheckActivation(scope, fleet.ActivationFacts{Unit: &unit}); err != nil {
						return err
					}
					view = current
					view.State = fleet.Active
					view.Revision++
				} else {
					category, label, err := s.category(ctx, queries, scope, v.Category)
					if err != nil {
						return err
					}
					profile := v.Profile
					profile.Category = category
					checked, err = validated(scope, id, profile)
					if err != nil {
						return err
					}
					view = app.AssetView{Ref: checked.Ref, Profile: checked.Profile, CategoryLabel: label, State: fleet.Draft, Revision: 1, UnitID: v.UnitID}
					if v.Kind == app.ChangeAsset {
						view.State = current.State
						view.Revision = current.Revision + 1
						if current.State != fleet.Draft && current.State != fleet.Active {
							return app.ErrConflict
						}
						if current.State == fleet.Active {
							if view.Profile.Kind != fleet.Implement {
								return app.ErrUnavailable
							}
							if v.UnitID != "" && v.UnitID != current.UnitID {
								return app.ErrConflict
							}
							view.UnitID = current.UnitID
							view.Allocation = current.Allocation
						}
					}
				}
				if v.Kind == app.CreateAsset {
					if err := queries.InsertAsset(ctx, q.InsertAssetParams{OrgID: org, ID: id, Kind: string(view.Profile.Kind), CategoryID: view.Profile.Category, Name: view.Profile.Name, FleetNumber: view.Profile.FleetNumber, Plate: view.Profile.Plate, SerialNumber: view.Profile.SerialNumber, FleetKey: checked.FleetKey, PlateKey: checked.PlateKey, UsageBasis: string(view.Profile.UsageBasis), UnitID: view.UnitID}); err != nil {
						return err
					}
				} else {
					n, err := queries.UpdateAsset(ctx, q.UpdateAssetParams{OrgID: org, ID: id, Kind: string(view.Profile.Kind), CategoryID: view.Profile.Category, Name: view.Profile.Name, FleetNumber: view.Profile.FleetNumber, Plate: view.Profile.Plate, SerialNumber: view.Profile.SerialNumber, FleetKey: checked.FleetKey, PlateKey: checked.PlateKey, UsageBasis: string(view.Profile.UsageBasis), State: string(view.State), UnitID: view.UnitID, ExpectedRevision: v.ExpectedRevision})
					if err != nil {
						return err
					}
					if n != 1 {
						return app.ErrConflict
					}
				}
				if v.Kind != app.ActivateAsset {
					if err := saveConfiguration(ctx, queries, org, id, view.Profile); err != nil {
						return err
					}
				} else {
					if err := queries.InsertAssetAllocation(ctx, q.InsertAssetAllocationParams{OrgID: org, AssetID: id, UnitID: view.UnitID}); err != nil {
						return err
					}
				}
				raw, err := snapshotBytes(view)
				if err != nil {
					return app.ErrUnavailable
				}
				if err := queries.InsertAssetRevision(ctx, q.InsertAssetRevisionParams{OrgID: org, AssetID: id, Revision: view.Revision, OperationID: v.OperationID, Snapshot: raw}); err != nil {
					return err
				}
				result.State = view.State
				result.Revision = view.Revision
			}
			if err := queries.InsertAssetCommand(ctx, q.InsertAssetCommandParams{OrgID: org, OperationID: v.OperationID, ActorID: actor.AccountID, AssetID: id, Kind: string(v.Kind), RequestHash: digest, ResultState: string(result.State), ResultRevision: result.Revision, Discarded: result.Discarded}); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		if repeated {
			return app.ErrUnavailable
		}
		return s.authorize(ctx, tx, scope, actor, member, target, app.FleetManage)
	})
	if err != nil {
		return app.AssetResult{}, false, storeError(ctx, err)
	}
	return result, replayed, nil
}
func (s *AssetStore) FindAsset(ctx context.Context, scope kernel.OrgScope, id string) (app.AssetView, error) {
	if scope.Validate() != nil {
		return app.AssetView{}, fleet.ErrScope
	}
	if !validID(id) {
		return app.AssetView{}, fleet.ErrInvalid
	}
	if s == nil || s.Pool == nil {
		return app.AssetView{}, app.ErrUnavailable
	}
	var view app.AssetView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if _, err := q.New(tx).AssetRootShare(ctx, scope.OrgID().String()); err != nil {
			return err
		}
		var err error
		view, err = loadAsset(ctx, tx, scope, id)
		return err
	})
	if err != nil {
		return app.AssetView{}, storeError(ctx, err)
	}
	return view, nil
}
func (s *AssetStore) ReadAsset(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member, id string) (app.AssetView, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AssetView{}, err
	}
	if !validID(id) {
		return app.AssetView{}, fleet.ErrInvalid
	}
	if s == nil || s.Pool == nil || s.Authority == nil {
		return app.AssetView{}, app.ErrUnavailable
	}
	var view app.AssetView
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		if err := s.authorize(ctx, tx, scope, actor, member, id, app.FleetRead); err != nil {
			return err
		}
		var err error
		view, err = loadAsset(ctx, tx, scope, id)
		if err != nil {
			return err
		}
		return s.authorize(ctx, tx, scope, actor, member, id, app.FleetRead)
	})
	if err != nil {
		return app.AssetView{}, storeError(ctx, err)
	}
	return view, nil
}
func (s *AssetStore) ListAssets(ctx context.Context, scope kernel.OrgScope, actor app.FleetActor, member string, filter app.AssetFilter) (app.AssetPage, error) {
	if err := validCall(scope, actor, member); err != nil {
		return app.AssetPage{}, err
	}
	if err := filter.Validate(); err != nil {
		return app.AssetPage{}, err
	}
	if s == nil || s.Pool == nil || s.Authority == nil {
		return app.AssetPage{}, app.ErrUnavailable
	}
	page := app.AssetPage{Assets: []app.AssetView{}}
	err := database.WithOrgTx(ctx, s.Pool, scope, func(tx pgx.Tx) error {
		org := scope.OrgID().String()
		if err := s.authorize(ctx, tx, scope, actor, member, org, app.FleetRead); err != nil {
			return err
		}
		ids, err := q.New(tx).ListAssetIDs(ctx, q.ListAssetIDsParams{OrgID: org, AfterID: filter.AfterID, State: string(filter.State), Kind: string(filter.Kind), RowLimit: int32(filter.Limit + 1)})
		if err != nil {
			return err
		}
		more := len(ids) > filter.Limit
		if more {
			ids = ids[:filter.Limit]
		}
		for _, id := range ids {
			view, err := loadAsset(ctx, tx, scope, id)
			if err != nil {
				return err
			}
			page.Assets = append(page.Assets, view)
		}
		if more && len(ids) > 0 {
			page.NextID = ids[len(ids)-1]
		}
		return s.authorize(ctx, tx, scope, actor, member, org, app.FleetRead)
	})
	if err != nil {
		return app.AssetPage{}, storeError(ctx, err)
	}
	return page, nil
}
