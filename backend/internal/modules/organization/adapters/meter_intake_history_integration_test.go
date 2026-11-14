//go:build integration

package adapters_test

import (
	"errors"
	"testing"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	permission "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func meterHistoricalSource(t *testing.T, fx deviceFixtureState, person devicePerson) syncapp.Source {
	t.Helper()
	enrolled, err := fx.devices.EnrollDevice(t.Context(), fx.scope, person.actor, deviceEnrollCommand(t, "Synthetic retained history source"))
	if err != nil {
		t.Fatal("actual device enrollment unavailable", err)
	}
	state, err := enrolled.Device.Enrollment.Snapshot(fx.scope)
	if err != nil {
		t.Fatal(err)
	}
	return syncapp.Source{Scope: fx.scope, Origin: deviceTestOrigin, Actor: person.actor, MembershipID: person.member, EnrollmentID: state.ID}
}

// The general device fixture starts with DRIVER/SELF. A never-ORG-authorized
// case must be provisioned as FINANCE from its first retained period, not erase
// the old role or reinterpret unsupported SELF history as proof of absence.
func meterHistoryFinancePerson(t *testing.T, fx deviceFixtureState) devicePerson {
	t.Helper()
	key, err := fx.accounts.CreateKeyAccount(t.Context(), "synhistory"+onboardingID(t)[:8])
	if err != nil {
		t.Fatal(err)
	}
	login, err := fx.accounts.LoginWithKey(t.Context(), key.Key)
	if err != nil {
		t.Fatal(err)
	}
	p := devicePerson{auth: login, employee: onboardingID(t), member: onboardingID(t)}
	org := fx.scope.OrgID().String()
	for _, command := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO org_employee_identities(org_id,id,state)VALUES($1,$2,'ACTIVE')`, []any{org, p.employee}},
		{`INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state)VALUES($1,$2,$3,$4,'EXISTING_ACCOUNT','INVITED')`, []any{org, p.member, p.employee, login.Account.ID}},
		{`INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'FINANCE','ORG',$1,clock_timestamp(),$4,'Synthetic first retained finance period')`, []any{org, onboardingID(t), p.member, fx.member}},
		{`UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, []any{org, p.member}},
	} {
		if err := unitSchemaExec(t, fx.legalFixtureState, command.sql, command.args...); err != nil {
			t.Fatal("actual finance history fixture failed", err)
		}
	}
	p.signer, p.actor = deviceAddSigner(t, fx, p)
	return p
}

func TestMeterIntakeHistoryUsesActualRetainedOrgPeriods(t *testing.T) {
	fx := devicesFixture(t)
	matrixAssign(t, fx, matrixPerson{member: fx.person.member}, "ADMIN", "ORG", "ACTIVE")
	source := meterHistoricalSource(t, fx, fx.person)
	assets := fleetStore(t, fx.legalFixtureState)
	created, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	meters := meterStore(t, fx.legalFixtureState)
	installed, _, err := meterInstall(t, fx.legalFixtureState, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	authority := &orgpg.MeterAuthority{Memberships: fx.store.Memberships, Roles: accesspg.NewStore(fx.f.Runtime), Capabilities: fx.flags, Devices: fx.devices}
	var declared time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT clock_timestamp()`).Scan(&declared); err != nil {
		t.Fatal(err)
	}
	value, err := meter.NewValue(meter.Odometer, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	command, err := fleet.NewMeterReadingCommand(fleet.MeterReadingInput{OperationID: onboardingID(t), AssetID: created.AssetID, MeterID: installed.MeterID, SegmentID: installed.SegmentID, ReadingID: onboardingID(t), Value: value, Declared: orgtime.Declaration{OccurredAt: declared, EntryZone: "UTC"}, Origin: meter.Standalone, Method: meter.Typed})
	if err != nil {
		t.Fatal(err)
	}
	classify := func(source syncapp.Source, want fleet.MeterDisposition, wantErr error) {
		t.Helper()
		var actual fleet.MeterDisposition
		err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
			var err error
			actual, err = authority.ClassifyMeterTx(t.Context(), syncapp.Borrow(tx), fx.scope, fleet.MeterSource{Device: &source}, source.MembershipID, command)
			return err
		})
		if actual != want || !errors.Is(err, wantErr) {
			t.Fatal("actual retained source history did not classify correctly", actual, err)
		}
	}
	classify(source, fleet.MeterApply, nil)
	matrixAssign(t, fx, matrixPerson{member: fx.person.member}, "FINANCE", "ORG", "ACTIVE")
	classify(source, fleet.MeterHoldPermissionChanged, nil)
	matrixAssign(t, fx, matrixPerson{member: fx.person.member}, "ADMIN", "ORG", "ACTIVE")
	classify(source, fleet.MeterHoldPermissionChanged, nil)
	never := meterHistoryFinancePerson(t, fx)
	neverSource := meterHistoricalSource(t, fx, never)
	classify(neverSource, "", permission.ErrPermissionDenied)
	if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason)VALUES($1,$2,$3,'ADMIN','ORG',$1,clock_timestamp()+interval '1 minute',$4,'Synthetic future grant cannot authorize intake')`, fx.scope.OrgID().String(), onboardingID(t), never.member, fx.member); err != nil {
		t.Fatal(err)
	}
	classify(neverSource, "", permission.ErrPermissionDenied)
	self := deviceNewPerson(t, fx)
	matrixAssign(t, fx, matrixPerson{member: self.member}, "DRIVER", "SELF", "ACTIVE")
	classify(meterHistoricalSource(t, fx, self), "", fleet.ErrUnavailable)
}

func TestRecordedRolePeriodsPreservesEndedRowsAndValidatesBorrowedScope(t *testing.T) {
	fx := devicesFixture(t)
	matrixAssign(t, fx, matrixPerson{member: fx.person.member}, "ADMIN", "ORG", "ACTIVE")
	matrixAssign(t, fx, matrixPerson{member: fx.person.member}, "FINANCE", "ORG", "ACTIVE")
	repository := accesspg.NewStore(fx.f.Runtime)
	if err := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		periods, err := repository.RecordedPeriodsDBTX(t.Context(), syncapp.Borrow(tx), fx.scope, fx.person.member)
		if err != nil {
			return err
		}
		ended := false
		for _, period := range periods {
			value, err := period.Assignment.Snapshot(fx.scope)
			if err != nil {
				return err
			}
			if period.CreatedAt.IsZero() {
				t.Fatal("history omitted recorded server creation")
			}
			if value.Role == permission.Admin && !value.EndedAt.IsZero() {
				ended = true
			}
		}
		if !ended {
			t.Fatal("history reused current candidates and lost the ended period")
		}
		other := scopeFor(t, onboardingID(t))
		if _, err := repository.RecordedPeriodsDBTX(t.Context(), syncapp.Borrow(tx), other, fx.person.member); err == nil {
			t.Fatal("borrowed query crossed real SQL scope")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
