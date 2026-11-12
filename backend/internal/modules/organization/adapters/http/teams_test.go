package http

import (
	"encoding/json"
	"fmt"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"strings"
	"testing"
)

func TestTeamDTOsRejectAuthorityAndRetainPrivateFormatting(t *testing.T) {
	for _, raw := range []string{`{"Name":"alias"}`, `{"name":null}`, `{"unit_id":null}`, `{"employee_id":"synthetic"}`, `{"members":[]}`, `{"roles":[]}`, `{"state":"ACTIVE"}`, `{"name":"one","name":"two"}`, `{"usage":{"employees":0}}`} {
		var v teamCreateRequest
		if json.Unmarshal([]byte(raw), &v) == nil {
			t.Fatal("ambiguous or privileged team DTO")
		}
	}
	var v teamCreateRequest
	if json.Unmarshal([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001007","name":"Synthetic Private Crew","unit_id":""}`), &v) != nil || v.Name != "Synthetic Private Crew" {
		t.Fatal("team DTO lost fields")
	}
	for _, value := range []any{v, v.teamFields, teamChangeRequest{teamFields: v.teamFields}} {
		raw, _ := json.Marshal(value)
		if strings.Contains(fmt.Sprintf("%+v %#v %s", value, value, raw), "Synthetic") {
			t.Fatal("private team formatting")
		}
	}
	var member teamMemberRequest
	if json.Unmarshal([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001007","revision":2}`), &member) != nil || member.Revision == nil || *member.Revision != 2 {
		t.Fatal("link revision DTO")
	}
	if json.Unmarshal([]byte(`{"employee_id":"synthetic"}`), &member) == nil {
		t.Fatal("employee accepted in link body")
	}
	if teamMapped(app.ErrTeamNameDuplicate) == orgapi.Unavailable || teamMapped(d.ErrConditions) != orgapi.Conflict {
		t.Fatal("team refusal mapping")
	}
}
