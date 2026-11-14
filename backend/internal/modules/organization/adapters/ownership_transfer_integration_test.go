//go:build integration

package adapters_test

import (
	"errors"
	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	companypg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"testing"
	"time"

	accessadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters"
	accessd "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	auditpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/adapters"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/jackc/pgx/v5"
)

type transferFixtureState struct {
	legalFixtureState
	store       *orgadapters.OwnershipTransferStore
	assignments *accessadapters.Store
	actors      map[string]app.CompanyActor
}

func transferFixture(t *testing.T) transferFixtureState {
	t.Helper()
	fx := legalFixture(t)
	r := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON org_ownership_transfers TO "+r+"; GRANT SELECT,INSERT,UPDATE ON accounts TO "+r); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx, `UPDATE organizations SET state='ACTIVE',revision=revision+1 WHERE org_id=$1`, fx.scope.OrgID().String()); err != nil {
		t.Fatal(err)
	}
	assignmentsStore := accessadapters.NewStore(fx.f.Runtime)
	catalog, err := audit.NewCatalog(orgadapters.OwnershipTransferAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	auditStore := auditpg.NewStore(fx.f.Runtime, catalog)
	transferStore := orgadapters.NewOwnershipTransferStore(fx.f.Runtime, assignmentsStore, auditStore)
	transferStore.Memberships, transferStore.Capabilities = fx.store.Memberships, fx.flags
	return transferFixtureState{
		legalFixtureState: fx,
		store:             transferStore,
		assignments:       assignmentsStore,
		actors:            map[string]app.CompanyActor{fx.member: fx.actor},
	}
}

func createTransferMember(t *testing.T, fx transferFixtureState, managed bool) (string, string) {
	t.Helper()
	org := fx.scope.OrgID().String()
	memberID := onboardingID(t)
	employeeID := onboardingID(t)
	accountID := onboardingID(t)

	var managedVal any = nil
	accessModel := "EXISTING_ACCOUNT"
	if managed {
		managedVal = org
		accessModel = "MANAGED"
	}
	if managed {
		if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO accounts(id,alias,status,enterprise_managed_org_id) VALUES($1,$2,'active',$3)`, accountID, "synthetic-user-"+memberID[:8], managedVal); err != nil {
			t.Fatal(err)
		}
	} else {
		actor := transferPersonalActor(t, fx)
		accountID = actor.AccountID
		fx.actors[memberID] = actor
	}
	if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_employee_identities(org_id,id,state) VALUES($1,$2,'ACTIVE')`, org, employeeID); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_memberships(org_id,id,employee_id,account_id,access_model,state) VALUES($1,$2,$3,$4,$5,'INVITED')`, org, memberID, employeeID, accountID, accessModel); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx.legalFixtureState, `INSERT INTO org_role_assignments(org_id,id,membership_id,role,scope_type,scope_target,valid_from,assigned_by,reason) VALUES($1,$2,$3,'ADMIN','ORG',$1,clock_timestamp()-interval '1 minute',$4,'synthetic admin')`, org, onboardingID(t), memberID, fx.member); err != nil {
		t.Fatal(err)
	}
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state='ACTIVE',revision=revision+1 WHERE org_id=$1 AND id=$2`, org, memberID); err != nil {
		t.Fatal(err)
	}
	return memberID, accountID
}

func TestOwnershipTransfer_Propose_PositiveAndReplay(t *testing.T) {
	fx := transferFixture(t)
	targetMemberID, _ := createTransferMember(t, fx, false)

	now := time.Now().UTC()
	opID := onboardingID(t)
	transferID := onboardingID(t)

	input := d.OwnershipTransfer{
		TransferID:            transferID,
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    targetMemberID,
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           opID,
	}

	// 1. Propose transfer
	res, replayed, err := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input)
	if err != nil {
		t.Fatalf("unexpected error proposing transfer: %v", err)
	}
	if replayed {
		t.Fatal("expected replayed to be false on initial propose")
	}
	if res.State != d.OwnershipTransferPending {
		t.Fatalf("expected state PENDING, got %s", res.State)
	}
	if res.TransferID != transferID {
		t.Fatalf("expected transfer ID %s, got %s", transferID, res.TransferID)
	}

	// 2. Exact replay
	res2, replayed2, err := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, input)
	if err != nil {
		t.Fatalf("unexpected error on replay: %v", err)
	}
	if !replayed2 {
		t.Fatal("expected replayed to be true on exact replay")
	}
	if res2.TransferID != transferID {
		t.Fatalf("expected transfer ID %s on replay, got %s", transferID, res2.TransferID)
	}

	// 3. Reused operation ID with changed parameters
	inputChanged := input
	inputChanged.StepDownToAdmin = false
	_, _, err = fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, inputChanged)
	if err != d.ErrTransferOperationReused {
		t.Fatalf("expected ErrTransferOperationReused, got %v", err)
	}
}

