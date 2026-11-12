package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestCostCentreDTOsRejectAuthorityAndRedactHistoricalCodes(t *testing.T) {
	for _, raw := range []string{`{"account_id":"synthetic"}`, `{"code_key":"synthetic"}`, `{"state":"ACTIVE"}`, `{"usage":{}}`, `{"Code":"alias"}`, `{"code":null}`, `{"code":"A","code":"B"}`, `{"time_zone":"UTC"}`} {
		var input costCentreCreateRequest
		if json.Unmarshal([]byte(raw), &input) == nil {
			t.Fatal("ambiguous or authoritative cost-centre input accepted")
		}
	}
	var input costCentreCreateRequest
	if json.Unmarshal([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001006","name":"Synthetic private centre","code":"PRIVATE-CODE"}`), &input) != nil || input.Name != "Synthetic private centre" || input.Code != "PRIVATE-CODE" || input.CommandFields().SchemaVersion != 1 {
		t.Fatal("canonical cost-centre input lost")
	}
	revision := int64(1)
	change := costCentreChangeRequest{unitRevisionCommand: unitRevisionCommand{Revision: &revision}, costCentreFields: input.costCentreFields}
	lookup := costCentreCodeRequest{Code: input.Code}
	for _, v := range []any{input, input.costCentreFields, change, lookup} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			value := fmt.Sprintf(verb, v)
			if strings.Contains(value, "Synthetic") || strings.Contains(value, "PRIVATE-CODE") {
				t.Fatal("private centre DTO formatting leaked")
			}
		}
		raw, err := json.Marshal(v)
		if err != nil || strings.Contains(string(raw), "Synthetic") || strings.Contains(string(raw), "PRIVATE-CODE") {
			t.Fatal("private centre DTO JSON leaked", err)
		}
	}
	var transition costCentreTransitionRequest
	if json.Unmarshal([]byte(`{"revision":1,"code":"ignored"}`), &transition) == nil {
		t.Fatal("transition accepted a code correction")
	}
	if !errors.Is(costCentreMapped(app.ErrCostCentreCodeReserved), orgapi.ValidationFailed) {
		t.Fatal("reserved code did not map to safe field error")
	}
}
