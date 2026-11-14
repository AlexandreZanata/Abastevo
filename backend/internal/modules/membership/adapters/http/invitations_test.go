package http

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestInvitationDTOsRejectNullAliasAndNestedAuthority(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"schema_version":1,"Schema_version":1}`, `{"grants":null}`, `{"grants":[{"Role":"DRIVER","scope":"SELF"}]}`, `{"grants":[{"role":"DRIVER","scope":"SELF","account_id":"synthetic"}]}`, `{"schema_version":1,"schema_version":1}`, `{"employee_id":"synthetic","unknown":true}`} {
		var v invitationIssueRequest
		if json.Unmarshal([]byte(raw), &v) == nil {
			t.Fatal("ambiguous invitation DTO accepted")
		}
	}
	for _, raw := range []string{`{"limit":0}`, `{"limit":101}`, `{"limit":null}`, `{"cursor":null}`, `{"state":null}`, `{"LIMIT":1}`} {
		var v invitationListRequest
		if json.Unmarshal([]byte(raw), &v) == nil {
			t.Fatal("ambiguous page accepted")
		}
	}
	var value invitationIssueRequest
	if e := json.Unmarshal([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001001","employee_id":"7b0e2a54-0000-4000-8000-000000000501","grants":[{"role":"DRIVER","scope":"SELF"}]}`), &value); e != nil {
		t.Fatal(e)
	}
	for _, render := range []string{fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
		if strings.Contains(render, value.EmployeeID) {
			t.Fatal("generic request formatting exposed selected person")
		}
	}
	for _, raw := range []string{`{"revision":null}`, `{"Revision":1}`, `{"schema_version":1,"revision":1,"code":"synthetic"}`} {
		var v invitationRevisionRequest
		if json.Unmarshal([]byte(raw), &v) == nil {
			t.Fatal("ambiguous replacement accepted")
		}
	}
}
