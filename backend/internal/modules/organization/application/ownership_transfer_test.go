package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

type mockTransferRepo struct {
	proposeFn func(ctx context.Context, scope kernel.OrgScope, t d.OwnershipTransfer) (d.OwnershipTransfer, bool, error)
	acceptFn  func(ctx context.Context, scope kernel.OrgScope, transferID string, targetMemberID string, opID string) (d.OwnershipTransfer, bool, error)
	cancelFn  func(ctx context.Context, scope kernel.OrgScope, transferID string, actorMemberID string, opID string, isInitiator bool) (d.OwnershipTransfer, bool, error)
	getFn     func(ctx context.Context, scope kernel.OrgScope) (d.OwnershipTransfer, error)
}

func (m mockTransferRepo) Propose(ctx context.Context, scope kernel.OrgScope, t d.OwnershipTransfer) (d.OwnershipTransfer, bool, error) {
	if m.proposeFn != nil {
		return m.proposeFn(ctx, scope, t)
	}
	return t, false, nil
}

func (m mockTransferRepo) Accept(ctx context.Context, scope kernel.OrgScope, transferID string, targetMemberID string, opID string) (d.OwnershipTransfer, bool, error) {
	if m.acceptFn != nil {
		return m.acceptFn(ctx, scope, transferID, targetMemberID, opID)
	}
	return d.OwnershipTransfer{TransferID: transferID, State: d.OwnershipTransferAccepted}, false, nil
}

func (m mockTransferRepo) Cancel(ctx context.Context, scope kernel.OrgScope, transferID string, actorMemberID string, opID string, isInitiator bool) (d.OwnershipTransfer, bool, error) {
	if m.cancelFn != nil {
		return m.cancelFn(ctx, scope, transferID, actorMemberID, opID, isInitiator)
	}
	state := d.OwnershipTransferDeclined
	if isInitiator {
		state = d.OwnershipTransferCancelled
	}
	return d.OwnershipTransfer{TransferID: transferID, State: state}, false, nil
}

func (m mockTransferRepo) GetCurrent(ctx context.Context, scope kernel.OrgScope) (d.OwnershipTransfer, error) {
	if m.getFn != nil {
		return m.getFn(ctx, scope)
	}
	return d.OwnershipTransfer{}, nil
}

func TestOwnershipTransferService_Propose(t *testing.T) {
	t.Parallel()

	oid, _ := kernel.ParseOrgID("00000000-0000-0000-0000-000000000001")
	scope, _ := kernel.NewOrgScope(oid)
	now := time.Now()

	svc := app.OwnershipTransferService{
		Enabled: true,
		Repository: mockTransferRepo{
			proposeFn: func(ctx context.Context, s kernel.OrgScope, tr d.OwnershipTransfer) (d.OwnershipTransfer, bool, error) {
				return tr, false, nil
			},
		},
		NewID: func() (string, error) {
			return "00000000-0000-0000-0000-000000000099", nil
		},
		Now: func() time.Time {
			return now
		},
	}

	// Self-transfer fails fast
	_, err := svc.Propose(context.Background(), scope, app.ProposeTransferInput{
		OrgID:             scope.OrgID().String(),
		InitiatorMemberID: "member-1",
		TargetMemberID:    "member-1",
	})
	if err != d.ErrSelfTransferForbidden {
		t.Fatalf("expected ErrSelfTransferForbidden, got %v", err)
	}

	// Normal propose succeeds
	res, err := svc.Propose(context.Background(), scope, app.ProposeTransferInput{
		OrgID:             scope.OrgID().String(),
		InitiatorMemberID: "member-1",
		TargetMemberID:    "member-2",
		StepDownToAdmin:   true,
		OperationID:       "00000000-0000-0000-0000-000000000088",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Transfer.TransferID != "00000000-0000-0000-0000-000000000099" {
		t.Fatalf("expected transfer ID 00000000-0000-0000-0000-000000000099, got %s", res.Transfer.TransferID)
	}
	if res.Transfer.State != d.OwnershipTransferPending {
		t.Fatalf("expected state PENDING, got %s", res.Transfer.State)
	}
	if !res.Transfer.ExpiresAt.Equal(now.Add(d.OwnershipTransferValidity)) {
		t.Fatalf("expected expiry %v, got %v", now.Add(d.OwnershipTransferValidity), res.Transfer.ExpiresAt)
	}
}

func TestOwnershipTransferService_AcceptAndCancel(t *testing.T) {
	t.Parallel()

	oid, _ := kernel.ParseOrgID("00000000-0000-0000-0000-000000000001")
	scope, _ := kernel.NewOrgScope(oid)
	svc := app.OwnershipTransferService{
		Enabled:    true,
		Repository: mockTransferRepo{},
	}

	resAccept, err := svc.Accept(context.Background(), scope, app.AcceptTransferInput{
		OrgID:          scope.OrgID().String(),
		TransferID:     "tr-1",
		TargetMemberID: "target-1",
		OperationID:    "op-1",
	})
	if err != nil {
		t.Fatalf("unexpected accept error: %v", err)
	}
	if resAccept.Transfer.State != d.OwnershipTransferAccepted {
		t.Fatalf("expected state ACCEPTED, got %s", resAccept.Transfer.State)
	}

	resCancel, err := svc.Cancel(context.Background(), scope, app.CancelTransferInput{
		OrgID:         scope.OrgID().String(),
		TransferID:    "tr-1",
		ActorMemberID: "target-1",
		OperationID:   "op-2",
		IsInitiator:   false,
	})
	if err != nil {
		t.Fatalf("unexpected cancel error: %v", err)
	}
	if resCancel.Transfer.State != d.OwnershipTransferDeclined {
		t.Fatalf("expected state DECLINED, got %s", resCancel.Transfer.State)
	}
}
