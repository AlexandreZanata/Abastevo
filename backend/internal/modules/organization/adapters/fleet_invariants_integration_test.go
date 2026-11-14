//go:build integration

package adapters_test

import (
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"math"
	"strconv"
	"testing"
)

func TestFleetStoreRoundTripsOrderedTanksAndExactReferences(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	input := fleetCreateInput()
	input.Profile.Kind = fleet.Vehicle
	input.Profile.UsageBasis = fleet.Distance
	for _, tc := range []struct {
		role     fleet.TankRole
		amount   string
		products []string
	}{{fleet.Main, "1000000000", []string{"DIESEL_S10", "DIESEL_S500"}}, {fleet.Auxiliary, "17", []string{"CNG"}}, {fleet.Battery, "99", []string{"ELECTRICITY"}}} {
		v, err := fleet.NewTank(fuel.Catalog{}, tc.role, tc.amount, tc.products)
		if err != nil {
			t.Fatal(err)
		}
		input.Profile.Tanks = append(input.Profile.Tanks, v)
	}
	for _, code := range []string{"ELECTRICITY", "DIESEL_S10", "CNG"} {
		r, err := fleet.NewExpectedConsumption(fuel.Catalog{}, code, fleet.Distance, strconv.FormatInt(math.MaxInt64, 10), "Synthetic explicit comparison source", "0001-01-01")
		if err != nil {
			t.Fatal(err)
		}
		input.Profile.Expected = append(input.Profile.Expected, r)
	}
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Profile.Tanks) != 3 || len(view.Profile.Expected) != 3 {
		t.Fatal("configuration lost normalized rows")
	}
	for i, v := range input.Profile.Tanks {
		want, got := v.Snapshot(), view.Profile.Tanks[i].Snapshot()
		if want.Role != got.Role || want.Unit != got.Unit || want.Capacity != got.Capacity || len(want.Products) != len(got.Products) {
			t.Fatal("tank identity/order/quantity changed")
		}
		for n := range want.Products {
			if want.Products[n] != got.Products[n] {
				t.Fatal("tank product changed")
			}
		}
	}
	for i, v := range input.Profile.Expected {
		want, got := v.Snapshot(), view.Profile.Expected[i].Snapshot()
		if want.Product != got.Product || want.Basis != got.Basis || want.Value != got.Value || want.Unit != got.Unit || want.Source != got.Source || want.Date != got.Date {
			t.Fatal("exact declared-reference order/content changed")
		}
	}
	var intact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT jsonb_array_length(snapshot->'tanks')=3 AND jsonb_array_length(snapshot->'expected')=3 AND snapshot->'expected'->0->>'product'='ELECTRICITY' AND(snapshot->'expected'->0->>'value')::bigint=9223372036854775807 FROM org_asset_revisions WHERE org_id=$1 AND asset_id=$2 AND revision=1`, fx.scope.OrgID().String(), created.AssetID).Scan(&intact); err != nil || !intact {
		t.Fatal("immutable revision redacted or narrowed private fields", err)
	}
}
func TestFleetSchemaRequiresActualInitialAllocationAtCommit(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	input := fleetCreateInput()
	input.UnitID = unit
	r, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	fleetSQLCode(t, fleetSQL(t, fx, `UPDATE org_assets SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, fx.scope.OrgID().String(), r.AssetID), "23514")
	activated, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: r.AssetID, ExpectedRevision: 1}))
	if err != nil || activated.State != fleet.Active {
		t.Fatal("atomic actual allocation denied", err)
	}
}
