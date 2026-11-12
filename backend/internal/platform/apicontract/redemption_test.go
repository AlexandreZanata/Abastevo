package apicontract

import (
	"net/http"
	"testing"
)

func TestRedemptionContractsRequireAccountProofAndExplicitConsent(t *testing.T) {
	doc, router := companyRoutes(t)
	if e := ValidateOrganizationRoutes(router, doc); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"EnterpriseInvitationPreviewRequest", "EnterpriseInvitationAcceptRequest", "EnterpriseInvitationDeclineRequest", "EnterpriseInvitationPreview", "EnterpriseInvitationRedemptionResult", "EnterpriseInvitationAcceptanceView"} {
		schema := doc.Components.Schemas[name].Value
		if e := schema.VisitJSON(enterpriseContractObject(t, schema.Example)); e != nil {
			t.Fatal(name, e)
		}
		for _, key := range []string{"account_id", "verifier", "family_id", "access_token"} {
			v := enterpriseContractObject(t, schema.Example)
			v[key] = "synthetic"
			if schema.VisitJSON(v) == nil {
				t.Fatal("authority field accepted", name, key)
			}
		}
		for _, key := range schema.Required {
			v := enterpriseContractObject(t, schema.Example)
			delete(v, key)
			if schema.VisitJSON(v) == nil {
				t.Fatal("missing intent", name, key)
			}
		}
	}
	for _, action := range []string{"preview", "accept", "decline"} {
		path := "/v1/org-access/invitations/" + action
		t.Run(action+"/bare", func(t *testing.T) {
			d, r := companyRoutes(t)
			r.Post(path, func(http.ResponseWriter, *http.Request) {})
			if ValidateOrganizationRoutes(r, d) == nil {
				t.Fatal("bare redemption accepted")
			}
		})
		t.Run(action+"/drift", func(t *testing.T) {
			d, r := companyRoutes(t)
			d.Paths.Value(path).Post.Extensions["x-abastevo-personal-invitation"] = map[string]any{"cookie": false}
			if ValidateOrganizationRoutes(r, d) == nil {
				t.Fatal("guard drift accepted")
			}
		})
	}
	accept := doc.Components.Schemas["EnterpriseInvitationAcceptRequest"].Value
	v := enterpriseContractObject(t, accept.Example)
	v["consent"] = false
	if accept.VisitJSON(v) == nil {
		t.Fatal("implicit consent")
	}
}
