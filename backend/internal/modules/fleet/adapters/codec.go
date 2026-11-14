package adapters

import (
	"context"
	"encoding/json"
	"strconv"

	q "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/fleet"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/jackc/pgx/v5"
)

// No private domain MarshalJSON is used: it deliberately redacts itself.
type tankRecord struct {
	Role     string   `json:"role"`
	Capacity int64    `json:"capacity"`
	Unit     string   `json:"unit"`
	Products []string `json:"products"`
}
type expectedRecord struct {
	Product string `json:"product"`
	Basis   string `json:"basis"`
	Value   int64  `json:"value"`
	Unit    string `json:"unit"`
	Source  string `json:"source"`
	Date    string `json:"date"`
}
type revisionRecord struct {
	Kind          string           `json:"kind"`
	CategoryID    string           `json:"category_id"`
	CategoryLabel string           `json:"category_label"`
	Name          string           `json:"name"`
	FleetNumber   string           `json:"fleet_number"`
	Plate         string           `json:"plate"`
	SerialNumber  string           `json:"serial_number"`
	UsageBasis    string           `json:"usage_basis"`
	State         string           `json:"state"`
	UnitID        string           `json:"unit_id"`
	Tanks         []tankRecord     `json:"tanks"`
	Expected      []expectedRecord `json:"expected"`
}

func snapshotBytes(v app.AssetView) ([]byte, error) {
	r := revisionRecord{Kind: string(v.Profile.Kind), CategoryID: v.Profile.Category, CategoryLabel: v.CategoryLabel, Name: v.Profile.Name, FleetNumber: v.Profile.FleetNumber, Plate: v.Profile.Plate, SerialNumber: v.Profile.SerialNumber, UsageBasis: string(v.Profile.UsageBasis), State: string(v.State), UnitID: v.UnitID, Tanks: []tankRecord{}, Expected: []expectedRecord{}}
	for _, t := range v.Profile.Tanks {
		x := t.Snapshot()
		r.Tanks = append(r.Tanks, tankRecord{string(x.Role), x.Capacity, string(x.Unit), x.Products})
	}
	for _, e := range v.Profile.Expected {
		x := e.Snapshot()
		r.Expected = append(r.Expected, expectedRecord{x.Product, string(x.Basis), x.Value, string(x.Unit), x.Source, x.Date})
	}
	return json.Marshal(r)
}

type ownedCatalog struct {
	fuel.Catalog
	categoryID string
}

