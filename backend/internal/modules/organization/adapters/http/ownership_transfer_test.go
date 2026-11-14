package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/getkin/kin-openapi/openapi3"
)

func TestOwnershipTransferDTOsRejectAmbiguousAuthorityAndNulls(t *testing.T) {
	valid := `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001001","target_membership_id":"7b0e2a54-0000-4000-8000-000000000602","step_down_to_admin":false}`
	for _, raw := range []string{`null`, `[]`, `{}`, `{"account_id":"synthetic"}`, `{"step_down_to_admin":false,"target_membership_id":null}`, `{"step_down_to_admin":false,"Target_membership_id":"7b0e2a54-0000-4000-8000-000000000602"}`, strings.Replace(valid, `"step_down_to_admin":false`, `"step_down_to_admin":null`, 1), strings.Replace(valid, `,"step_down_to_admin":false`, "", 1), strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1), strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"SCHEMA_VERSION":1`, 1), strings.TrimSuffix(valid, "}") + `,"unknown":true}`} {
		var input ownershipTransferProposeRequest
		if json.Unmarshal([]byte(raw), &input) == nil {
			t.Fatal("ambiguous transfer proposal decoded", raw)
		}
	}
	var input ownershipTransferProposeRequest
	if e := json.Unmarshal([]byte(valid), &input); e != nil || input.StepDownToAdmin == nil || *input.StepDownToAdmin || input.TargetMembershipID != "7b0e2a54-0000-4000-8000-000000000602" {
		t.Fatal("explicit co-ownership intent lost", e)
	}
	for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
		formatted := fmt.Sprintf(verb, input)
		if strings.Contains(formatted, input.OperationID) || strings.Contains(formatted, input.TargetMembershipID) {
			t.Fatal("proposal formatting leaked private identifiers")
		}
	}
	encoded, e := json.Marshal(input)
	if e != nil || strings.Contains(string(encoded), input.OperationID) || strings.Contains(string(encoded), input.TargetMembershipID) {
		t.Fatal("proposal JSON bypassed redaction", e)
	}
}

func TestOwnershipTransferParticipantDTOsRejectAliasesDuplicatesAndNulls(t *testing.T) {
	valid := `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001001","transfer_id":"7b0e2a54-0000-4000-8000-000000000901"}`
	for _, fresh := range []func() any{func() any { return &ownershipTransferAcceptRequest{} }, func() any { return &ownershipTransferCancelRequest{} }} {
		if e := json.Unmarshal([]byte(valid), fresh()); e != nil {
			t.Fatal("canonical participant command refused", e)
		}
		for _, raw := range []string{`null`, `[]`, strings.Replace(valid, `"transfer_id":`, `"TRANSFER_ID":`, 1), strings.Replace(valid, `"schema_version":1`, `"schema_version":null`, 1), strings.Replace(valid, `"operation_id":"7b0e2a54-0000-4000-8000-000000001001"`, `"operation_id":null`, 1), strings.Replace(valid, `"transfer_id":"7b0e2a54-0000-4000-8000-000000000901"`, `"transfer_id":null`, 1), strings.Replace(valid, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1), strings.TrimSuffix(valid, "}") + `,"is_initiator":true}`, strings.TrimSuffix(valid, "}") + `,"actor_membership_id":"7b0e2a54-0000-4000-8000-000000000601"}`} {
			if json.Unmarshal([]byte(raw), fresh()) == nil {
				t.Fatal("ambiguous participant command decoded", raw)
			}
		}
	}
}

