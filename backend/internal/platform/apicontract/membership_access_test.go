package apicontract

import (
	"net/http"
	"testing"
)

func TestMembershipAccessContractsAndGuardDrift(t *testing.T) {
	doc, r := companyRoutes(t)
	if e := ValidateOrganizationRoutes(r, doc); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"EnterpriseMembershipAccessReadRequest", "EnterpriseMembershipAccessRequest", "EnterpriseMembershipAccessView", "EnterpriseMembershipAccessResult"} {
		s := doc.Components.Schemas[name].Value
		if e := s.VisitJSON(enterpriseContractObject(t, s.Example)); e != nil {
			t.Fatal(name, e)
		}
		for _, field := range []string{"account_id", "session_id", "roles", "family_id"} {
			v := enterpriseContractObject(t, s.Example)
			v[field] = "synthetic"
			if s.VisitJSON(v) == nil {
				t.Fatal("private authority selector accepted", name, field)
			}
		}
		for _, field := range s.Required {
			v := enterpriseContractObject(t, s.Example)
			delete(v, field)
			if s.VisitJSON(v) == nil {
				t.Fatal("missing membership intent", name, field)
			}
		}
	}
	for _, action := range []string{"read", "suspend", "reactivate"} {
		path := "/v1/orgs/{org_id}/memberships/{membership_id}/" + action
		t.Run(action+"/unguarded", func(t *testing.T) {
			doc, r := companyRoutes(t)
			r.Post(path, func(http.ResponseWriter, *http.Request) {})
			if ValidateOrganizationRoutes(r, doc) == nil {
				t.Fatal("bare route")
			}
		})
		t.Run(action+"/target", func(t *testing.T) {
			doc, r := companyRoutes(t)
			op := doc.Paths.Value(path).Post
			for i, p := range op.Parameters {
				if p.Value != nil && p.Value.Name == "membership_id" {
					op.Parameters = append(op.Parameters[:i], op.Parameters[i+1:]...)
					break
				}
			}
			if ValidateOrganizationRoutes(r, doc) == nil {
				t.Fatal("missing selector accepted")
			}
		})
	}
}