func (c ownedCatalog) Category(code string) (fleet.CategoryDescriptor, bool) {
	return fleet.CategoryDescriptor{Code: c.categoryID}, code == c.categoryID
}
func assetRef(scope kernel.OrgScope, id string) (fleet.AssetRef, error) {
	parsed, err := fleet.ParseAssetID(id)
	if err != nil {
		return fleet.AssetRef{}, err
	}
	return fleet.NewAssetRef(scope, parsed)
}
func validated(scope kernel.OrgScope, id string, p fleet.Profile) (fleet.Snapshot, error) {
	ref, err := assetRef(scope, id)
	if err != nil {
		return fleet.Snapshot{}, err
	}
	asset, err := fleet.NewDraft(ref, p, ownedCatalog{categoryID: p.Category})
	if err != nil {
		return fleet.Snapshot{}, err
	}
	return asset.Snapshot(scope)
}
func loadAsset(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, id string) (app.AssetView, error) {
	queries := q.New(tx)
	org := scope.OrgID().String()
	row, err := queries.GetAsset(ctx, q.GetAssetParams{OrgID: org, ID: id})
	if err != nil {
		return app.AssetView{}, err
	}
	p := fleet.Profile{Kind: fleet.Kind(row.Kind), Category: row.CategoryID, Name: row.Name, FleetNumber: row.FleetNumber, Plate: row.Plate, SerialNumber: row.SerialNumber, UsageBasis: fleet.Basis(row.UsageBasis)}
	tanks, err := queries.AssetTanks(ctx, q.AssetTanksParams{OrgID: org, AssetID: id})
	if err != nil {
		return app.AssetView{}, err
	}
	for _, t := range tanks {
		products, err := queries.AssetTankProducts(ctx, q.AssetTankProductsParams{OrgID: org, AssetID: id, TankPosition: t.Position})
		if err != nil {
			return app.AssetView{}, err
		}
		value, err := fleet.NewTank(fuel.Catalog{}, fleet.TankRole(t.Role), strconv.FormatInt(t.Capacity, 10), products)
		if err != nil || value.Snapshot().Unit != fleet.Unit(t.Unit) {
			return app.AssetView{}, app.ErrUnavailable
		}
		p.Tanks = append(p.Tanks, value)
	}
	expected, err := queries.AssetExpected(ctx, q.AssetExpectedParams{OrgID: org, AssetID: id})
	if err != nil {
		return app.AssetView{}, err
	}
	for _, e := range expected {
		v, err := fleet.NewExpectedConsumption(fuel.Catalog{}, e.Product, fleet.Basis(e.Basis), strconv.FormatInt(e.Value, 10), e.Source, e.ReferenceDate)
		if err != nil || v.Snapshot().Unit != fleet.Unit(e.Unit) {
			return app.AssetView{}, app.ErrUnavailable
		}
		p.Expected = append(p.Expected, v)
	}
	checked, err := validated(scope, id, p)
	if err != nil || checked.FleetKey != row.FleetKey || checked.PlateKey != row.PlateKey {
		return app.AssetView{}, app.ErrUnavailable
	}
	state, err := fleet.ParseState(row.State)
	if err != nil || row.Revision < 1 {
		return app.AssetView{}, app.ErrUnavailable
	}
	view := app.AssetView{Ref: checked.Ref, Profile: checked.Profile, CategoryLabel: row.CategoryLabel, State: state, Revision: row.Revision, UnitID: row.UnitID}
	allocation, err := queries.AssetCurrentAllocation(ctx, q.AssetCurrentAllocationParams{OrgID: org, AssetID: id})
	if err == nil {
		view.Allocation = &app.UnitAllocation{UnitID: allocation.UnitID, StartedAt: allocation.StartedAt.Time}
	} else if err != pgx.ErrNoRows {
		return app.AssetView{}, err
	}
	if state == fleet.Active && (view.Allocation == nil || view.UnitID == "" || view.Allocation.UnitID != view.UnitID) {
		return app.AssetView{}, app.ErrUnavailable
	}
	return view, nil
}
func saveConfiguration(ctx context.Context, queries *q.Queries, org, id string, p fleet.Profile) error {
	if err := queries.DeleteAssetExpected(ctx, q.DeleteAssetExpectedParams{OrgID: org, AssetID: id}); err != nil {
		return err
	}
	if err := queries.DeleteAssetTanks(ctx, q.DeleteAssetTanksParams{OrgID: org, AssetID: id}); err != nil {
		return err
	}
	for pos, tank := range p.Tanks {
		t := tank.Snapshot()
		if err := queries.InsertAssetTank(ctx, q.InsertAssetTankParams{OrgID: org, AssetID: id, Position: int32(pos), Role: string(t.Role), Capacity: t.Capacity, Unit: string(t.Unit)}); err != nil {
			return err
		}
		for _, product := range t.Products {
			if err := queries.InsertAssetTankProduct(ctx, q.InsertAssetTankProductParams{OrgID: org, AssetID: id, TankPosition: int32(pos), Product: product, Unit: string(t.Unit)}); err != nil {
				return err
			}
		}
	}
	for position, reference := range p.Expected {
		e := reference.Snapshot()
		if err := queries.InsertAssetExpected(ctx, q.InsertAssetExpectedParams{OrgID: org, AssetID: id, Position: int32(position), Product: e.Product, Basis: string(e.Basis), Value: e.Value, Unit: string(e.Unit), Source: e.Source, ReferenceDate: e.Date}); err != nil {
			return err
		}
	}
	return nil
}
