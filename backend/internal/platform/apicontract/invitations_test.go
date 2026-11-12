package apicontract

import (
	"net/http"
	"testing"
)

func TestInvitationContractsMatchIssuerOnlyRoutesAndIntent(t *testing.T) {
	doc, router := companyRoutes(t)
	if e := ValidateOrganizationRoutes(router, doc); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"EnterpriseInvitationIssueRequest", "EnterpriseInvitationRevisionRequest", "EnterpriseInvitationReadRequest", "EnterpriseInvitationListRequest", "EnterpriseInvitationGrant", "EnterpriseInvitationDelivery", "EnterpriseInvitationView", "EnterpriseInvitationResult", "EnterpriseInvitationPage"} {
		s := doc.Components.Schemas[name]
		if s == nil || s.Value == nil || s.Value.Example == nil {
			t.Fatal("missing invitation schema example", name)
		}
		value := enterpriseContractObject(t, s.Value.Example)
		if e := s.Value.VisitJSON(value); e != nil {
			t.Fatal("invalid invitation example", name, e)
		}
		for _, unknown := range []string{"account_id", "verifier", "secret"} {
			v := enterpriseContractObject(t, s.Value.Example)
			v[unknown] = "synthetic"
			if s.Value.VisitJSON(v) == nil {
				t.Fatal("unknown authority or recoverable verifier field", name, unknown)
			}
		}
		for _, required := range s.Value.Required {
			v := enterpriseContractObject(t, s.Value.Example)
			delete(v, required)
			if s.Value.VisitJSON(v) == nil {
				t.Fatal("missing explicit invitation intent", name, required)
			}
		}
	}
	for _, suffix := range []string{"issue", "{invitation_id}/resend", "{invitation_id}/revoke", "{invitation_id}/read", "list"} {
		path := "/v1/orgs/{org_id}/invitations/" + suffix
		op := doc.Paths.Value(path).Post
		if e := op.RequestBody.Value.Content.Get("application/json").Schema.Value.VisitJSON(enterpriseContractObject(t, op.RequestBody.Value.Content.Get("application/json").Example)); e != nil {
			t.Fatal("request contract", suffix, e)
		}
		response := op.Responses.Value("200").Value.Content.Get("application/json")
		if e := response.Schema.Value.VisitJSON(enterpriseContractObject(t, response.Example)); e != nil {
			t.Fatal("response contract", suffix, e)
		}
		t.Run(suffix+"/unguarded", func(t *testing.T) {
			document, r := companyRoutes(t)
			r.Post(path, func(http.ResponseWriter, *http.Request) {})
			if ValidateOrganizationRoutes(r, document) == nil {
				t.Fatal("bare invitation route accepted")
			}
		})
	}
	// A retry with only original metadata is valid; a status cannot carry delivery.
	result := doc.Components.Schemas["EnterpriseInvitationResult"].Value
	retry := enterpriseContractObject(t, result.Example)
	delete(retry, "delivery")
	retry["replayed"] = true
	if e := result.VisitJSON(retry); e != nil {
		t.Fatal("nonrecoverable retry rejected", e)
	}
	view := doc.Components.Schemas["EnterpriseInvitationView"].Value
	leak := enterpriseContractObject(t, view.Example)
	leak["delivery"] = doc.Components.Schemas["EnterpriseInvitationDelivery"].Value.Example
	if view.VisitJSON(leak) == nil {
		t.Fatal("status could redisplay code")
	}
}
