//go:build integration

package adapters_test

import (
	"encoding/json"
	"errors"
	fa "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
	"sync"
	"testing"
)

// The named SQL intake producer proves persisted-reference guarding only. It is
// not a production fleet-operation producer and is never installed in runtime.
func TestFleetUsageRequiresOwnedCommittedEntityProvenance(t *testing.T) {
	for _, token := range []string{"setting", fa.AssetEntityType} {
		t.Run(token, func(t *testing.T) {
			fx := syncFixture(t)
			s := fleetStore(t, fx.legalFixtureState)
			r, _, err := fleetWrite(t, fx.legalFixtureState, s, fleetCommand(t, fleetCreateInput()))
			if err != nil {
				t.Fatal(err)
			}
			fx.target = r.AssetID
			if err := fleetSQL(t, fx.legalFixtureState, `INSERT INTO org_sync_test_targets(org_id,id,owner_member,revision) VALUES($1,$2,$3,1)`, fx.scope.OrgID().String(), r.AssetID, fx.source.MembershipID); err != nil {
				t.Fatal(err)
			}
			original := syncOperation(t, fx, onboardingID(t), "Synthetic provenance-only effect", nil, nil, nil)
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(original.CanonicalBytes(), &raw); err != nil {
				t.Fatal(err)
			}
			delete(raw, "payload_digest")
			entity, err := json.Marshal(map[string]any{"type": token, "id": r.AssetID, "expected_revision": nil})
			if err != nil {
				t.Fatal(err)
			}
			raw["entity"] = entity
			body, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			binding, err := protocol.NewBinding(fx.source.Actor.AccountID, fx.source.Origin, fx.scope.OrgID().String())
			if err != nil {
				t.Fatal(err)
			}
			op, err := protocol.Seal(body, binding)
			if err != nil {
				t.Fatal(err)
			}
			ack, err := fx.store.Process(t.Context(), fx.source, op, protocol.DefaultLimits(), fx.handler)
			if err != nil || ack.Result() != protocol.Accepted {
				t.Fatal("actual committed synthetic source effect missing", err)
			}
			var use int64
			if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
				return tx.QueryRow(t.Context(), `SELECT (org_asset_current_usage($1,$2)->>'operations')::bigint`, fx.scope.OrgID().String(), r.AssetID).Scan(&use)
			}); err != nil {
				t.Fatal(err)
			}
			expected := int64(0)
			if token == fa.AssetEntityType {
				expected = 1
			}
			if use != expected {
				t.Fatal("UUID equality substituted for exact owned entity provenance")
			}
			if token == fa.AssetEntityType {
				change := fleetCreateInput()
				change.Kind = fa.ChangeAsset
				change.AssetID = r.AssetID
				change.ExpectedRevision = 1
				change.Profile.Kind = fleet.Machine
				if _, _, err := fleetWrite(t, fx.legalFixtureState, s, fleetCommand(t, change)); !errors.Is(err, fa.ErrConflict) {
					t.Fatal("accepted source did not lock kind", err)
				}
			}
			_, _, err = fleetWrite(t, fx.legalFixtureState, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.DiscardAsset, AssetID: r.AssetID, ExpectedRevision: 1}))
			if token == fa.AssetEntityType {
				if !errors.Is(err, fa.ErrConflict) {
					t.Fatal("used asset discarded", err)
				}
			} else if err != nil {
				t.Fatal("foreign entity namespace blocked unused draft", err)
			}
			if syncCount(t, fx, "org_sync_effects") != 1 {
				t.Fatal("asset operation rewrote unrelated immutable intake effect")
			}
		})
	}
}
func TestFleetAssetUnitUsagePreservesHomeTeamUnion(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	var unit string
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT id::text FROM org_units WHERE org_id=$1`, fx.scope.OrgID().String()).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	fleetPriorMemberAndTeam(t, fx, unit)
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	input := fleetCreateInput()
	input.UnitID = unit
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	if assets, employees := fleetUnitCounts(t, fx, unit); assets != 0 || employees != 1 {
		t.Fatal("provisional draft counted as admitted unit work")
	}
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: 1})); err != nil {
		t.Fatal(err)
	}
	if assets, employees := fleetUnitCounts(t, fx, unit); assets != 1 || employees != 1 {
		t.Fatal("asset source lost distinct home/team employee count")
	}
	change := fleetCreateInput()
	change.Kind = fa.ChangeAsset
	change.AssetID = created.AssetID
	change.ExpectedRevision = 2
	change.Profile.Name = "Synthetic active metadata correction"
	if _, _, err := fleetWrite(t, fx, s, fleetCommand(t, change)); err != nil {
		t.Fatal("omitted active unit did not preserve existing allocation", err)
	}
	view, err := s.ReadAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, created.AssetID)
	if err != nil || view.UnitID != unit || view.Allocation == nil || view.Allocation.UnitID != unit {
		t.Fatal("active metadata edit transferred unit", err)
	}
}
func TestFleetActivationAndUnitArchiveSerializeOnActualRoot(t *testing.T) {
	fx := legalFixture(t)
	s := fleetStore(t, fx)
	units := operatingUnitStore(t, fx)
	unit, _, err := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), operatingUnitCommand(t, org.UnitWriteInput{Kind: org.CreateUnit, Profile: d.UnitProfile{Name: "Synthetic activation race unit", Code: "RACE"}}))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := fx.onboarding.Repository.ActivateOrganization(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), 1); err != nil {
		t.Fatal(err)
	}
	input := fleetCreateInput()
	input.UnitID = unit.UnitID
	created, _, err := fleetWrite(t, fx, s, fleetCommand(t, input))
	if err != nil {
		t.Fatal(err)
	}
	activate := fleetCommand(t, fa.AssetWriteInput{Kind: fa.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: 1})
	archive := operatingUnitCommand(t, org.UnitWriteInput{Kind: org.ArchiveUnit, UnitID: unit.UnitID, ExpectedRevision: 1})
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		_, _, err := s.WriteAsset(t.Context(), fx.scope, fleetActor(fx), fx.member, "", activate)
		errs <- err
	})
	wg.Go(func() {
		<-start
		_, _, err := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, "", archive)
		errs <- err
	})
	close(start)
	wg.Wait()
	close(errs)
	success, refused := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, fa.ErrNotFound) || errors.Is(err, d.ErrStructureInUse) {
			refused++
		} else {
			t.Fatal("unexpected root-serialization result", err)
		}
	}
	if success != 1 || refused != 1 {
		t.Fatal("unit archive and active admission both committed")
	}
	var valid bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(a.state='ACTIVE' AND u.state='ACTIVE' AND(SELECT count(*) FROM org_asset_unit_allocations p WHERE p.org_id=a.org_id AND p.asset_id=a.id)=1) OR(a.state='DRAFT' AND u.state='ARCHIVED' AND NOT EXISTS(SELECT 1 FROM org_asset_unit_allocations p WHERE p.org_id=a.org_id AND p.asset_id=a.id))FROM org_assets a JOIN org_units u ON(u.org_id,u.id)=(a.org_id,a.unit_id)WHERE a.org_id=$1 AND a.id=$2`, fx.scope.OrgID().String(), created.AssetID).Scan(&valid); err != nil || !valid {
		t.Fatal("invalid committed asset/unit state", err)
	}
}