func TestOwnershipTransferReadDTO(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"schema_version":1}`, `{"transfer_id":null}`, `{"transfer_id":""}`, `{"transfer_id":"not-an-id"}`, `{"TRANSFER_ID":"7b0e2a54-0000-4000-8000-000000000901"}`, `{"transfer_id":"7b0e2a54-0000-4000-8000-000000000901","transfer_id":"7b0e2a54-0000-4000-8000-000000000901"}`} {
		var input ownershipTransferReadRequest
		if json.Unmarshal([]byte(raw), &input) == nil {
			t.Fatal("ambiguous read selector accepted", raw)
		}
	}
	for _, raw := range []string{`{}`, `{"transfer_id":"7b0e2a54-0000-4000-8000-000000000901"}`} {
		var input ownershipTransferReadRequest
		if e := json.Unmarshal([]byte(raw), &input); e != nil {
			t.Fatal("canonical read selector refused", e)
		}
	}
}

func TestOwnershipTransferMappedErrors(t *testing.T) {
	tests := []struct {
		err  error
		want orgapi.Code
	}{
		{d.ErrOwnerRequiresPersonalAccount, orgapi.OwnerRequiresPersonal}, {d.ErrTransferPending, orgapi.TransferPending},
		{d.ErrSelfTransferForbidden, orgapi.Conflict}, {d.ErrTargetAlreadyOwner, orgapi.Conflict}, {permissions.ErrAssignmentConflict, orgapi.Conflict}, {d.ErrTargetNotActive, orgapi.Conflict}, {d.ErrTargetEmployeeNotActive, orgapi.Conflict},
		{d.ErrTransferNotFound, orgapi.NotFound}, {d.ErrTransferNotPending, orgapi.Conflict}, {d.ErrTransferExpired, orgapi.Conflict},
		{d.ErrNotTransferTarget, orgapi.NotFound}, {d.ErrNotTransferParticipant, orgapi.NotFound},
		{d.ErrTransferOperationReused, orgapi.OperationReused}, {d.ErrOrganizationNotActive, orgapi.OrgSuspended},
		{app.ErrUnavailable, orgapi.Unavailable}, {errors.New("private SQL details and synthetic identifiers"), orgapi.Unavailable},
	}
	for _, test := range tests {
		got := ownershipTransferMapped(test.err)
		code, ok := orgapi.Registered(got)
		if !ok || code != test.want {
			t.Fatal("unsafe ownership error mapping", test.err, got)
		}
	}
	if ownershipTransferMapped(nil) != nil {
		t.Fatal("nil error mapped as a failure")
	}
}

func TestOwnershipTransferRulesRetainCurrentParticipantAuthority(t *testing.T) {
	for _, test := range []struct {
		policy access.Policy
		write  bool
	}{
		{app.OwnershipTransferProposePolicy(), true}, {app.OwnershipTransferReadPolicy(), false}, {app.OwnershipTransferAcceptPolicy(), true}, {app.OwnershipTransferCancelPolicy(), true},
	} {
		rule := ownershipTransferRule(test.policy, test.write)
		if rule.Permission != string(test.policy.Permission) || rule.Write != test.write || !rule.Fresh || rule.RequireRevision || !rule.CompanyOnly || rule.Success != 200 {
			t.Fatal("ownership guard drift", rule)
		}
	}
}

func TestOwnershipTransferRecordsRejectForgedSubjects(t *testing.T) {
	org, e := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if e != nil {
		t.Fatal(e)
	}
	scope, e := kernel.NewOrgScope(org)
	if e != nil {
		t.Fatal(e)
	}
	records := OwnershipTransferRecords{}
	for _, policy := range []access.Policy{{}, app.OwnershipTransferProposePolicy(), app.OwnershipTransferAcceptPolicy(), app.OwnershipTransferReadPolicy(), app.OwnershipTransferCancelPolicy()} {
		if e := records.Check(context.Background(), scope, access.Subject{}, policy, permissions.ResourceFacts{OrgID: org, ID: org.String()}, nil); e == nil {
			t.Fatal("empty authority accepted as transfer participation")
		}
	}
	if e := records.Check(context.Background(), kernel.OrgScope{}, access.Subject{}, access.Policy{}, permissions.ResourceFacts{}, nil); e == nil {
		t.Fatal("invalid transfer scope accepted")
	}
}

func TestOwnershipTransferActualWireViewMatchesOpenAPI(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source unavailable")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", "..", "contracts", "openapi", "v1.yaml")
	doc, e := openapi3.NewLoader().LoadFromFile(path)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 11, 12, 12, 0, 0, 123456000, time.UTC)
	base := d.OwnershipTransfer{TransferID: "7b0e2a54-0000-4000-8000-000000000901", OrgID: "7b0e2a54-0000-4000-8000-000000000301", InitiatorMembershipID: "7b0e2a54-0000-4000-8000-000000000601", TargetMembershipID: "7b0e2a54-0000-4000-8000-000000000602", StepDownToAdmin: true, ExpiresAt: now.Add(d.OwnershipTransferValidity), CreatedAt: now, OperationID: "7b0e2a54-0000-4000-8000-000000001001"}
	for _, state := range []d.OwnershipTransferState{d.OwnershipTransferPending, d.OwnershipTransferAccepted, d.OwnershipTransferCancelled, d.OwnershipTransferDeclined, d.OwnershipTransferExpired} {
		transfer := base
		transfer.State = state
		completed := ""
		switch state {
		case d.OwnershipTransferAccepted:
			transfer.AcceptedAt = &now
			completed = "accepted_at"
		case d.OwnershipTransferCancelled:
			transfer.CancelledAt = &now
			completed = "cancelled_at"
		case d.OwnershipTransferDeclined:
			transfer.DeclinedAt = &now
			completed = "declined_at"
		}
		wire := ownershipTransferView(transfer)
		if e := doc.Components.Schemas["EnterpriseOwnershipTransferView"].Value.VisitJSON(wire); e != nil {
			t.Fatal("actual transfer wire violates schema", state, e)
		}
		if wire["created_at"] != now.Format(time.RFC3339Nano) || wire["data_mode"] != "SYNTHETIC" {
			t.Fatal("private transfer precision or source marker lost")
		}
		if completed != "" && wire[completed] != now.Format(time.RFC3339Nano) {
			t.Fatal("terminal timestamp precision lost", state)
		}
		if _, exists := wire["completed_at"]; exists {
			t.Fatal("unimplemented completion field leaked")
		}
	}
}
