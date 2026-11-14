//go:build integration

package adapters_test

import (
	"errors"
	"testing"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestMeterReviewCurrentCompanyAuthority(t *testing.T) {
	fx := legalFixture(t)
	guard := &orgpg.MeterReviewAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}
	actor := fleetActor(fx)
	check := func(a fleet.FleetActor, member string, write bool) (fleet.MeterReviewReviewer, error) {
		t.Helper()
		var result fleet.MeterReviewReviewer
		err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			var e error
			result, e = guard.CheckMeterReviewTx(t.Context(), tx, fx.scope, a, member, fx.scope.OrgID().String(), write)
			return e
		})
		return result, err
	}
	reviewer, e := check(actor, fx.member, false)
	if e != nil || reviewer.AccountID != actor.AccountID || reviewer.MembershipID != fx.member || reviewer.EmployeeID == "" {
		t.Fatal("current real company reviewer read unavailable", e)
	}
	if _, e := check(actor, fx.member, true); !errors.Is(e, access.ErrSetupIncomplete) {
		t.Fatal("pending setup permitted operational review", e)
	}
	if e := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); e != nil {
		t.Fatal(e)
	}
	if _, e := check(actor, fx.member, true); e != nil {
		t.Fatal("fresh active reviewer refused", e)
	}
	if _, e := check(actor, onboardingID(t), true); !errors.Is(e, access.ErrNoMembership) {
		t.Fatal("foreign member reached review", e)
	}
	wrong := actor
	wrong.SessionID = onboardingID(t)
	if _, e := check(wrong, fx.member, false); !errors.Is(e, fleet.ErrUnauthenticated) {
		t.Fatal("foreign company session reached review", e)
	}
	if _, e := fx.flags.Set(t.Context(), fx.scope, capability.Actor{AccountID: actor.AccountID}, capability.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1}); e != nil {
		t.Fatal(e)
	}
	if _, e := check(actor, fx.member, true); !errors.Is(e, access.ErrEntitlementRequired) {
		t.Fatal("disabled capability permitted resolution", e)
	}
	if _, e := check(actor, fx.member, false); e != nil {
		t.Fatal("disabled capability erased private review reads", e)
	}
}