func TestOwnershipTransfer_Propose_NegativeGuards(t *testing.T) {
	fx := transferFixture(t)
	targetMemberID, _ := createTransferMember(t, fx, false)
	managedTargetID, _ := createTransferMember(t, fx, true)

	now := time.Now().UTC()

	// A. Self-transfer
	_, _, err := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, d.OwnershipTransfer{
		TransferID:            onboardingID(t),
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    fx.member,
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           onboardingID(t),
	})
	if err == nil {
		t.Fatal("expected error on self-transfer")
	}

	// B. Managed account target (BR-ORG-026)
	_, _, err = fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, d.OwnershipTransfer{
		TransferID:            onboardingID(t),
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    managedTargetID,
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           onboardingID(t),
	})
	if err != d.ErrOwnerRequiresPersonalAccount {
		t.Fatalf("expected ErrOwnerRequiresPersonalAccount, got %v", err)
	}

	// C. Pending transfer conflict (at most one open transfer)
	_, _, err = fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, d.OwnershipTransfer{
		TransferID:            onboardingID(t),
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    targetMemberID,
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           onboardingID(t),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Attempting a second open transfer
	anotherTargetID, _ := createTransferMember(t, fx, false)
	_, _, err = fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, d.OwnershipTransfer{
		TransferID:            onboardingID(t),
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    anotherTargetID,
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           onboardingID(t),
	})
	if err != d.ErrTransferPending {
		t.Fatalf("expected ErrTransferPending, got %v", err)
	}
}

func TestOwnershipTransfer_Accept_PositiveWithStepDown(t *testing.T) {
	fx := transferFixture(t)
	targetMemberID, _ := createTransferMember(t, fx, false)

	now := time.Now().UTC()
	opID := onboardingID(t)
	transferID := onboardingID(t)

	// Propose transfer
	_, _, err := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, d.OwnershipTransfer{
		TransferID:            transferID,
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    targetMemberID,
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           opID,
	})
	if err != nil {
		t.Fatalf("failed proposing transfer: %v", err)
	}

	// Target accepts
	acceptOpID := onboardingID(t)
	accepted, replayed, err := fx.store.Accept(t.Context(), fx.scope, fx.actors[targetMemberID], targetMemberID, transferID, acceptOpID)
	if err != nil {
		t.Fatalf("unexpected error accepting transfer: %v", err)
	}
	if replayed {
		t.Fatal("expected replayed to be false")
	}
	if accepted.State != d.OwnershipTransferAccepted {
		t.Fatalf("expected state ACCEPTED, got %s", accepted.State)
	}
	if accepted.AcceptedAt == nil {
		t.Fatal("expected AcceptedAt to be set")
	}

	// Verify target member is now OWNER
	targetRoles, err := fx.assignments.Candidates(t.Context(), fx.scope, targetMemberID)
	if err != nil {
		t.Fatalf("failed checking target roles: %v", err)
	}
	var targetHasOwner bool
	for _, cand := range targetRoles {
		snap, _ := cand.Snapshot(fx.scope)
		if snap.Role == accessd.Owner && snap.EndedAt.IsZero() {
			targetHasOwner = true
		}
	}
	if !targetHasOwner {
		t.Fatal("expected target member to have active OWNER role")
	}

	// Verify initiator stepped down: OWNER ended, ADMIN active
	initRoles, err := fx.assignments.Candidates(t.Context(), fx.scope, fx.member)
	if err != nil {
		t.Fatalf("failed checking initiator roles: %v", err)
	}
	var initHasActiveOwner bool
	var initHasActiveAdmin bool
	for _, cand := range initRoles {
		snap, _ := cand.Snapshot(fx.scope)
		if snap.Role == accessd.Owner && snap.EndedAt.IsZero() {
			initHasActiveOwner = true
		}
		if snap.Role == accessd.Admin && snap.EndedAt.IsZero() {
			initHasActiveAdmin = true
		}
	}
	if initHasActiveOwner {
		t.Fatal("expected initiator OWNER role to be ended")
	}
	if !initHasActiveAdmin {
		t.Fatal("expected initiator to have active ADMIN role")
	}
}

func TestOwnershipTransfer_Accept_WithoutStepDown(t *testing.T) {
	fx := transferFixture(t)
	targetMemberID, _ := createTransferMember(t, fx, false)

	now := time.Now().UTC()
	opID := onboardingID(t)
	transferID := onboardingID(t)

	// Propose transfer without stepping down
	_, _, err := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, d.OwnershipTransfer{
		TransferID:            transferID,
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    targetMemberID,
		StepDownToAdmin:       false,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           opID,
	})
	if err != nil {
		t.Fatalf("failed proposing transfer: %v", err)
	}

	// Accept
	_, _, err = fx.store.Accept(t.Context(), fx.scope, fx.actors[targetMemberID], targetMemberID, transferID, onboardingID(t))
	if err != nil {
		t.Fatalf("unexpected error accepting transfer: %v", err)
	}

	// Both initiator and target should now have active OWNER roles
	for _, memberID := range []string{fx.member, targetMemberID} {
		cands, err := fx.assignments.Candidates(t.Context(), fx.scope, memberID)
		if err != nil {
			t.Fatal(err)
		}
		var isOwner bool
		for _, c := range cands {
			snap, _ := c.Snapshot(fx.scope)
			if snap.Role == accessd.Owner && snap.EndedAt.IsZero() {
				isOwner = true
			}
		}
		if !isOwner {
			t.Fatalf("expected member %s to have active OWNER role", memberID)
		}
	}
}

