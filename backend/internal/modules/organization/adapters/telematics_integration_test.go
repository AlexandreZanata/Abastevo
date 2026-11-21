//go:build integration

package adapters_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	cappg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	connpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/adapters"
	connect "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	"github.com/jackc/pgx/v5"
)

type telematicsCredentialFixture func(context.Context, kernel.OrgScope, domain.CredentialRef) ([]byte, error)

func (f telematicsCredentialFixture) SecretFor(ctx context.Context, scope kernel.OrgScope, ref domain.CredentialRef) ([]byte, error) {
	return f(ctx, scope, ref)
}

func TestTelematicsKeepsScopedQualityEvidenceOutsidePhoneTracks(t *testing.T) {
	x := trackingFixture(t)
	ctx := t.Context()
	org, asset := x.scope.OrgID().String(), x.vehicle.AssetID
	role := pgx.Identifier{x.f.RuntimeRole}.Sanitize()
	if _, err := x.f.Admin.Exec(ctx, "GRANT SELECT ON org_connector_approvals TO "+role+
		"; GRANT SELECT,INSERT,UPDATE ON org_connections TO "+role+
		"; GRANT SELECT,INSERT ON org_telematics_evidence TO "+role); err != nil {
		t.Fatal(err)
	}
	known, err := capability.NewCatalog([]string{"organizations", "integrations"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := audit.NewCatalog(append(connpg.AuditDefinitions(), cappg.AuditDefinitions()...))
	if err != nil {
		t.Fatal(err)
	}
	pool := x.f.RuntimePool(t, 4)
	audits := auditpg.NewStore(pool, catalog)
	flags := cappg.NewStore(pool, known, audits)
	store := connpg.Store{Pool: pool, Audit: audits,
		Authority: adapters.ConnectorAuthority{Memberships: x.legalFixtureState.store.Memberships, Roles: x.legalFixtureState.store.Roles, Capabilities: flags}}
	if _, err := flags.Set(ctx, x.scope, capability.Actor{AccountID: x.actor.AccountID}, capability.Change{
		OperationID: onboardingID(t), Capability: "integrations", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	approval, reference := onboardingID(t), onboardingID(t)
	if _, err := x.f.Admin.Exec(ctx, `INSERT INTO org_connector_approvals
(org_id,id,revision,kind,provider_code,resource_id,credential_ref,maximum_bytes,maximum_items,decision,decision_ref,ready_at)
VALUES($1,$2,1,'TELEMATICS','synthetic-provider',$3,$4,8192,3,'APPROVED','synthetic-reviewed-purpose',clock_timestamp())`,
		org, approval, asset, reference); err != nil {
		t.Fatal(err)
	}
	config := connect.Change{Action: connect.Configure, OperationID: onboardingID(t), ApprovalID: approval, ApprovalRevision: 1, Label: "Synthetic telemetry"}
	configured, err := store.Change(ctx, x.scope, x.actor, x.member, config)
	if err != nil {
		t.Fatal(err)
	}
	id := configured.Connection.ID
	if _, err := store.Change(ctx, x.scope, x.actor, x.member, connect.Change{
		Action: connect.Activate, OperationID: onboardingID(t), TargetID: id, ExpectedRevision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	key := []byte("synthetic approval test key for telematics only")
	source := telematicsCredentialFixture(func(_ context.Context, scope kernel.OrgScope, ref domain.CredentialRef) ([]byte, error) {
		if scope != x.scope || ref.Reference() != reference {
			return nil, domain.ErrDenied
		}
		return key, nil
	})
	service := connpg.Telematics{Access: connpg.Access{Pool: pool, Flags: flags}, Credentials: source, Tracking: x.tracking, Trips: x.tripState.store}
	value := map[string]any{"schema_version": 1, "batch_id": onboardingID(t), "trip_id": x.trip, "asset_id": asset,
		"observations": []map[string]any{{"observed_at": x.at(0).Add(time.Microsecond), "quality": "APPROXIMATE"},
			{"observed_at": x.at(0).Add(2 * time.Microsecond), "quality": "PRECISE"}}}
	body := func() ([]byte, string) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte("abastevo.telematics.v1\n" + org + "\n" + id + "\n"))
		mac.Write(raw)
		return raw, hex.EncodeToString(mac.Sum(nil))
	}
	proof, signature := body()
	if _, err := service.Accept(ctx, x.scope, id, proof, signature); !errors.Is(err, domain.ErrDenied) {
		t.Fatalf("provider could substitute for the driver's recording start: %v", err)
	}
	x.inForce(t)
	x.startTracking(t, x.syncFixtureState, onboardingID(t), 0, "PRECISE", 2)
	if _, err := service.Accept(ctx, x.scope, id, proof, hex.EncodeToString(make([]byte, 32))); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("forged source evidence was admitted", err)
	}
	value["latitude"] = 1.0
	invalid, badSignature := body()
	if _, err := service.Accept(ctx, x.scope, id, invalid, badSignature); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("unapproved coordinate field was admitted", err)
	}
	delete(value, "latitude")
	foreign := x.f.Organizations[1].Scope(t)
	if _, err := service.Accept(ctx, foreign, id, proof, signature); !errors.Is(err, domain.ErrDenied) {
		t.Fatal("foreign company could use the connector", err)
	}
	first, err := service.Accept(ctx, x.scope, id, proof, signature)
	if err != nil || first.Replayed || first.Provider != "synthetic-provider" || first.Precise != 1 || first.Approximate != 1 {
		t.Fatalf("verified provider quality was not recorded: %+v %v", first, err)
	}
	again, err := service.Accept(ctx, x.scope, id, proof, signature)
	if err != nil || !again.Replayed || !again.ReceivedAt.Equal(first.ReceivedAt) {
		t.Fatalf("duplicate segment did not converge: %+v %v", again, err)
	}
	value["observations"].([]map[string]any)[0]["quality"] = "INSUFFICIENT"
	changed, changedSignature := body()
	if _, err := service.Accept(ctx, x.scope, id, changed, changedSignature); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("same batch identity changed its accepted evidence", err)
	}
	var vendorRows, phoneRows int
	if err := x.f.Admin.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM org_telematics_evidence WHERE org_id=$1),
 (SELECT count(*) FROM org_tracking_segments WHERE org_id=$1)`, org).Scan(&vendorRows, &phoneRows); err != nil ||
		vendorRows != 1 || phoneRows != 0 {
		t.Fatalf("provider evidence altered device tracks: vendor=%d phone=%d err=%v", vendorRows, phoneRows, err)
	}
}
