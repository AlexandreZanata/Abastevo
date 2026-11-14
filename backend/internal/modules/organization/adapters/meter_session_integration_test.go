//go:build integration

package adapters_test

import (
	"errors"
	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"testing"
	"time"
)

func TestMeterWebCaptureSurvivesSessionCleanupAndActualReauthentication(t *testing.T) {
	fx := syncFixture(t)
	assets := fleetStore(t, fx.legalFixtureState)
	asset, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	store := meterStore(t, fx.legalFixtureState)
	actor := app.FleetActor{AccountID: fx.person.web.AccountID, SessionID: fx.person.web.SessionID}
	command := meterInstallCommand(t, meterInstallInput(t, asset.AssetID, time.Now().UTC().Add(-time.Hour)))
	original, replay, err := store.InstallMeter(t.Context(), fx.scope, actor, fx.person.member, onboardingID(t), onboardingID(t), command)
	if err != nil || replay {
		t.Fatal("actual manager WEB installation", err)
	}
	if _, err := fx.f.Runtime.Exec(t.Context(), `DELETE FROM company_web_sessions WHERE account_id=$1 AND id=$2`, actor.AccountID, actor.SessionID); err != nil {
		t.Fatal("ephemeral company session cleanup", err)
	}
	if _, err := store.ReadLatestMeter(t.Context(), fx.scope, actor, fx.person.member, asset.AssetID, original.MeterID); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatal("deleted session retained current authority", err)
	}
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	accounts.Clock = syncHTTPClock{}
	sessions := &company.Service{Enabled: true, Store: &companypg.Store{Pool: fx.f.Runtime}, Hasher: accountd.SHA256Hasher{}, Prove: companypg.AccountProof(&accounts), Now: time.Now, NewID: jobs.NewUUIDv4, NewSecret: accountd.GenerateToken}
	renewed, err := sessions.Login(t.Context(), fx.person.auth.Session.FamilyID, fx.person.auth.Session.AccessToken, "")
	if err != nil {
		t.Fatal("actual ordinary account proof/new company session", err)
	}
	next := app.FleetActor{AccountID: renewed.Row.AccountID, SessionID: renewed.Row.ID}
	if next.AccountID != actor.AccountID || next.SessionID == actor.SessionID {
		t.Fatal("fixture failed to obtain actual new source period")
	}
	again, replay, err := store.InstallMeter(t.Context(), fx.scope, next, fx.person.member, onboardingID(t), onboardingID(t), command)
	if err != nil || !replay || again != original {
		t.Fatal("new authenticated session changed original installation ACK", err)
	}
	view, err := store.ReadLatestMeter(t.Context(), fx.scope, next, fx.person.member, asset.AssetID, original.MeterID)
	if err != nil || view.Latest == nil {
		t.Fatal("historic reading lost with ephemeral session", err)
	}
	capture := view.Latest.CaptureSource()
	if capture.Kind() != meter.CaptureWebSession || capture.ID() != actor.SessionID || view.Latest.Snapshot().Author.ID() != fx.person.member {
		t.Fatal("historical capture was retagged to current proof")
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_meter_commands WHERE org_id=$1)=1 AND NOT EXISTS(SELECT 1 FROM company_web_sessions WHERE id=$2)`, fx.scope.OrgID().String(), actor.SessionID).Scan(&exact); err != nil || !exact {
		t.Fatal("session cleanup altered durable company evidence", err)
	}
}
