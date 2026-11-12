package http

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestEmployeeDTOsRefuseAuthorityAmbiguityAndPrivateFormatting(t *testing.T) {
	for _, raw := range []string{`{"account_id":"synthetic"}`, `{"state":"ACTIVE"}`, `{"profile_revision":1}`, `{"Display_Name":"alias"}`, `{"display_name":null}`, `{"work_email":null}`, `{"display_name":"a","display_name":"b"}`, `{"home_unit_id":null}`, `{"permission":"idn.employee.manage"}`, `{"cpf":"synthetic"}`, `{"team_ids":[]}`, `{"revision":1}`} {
		var v createRequest
		if json.Unmarshal([]byte(raw), &v) == nil {
			t.Fatal("ambiguous or privileged creation DTO accepted")
		}
	}
	raw := []byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001002","display_name":"Synthetic Private Employee","work_email":"private@example.invalid","allow_possible_duplicate":true}`)
	var create createRequest
	if json.Unmarshal(raw, &create) != nil || create.DisplayName != "Synthetic Private Employee" || !create.AllowPossibleDuplicate || create.CommandFields().SchemaVersion != 1 {
		t.Fatal("create DTO")
	}
	var change changeRequest
	if json.Unmarshal([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001002","revision":3,"display_name":"Synthetic Private Employee"}`), &change) != nil || change.Revision == nil || *change.Revision != 3 || change.DisplayName != "Synthetic Private Employee" {
		t.Fatal("change revision concealed by embedding")
	}
	for _, value := range []any{create, create.profileFields, change, completeRequest{change}, matchRequest{DisplayName: "Synthetic Private Employee", WorkEmail: "private@example.invalid"}} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "Synthetic") || strings.Contains(string(raw), "private@example.invalid") {
			t.Fatal("JSON private fields")
		}
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			if strings.Contains(fmt.Sprintf(verb, value), "Synthetic") || strings.Contains(fmt.Sprintf(verb, value), "private@example.invalid") {
				t.Fatal("private formatter")
			}
		}
	}
	if mapped(app.ErrDuplicateReview) != orgapi.DuplicateReviewRequired || mapped(app.ErrEmployeeOperationReused) != orgapi.OperationReused || mapped(d.ErrConflict) != orgapi.Conflict || mapped(fmt.Errorf("SQL synthetic private")) != orgapi.Unavailable {
		t.Fatal("unsafe error mapping")
	}
}
