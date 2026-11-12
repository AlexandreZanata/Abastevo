package apicontract

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func enterpriseContractObject(t *testing.T, example any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(example)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestOwnershipTransferContractRequestsAndWireViews(t *testing.T) {
	document := loadSpec(t)
	for _, name := range []string{
		"EnterpriseOwnershipTransferProposeRequest", "EnterpriseOwnershipTransferAcceptRequest",
		"EnterpriseOwnershipTransferCancelRequest", "EnterpriseOwnershipTransferReadRequest",
		"EnterpriseOwnershipTransferView", "EnterpriseOwnershipTransferResult",
	} {
		t.Run(name, func(t *testing.T) {
			ref := document.Components.Schemas[name]
			if ref == nil || ref.Value == nil || ref.Value.Example == nil {
				t.Fatal("missing ownership schema/example", name)
			}
			if err := ref.Value.VisitJSON(enterpriseContractObject(t, ref.Value.Example)); err != nil {
				t.Fatal("invalid ownership example", err)
			}
			for _, field := range []string{"account_id", "TRANSFER_ID"} {
				value := enterpriseContractObject(t, ref.Value.Example)
				value[field] = "7b0e2a54-0000-4000-8000-000000000201"
				if ref.Value.VisitJSON(value) == nil {
					t.Fatal("unknown or aliased ownership field accepted", field)
				}
			}
			if strings.HasSuffix(name, "Request") {
				for _, field := range ref.Value.Required {
					value := enterpriseContractObject(t, ref.Value.Example)
					delete(value, field)
					if ref.Value.VisitJSON(value) == nil {
						t.Fatal("omitted command intent accepted", field)
					}
				}
				for field := range ref.Value.Properties {
					value := enterpriseContractObject(t, ref.Value.Example)
					value[field] = nil
					if ref.Value.VisitJSON(value) == nil {
						t.Fatal("null ownership intent accepted", field)
					}
				}
			}
		})
	}
	proposal := document.Components.Schemas["EnterpriseOwnershipTransferProposeRequest"].Value
	intent := enterpriseContractObject(t, proposal.Example)
	intent["step_down_to_admin"] = false
	if err := proposal.VisitJSON(intent); err != nil {
		t.Fatal("explicit co-ownership intent rejected", err)
	}
	delete(intent, "step_down_to_admin")
	if proposal.VisitJSON(intent) == nil {
		t.Fatal("omitted step-down choice accepted")
	}
	read := document.Components.Schemas["EnterpriseOwnershipTransferReadRequest"].Value
	if err := read.VisitJSON(map[string]any{}); err != nil {
		t.Fatal("current pending selector rejected", err)
	}
	if err := read.VisitJSON(map[string]any{"transfer_id": "7b0e2a54-0000-4000-8000-000000003001"}); err != nil {
		t.Fatal("retained transfer selector rejected", err)
	}
	if read.VisitJSON(map[string]any{"transfer_id": ""}) == nil {
		t.Fatal("explicit empty transfer selector accepted")
	}
	view := document.Components.Schemas["EnterpriseOwnershipTransferView"].Value
	for state, date := range map[string]string{"ACCEPTED": "accepted_at", "CANCELLED": "cancelled_at", "DECLINED": "declined_at"} {
		wire := enterpriseContractObject(t, view.Example)
		wire["state"] = state
		wire["data_mode"] = "SYNTHETIC"
		wire[date] = "2026-11-12T12:30:00.123456Z"
		if err := view.VisitJSON(wire); err != nil {
			t.Fatal("terminal ownership wire view rejected", state, err)
		}
		wire["completed_at"] = wire[date]
		if view.VisitJSON(wire) == nil {
			t.Fatal("unimplemented completion field accepted")
		}
	}
	for _, suffix := range []string{"propose", "read", "accept", "cancel"} {
		op := document.Paths.Value("/v1/orgs/{org_id}/ownership-transfer/" + suffix).Post
		request := op.RequestBody.Value.Content.Get("application/json")
		if err := request.Schema.Value.VisitJSON(enterpriseContractObject(t, request.Example)); err != nil {
			t.Fatal("invalid ownership operation request example", suffix, err)
		}
		response := op.Responses.Value("200").Value.Content.Get("application/json")
		if err := response.Schema.Value.VisitJSON(enterpriseContractObject(t, response.Example)); err != nil {
			t.Fatal("invalid ownership operation response example", suffix, err)
		}
	}
	failure := enterpriseContractObject(t, document.Components.Schemas["EnterpriseError"].Value.Example)
	failure["error"].(map[string]any)["code"] = "TRANSFER_PENDING"
	if err := document.Components.Schemas["EnterpriseError"].Value.VisitJSON(failure); err != nil {
		t.Fatal("registered pending-transfer conflict rejected", err)
	}
}

func TestOwnershipTransferActualRouterRejectsGuardDrift(t *testing.T) {
	document, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, document); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"propose", "read", "accept", "cancel"} {
		path := "/v1/orgs/{org_id}/ownership-transfer/" + suffix
		for _, drift := range []string{"cookie", "csrf", "permission", "write", "fresh", "selector", "bare"} {
			t.Run(suffix+"/"+drift, func(t *testing.T) {
				doc, actual := companyRoutes(t)
				op := doc.Paths.Value(path).Post
				switch drift {
				case "cookie":
					op.Security = nil
				case "csrf", "selector":
					name := "X-CSRF"
					if drift == "selector" {
						name = "org_id"
					}
					for i, param := range op.Parameters {
						if param.Value != nil && param.Value.Name == name {
							op.Parameters = append(op.Parameters[:i], op.Parameters[i+1:]...)
							break
						}
					}
				case "bare":
					actual.Post(path, func(http.ResponseWriter, *http.Request) {})
				default:
					raw, err := json.Marshal(op.Extensions["x-abastevo-organization"])
					if err != nil {
						t.Fatal(err)
					}
					var rule orgapi.Rule
					if err := json.Unmarshal(raw, &rule); err != nil {
						t.Fatal(err)
					}
					switch drift {
					case "permission":
						rule.Permission = "org.settings.manage"
					case "write":
						rule.Write = !rule.Write
					case "fresh":
						rule.Fresh = !rule.Fresh
					}
					op.Extensions["x-abastevo-organization"] = rule
				}
				if ValidateOrganizationRoutes(actual, doc) == nil {
					t.Fatal("ownership guard drift accepted", drift)
				}
			})
		}
	}
}
