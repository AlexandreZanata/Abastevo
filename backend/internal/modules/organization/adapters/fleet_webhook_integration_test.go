//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	fleetapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	fuel "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fueling/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	connectorpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/adapters"
	connector "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type fleetWebhookReceiver func(string, domain.CredentialRef, string, []byte) connectorpg.WebhookResult

func (f fleetWebhookReceiver) SendWebhook(_ context.Context, destination string, credential domain.CredentialRef, id string, body []byte) connectorpg.WebhookResult {
	return f(destination, credential, id, body)
}

func TestReviewedFleetWebhookDeliveryIsScopedAndBounded(t *testing.T) {
	t.Setenv("ANPFUEL_TEST_RUNTIME_GRANTS", "script")
	fx, assets, unit := lifecycleFixture(t)
	ctx := t.Context()
	org := fx.scope.OrgID().String()
	pool := fx.f.Runtime
	assets.LifecycleJobs = connectorpg.FleetWebhooks{}
	meters := meterStore(t, fx)
	tank, err := fleet.NewTank(fuel.Catalog{}, fleet.Main, "100000", []string{"DIESEL_S10"})
	if err != nil {
		t.Fatal(err)
	}
	newVehicle := func(label string) fleetapp.AssetResult {
		t.Helper()
		input := fleetCreateInput()
		input.Profile.Kind, input.Profile.UsageBasis = fleet.Vehicle, fleet.Distance
		input.Profile.FleetNumber, input.UnitID, input.Profile.Tanks = label, unit, []fleet.Tank{tank}
		created, _, err := fleetWrite(t, fx, assets, fleetCommand(t, input))
		if err != nil {
			t.Fatal(err)
		}
		installed, _, err := meterInstall(t, fx, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, lifecycleDBNow(t, fx).Add(-time.Hour))))
		if err != nil {
			t.Fatal(err)
		}
		active, _, err := fleetWrite(t, fx, assets, fleetCommand(t, fleetapp.AssetWriteInput{Kind: fleetapp.ActivateAsset,
			AssetID: created.AssetID, ExpectedRevision: installed.AssetRevision}))
		if err != nil {
			t.Fatal(err)
		}
		return active
	}
	vehicle, other := newVehicle("WEBHOOK-ONE"), newVehicle("WEBHOOK-TWO")
	known, err := capability.NewCatalog(enterprisesuite.CapabilityCodes())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(append(connectorpg.AuditDefinitions(), cappg.AuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	audits := auditpg.NewStore(pool, catalog)
	flags := cappg.NewStore(pool, known, audits)
	if _, err := flags.Set(ctx, fx.scope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{
		OperationID: onboardingID(t), Capability: "integrations", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	store := connectorpg.Store{Pool: pool, Audit: audits,
		Authority: adapters.ConnectorAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: flags}}
	approval, credential, connection := onboardingID(t), onboardingID(t), onboardingID(t)
	const endpoint = "https://synthetic-receiver.example/v1/fleet"
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_connector_approvals
(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,maximum_items,decision,decision_ref,ready_at)
VALUES($1,$2,1,'OUTBOUND_WEBHOOK','synthetic-webhook',$3,$4,1024,1,'APPROVED','synthetic-v1',clock_timestamp())`, org, approval, vehicle.AssetID, credential); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Change(ctx, fx.scope, fx.actor, fx.member, connector.Change{Action: connector.Configure,
		OperationID: connection, ApprovalID: approval, ApprovalRevision: 1, Label: "Synthetic endpoint"}); err != nil {
		t.Fatal(err)
	}
	activate := connector.Change{Action: connector.Activate, OperationID: onboardingID(t), TargetID: connection, ExpectedRevision: 1}
	if _, err := store.Change(ctx, fx.scope, fx.actor, fx.member, activate); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("missing reviewed endpoint: %v", err)
	}
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_webhook_endpoints (org_id,approval_id,approval_revision,endpoint) VALUES($1,$2,1,$3)`,
		org, approval, endpoint); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Change(ctx, fx.scope, fx.actor, fx.member, activate); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lifecycleWrite(t, fx, assets, lifecycleCommand(t, lifecycleInputFor(t, fx, fleet.Deactivate, other.AssetID, other.Revision))); err != nil {
		t.Fatal(err)
	}
	transition := func(command fleet.LifecycleCommand, before fleetapp.AssetResult) (fleetapp.AssetResult, string) {
		t.Helper()
		input := lifecycleInputFor(t, fx, command, before.AssetID, before.Revision)
		input.OperationID = onboardingID(t)
		signed := lifecycleCommand(t, input)
		after, replay, err := lifecycleWrite(t, fx, assets, signed)
		if err != nil || replay {
			t.Fatalf("vehicle transition: %+v, %v", after, err)
		}
		again, replay, err := lifecycleWrite(t, fx, assets, signed)
		if err != nil || !replay || after != again {
			t.Fatalf("vehicle transition replay: %+v, %v", again, err)
		}
		return after, input.OperationID
	}
	vehicle, first := transition(fleet.Deactivate, vehicle)
	var calls int
	received := map[string][]byte{}
	retryAll := false
	receiver := fleetWebhookReceiver(func(target string, ref domain.CredentialRef, id string, body []byte) connectorpg.WebhookResult {
		calls++
		var fields map[string]any
		if err := json.Unmarshal(body, &fields); err != nil || len(fields) != 5 || fields["delivery_id"] != id ||
			fields["vehicle_id"] != vehicle.AssetID || fields["version"] != float64(1) || target != endpoint || ref.Reference() != credential ||
			bytes.Contains(body, []byte(fx.actor.AccountID)) || bytes.Contains(body, []byte(credential)) || bytes.Contains(body, []byte("Synthetic seasonal")) {
			t.Fatalf("unexpected outbound data: %v", fields)
		}
		if _, err := time.Parse(time.RFC3339Nano, fields["occurred_at"].(string)); err != nil {
			t.Fatal(err)
		}
		if prior := received[id]; prior != nil && !bytes.Equal(prior, body) {
			t.Fatal("retried payload changed")
		}
		received[id] = append([]byte(nil), body...)
		if calls == 1 || retryAll {
			return connectorpg.WebhookRetry
		}
		return connectorpg.WebhookDelivered
	})
	worker, err := enterprisesuite.FleetWebhookHandler(enterprisesuite.Options{Enabled: true, BillingEnabled: true, Pool: pool}, receiver)
	if err != nil {
		t.Fatal(err)
	}
	queue := jobs.NewQueue(pool)
	job, err := queue.Claim(ctx, worker.Kind(), "synthetic-fixture", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	foreignJob := job
	foreignJob.Payload = bytes.ReplaceAll(job.Payload, []byte(org), []byte(fx.f.Organizations[1].ID))
	if err := worker.Handle(ctx, foreignJob); err == nil || calls != 0 {
		t.Fatal("a foreign job reached the receiver")
	}
	if err := worker.Handle(ctx, job); err == nil || calls != 1 {
		t.Fatal("the first failed delivery did not request a retry")
	}
	if err := worker.Handle(ctx, job); err == nil || calls != 1 {
		t.Fatal("the same lease was used for a second delivery")
	}
	if _, err := queue.Fail(ctx, job.ID, job.LeaseToken, 0, "synthetic retry"); err != nil {
		t.Fatal(err)
	}
	dispatcher := jobs.Dispatcher{Queue: queue, Kind: worker.Kind(), Handlers: map[string]jobs.Handler{worker.Kind(): worker},
		WorkerID: "synthetic-fixture", RetryDelay: time.Nanosecond}
	run := func() {
		t.Helper()
		if found, err := dispatcher.RunOnce(ctx); err != nil || !found {
			t.Fatalf("dispatcher found=%v: %v", found, err)
		}
	}
	check := func(operation, outcome string, attempts int) {
		t.Helper()
		var actual string
		var count int
		if err := fx.f.Admin.QueryRow(ctx, `SELECT outcome,attempts FROM org_webhook_deliveries WHERE org_id=$1 AND operation_id=$2`,
			org, operation).Scan(&actual, &count); err != nil || outcome != actual || attempts != count {
			t.Fatalf("outcome=%s attempts=%d: %v", actual, count, err)
		}
	}
	run()
	check(first, "DELIVERED", 2)
	if calls != 2 || len(received) != 1 {
		t.Fatal("retry did not retain one delivery identity")
	}
	retryAll = true
	vehicle, next := transition(fleet.Reactivate, vehicle)
	for range 3 {
		run()
	}
	check(next, "FAILED", 3)
	vehicle, pending := transition(fleet.Deactivate, vehicle)
	if _, err := fx.f.Admin.Exec(ctx, `INSERT INTO org_connector_approvals
(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,maximum_items,decision,decision_ref)
VALUES($1,$2,2,'OUTBOUND_WEBHOOK','synthetic-webhook',$3,$4,1024,1,'WITHDRAWN','synthetic-withdrawal')`, org, approval, vehicle.AssetID, credential); err != nil {
		t.Fatal(err)
	}
	run()
	check(pending, "SUPPRESSED", 1)
	if calls != 5 || len(received) != 2 {
		t.Fatal("withdrawal or bound failed")
	}
	if found, err := dispatcher.RunOnce(ctx); err != nil || found {
		t.Fatalf("replayed or unreviewed transitions queued delivery: found=%v err=%v", found, err)
	}
}
