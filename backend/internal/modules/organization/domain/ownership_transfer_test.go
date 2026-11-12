package domain_test

import (
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestValidateTransferProposal(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                 string
		initiatorID          string
		targetID             string
		targetHasOwner       bool
		targetIsManaged      bool
		targetActive         bool
		targetEmployeeActive bool
		wantErr              error
	}{
		{
			name:                 "valid proposal",
			initiatorID:          "m-initiator",
			targetID:             "m-target",
			targetHasOwner:       false,
			targetIsManaged:      false,
			targetActive:         true,
			targetEmployeeActive: true,
			wantErr:              nil,
		},
		{
			name:                 "self transfer forbidden",
			initiatorID:          "m-same",
			targetID:             "m-same",
			targetHasOwner:       false,
			targetIsManaged:      false,
			targetActive:         true,
			targetEmployeeActive: true,
			wantErr:              d.ErrSelfTransferForbidden,
		},
		{
			name:                 "target not active",
			initiatorID:          "m-initiator",
			targetID:             "m-target",
			targetHasOwner:       false,
			targetIsManaged:      false,
			targetActive:         false,
			targetEmployeeActive: true,
			wantErr:              d.ErrTargetNotActive,
		},
		{
			name:                 "target employee not active",
			initiatorID:          "m-initiator",
			targetID:             "m-target",
			targetHasOwner:       false,
			targetIsManaged:      false,
			targetActive:         true,
			targetEmployeeActive: false,
			wantErr:              d.ErrTargetEmployeeNotActive,
		},
		{
			name:                 "target is managed account",
			initiatorID:          "m-initiator",
			targetID:             "m-target",
			targetHasOwner:       false,
			targetIsManaged:      true,
			targetActive:         true,
			targetEmployeeActive: true,
			wantErr:              d.ErrOwnerRequiresPersonalAccount,
		},
		{
			name:                 "target already owner",
			initiatorID:          "m-initiator",
			targetID:             "m-target",
			targetHasOwner:       true,
			targetIsManaged:      false,
			targetActive:         true,
			targetEmployeeActive: true,
			wantErr:              d.ErrTargetAlreadyOwner,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := d.ValidateTransferProposal(
				tc.initiatorID, tc.targetID,
				tc.targetHasOwner, tc.targetIsManaged,
				tc.targetActive, tc.targetEmployeeActive,
			)
			if err != tc.wantErr {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestOwnershipTransfer_IsPendingAt(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tr := d.OwnershipTransfer{
		TransferID: "tr-1",
		State:      d.OwnershipTransferPending,
		ExpiresAt:  now.Add(1 * time.Hour),
	}

	if !tr.IsPendingAt(now) {
		t.Fatal("expected transfer to be pending at now")
	}
	if tr.IsPendingAt(now.Add(2 * time.Hour)) {
		t.Fatal("expected transfer to be expired 2 hours later")
	}

	tr.State = d.OwnershipTransferAccepted
	if tr.IsPendingAt(now) {
		t.Fatal("expected accepted transfer to not be pending")
	}
}
