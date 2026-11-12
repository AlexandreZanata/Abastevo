//go:build integration

package adapters_test

import (
	"errors"
	"sync"
	"testing"

	qorgaudit "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgaudit"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	accessadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func roleStoreFixture(t *testing.T) (*orgtest.Fixture, kernel.OrgScope, *accessadapters.RoleStore, *accessadapters.Store) {
	t.Helper()
	f := orgtest.New(t)
	f.GrantTeamReductions(t)
	f.GrantSettingsLifecycle(t)

	r := pgx.Identifier{f.RuntimeRole}.Sanitize()
	grantSQL := "GRANT USAGE ON SCHEMA public TO " + r + ";" +
		" GRANT SELECT,INSERT,UPDATE ON organizations,org_employee_identities,org_memberships,org_role_assignments,org_audit_events TO " + r + ";" +
		" GRANT SELECT,UPDATE ON accounts TO " + r + ";" +
		" GRANT SELECT,INSERT ON payment_billing_exits TO " + r + ";"
	if _, err := f.Admin.Exec(t.Context(), grantSQL); err != nil {
		t.Fatal(err)
	}

	scopeOrg := scope(t, 801)
	v, err := org.NewPending(scopeOrg, org.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orgpg.NewStore(f.Runtime).Create(t.Context(), scopeOrg, v); err != nil {
		t.Fatal(err)
	}

	assignmentsStore := accessadapters.NewStore(f.Runtime)
	catalog, err := audit.NewCatalog(accessadapters.RoleAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	auditStore := auditpg.NewStore(f.Runtime, catalog)
	roleStore := accessadapters.NewRoleStore(f.Runtime, assignmentsStore, auditStore)

	return f, scopeOrg, roleStore, assignmentsStore
}

func TestRoleStore_AssignRole_PositiveAndReplay(t *testing.T) {
	f, s, roleStore, _ := roleStoreFixture(t)

	// Owner: member 10, assignment 20
	activate(t, f, s, 10, 20, "OWNER")
	// Target member: member 11 (in INVITED state, ready to receive role)
	seed(t, f, s, 11)

	ownerActor := app.CompanyActor{AccountID: key(110), SessionID: key(910)}
	opID := key(1001)
	newAssignID := key(2001)

	input := app.RoleAssignInput{
		OperationID:  opID,
		MembershipID: key(11),
		Role:         d.Admin,
		Scope:        d.OrgScope,
		TargetID:     s.OrgID().String(),
		Reason:       "Assigning regional administrator",
	}

	res, err := roleStore.AssignRole(t.Context(), s, ownerActor, key(10), newAssignID, input)
	if err != nil {
		t.Fatalf("unexpected assign error: %v", err)
	}
	if res.Replayed {
		t.Fatal("expected initial execution, not replay")
	}

	snap, err := res.Assignment.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ID != newAssignID || snap.Role != d.Admin || snap.MembershipID != key(11) || snap.Revision != 1 || !snap.EndedAt.IsZero() {
		t.Fatalf("unexpected assignment snapshot: %+v", snap)
	}

	// Verify audit event in org_audit_events
	if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		row, err := qorgaudit.New(tx).GetEvent(t.Context(), qorgaudit.GetEventParams{
			OrgID:       s.OrgID().String(),
			OperationID: opID,
		})
		if err != nil {
			return err
		}
		if row.Action != "auth.assignment.created" || row.ReasonCode != "granted" || row.TargetID != newAssignID || row.ActorID != key(110) {
			t.Fatalf("unexpected audit row: %+v", row)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Test exact replay: same OperationID and input
	replayRes, err := roleStore.AssignRole(t.Context(), s, ownerActor, key(10), key(2002), input)
	if err != nil {
		t.Fatalf("unexpected replay error: %v", err)
	}
	if !replayRes.Replayed {
		t.Fatal("expected replayed = true")
	}
	replaySnap, err := replayRes.Assignment.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if replaySnap.ID != newAssignID {
		t.Fatalf("expected replayed assignment ID %s, got %s", newAssignID, replaySnap.ID)
	}

	// Test reused OperationID with different input -> ErrRoleOperationReused
	conflictInput := input
	conflictInput.Reason = "Different reason entirely"
	_, err = roleStore.AssignRole(t.Context(), s, ownerActor, key(10), key(2003), conflictInput)
	if !errors.Is(err, app.ErrRoleOperationReused) {
		t.Fatalf("expected ErrRoleOperationReused, got %v", err)
	}
}

func TestRoleStore_AssignRole_NegativeGuards(t *testing.T) {
	f, s, roleStore, _ := roleStoreFixture(t)

	// Owner: member 10, assignment 20
	activate(t, f, s, 10, 20, "OWNER")
	// Admin: member 11, assignment 21
	activate(t, f, s, 11, 21, "ADMIN")
	// Normal member: member 12 (in INVITED state)
	seed(t, f, s, 12)

	adminActor := app.CompanyActor{AccountID: key(111), SessionID: key(911)}

	// 1. Escalation: Admin tries to assign OWNER
	_, err := roleStore.AssignRole(t.Context(), s, adminActor, key(11), key(2005), app.RoleAssignInput{
		OperationID:  key(1005),
		MembershipID: key(12),
		Role:         d.Owner,
		Scope:        d.OrgScope,
		TargetID:     s.OrgID().String(),
		Reason:       "Admin escalating to owner",
	})
	if !errors.Is(err, d.ErrCannotAssignOwner) {
		t.Fatalf("expected ErrCannotAssignOwner, got %v", err)
	}

	// 2. Self-assignment: Admin tries to assign role to self
	_, err = roleStore.AssignRole(t.Context(), s, adminActor, key(11), key(2006), app.RoleAssignInput{
		OperationID:  key(1006),
		MembershipID: key(11),
		Role:         d.Driver,
		Scope:        d.SelfScope,
		TargetID:     key(11),
		Reason:       "Self assignment attempt",
	})
	if !errors.Is(err, d.ErrSelfAssignmentForbidden) {
		t.Fatalf("expected ErrSelfAssignmentForbidden, got %v", err)
	}

	// 3. Target membership not active (e.g. member 13 is ended)
	seed(t, f, s, 13)
	if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE org_memberships SET state='ENDED',end_reason='OFFBOARDED',revision=revision+1 WHERE org_id=$1 AND id=$2`, s.OrgID().String(), key(13))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ownerActor := app.CompanyActor{AccountID: key(110), SessionID: key(910)}
	_, err = roleStore.AssignRole(t.Context(), s, ownerActor, key(10), key(2007), app.RoleAssignInput{
		OperationID:  key(1007),
		MembershipID: key(13),
		Role:         d.Finance,
		Scope:        d.OrgScope,
		TargetID:     s.OrgID().String(),
		Reason:       "Assigning ended member",
	})
	if !errors.Is(err, app.ErrRoleTargetNotActive) {
		t.Fatalf("expected ErrRoleTargetNotActive, got %v", err)
	}

	// 4. Suspended organization blocks assign
	// Use f.Admin to set the organization state to SUSPENDED
	if _, err := f.Admin.Exec(t.Context(), `ALTER TABLE organizations DISABLE TRIGGER org_setup_activation`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), `UPDATE organizations SET display_name='Synthetic Org',time_zone='America/Sao_Paulo',state='ACTIVE',revision=revision+1 WHERE org_id=$1`, s.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), `UPDATE organizations SET state='SUSPENDED',suspension_reason='BILLING',revision=revision+1 WHERE org_id=$1`, s.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), `ALTER TABLE organizations ENABLE TRIGGER org_setup_activation`); err != nil {
		t.Fatal(err)
	}
	_, err = roleStore.AssignRole(t.Context(), s, ownerActor, key(10), key(2008), app.RoleAssignInput{
		OperationID:  key(1008),
		MembershipID: key(12),
		Role:         d.FleetManager,
		Scope:        d.OrgScope,
		TargetID:     s.OrgID().String(),
		Reason:       "Assigning on suspended org",
	})
	if !errors.Is(err, app.ErrOrgSuspended) {
		t.Fatalf("expected ErrOrgSuspended, got %v", err)
	}
}

func TestRoleStore_EndRole_PositiveAndLastOwner(t *testing.T) {
	f, s, roleStore, _ := roleStoreFixture(t)

	// Owner A: member 10, assignment 20
	activate(t, f, s, 10, 20, "OWNER")
	// Admin: member 11, assignment 21
	activate(t, f, s, 11, 21, "ADMIN")
	// Driver: member 12, assignment 22
	activate(t, f, s, 12, 22, "DRIVER")

	adminActor := app.CompanyActor{AccountID: key(111), SessionID: key(911)}
	ownerAActor := app.CompanyActor{AccountID: key(110), SessionID: key(910)}

	// 1. Self-reduction by non-owner: Admin tries to end own role -> forbidden
	_, err := roleStore.EndRole(t.Context(), s, adminActor, key(11), app.RoleEndInput{
		OperationID:      key(1021),
		AssignmentID:     key(21),
		ExpectedRevision: 1,
		Reason:           "Admin self reduction",
	})
	if !errors.Is(err, d.ErrSelfModificationForbidden) {
		t.Fatalf("expected ErrSelfModificationForbidden, got %v", err)
	}

	// 2. Admin ends Driver assignment -> succeeds
	endRes, err := roleStore.EndRole(t.Context(), s, adminActor, key(11), app.RoleEndInput{
		OperationID:      key(1022),
		AssignmentID:     key(22),
		ExpectedRevision: 1,
		Reason:           "Driver assignment finished",
	})
	if err != nil {
		t.Fatalf("unexpected end role error: %v", err)
	}
	endSnap, err := endRes.Assignment.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if endSnap.Revision != 2 || endSnap.EndedAt.IsZero() {
		t.Fatalf("expected revision 2 and ended_at set, got %+v", endSnap)
	}

	// 3. Last owner invariant: Owner A is the only owner, tries to end own role -> fails with ErrLastOrgOwner
	_, err = roleStore.EndRole(t.Context(), s, ownerAActor, key(10), app.RoleEndInput{
		OperationID:      key(1023),
		AssignmentID:     key(20),
		ExpectedRevision: 1,
		Reason:           "Owner removing own assignment while sole owner",
	})
	if !errors.Is(err, kernel.ErrLastOrgOwner) {
		t.Fatalf("expected ErrLastOrgOwner, got %v", err)
	}

	// 4. Activate Owner B: member 14, assignment 24
	activate(t, f, s, 14, 24, "OWNER")

	// Now Owner A tries to end own role: since Owner B exists, it must SUCCEED!
	ownerEndRes, err := roleStore.EndRole(t.Context(), s, ownerAActor, key(10), app.RoleEndInput{
		OperationID:      key(1024),
		AssignmentID:     key(20),
		ExpectedRevision: 1,
		Reason:           "Owner resigning now that second owner exists",
	})
	if err != nil {
		t.Fatalf("unexpected owner self-end error: %v", err)
	}
	ownerSnap, err := ownerEndRes.Assignment.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if ownerSnap.Revision != 2 || ownerSnap.EndedAt.IsZero() {
		t.Fatalf("expected ended owner revision 2, got %+v", ownerSnap)
	}

	// Now Owner B is the only remaining owner. Trying to end Owner B must FAIL!
	ownerBActor := app.CompanyActor{AccountID: key(114), SessionID: key(914)}
	_, err = roleStore.EndRole(t.Context(), s, ownerBActor, key(14), app.RoleEndInput{
		OperationID:      key(1025),
		AssignmentID:     key(24),
		ExpectedRevision: 1,
		Reason:           "Owner B resigning as sole owner",
	})
	if !errors.Is(err, kernel.ErrLastOrgOwner) {
		t.Fatalf("expected ErrLastOrgOwner, got %v", err)
	}
}

func TestRoleStore_ConcurrentLastOwnerContention(t *testing.T) {
	f, s, roleStore, _ := roleStoreFixture(t)

	// Owner A: member 10, assignment 20
	activate(t, f, s, 10, 20, "OWNER")
	// Owner B: member 11, assignment 21
	activate(t, f, s, 11, 21, "OWNER")

	ownerAActor := app.CompanyActor{AccountID: key(110), SessionID: key(910)}
	ownerBActor := app.CompanyActor{AccountID: key(111), SessionID: key(911)}

	var wg sync.WaitGroup
	wg.Add(2)

	var errA, errB error
	start := make(chan struct{})

	go func() {
		defer wg.Done()
		<-start
		_, errA = roleStore.EndRole(t.Context(), s, ownerAActor, key(10), app.RoleEndInput{
			OperationID:      key(1050),
			AssignmentID:     key(20),
			ExpectedRevision: 1,
			Reason:           "Concurrent demote owner A",
		})
	}()

	go func() {
		defer wg.Done()
		<-start
		_, errB = roleStore.EndRole(t.Context(), s, ownerBActor, key(11), app.RoleEndInput{
			OperationID:      key(1051),
			AssignmentID:     key(21),
			ExpectedRevision: 1,
			Reason:           "Concurrent demote owner B",
		})
	}()

	close(start)
	wg.Wait()

	// Exactly one must have succeeded (nil error) and exactly one must have failed with ErrLastOrgOwner
	successes := 0
	lastOwnerErrors := 0
	for _, err := range []error{errA, errB} {
		if err == nil {
			successes++
		} else if errors.Is(err, kernel.ErrLastOrgOwner) {
			lastOwnerErrors++
		}
	}

	if successes != 1 || lastOwnerErrors != 1 {
		t.Fatalf("expected exactly 1 success and 1 ErrLastOrgOwner, got successes=%d, lastOwnerErrors=%d (errA=%v, errB=%v)", successes, lastOwnerErrors, errA, errB)
	}

	// Verify at least one owner remains active in database
	if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		var activeOwners int
		err := tx.QueryRow(t.Context(), `SELECT count(*) FROM org_role_assignments WHERE org_id=$1 AND role='OWNER' AND ended_at IS NULL`, s.OrgID().String()).Scan(&activeOwners)
		if err != nil {
			return err
		}
		if activeOwners != 1 {
			t.Fatalf("expected exactly 1 active owner remaining, found %d", activeOwners)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRoleStore_ChangeRole_Atomicity(t *testing.T) {
	f, s, roleStore, _ := roleStoreFixture(t)

	// Owner: member 10, assignment 20
	activate(t, f, s, 10, 20, "OWNER")
	// Member 11: assignment 21 as DRIVER
	activate(t, f, s, 11, 21, "DRIVER")

	ownerActor := app.CompanyActor{AccountID: key(110), SessionID: key(910)}
	newAssignID := key(2030)
	opID := key(1060)

	// Owner changes member 11 from DRIVER to FINANCE
	changeRes, err := roleStore.ChangeRole(t.Context(), s, ownerActor, key(10), newAssignID, app.RoleChangeInput{
		OperationID:      opID,
		AssignmentID:     key(21),
		ExpectedRevision: 1,
		Role:             d.Finance,
		Scope:            d.OrgScope,
		TargetID:         s.OrgID().String(),
		Reason:           "Promoting driver to finance manager",
	})
	if err != nil {
		t.Fatalf("unexpected change role error: %v", err)
	}

	endedSnap, err := changeRes.EndedAssignment.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if endedSnap.ID != key(21) || endedSnap.Revision != 2 || endedSnap.EndedAt.IsZero() {
		t.Fatalf("unexpected ended snapshot: %+v", endedSnap)
	}

	newSnap, err := changeRes.NewAssignment.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if newSnap.ID != newAssignID || newSnap.Role != d.Finance || newSnap.Revision != 1 || !newSnap.EndedAt.IsZero() {
		t.Fatalf("unexpected new snapshot: %+v", newSnap)
	}

	// Verify exact replay
	replayRes, err := roleStore.ChangeRole(t.Context(), s, ownerActor, key(10), key(2031), app.RoleChangeInput{
		OperationID:      opID,
		AssignmentID:     key(21),
		ExpectedRevision: 1,
		Role:             d.Finance,
		Scope:            d.OrgScope,
		TargetID:         s.OrgID().String(),
		Reason:           "Promoting driver to finance manager",
	})
	if err != nil {
		t.Fatalf("unexpected replay error: %v", err)
	}
	if !replayRes.Replayed {
		t.Fatal("expected replayed = true")
	}
}

func TestRoleStore_ReadAndListRoles(t *testing.T) {
	f, s, roleStore, _ := roleStoreFixture(t)

	// Owner: member 10, assignment 20
	activate(t, f, s, 10, 20, "OWNER")
	// Admin: member 11, assignment 21
	activate(t, f, s, 11, 21, "ADMIN")
	// Driver: member 12, assignment 22
	activate(t, f, s, 12, 22, "DRIVER")

	ownerActor := app.CompanyActor{AccountID: key(110), SessionID: key(910)}

	// 1. Read existing
	assignment, err := roleStore.ReadRole(t.Context(), s, ownerActor, key(10), key(21))
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	snap, err := assignment.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ID != key(21) || snap.Role != d.Admin {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}

	// 2. Read non-existent
	_, err = roleStore.ReadRole(t.Context(), s, ownerActor, key(10), key(9999))
	if !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 3. List all active
	pageActive, err := roleStore.ListRoles(t.Context(), s, ownerActor, key(10), app.RoleListFilter{
		State: "ACTIVE",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected list error: %v", err)
	}
	if len(pageActive.Assignments) != 3 {
		t.Fatalf("expected 3 active assignments, got %d", len(pageActive.Assignments))
	}

	// 4. End assignment 22
	_, err = roleStore.EndRole(t.Context(), s, ownerActor, key(10), app.RoleEndInput{
		OperationID:      key(1090),
		AssignmentID:     key(22),
		ExpectedRevision: 1,
		Reason:           "Ending driver assignment",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 5. List active -> should now be 2
	pageActiveAfterEnd, err := roleStore.ListRoles(t.Context(), s, ownerActor, key(10), app.RoleListFilter{
		State: "ACTIVE",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pageActiveAfterEnd.Assignments) != 2 {
		t.Fatalf("expected 2 active assignments, got %d", len(pageActiveAfterEnd.Assignments))
	}

	// 6. List ended -> should be 1
	pageEnded, err := roleStore.ListRoles(t.Context(), s, ownerActor, key(10), app.RoleListFilter{
		State: "ENDED",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pageEnded.Assignments) != 1 {
		t.Fatalf("expected 1 ended assignment, got %d", len(pageEnded.Assignments))
	}

	// 7. Pagination limit
	pageLimit, err := roleStore.ListRoles(t.Context(), s, ownerActor, key(10), app.RoleListFilter{
		State: "ALL",
		Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pageLimit.Assignments) != 2 || pageLimit.NextID == "" {
		t.Fatalf("expected 2 assignments with next_id, got len=%d, nextID=%s", len(pageLimit.Assignments), pageLimit.NextID)
	}
}
