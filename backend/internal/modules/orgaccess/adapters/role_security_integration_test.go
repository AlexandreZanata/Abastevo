//go:build integration

package adapters_test

import (
	"errors"
	orgpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
	"github.com/jackc/pgx/v5"
)

func roleOwner() app.CompanyActor { return app.CompanyActor{AccountID: key(110), SessionID: key(910)} }
func roleGrant(s kernel.OrgScope, op int) app.RoleAssignInput {
	return app.RoleAssignInput{OperationID: key(op), MembershipID: key(11), Role: d.Admin, Scope: d.OrgScope, TargetID: s.OrgID().String(), Reason: "Synthetic scoped administration"}
}
func roleNoEffects(t *testing.T, f *orgtest.Fixture, operation, id string) {
	t.Helper()
	var count int
	if err := f.Admin.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM org_audit_events WHERE operation_id=$1)+(SELECT count(*) FROM org_role_commands WHERE operation_id=$1)+(SELECT count(*) FROM org_role_assignments WHERE id=$2)`, operation, id).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed command retained effects", count, err)
	}
}
func TestRoleSecurityImmutableReceiptAndCurrentAuthority(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	roleActivate(t, f, s, 11, 21, "DRIVER")
	actor := roleOwner()
	in := roleGrant(s, 5001)
	first, err := store.AssignRole(t.Context(), s, actor, key(10), key(6001), in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EndRole(t.Context(), s, actor, key(10), app.RoleEndInput{OperationID: key(5002), AssignmentID: key(6001), ExpectedRevision: 1, Reason: "Synthetic access reduction"}); err != nil {
		t.Fatal(err)
	}
	again, err := store.AssignRole(t.Context(), s, actor, key(10), key(6002), in)
	if err != nil || !again.Replayed {
		t.Fatal("accepted retry failed", err)
	}
	original, _ := first.Assignment.Snapshot(s)
	repeated, _ := again.Assignment.Snapshot(s)
	if original != repeated || repeated.Revision != 1 || !repeated.EndedAt.IsZero() {
		t.Fatal("retry used mutable assignment rather than original acknowledgement")
	}
	// Account and membership identity must agree, even when the supplied member
	// itself holds a genuine management assignment.
	if _, err = store.ReadRole(t.Context(), s, actor, key(11), key(20)); !errors.Is(err, app.ErrNoMembership) {
		t.Fatal("forged member accepted", err)
	}
	// A real session belonging to somebody else must not authorize this account.
	forged := actor
	forged.SessionID = key(911)
	if _, err = store.ReadRole(t.Context(), s, forged, key(10), key(20)); !errors.Is(err, app.ErrRoleUnauthenticated) {
		t.Fatal("foreign session accepted", err)
	}
	if _, err = f.Admin.Exec(t.Context(), `UPDATE account_session_families SET revoked_at=clock_timestamp() WHERE id=$1`, key(1910)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AssignRole(t.Context(), s, actor, key(10), key(6003), in); !errors.Is(err, app.ErrRoleUnauthenticated) {
		t.Fatal("revoked family replay accepted", err)
	}
	if _, err = store.ReadRole(t.Context(), s, actor, key(10), key(20)); !errors.Is(err, app.ErrRoleUnauthenticated) {
		t.Fatal("revoked family read accepted", err)
	}
	if _, err = store.ListRoles(t.Context(), s, actor, key(10), app.RoleListFilter{Limit: 20}); !errors.Is(err, app.ErrRoleUnauthenticated) {
		t.Fatal("revoked family list accepted", err)
	}
}
func TestRoleSecurityAdminUnionAndOwnerProtection(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	roleActivate(t, f, s, 11, 21, "ADMIN")
	roleActivate(t, f, s, 12, 22, "DRIVER")
	actor := app.CompanyActor{AccountID: key(111), SessionID: key(911)}
	in := roleGrant(s, 5010)
	in.MembershipID = key(12)
	in.Role = d.Finance
	if _, err := store.AssignRole(t.Context(), s, actor, key(11), key(6010), in); !errors.Is(err, d.ErrPermissionDenied) {
		t.Fatal("ADMIN invented financial rights", err)
	}
	roleNoEffects(t, f, in.OperationID, key(6010))
	if _, err := store.EndRole(t.Context(), s, actor, key(11), app.RoleEndInput{OperationID: key(5011), AssignmentID: key(20), ExpectedRevision: 1, Reason: "Synthetic owner removal attempt"}); !errors.Is(err, d.ErrPermissionDenied) {
		t.Fatal("ADMIN ended OWNER", err)
	}
	// The independent current FINANCE source contributes its real descriptor;
	// caller-provided roles cannot substitute for this server record.
	if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'FINANCE','ORG',$1,clock_timestamp(),$3,'synthetic financial authority')`, s.OrgID().String(), key(6020), key(11))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	in.OperationID = key(5012)
	if _, err := store.AssignRole(t.Context(), s, actor, key(11), key(6012), in); err != nil {
		t.Fatal("current role union ignored", err)
	}
}
func TestRoleSecurityStaleProofAndInactiveTarget(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		want      error
	}{
		{"stale company proof", `UPDATE company_web_sessions SET authenticated_at=clock_timestamp()-interval '16 minutes',created_at=clock_timestamp()-interval '16 minutes',last_seen_at=clock_timestamp(),expires_at=clock_timestamp()+interval '1 hour' WHERE id='` + key(910) + `'`, app.ErrRoleUnauthenticated},
		{"suspended actor", `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE id='` + key(10) + `'`, app.ErrNoMembership},
		{"suspended target", `UPDATE org_memberships SET state='SUSPENDED',suspension_origin='MANAGER',revision=revision+1 WHERE id='` + key(11) + `'`, app.ErrRoleTargetNotActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, s, store, _ := roleStoreFixture(t)
			roleActivate(t, f, s, 11, 21, "DRIVER")
			// Source invariants remain enabled. Stale time is changed as one consistent
			// synthetic session row; the actor suspension uses a second live owner.
			if tc.name == "suspended actor" {
				roleActivate(t, f, s, 12, 22, "OWNER")
			}
			if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error { _, err := tx.Exec(t.Context(), tc.sql); return err }); err != nil {
				t.Fatal(err)
			}
			in := roleGrant(s, 5030)
			result, err := store.AssignRole(t.Context(), s, roleOwner(), key(10), key(6030), in)
			if !errors.Is(err, tc.want) || result.Assignment != (d.Assignment{}) {
				t.Fatal("source rejection missing or provisional result exposed", err)
			}
			roleNoEffects(t, f, in.OperationID, key(6030))
		})
	}
}
func TestRoleSecurityReductionsRemainAvailableAndReplayRechecksCapability(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	roleActivate(t, f, s, 11, 21, "DRIVER")
	actor := roleOwner()
	in := roleGrant(s, 5040)
	if _, err := store.AssignRole(t.Context(), s, actor, key(10), key(6040), in); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Admin.Exec(t.Context(), `UPDATE org_capabilities SET enabled=false,revision=revision+1 WHERE org_id=$1 AND capability='organizations'`, s.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignRole(t.Context(), s, actor, key(10), key(6041), in); !errors.Is(err, app.ErrEntitlementRequired) {
		t.Fatal("disabled plan replay restored write admission", err)
	}
	if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		if _, err := tx.Exec(t.Context(), `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, s.OrgID().String()); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `UPDATE organizations SET state='ARCHIVED',revision=revision+1 WHERE org_id=$1`, s.OrgID().String())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReadRole(t.Context(), s, actor, key(10), key(6040)); err != nil {
		t.Fatal("archived read blocked", err)
	}
	if _, err := store.ListRoles(t.Context(), s, actor, key(10), app.RoleListFilter{Limit: 20}); err != nil {
		t.Fatal("archived list blocked", err)
	}
	if _, err := store.EndRole(t.Context(), s, actor, key(10), app.RoleEndInput{OperationID: key(5042), AssignmentID: key(6040), ExpectedRevision: 1, Reason: "Synthetic archived access reduction"}); err != nil {
		t.Fatal("archived access reduction blocked", err)
	}
}
func TestRoleSecurityConcurrentRetryAndCAS(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	roleActivate(t, f, s, 11, 21, "DRIVER")
	in := roleGrant(s, 5050)
	const n = 12
	var wg sync.WaitGroup
	results := make(chan app.RoleResult, n)
	failures := make(chan error, n)
	start := make(chan struct{})
	for i := range n {
		wg.Go(func() {
			<-start
			r, err := store.AssignRole(t.Context(), s, roleOwner(), key(10), key(6050+i), in)
			results <- r
			failures <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("concurrent retry failed", err)
		}
	}
	firstID := ""
	accepted := 0
	for result := range results {
		v, _ := result.Assignment.Snapshot(s)
		if firstID == "" {
			firstID = v.ID
		}
		if v.ID != firstID {
			t.Fatal("retry produced different acknowledgements")
		}
		if !result.Replayed {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatal("retry wrote more than once", accepted)
	}
	// Different operations contending for one revision cannot both change it.
	failures = make(chan error, n)
	start = make(chan struct{})
	for i := range n {
		wg.Go(func() {
			<-start
			_, err := store.ChangeRole(t.Context(), s, roleOwner(), key(10), key(6100+i), app.RoleChangeInput{OperationID: key(5100 + i), AssignmentID: firstID, ExpectedRevision: 1, Role: d.Auditor, Scope: d.OrgScope, TargetID: s.OrgID().String(), Reason: "Synthetic concurrent role replacement"})
			failures <- err
		})
	}
	close(start)
	wg.Wait()
	close(failures)
	accepted = 0
	conflicts := 0
	for err := range failures {
		if err == nil {
			accepted++
		} else if errors.Is(err, d.ErrAssignmentConflict) {
			conflicts++
		} else {
			t.Fatal("unexpected CAS outcome", err)
		}
	}
	if accepted != 1 || conflicts != n-1 {
		t.Fatal("role CAS lost update", accepted, conflicts)
	}
}
func TestRoleSecurityDeferredCommitFailureExposesNoAcknowledgement(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	roleActivate(t, f, s, 11, 21, "DRIVER")
	if _, err := f.Admin.Exec(t.Context(), `CREATE FUNCTION synthetic_role_commit_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION USING ERRCODE='23503',MESSAGE='synthetic commit fault'; END $$; CREATE CONSTRAINT TRIGGER synthetic_role_commit_fault AFTER INSERT ON org_role_commands DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION synthetic_role_commit_fault()`); err != nil {
		t.Fatal(err)
	}
	in := roleGrant(s, 5060)
	result, err := store.AssignRole(t.Context(), s, roleOwner(), key(10), key(6060), in)
	if !errors.Is(err, app.ErrUnavailable) || result.Assignment != (d.Assignment{}) || result.Replayed {
		t.Fatal("commit failure exposed provisional acknowledgement", err)
	}
	roleNoEffects(t, f, in.OperationID, key(6060))
}

func TestRoleSecurityOwnerMayReduceOwnNonmanagementAssignment(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'FINANCE','ORG',$1,clock_timestamp(),$3,'synthetic existing financial scope')`, s.OrgID().String(), key(6070), key(10))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	in := app.RoleChangeInput{OperationID: key(5070), AssignmentID: key(6070), ExpectedRevision: 1, Role: d.Finance, Scope: d.SelfScope, TargetID: key(10), Reason: "Synthetic owner scope reduction"}
	result, err := store.ChangeRole(t.Context(), s, roleOwner(), key(10), key(6071), in)
	if err != nil {
		t.Fatal("OWNER reducing own nonmanagement assignment rejected", err)
	}
	ended, _ := result.EndedAssignment.Snapshot(s)
	fresh, _ := result.NewAssignment.Snapshot(s)
	if ended.Revision != 2 || fresh.Scope != d.SelfScope {
		t.Fatal("self-reduction did not atomically narrow scope")
	}
	// A temporal extension is forbidden even for OWNER. The separately held
	// OWNER assignment remains current, so this checks self-modification only.
	in.OperationID = key(5072)
	in.AssignmentID = key(6071)
	in.ValidFrom = fresh.ValidFrom.Add(-time.Second)
	if _, err = store.ChangeRole(t.Context(), s, roleOwner(), key(10), key(6072), in); !errors.Is(err, d.ErrSelfModificationForbidden) {
		t.Fatal("OWNER self-increase accepted", err)
	}
	roleNoEffects(t, f, in.OperationID, key(6072))
}
func TestRoleSecurityReplayDoesNotRestoreRevokedPermission(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	roleActivate(t, f, s, 11, 21, "DRIVER")
	roleActivate(t, f, s, 12, 22, "OWNER")
	in := roleGrant(s, 5080)
	if _, err := store.AssignRole(t.Context(), s, roleOwner(), key(10), key(6080), in); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EndRole(t.Context(), s, app.CompanyActor{AccountID: key(112), SessionID: key(912)}, key(12), app.RoleEndInput{OperationID: key(5081), AssignmentID: key(20), ExpectedRevision: 1, Reason: "Synthetic actor permission revocation"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignRole(t.Context(), s, roleOwner(), key(10), key(6081), in); !errors.Is(err, d.ErrPermissionDenied) {
		t.Fatal("historical receipt restored revoked management permission", err)
	}
}
func TestRoleSecurityForeignTargetsStayInvisible(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	other := scope(t, 802)
	root, err := org.NewPending(other, org.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = orgpg.NewStore(f.Runtime).Create(t.Context(), other, root); err != nil {
		t.Fatal(err)
	}
	roleActivate(t, f, other, 31, 41, "OWNER")
	if _, err = store.ReadRole(t.Context(), s, roleOwner(), key(10), key(41)); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("foreign assignment visible", err)
	}
	in := roleGrant(s, 5090)
	in.MembershipID = key(31)
	if _, err = store.AssignRole(t.Context(), s, roleOwner(), key(10), key(6090), in); !errors.Is(err, app.ErrRoleTargetNotFound) {
		t.Fatal("foreign membership accepted", err)
	}
	roleNoEffects(t, f, in.OperationID, key(6090))
}

func TestRoleSecurityChangeRetryReturnsOriginalBothSnapshots(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	roleActivate(t, f, s, 11, 21, "DRIVER")
	in := app.RoleChangeInput{OperationID: key(5150), AssignmentID: key(21), ExpectedRevision: 1, Role: d.Admin, Scope: d.OrgScope, TargetID: s.OrgID().String(), Reason: "Synthetic original role replacement"}
	first, err := store.ChangeRole(t.Context(), s, roleOwner(), key(10), key(6150), in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EndRole(t.Context(), s, roleOwner(), key(10), app.RoleEndInput{OperationID: key(5151), AssignmentID: key(6150), ExpectedRevision: 1, Reason: "Synthetic later reduction"}); err != nil {
		t.Fatal(err)
	}
	repeated, err := store.ChangeRole(t.Context(), s, roleOwner(), key(10), key(6152), in)
	if err != nil || !repeated.Replayed {
		t.Fatal("change retry failed", err)
	}
	a, _ := first.EndedAssignment.Snapshot(s)
	b, _ := repeated.EndedAssignment.Snapshot(s)
	x, _ := first.NewAssignment.Snapshot(s)
	y, _ := repeated.NewAssignment.Snapshot(s)
	if a != b || x != y || y.Revision != 1 || !y.EndedAt.IsZero() {
		t.Fatal("change retry returned mutable acknowledgement")
	}
}

func TestRoleSecurityEndRetainsPrivateCommandReasonApartFromGrant(t *testing.T) {
	f, s, store, _ := roleStoreFixture(t)
	roleActivate(t, f, s, 11, 21, "DRIVER")
	const reason = "Synthetic access ended after team reassignment"
	in := app.RoleEndInput{OperationID: key(5160), AssignmentID: key(21), ExpectedRevision: 1, Reason: reason}
	result, err := store.EndRole(t.Context(), s, roleOwner(), key(10), in)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := result.Assignment.Snapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Reason == reason {
		t.Fatal("end rewrote the original grant history")
	}
	var retained, original, auditReason, auditJSON string
	if err := database.WithOrgTx(t.Context(), f.Runtime, s, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT c.result->>'CommandReason',c.result->'Assignment'->>'Reason',a.reason_code,to_jsonb(a)::text FROM org_role_commands c JOIN org_audit_events a USING(org_id,operation_id) WHERE c.org_id=$1 AND c.operation_id=$2`, s.OrgID().String(), in.OperationID).Scan(&retained, &original, &auditReason, &auditJSON)
	}); err != nil {
		t.Fatal(err)
	}
	if retained != reason || original != snapshot.Reason || auditReason != "ended" || strings.Contains(auditJSON, reason) {
		t.Fatal("private command reason lost or copied into central audit")
	}
	replay, err := store.EndRole(t.Context(), s, roleOwner(), key(10), in)
	if err != nil || !replay.Replayed {
		t.Fatal("reason-bearing receipt replay failed", err)
	}
}
