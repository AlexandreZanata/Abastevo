//go:build integration

package adapters_test

import (
	"context"
	"sync"
	"testing"
	"time"

	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	employee "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

// holdShared keeps the account and root rows locked FOR SHARE, as an in-flight
// read does, until the returned release runs.
func holdShared(t *testing.T, fx legalFixtureState) func() {
	t.Helper()
	tx, err := fx.f.Runtime.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SELECT set_config('anpfuel.org_id',$1,true)`, fx.scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SELECT 1 FROM accounts WHERE id=$1 FOR SHARE`, fx.actor.AccountID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SELECT 1 FROM organizations WHERE org_id=$1 FOR SHARE`, fx.scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	return func() { _ = tx.Rollback(context.Background()) }
}

// Reads share the account and root rows: they run while another read holds
// them, writers still wait for readers, and settings reads, which may
// initialize a version under the root, stay exclusive without deadlocking.
func TestCompanyReadsShareAccountAndRootLocks(t *testing.T) {
	fx, units := operatingUnitFixture(t)
	grantEmployeeTables(t, fx)
	grantCostCentreTables(t, fx)
	employees := employeeStore(t, fx)
	costs, teams, settings := costCentreStore(t, fx), teamStore(t, fx), settingsStore(t, fx)
	release := holdShared(t, fx)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	reads := map[string]func() error{
		"units": func() error {
			_, e := units.ListUnits(ctx, fx.scope, fx.actor, fx.member, app.UnitFilter{Limit: 10})
			return e
		},
		"legal": func() error {
			_, e := fx.store.ListLegal(ctx, fx.scope, fx.actor, fx.member, app.LegalFilter{Limit: 10})
			return e
		},
		"costs": func() error {
			_, e := costs.ListCostCentres(ctx, fx.scope, fx.actor, fx.member, app.CostCentreFilter{Limit: 10})
			return e
		},
		"teams": func() error {
			_, e := teams.ListTeams(ctx, fx.scope, fx.actor, fx.member, app.TeamFilter{Limit: 10})
			return e
		},
		"employees": func() error {
			_, e := employees.ListEmployees(ctx, fx.scope, employeeActor(fx), fx.member, employee.EmployeeFilter{Limit: 10})
			return e
		},
	}
	var wg sync.WaitGroup
	for name, read := range reads {
		for range 2 {
			wg.Go(func() {
				if err := read(); err != nil {
					t.Errorf("%s read waited for another read: %v", name, err)
				}
			})
		}
	}
	wg.Wait()
	cancel()
	blocked, stop := context.WithTimeout(t.Context(), 700*time.Millisecond)
	defer stop()
	write := operatingUnitCommand(t, app.UnitWriteInput{Kind: app.CreateUnit, Profile: d.UnitProfile{Name: "Synthetic shared-lock unit", Code: "SHR01"}})
	if _, _, err := units.WriteUnit(blocked, fx.scope, fx.actor, fx.member, onboardingID(t), write); err == nil {
		t.Fatal("a structure write did not wait for an in-flight read")
	}
	if _, err := settings.ReadSettings(blocked, fx.scope, fx.actor, fx.member, 0); err == nil {
		t.Fatal("a settings read did not take the exclusive root lock")
	}
	release()
	var settled sync.WaitGroup
	for range 6 {
		settled.Go(func() {
			if _, err := settings.ReadSettings(t.Context(), fx.scope, fx.actor, fx.member, 0); err != nil {
				t.Error("concurrent settings read failed", err)
			}
		})
	}
	settled.Wait()
	if _, _, err := units.WriteUnit(t.Context(), fx.scope, fx.actor, fx.member, onboardingID(t), write); err != nil {
		t.Fatal("write after release failed", err)
	}
	var shared int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT count(*) FROM org_units WHERE org_id=$1 AND code='SHR01'`, fx.scope.OrgID().String()).Scan(&shared); err != nil || shared != 1 {
		t.Fatal("released write did not persist once", shared, err)
	}
}

func employeeStore(t *testing.T, fx legalFixtureState) *memberpg.EmployeeStore {
	t.Helper()
	catalog, e := audit.NewCatalog(memberpg.EmployeeAuditDefinitions())
	if e != nil {
		t.Fatal(e)
	}
	return &memberpg.EmployeeStore{Pool: fx.f.Runtime, Authority: &adapters.EmployeeAuthority{Memberships: fx.store.Memberships, Roles: fx.store.Roles, Capabilities: fx.flags}, Audit: auditpg.NewStore(fx.f.Runtime, catalog)}
}
