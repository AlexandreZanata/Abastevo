package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestUnitDTOsRejectAuthorityNullsAliasesAndPrivateFormatting(t *testing.T) {
	for _, raw := range []string{`{"account_id":"synthetic"}`, `{"state":"ACTIVE"}`, `{"active_units":2}`, `{"usage":{"assets":0}}`, `{"Name":"alias"}`, `{"name":null}`, `{"establishment_id":null}`, `{"name":"a","name":"b"}`} {
		var input unitCreateRequest
		if json.Unmarshal([]byte(raw), &input) == nil {
			t.Fatal("ambiguous or authoritative unit input accepted")
		}
	}
	raw := []byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001005","name":"Synthetic private unit","code":"PRIVATE","time_zone":"America/Cuiaba"}`)
	var input unitCreateRequest
	if json.Unmarshal(raw, &input) != nil || input.Name != "Synthetic private unit" || input.CommandFields().SchemaVersion != 1 {
		t.Fatal("canonical unit fields lost")
	}
	revision := int64(1)
	change := unitChangeRequest{unitRevisionCommand: unitRevisionCommand{Revision: &revision}, unitFields: input.unitFields}
	for _, v := range []any{input, input.unitFields, change} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			if strings.Contains(fmt.Sprintf(verb, v), "Synthetic") {
				t.Fatal("private unit DTO formatting leaked")
			}
		}
		raw, err := json.Marshal(v)
		if err != nil || strings.Contains(string(raw), "Synthetic") {
			t.Fatal("private unit DTO JSON leaked")
		}
	}
	var transition unitTransitionRequest
	if json.Unmarshal([]byte(`{"revision":1,"name":"ignored"}`), &transition) == nil {
		t.Fatal("transition metadata accepted")
	}
	if !errors.Is(unitMapped(app.UnitInUse(d.UnitUsage{Assets: 1})), orgapi.StructureInUse) || unitMapped(d.ErrStructureInUse) != orgapi.Unavailable {
		t.Fatal("unsafe usage count mapping")
	}
	if !errors.Is(unitMapped(app.DuplicateUnit("code")), orgapi.ValidationFailed) || unitMapped(d.ErrLastUnit) != orgapi.LastUnit {
		t.Fatal("unit refusal mapping lost")
	}
}
