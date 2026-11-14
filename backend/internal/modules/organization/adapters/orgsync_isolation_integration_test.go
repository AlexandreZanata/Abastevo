//go:build integration

package adapters_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

func TestSyncIntakePopulatedTwoOrganizationIsolationAndReceiptOwnership(t *testing.T) {
	a := syncFixture(t)
	other := legalFixtureFor(t, a.f, a.onboarding, a.actor, a.flags)
	employee, member := teamDriver(t, other, a.person.auth.Account.ID)
	if _, _, e := other.onboarding.Repository.ActivateOrganization(t.Context(), other.scope, other.actor, other.member, onboardingID(t), 1); e != nil {
		t.Fatal(e)
	}
	b := a
	b.deviceFixtureState.legalFixtureState = other
	b.deviceFixtureState.person.employee = employee
	b.deviceFixtureState.person.member = member
	matrixAssign(t, b.deviceFixtureState, matrixPerson{member: member}, "ADMIN", "ORG", "ACTIVE")
	actorB := deviceVerifyActor(t, b.deviceFixtureState, b.person, b.person.signer, time.Time{})
	enrollmentB, e := a.devices.EnrollDevice(t.Context(), other.scope, actorB, deviceEnrollCommand(t, "Synthetic isolated second organization"))
	if e != nil {
		t.Fatal(e)
	}
	registrationB, e := enrollmentB.Device.Enrollment.Snapshot(other.scope)
	if e != nil {
		t.Fatal(e)
	}
	b.source = syncapp.Source{Scope: other.scope, Origin: a.source.Origin, Actor: actorB, MembershipID: member, EnrollmentID: registrationB.ID}
	// Deliberate collisions include both the operation ID and owning target ID.
	if e := unitSchemaExec(t, other, `INSERT INTO org_sync_test_targets VALUES($1,$2,$3,1)`, other.scope.OrgID().String(), b.target, member); e != nil {
		t.Fatal(e)
	}
	id := onboardingID(t)
	opA := syncOperation(t, a, id, "Synthetic organization A content", nil, nil, nil)
	opB := syncOperation(t, b, id, "Synthetic organization B content", nil, nil, nil)
	ackA, e := a.store.Process(t.Context(), a.source, opA, protocol.DefaultLimits(), a.handler)
	if e != nil || ackA.Result() != protocol.Accepted {
		t.Fatal(e)
	}
	ackB, e := b.store.Process(t.Context(), b.source, opB, protocol.DefaultLimits(), b.handler)
	if e != nil || ackB.Result() != protocol.Accepted {
		t.Fatal(e)
	}
	if bytes.Equal(ackA.CanonicalBytes(), ackB.CanonicalBytes()) {
		t.Fatal("organization-specific immutable digests were conflated")
	}
	for _, table := range []string{"org_sync_operations", "org_sync_acknowledgements", "org_sync_effects"} {
		for _, pair := range []struct{ own, foreign syncFixtureState }{{a, b}, {b, a}} {
			e := database.WithOrgTx(t.Context(), a.f.Runtime, pair.own.scope, func(tx pgx.Tx) error {
				var own, foreign int
				if e := tx.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE org_id=$1),count(*) FILTER(WHERE org_id=$2) FROM `+table, pair.own.scope.OrgID().String(), pair.foreign.scope.OrgID().String()).Scan(&own, &foreign); e != nil {
					return e
				}
				if own != 1 || foreign != 0 {
					t.Fatal("populated ordinary RLS crossed organization", table, own, foreign)
				}
				return nil
			})
			if e != nil {
				t.Fatal(e)
			}
		}
		var unscoped int
		if e := a.f.Runtime.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&unscoped); e != nil || unscoped != 0 {
			t.Fatal("bare connection exposed private intake", table, e)
		}
	}
	onlyA := syncOperation(t, a, onboardingID(t), "Synthetic A-only actual effect", nil, nil, nil)
	if _, e := a.store.Process(t.Context(), a.source, onlyA, protocol.DefaultLimits(), a.handler); e != nil {
		t.Fatal(e)
	}
	foreignDependency := syncOperation(t, b, onboardingID(t), "Synthetic unavailable foreign dependency", nil, []string{onlyA.ID()}, nil)
	missingDependency := syncOperation(t, b, onboardingID(t), "Synthetic absent dependency", nil, []string{onboardingID(t)}, nil)
	foreignAck, e := b.store.Process(t.Context(), b.source, foreignDependency, protocol.DefaultLimits(), b.handler)
	if e != nil {
		t.Fatal(e)
	}
	missingAck, e := b.store.Process(t.Context(), b.source, missingDependency, protocol.DefaultLimits(), b.handler)
	if e != nil {
		t.Fatal(e)
	}
	if foreignAck.Result() != protocol.Retry || foreignAck.Code() != "DEPENDENCY_PENDING" || foreignAck.Result() != missingAck.Result() || foreignAck.Code() != missingAck.Code() || foreignAck.IsFinal() || missingAck.IsFinal() {
		t.Fatal("foreign dependency revealed readiness")
	}
	snapshot := func() string {
		t.Helper()
		var state string
		e := a.f.Admin.QueryRow(t.Context(), `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_operations x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_acknowledgements x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_effects x),(SELECT jsonb_agg(to_jsonb(x)ORDER BY org_id,operation_id)FROM org_sync_test_records x))::text`).Scan(&state)
		if e != nil {
			t.Fatal(e)
		}
		return state
	}
	before := snapshot()
	_, secondKey := deviceAddSigner(t, a.deviceFixtureState, a.person)
	secondRegistration, e := a.devices.EnrollDevice(t.Context(), a.scope, secondKey, deviceEnrollCommand(t, "Synthetic another enrollment same member"))
	if e != nil {
		t.Fatal(e)
	}
	secondBinding, e := secondRegistration.Device.Enrollment.Snapshot(a.scope)
	if e != nil {
		t.Fatal(e)
	}
	second := a
	second.source.Actor = secondKey
	second.source.EnrollmentID = secondBinding.ID
	resealed := syncOperation(t, second, id, "Synthetic attempted receipt adoption", nil, nil, nil)
	if got, e := a.store.Process(t.Context(), second.source, resealed, protocol.DefaultLimits(), a.handler); !errors.Is(e, syncapp.ErrSourceDenied) || len(got.CanonicalBytes()) != 0 {
		t.Fatal("another actual enrollment adopted the original receipt", e)
	}
	otherPerson := deviceNewPerson(t, a.deviceFixtureState)
	matrixAssign(t, a.deviceFixtureState, matrixPerson{member: otherPerson.member}, "ADMIN", "ORG", "ACTIVE")
	otherRegistration, e := a.devices.EnrollDevice(t.Context(), a.scope, otherPerson.actor, deviceEnrollCommand(t, "Synthetic another receipt author"))
	if e != nil {
		t.Fatal(e)
	}
	otherBinding, e := otherRegistration.Device.Enrollment.Snapshot(a.scope)
	if e != nil {
		t.Fatal(e)
	}
	another := a
	another.source = syncapp.Source{Scope: a.scope, Origin: a.source.Origin, Actor: otherPerson.actor, MembershipID: otherPerson.member, EnrollmentID: otherBinding.ID}
	otherIntent := syncOperation(t, another, id, "Synthetic foreign author attempt", nil, nil, nil)
	if got, e := a.store.Process(t.Context(), another.source, otherIntent, protocol.DefaultLimits(), a.handler); !errors.Is(e, syncapp.ErrSourceDenied) || len(got.CanonicalBytes()) != 0 {
		t.Fatal("another actual author learned the receipt", e)
	}
	if snapshot() != before {
		t.Fatal("foreign receipt attempts changed retained intent/effect/ACK bytes")
	}
	// A still-current original device proof alone does not confer current read scope.
	matrixAssign(t, a.deviceFixtureState, matrixPerson{member: a.source.MembershipID}, "ADMIN", "SELF", "ACTIVE")
	if e := unitSchemaExec(t, a.legalFixtureState, `UPDATE org_sync_test_targets SET owner_member=$3 WHERE org_id=$1 AND id=$2`, a.scope.OrgID().String(), a.target, otherPerson.member); e != nil {
		t.Fatal(e)
	}
	if _, e := a.devices.CheckDevice(t.Context(), a.scope, a.source.Actor); e != nil {
		t.Fatal("receipt read denial was not independent of live proof", e)
	}
	if got, e := a.store.Process(t.Context(), a.source, opA, protocol.DefaultLimits(), a.handler); !errors.Is(e, syncapp.ErrSourceDenied) || len(got.CanonicalBytes()) != 0 {
		t.Fatal("current SELF scope exposed another holder's receipt", e)
	}
	if e := unitSchemaExec(t, a.legalFixtureState, `UPDATE org_sync_test_targets SET owner_member=$3 WHERE org_id=$1 AND id=$2`, a.scope.OrgID().String(), a.target, a.source.MembershipID); e != nil {
		t.Fatal(e)
	}
	matrixAssign(t, a.deviceFixtureState, matrixPerson{member: a.source.MembershipID}, "ADMIN", "ORG", "NONE")
	if got, e := a.store.Process(t.Context(), a.source, opA, protocol.DefaultLimits(), a.handler); !errors.Is(e, syncapp.ErrSourceDenied) || len(got.CanonicalBytes()) != 0 {
		t.Fatal("revoked receipt-read coverage reused an earlier decision", e)
	}
	matrixAssign(t, a.deviceFixtureState, matrixPerson{member: a.source.MembershipID}, "ADMIN", "ORG", "ACTIVE")
	replay, e := a.store.Process(t.Context(), a.source, opA, protocol.DefaultLimits(), a.handler)
	if e != nil || !bytes.Equal(replay.CanonicalBytes(), ackA.CanonicalBytes()) {
		t.Fatal("fresh actual read authority could not recover immutable ACK", e)
	}
	if snapshot() != before {
		t.Fatal("read authority transitions rewrote intake history")
	}
	var unsafe bool
	if e := a.f.Runtime.QueryRow(context.Background(), `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname=current_user`).Scan(&unsafe); e != nil || unsafe {
		t.Fatal("isolation did not use an ordinary runtime", e)
	}
}
