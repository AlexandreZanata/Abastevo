package domain

import (
	"encoding/json"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"strings"
	"testing"
	"time"
)

func retainedOwnership(t *testing.T) (kernel.OrgScope, OwnershipTransfer) {
	t.Helper()
	oid, e := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000009001")
	if e != nil {
		t.Fatal(e)
	}
	scope, e := kernel.NewOrgScope(oid)
	if e != nil {
		t.Fatal(e)
	}
	at := time.Date(2026, 11, 12, 12, 30, 0, 123456000, time.UTC)
	return scope, OwnershipTransfer{TransferID: "7b0e2a54-0000-4000-8000-000000009002", OrgID: oid.String(), InitiatorMembershipID: "7b0e2a54-0000-4000-8000-000000009003", TargetMembershipID: "7b0e2a54-0000-4000-8000-000000009004", StepDownToAdmin: true, State: OwnershipTransferPending, CreatedAt: at, ExpiresAt: at.Add(OwnershipTransferValidity), OperationID: "7b0e2a54-0000-4000-8000-000000009005"}
}
func TestOwnershipRetainedValuesValidateScopedFiniteHistory(t *testing.T) {
	scope, pending := retainedOwnership(t)
	if e := pending.Validate(scope); e != nil {
		t.Fatal(e)
	}
	instant := pending.CreatedAt.Add(time.Hour)
	for _, state := range []OwnershipTransferState{OwnershipTransferAccepted, OwnershipTransferCancelled, OwnershipTransferDeclined, OwnershipTransferExpired} {
		v := pending
		v.State = state
		switch state {
		case OwnershipTransferAccepted:
			v.AcceptedAt = &instant
		case OwnershipTransferCancelled:
			v.CancelledAt = &instant
		case OwnershipTransferDeclined:
			v.DeclinedAt = &instant
		}
		if e := v.Validate(scope); e != nil {
			t.Fatal("valid retained state refused", state, e)
		}
	}
	mutations := map[string]func(*OwnershipTransfer){
		"foreign scope":         func(v *OwnershipTransfer) { v.OrgID = v.TargetMembershipID },
		"malformed transfer":    func(v *OwnershipTransfer) { v.TransferID = "not-an-id" },
		"malformed initiator":   func(v *OwnershipTransfer) { v.InitiatorMembershipID = "" },
		"malformed target":      func(v *OwnershipTransfer) { v.TargetMembershipID = "00000000-0000-0000-0000-000000000000" },
		"malformed operation":   func(v *OwnershipTransfer) { v.OperationID = "NOT-CANONICAL" },
		"self target":           func(v *OwnershipTransfer) { v.TargetMembershipID = v.InitiatorMembershipID },
		"missing creation":      func(v *OwnershipTransfer) { v.CreatedAt = time.Time{} },
		"missing expiry":        func(v *OwnershipTransfer) { v.ExpiresAt = time.Time{} },
		"wrong validity":        func(v *OwnershipTransfer) { v.ExpiresAt = v.ExpiresAt.Add(time.Nanosecond) },
		"unknown state":         func(v *OwnershipTransfer) { v.State = "UNKNOWN" },
		"pending accepted":      func(v *OwnershipTransfer) { v.AcceptedAt = &instant },
		"accepted missing time": func(v *OwnershipTransfer) { v.State = OwnershipTransferAccepted },
		"accepted after expiry": func(v *OwnershipTransfer) { v.State = OwnershipTransferAccepted; v.AcceptedAt = &v.ExpiresAt },
		"accepted contradictory": func(v *OwnershipTransfer) {
			v.State = OwnershipTransferAccepted
			v.AcceptedAt = &instant
			v.CancelledAt = &instant
		},
		"cancelled missing time": func(v *OwnershipTransfer) { v.State = OwnershipTransferCancelled },
		"cancelled contradictory": func(v *OwnershipTransfer) {
			v.State = OwnershipTransferCancelled
			v.CancelledAt = &instant
			v.DeclinedAt = &instant
		},
		"declined missing time": func(v *OwnershipTransfer) { v.State = OwnershipTransferDeclined },
		"declined contradictory": func(v *OwnershipTransfer) {
			v.State = OwnershipTransferDeclined
			v.DeclinedAt = &instant
			v.AcceptedAt = &instant
		},
		"terminal predates proposal": func(v *OwnershipTransfer) {
			at := v.CreatedAt.Add(-time.Nanosecond)
			v.State = OwnershipTransferCancelled
			v.CancelledAt = &at
		},
		"invalid terminal": func(v *OwnershipTransfer) { at := time.Time{}; v.State = OwnershipTransferDeclined; v.DeclinedAt = &at },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			v := pending
			mutate(&v)
			if v.Validate(scope) == nil {
				t.Fatal("invalid retained history accepted")
			}
		})
	}
	if pending.Validate(kernel.OrgScope{}) != ErrScope {
		t.Fatal("missing isolation scope accepted")
	}
}
func TestOwnershipGenericFormattingNeverExportsParticipants(t *testing.T) {
	_, v := retainedOwnership(t)
	raw, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	for _, render := range []string{string(raw), fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), v.LogValue().String()} {
		for _, private := range []string{v.TransferID, v.OrgID, v.InitiatorMembershipID, v.TargetMembershipID, v.OperationID} {
			if strings.Contains(render, private) {
				t.Fatal("generic formatting exposed participant/source ID")
			}
		}
	}
}
