//go:build integration

package adapters_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	billpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/adapters"
	billing "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestFleetPlanSerializesTheLastVehicleSlot(t *testing.T) {
	fx, assets, unit := lifecycleFixture(t)
	ctx := t.Context()
	assets.Pool = fx.f.RuntimePool(t, 3)
	meters := meterStore(t, fx)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(ctx, "GRANT SELECT ON enterprise_plan_catalog TO "+role+
		"; GRANT SELECT, INSERT ON org_billing_subscription_events TO "+role); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations"})
	if err != nil {
		t.Fatal(err)
	}
	key := billing.PlanKey{Product: "fleet", Plan: "synthetic", Revision: 1}
	const ceiling = 10
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO enterprise_plan_catalog
(product_code,plan_code,revision,display_name,capabilities,active_vehicles)
VALUES('fleet','synthetic',1,'Synthetic company',ARRAY['organizations'],$1)`, ceiling); err != nil {
		t.Fatal(err)
	}
	at := lifecycleDBNow(t, fx)
	opening := billing.LifecycleEvent{
		Scope: fx.scope, SubscriptionID: onboardingID(t), Kind: billing.StartTrial, Source: billing.TrialPolicy,
		At: at, Period: billing.Period{Start: at, End: at.Add(time.Hour)}, Reference: "synthetic-trial-policy",
	}
	proof := fx.f.PrepareSubscriptionOpening(t, key, opening)
	if err := database.WithOrgTx(ctx, assets.Pool, fx.scope, func(tx pgx.Tx) error {
		if _, err := (billpg.SubscriptionLedger{Capabilities: known}).ApplyTx(ctx, tx, key, opening); err != nil {
			return err
		}
		return proof(tx)
	}); err != nil {
		t.Fatal(err)
	}
	plans := billpg.EntitlementStore{Pool: assets.Pool, Capabilities: known, Now: time.Now}
	fx.flags.PlanWrites, assets.VehicleSlots = plans, plans

	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	var activate [2]fleetapp.AssetWrite
	prior := make([]fleetapp.AssetResult, 0, ceiling-1)
	for i := range ceiling + 1 {
		input := fleetCreateInput()
		input.Profile.Kind, input.Profile.UsageBasis = fleet.Vehicle, fleet.Distance
		input.Profile.FleetNumber, input.UnitID = fmt.Sprintf("TEST-VEHICLE-%d", i+1), unit
		input.Profile.Tanks = []fleet.Tank{tank}
		created, _, err := fleetWrite(t, fx, assets, fleetCommand(t, input))
		if err != nil {
			t.Fatal(err)
		}
		meter, _, err := meterInstall(t, fx, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, at.Add(-time.Hour))))
		if err != nil {
			t.Fatal(err)
		}
		command := fleetCommand(t, fleetapp.AssetWriteInput{
			Kind: fleetapp.ActivateAsset, AssetID: created.AssetID, ExpectedRevision: meter.AssetRevision,
		})
		if i >= ceiling-1 {
			activate[i-(ceiling-1)] = command
			continue
		}
		saved, _, err := fleetWrite(t, fx, assets, command)
		if err != nil {
			t.Fatal(err)
		}
		prior = append(prior, saved)
	}
	type result struct {
		index int
		value fleetapp.AssetResult
		err   error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for index, command := range activate {
		go func() {
			<-start
			value, _, err := assets.WriteAsset(ctx, fx.scope, fleetActor(fx), fx.member, "", command)
			results <- result{index, value, err}
		}()
	}
	close(start)
	winner, denied := result{index: -1}, result{index: -1}
	for range activate {
		outcome := <-results
		switch {
		case outcome.err == nil && winner.index == -1:
			winner = outcome
		case errors.Is(outcome.err, access.ErrLimitReached) && denied.index == -1:
			denied = outcome
		default:
			t.Fatalf("expected exactly one activation and one limit denial: %+v", outcome)
		}
	}
	if winner.index < 0 || denied.index < 0 {
		t.Fatal("the final plan slot did not converge")
	}
	var activeCount int
	if err := fx.f.Admin.QueryRow(ctx, `SELECT count(*) FROM org_assets WHERE org_id=$1 AND kind='VEHICLE' AND state='ACTIVE'`,
		fx.scope.OrgID().String()).Scan(&activeCount); err != nil || activeCount != ceiling {
		t.Fatalf("eleventh activation changed the covered fleet: active=%d err=%v", activeCount, err)
	}
	for _, saved := range prior {
		row, err := assets.ReadAsset(ctx, fx.scope, fleetActor(fx), fx.member, saved.AssetID)
		if err != nil || row.State != fleet.Active || row.Revision != saved.Revision {
			t.Fatalf("pre-existing vehicle changed while resolving the last slot: %v", err)
		}
	}
	if replay, repeated, err := fleetWrite(t, fx, assets, activate[winner.index]); err != nil || !repeated || replay != winner.value {
		t.Fatalf("activation replay consumed another slot: %+v %v", replay, err)
	}

	loser, err := activate[denied.index].Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	view, err := assets.ReadAsset(ctx, fx.scope, fleetActor(fx), fx.member, loser.AssetID)
	if err != nil || view.State != fleet.Draft || view.Revision != loser.ExpectedRevision {
		t.Fatalf("refused activation changed its draft: %+v %v", view, err)
	}
	deactivate := lifecycleInputFor(t, fx, fleet.Deactivate, winner.value.AssetID, winner.value.Revision)
	inactive, _, err := lifecycleWrite(t, fx, assets, lifecycleCommand(t, deactivate))
	if err != nil {
		t.Fatal(err)
	}
	if _, repeated, err := fleetWrite(t, fx, assets, activate[denied.index]); err != nil || repeated {
		t.Fatalf("freed slot did not allow the existing draft to activate: replay=%v %v", repeated, err)
	}
	reactivate := lifecycleInputFor(t, fx, fleet.Reactivate, inactive.AssetID, inactive.Revision)
	if _, _, err := lifecycleWrite(t, fx, assets, lifecycleCommand(t, reactivate)); !errors.Is(err, access.ErrLimitReached) {
		t.Fatalf("reactivation bypassed the approved ceiling: %v", err)
	}
}
