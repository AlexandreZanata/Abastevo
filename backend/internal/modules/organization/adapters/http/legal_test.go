package http

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestLegalDTOsRejectAmbiguousAuthorityNullsAndAliases(t *testing.T) {
	for _, raw := range []string{`{"family_id":"synthetic"}`, `{"account_id":"synthetic"}`, `{"state":"VERIFIED"}`, `{"legal_name":null}`, `{"Kind":"COMPANY"}`, `{"kind":"COMPANY","Kind":"NATURAL_PERSON"}`, `{"establishment":null}`, `{"establishment":{"cnpj":null}}`} {
		var input legalCreateRequest
		if json.Unmarshal([]byte(raw), &input) == nil {
			t.Fatal("ambiguous legal command decoded")
		}
	}
	var input legalCreateRequest
	if err := json.Unmarshal([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001001","kind":"COMPANY","legal_name":"Synthetic company","identifier":"EXEMPL01000170","establishment":{"declared_role":"NOT_INFORMED","trade_name":"Synthetic trade"}}`), &input); err != nil {
		t.Fatal("canonical legal command refused", err)
	}
	if input.Establishment == nil || input.Kind != d.Company || input.Establishment.Role != d.NotInformed || input.CommandFields().SchemaVersion != 1 {
		t.Fatal("command fields lost in strict decoding")
	}
	for _, value := range []any{input, *input.Establishment} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			if strings.Contains(fmt.Sprintf(verb, value), "Synthetic") {
				t.Fatal("private legal DTO formatting leaked")
			}
		}
		raw, err := json.Marshal(value)
		if err != nil || strings.Contains(string(raw), "Synthetic") {
			t.Fatal("private legal DTO JSON leaked", err)
		}
	}
	var read legalReadRequest
	if json.Unmarshal([]byte(`{"reveal_identifier":true}`), &read) == nil {
		t.Fatal("client selected sensitive read policy")
	}
	var edit establishmentChangeRequest
	if json.Unmarshal([]byte(`{"cnpj":"EXEMPL01000170"}`), &edit) == nil {
		t.Fatal("client changed immutable establishment identifier")
	}
}
func TestLegalRuleAndFieldMappingRemainSourceOwned(t *testing.T) {
	rule := legalRule(true, true, true, 201)
	if rule.Permission != "org.legal_entity.manage" || !rule.CompanyOnly || !rule.RequireRevision || !rule.Fresh || !rule.Write {
		t.Fatal("unguarded legal rule")
	}
	if code, ok := orgapi.Registered(legalMapped(d.ErrInvalidCPF, "identifier")); !ok || code != orgapi.ValidationFailed {
		t.Fatal("CPF refusal lost")
	}
	if code, ok := orgapi.Registered(legalMapped(d.ErrInvalidCNPJ, "cnpj")); !ok || code != orgapi.ValidationFailed {
		t.Fatal("CNPJ refusal lost")
	}
}