func TestOwnershipTransfer_CancelAndDecline(t *testing.T) {
	fx := transferFixture(t)
	targetMemberID1, _ := createTransferMember(t, fx, false)
	targetMemberID2, _ := createTransferMember(t, fx, false)

	now := time.Now().UTC()

	// 1. Propose and cancel by initiator
	tr1 := onboardingID(t)
	_, _, err := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, d.OwnershipTransfer{
		TransferID:            tr1,
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    targetMemberID1,
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           onboardingID(t),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cancel by initiator
	resCancel, _, err := fx.store.Cancel(t.Context(), fx.scope, fx.actors[fx.member], fx.member, tr1, onboardingID(t))
	if err != nil {
		t.Fatalf("unexpected cancel error: %v", err)
	}
	if resCancel.State != d.OwnershipTransferCancelled {
		t.Fatalf("expected CANCELLED, got %s", resCancel.State)
	}

	// 2. Propose new transfer and decline by target
	tr2 := onboardingID(t)
	_, _, err = fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, d.OwnershipTransfer{
		TransferID:            tr2,
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    targetMemberID2,
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           onboardingID(t),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Decline by target
	resDecline, _, err := fx.store.Cancel(t.Context(), fx.scope, fx.actors[targetMemberID2], targetMemberID2, tr2, onboardingID(t))
	if err != nil {
		t.Fatalf("unexpected decline error: %v", err)
	}
	if resDecline.State != d.OwnershipTransferDeclined {
		t.Fatalf("expected DECLINED, got %s", resDecline.State)
	}
}

func TestOwnershipTransfer_MembershipEndTrigger(t *testing.T) {
	fx := transferFixture(t)
	targetMemberID, _ := createTransferMember(t, fx, false)

	now := time.Now().UTC()
	tr := onboardingID(t)

	_, _, err := fx.store.Propose(t.Context(), fx.scope, fx.actor, fx.member, d.OwnershipTransfer{
		TransferID:            tr,
		OrgID:                 fx.scope.OrgID().String(),
		InitiatorMembershipID: fx.member,
		TargetMembershipID:    targetMemberID,
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(d.OwnershipTransferValidity),
		CreatedAt:             now,
		OperationID:           onboardingID(t),
	})
	if err != nil {
		t.Fatal(err)
	}

	// End target member's membership: trigger should set transfer to CANCELLED
	if err := unitSchemaExec(t, fx.legalFixtureState, `UPDATE org_memberships SET state = 'ENDED', end_reason = 'OFFBOARDED', ended_at = clock_timestamp(), revision = revision + 1 WHERE org_id = $1 AND id = $2`, fx.scope.OrgID().String(), targetMemberID); err != nil {
		t.Fatal(err)
	}

	// Attempt to accept must now fail
	_, _, err = fx.store.Accept(t.Context(), fx.scope, fx.actors[targetMemberID], targetMemberID, tr, onboardingID(t))
	if !errors.Is(err, access.ErrNoMembership) {
		t.Fatalf("ended member accepted: %v", err)
	}
	var state string
	if e := fx.f.Admin.QueryRow(t.Context(), `SELECT state FROM org_ownership_transfers WHERE org_id=$1 AND transfer_id=$2`, fx.scope.OrgID().String(), tr).Scan(&state); e != nil || state != "CANCELLED" {
		t.Fatal("membership end not projected", e)
	}
}

func transferPersonalActor(t *testing.T, fx transferFixtureState) app.CompanyActor {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	accounts := &account.Service{Clock: onboardingClock{now}, Store: accountpg.NewPGStore(fx.f.Admin), Hasher: accountdomain.SHA256Hasher{}, IDGen: jobs.NewUUIDv4, TokenGen: accountdomain.GenerateToken, KeyGen: accountdomain.GenerateAccountKey}
	key, e := accounts.CreateKeyAccount(t.Context(), "synthetic"+onboardingID(t)[:8])
	if e != nil {
		t.Fatal(e)
	}
	auth, e := accounts.LoginWithKey(t.Context(), key.Key)
	if e != nil {
		t.Fatal(e)
	}
	cs := &company.Service{Enabled: true, Store: &companypg.Store{Pool: fx.f.Runtime}, Hasher: accountdomain.SHA256Hasher{}, Prove: companypg.AccountProof(accounts), Now: func() time.Time { return now }, NewID: jobs.NewUUIDv4, NewSecret: accountdomain.GenerateToken}
	result, e := cs.Login(t.Context(), auth.Session.FamilyID, auth.Session.AccessToken, "")
	if e != nil {
		t.Fatal(e)
	}
	return app.CompanyActor{AccountID: result.Row.AccountID, SessionID: result.Row.ID}
}
