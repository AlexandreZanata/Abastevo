//go:build integration

package adapters_test

import (
	"testing"
	"time"

	accesspg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestSyncCurrentDeviceSourceUsesCallerTransactionAndExactEnrollment(t *testing.T) {
	fx := devicesFixture(t)
	first, e := fx.devices.EnrollDevice(t.Context(), fx.scope, fx.person.actor, deviceEnrollCommand(t, "Synthetic intake source"))
	if e != nil {
		t.Fatal(e)
	}
	s, e := first.Device.Enrollment.Snapshot(fx.scope)
	if e != nil {
		t.Fatal(e)
	}
	e = database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		v, at, e := fx.devices.CheckCurrentDeviceTx(t.Context(), tx, fx.scope, fx.person.actor, fx.person.member, s.ID)
		if e != nil || v.Validate(fx.scope) != nil || at.IsZero() || at.After(time.Now().UTC()) {
			t.Fatal("current bound source", e)
		}
		for _, ids := range [][2]string{{fx.member, s.ID}, {fx.person.member, onboardingID(t)}} {
			bad, at, e := fx.devices.CheckCurrentDeviceTx(t.Context(), tx, fx.scope, fx.person.actor, ids[0], ids[1])
			if e == nil || !at.IsZero() || bad.Validate(fx.scope) == nil {
				t.Fatal("foreign tuple admitted", e)
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}

func TestSyncAuthorityCanUseBorrowedCurrentTransactionWithoutOpeningAnother(t *testing.T) {
	fx := devicesFixture(t)
	roles := accesspg.NewStore(fx.f.Runtime)
	e := database.WithOrgTx(t.Context(), fx.f.Runtime, fx.scope, func(tx pgx.Tx) error {
		v, e := roles.LoadAuthorityTx(t.Context(), tx, fx.scope, fx.person.auth.Account.ID)
		if e != nil || v.MembershipID != fx.person.member || v.AccountID != fx.person.auth.Account.ID {
			t.Fatal("joined current authority", e)
		}
		if _, e := roles.LoadAuthorityTx(t.Context(), tx, fx.f.Organizations[1].Scope(t), fx.person.auth.Account.ID); e == nil {
			t.Fatal("foreign transaction accepted")
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
}
