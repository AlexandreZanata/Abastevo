package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestOwnershipTransferDTOsRejectAmbiguousAuthorityAndNulls(t *testing.T) {
	for _, raw := range []string{
		`{"account_id":"synthetic"}`,
		`{"target_membership_id":null}`,
		`{"Target_membership_id":"7b0e2a54-0000-4000-8000-000000000001"}`,
		`{"schema_version":1,"target_membership_id":"7b0e2a54-0000-4000-8000-000000000001","unknown":"field"}`,
	} {
		var input ownershipTransferProposeRequest
		if json.Unmarshal([]byte(raw), &input) == nil {
			t.Fatalf("ambiguous or non-canonical transfer propose decoded: %s", raw)
		}
	}

	var input ownershipTransferProposeRequest
	valid := `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001001","target_membership_id":"7b0e2a54-0000-4000-8000-000000000002","step_down_to_admin":true}`
	if err := json.Unmarshal([]byte(valid), &input); err != nil {
		t.Fatalf("canonical transfer propose refused: %v", err)
	}
	if input.TargetMembershipID != "7b0e2a54-0000-4000-8000-000000000002" || !input.StepDownToAdmin || input.OperationID != "7b0e2a54-0000-4000-8000-000000001001" {
		t.Fatalf("fields lost in canonical decoding: %+v", input)
	}

	for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
		if strings.Contains(fmt.Sprintf(verb, input), "private-secret") {
			t.Fatal("DTO formatting leaked secret")
		}
	}
}

func TestOwnershipTransferReadDTO(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":1,"extra":"field"}`,
		`{"schema_version":null}`,
		`{"invalid":true}`,
	} {
		var input ownershipTransferReadRequest
		if json.Unmarshal([]byte(raw), &input) == nil {
			t.Fatalf("ambiguous read request accepted: %s", raw)
		}
	}
	var input ownershipTransferReadRequest
	if err := json.Unmarshal([]byte(`{}`), &input); err != nil {
		t.Fatalf("canonical read request refused: %v", err)
	}
}

func TestOwnershipTransferMappedErrors(t *testing.T) {
	tests := []struct {
		err  error
		want orgapi.Code
	}{
		{d.ErrOwnerRequiresPersonalAccount, orgapi.OwnerRequiresPersonal},
		{d.ErrTransferPending, orgapi.TransferPending},
		{d.ErrSelfTransferForbidden, orgapi.Conflict},
		{d.ErrTargetAlreadyOwner, orgapi.Conflict},
		{d.ErrTargetNotActive, orgapi.Conflict},
		{d.ErrTargetEmployeeNotActive, orgapi.Conflict},
		{d.ErrTransferNotFound, orgapi.NotFound},
		{d.ErrTransferNotPending, orgapi.Conflict},
		{d.ErrTransferExpired, orgapi.Conflict},
		{d.ErrNotTransferTarget, orgapi.Forbidden},
		{d.ErrNotTransferParticipant, orgapi.Forbidden},
		{d.ErrTransferOperationReused, orgapi.OperationReused},
		{d.ErrOrganizationNotActive, orgapi.OrgSuspended},
		{app.ErrUnavailable, orgapi.Unavailable},
		{errors.New("arbitrary SQL error"), orgapi.Unavailable},
	}

	for _, tt := range tests {
		got := ownershipTransferMapped(tt.err)
		code, ok := orgapi.Registered(got)
		if !ok || code != tt.want {
			t.Fatalf("error %v mapped to %v (code %v, ok %v), want code %v", tt.err, got, code, ok, tt.want)
		}
	}

	if ownershipTransferMapped(nil) != nil {
		t.Fatal("nil error did not map to nil")
	}
}

func TestOwnershipTransferRule(t *testing.T) {
	writeRule := ownershipTransferRule(true)
	if !writeRule.Write || !writeRule.Fresh || writeRule.RequireRevision || !writeRule.CompanyOnly || writeRule.Success != 200 {
		t.Fatalf("unexpected write rule: %+v", writeRule)
	}
	if writeRule.Permission != "org.ownership.transfer" {
		t.Fatalf("unexpected permission in write rule: %s", writeRule.Permission)
	}

	readRule := ownershipTransferRule(false)
	if readRule.Write || !readRule.Fresh || readRule.RequireRevision || !readRule.CompanyOnly || readRule.Success != 200 {
		t.Fatalf("unexpected read rule: %+v", readRule)
	}
}

func TestOwnershipTransferRecordsCheck(t *testing.T) {
	records := OwnershipTransferRecords{}
	oid, _ := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000001")
	scope, _ := kernel.NewOrgScope(oid)

	// Zero / unauthenticated subject must fail
	if err := records.Check(context.Background(), scope, access.Subject{}, access.Policy{}, permissions.ResourceFacts{OrgID: scope.OrgID()}, []permissions.Rule{}); err == nil {
		t.Fatal("expected error on unauthenticated subject")
	}

	// Invalid scope must fail
	if err := records.Check(context.Background(), kernel.OrgScope{}, access.Subject{}, access.Policy{}, permissions.ResourceFacts{}, nil); err == nil {
		t.Fatal("expected error on invalid scope")
	}
}

func TestOwnershipTransferView(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	tr := d.OwnershipTransfer{
		TransferID:            "transfer-123",
		OrgID:                 "org-1",
		InitiatorMembershipID: "mem-1",
		TargetMembershipID:    "mem-2",
		StepDownToAdmin:       true,
		State:                 d.OwnershipTransferPending,
		ExpiresAt:             now.Add(7 * 24 * time.Hour),
		CreatedAt:             now,
		OperationID:           "op-1",
	}
	view := ownershipTransferView(tr)
	if view["transfer_id"] != "transfer-123" || view["state"] != "PENDING" || view["step_down_to_admin"] != true {
		t.Fatalf("unexpected view: %+v", view)
	}
}
