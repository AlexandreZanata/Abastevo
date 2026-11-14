package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	members "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type meterNoProofQueries struct{}

func (meterNoProofQueries) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("invalid source reached SQL")
}
func (meterNoProofQueries) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("invalid source reached SQL")
}
func (meterNoProofQueries) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("invalid source reached SQL")
}

type meterNoProofCapabilities struct{}

func (meterNoProofCapabilities) GuardWrite(context.Context, pgx.Tx, kernel.OrgScope, string) error {
	panic("invalid source reached capabilities")
}
func TestMeterAuthorityRejectsWrongChannelAndBorrowedWebBeforeSQL(t *testing.T) {
	id, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000101")
	scope, _ := kernel.NewOrgScope(id)
	member, asset := "7b0e2a54-0000-4000-8000-000000000102", "7b0e2a54-0000-4000-8000-000000000103"
	web := fleet.MeterSource{Web: &fleet.FleetActor{AccountID: "7b0e2a54-0000-4000-8000-000000000104", SessionID: "7b0e2a54-0000-4000-8000-000000000105"}}
	guard := &MeterAuthority{Memberships: memberpg.NewStore(nil), Roles: accesspg.NewStore(nil), Capabilities: meterNoProofCapabilities{}}
	if err := guard.CheckMeterTx(t.Context(), meterNoProofQueries{}, scope, web, member, asset, fleet.MeterRecordAction); !errors.Is(err, fleet.ErrUnauthenticated) {
		t.Fatal("web source may not record signed operations", err)
	}
	for _, source := range []fleet.MeterSource{{}, {Web: &fleet.FleetActor{}}, {Web: web.Web, Device: &syncapp.Source{}}} {
		if err := guard.CheckMeterTx(t.Context(), meterNoProofQueries{}, scope, source, member, asset, fleet.MeterInstallAction); !errors.Is(err, fleet.ErrUnauthenticated) {
			t.Fatal("malformed source reached authority", err)
		}
	}
	for _, action := range []fleet.MeterAction{fleet.MeterInstallAction, fleet.MeterReadAction} {
		if err := guard.CheckMeterTx(t.Context(), meterNoProofQueries{}, scope, web, member, asset, action); !errors.Is(err, fleet.ErrUnavailable) {
			t.Fatal("borrowed web query object impersonated current company transaction", err)
		}
	}
	device := fleet.MeterSource{Device: &syncapp.Source{Scope: scope, Origin: "https://company.example.invalid", MembershipID: member, EnrollmentID: "7b0e2a54-0000-4000-8000-000000000106", Actor: members.DeviceActor{AccountID: web.Web.AccountID, FamilyID: "7b0e2a54-0000-4000-8000-000000000107", ContributorID: "7b0e2a54-0000-4000-8000-000000000108", KeyID: "7b0e2a54-0000-4000-8000-000000000109", AccessToken: "synthetic-proof", Fingerprint: "fp:" + strings.Repeat("a", 64), ProofExpires: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}}}
	if device.Validate(scope) != nil {
		t.Fatal("invalid named source-shape fixture")
	}
	for _, action := range []fleet.MeterAction{fleet.MeterInstallAction, fleet.MeterRecordAction, fleet.MeterReadAction, fleet.MeterAction(255)} {
		t.Run("device action "+string(rune('0'+action)), func(t *testing.T) {
			want := fleet.ErrUnavailable
			if action == fleet.MeterInstallAction {
				want = fleet.ErrUnauthenticated
			}
			if err := guard.CheckMeterTx(t.Context(), meterNoProofQueries{}, scope, device, member, asset, action); !errors.Is(err, want) {
				t.Fatal("device action/source boundary changed", err)
			}
		})
	}
	for _, source := range []fleet.MeterSource{web, device} {
		if err := guard.CheckMeterTx(t.Context(), meterNoProofQueries{}, scope, source, member, asset, fleet.MeterAction(255)); !errors.Is(err, fleet.ErrUnavailable) {
			t.Fatal("unknown source action accepted")
		}
	}
	if err := guard.CheckMeterTx(t.Context(), meterNoProofQueries{}, scope, device, "7b0e2a54-0000-4000-8000-000000000110", asset, fleet.MeterReadAction); !errors.Is(err, fleet.ErrUnauthenticated) {
		t.Fatal("foreign selected member reached source SQL")
	}
}
