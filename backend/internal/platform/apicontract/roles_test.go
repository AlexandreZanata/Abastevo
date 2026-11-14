package apicontract

import (
	"net/http"
	"slices"
	"testing"
)

func TestRoleContractUsesGrantableSourceRolesAndStrictIntent(t *testing.T) {
	doc := loadSpec(t)
	roles := []string{"ADMIN", "APPROVER", "AUDITOR", "DATA_MANAGER", "DRIVER", "FINANCE", "FLEET_MANAGER", "OPERATIONS_STAFF", "OWNER", "SUPERVISOR"}
	for _, name := range []string{"EnterpriseRoleAssignRequest", "EnterpriseRoleChangeRequest"} {
		schema := doc.Components.Schemas[name].Value
		var declared []string
		for _, value := range schema.Properties["role"].Value.Enum {
			role, ok := value.(string)
			if !ok {
				t.Fatal("non-string role", value)
			}
			declared = append(declared, role)
		}
		slices.Sort(declared)
		if !slices.Equal(declared, roles) {
			t.Fatal("role contract differs from grantable F1 catalog", name, declared)
		}
	}
	for _, name := range []string{"EnterpriseRoleAssignRequest", "EnterpriseRoleEndRequest", "EnterpriseRoleChangeRequest", "EnterpriseRoleReadRequest", "EnterpriseRoleListRequest"} {
		schema := doc.Components.Schemas[name]
		if schema == nil || schema.Value == nil || schema.Value.Example == nil {
			t.Fatal("missing role schema/example", name)
		}
		valid := enterpriseContractObject(t, schema.Value.Example)
		if err := schema.Value.VisitJSON(valid); err != nil {
			t.Fatal("invalid role schema example", name, err)
		}
		for _, required := range schema.Value.Required {
			value := enterpriseContractObject(t, schema.Value.Example)
			delete(value, required)
			if schema.Value.VisitJSON(value) == nil {
				t.Fatal("missing role intent accepted", name, required)
			}
		}
		for field := range schema.Value.Properties {
			value := enterpriseContractObject(t, schema.Value.Example)
			value[field] = nil
			if schema.Value.VisitJSON(value) == nil {
				t.Fatal("null role intent accepted", name, field)
			}
		}
		for _, field := range []string{"account_id", "ROLE"} {
			value := enterpriseContractObject(t, schema.Value.Example)
			value[field] = "ADMIN"
			if schema.Value.VisitJSON(value) == nil {
				t.Fatal("unknown or aliased role intent accepted", name, field)
			}
		}
	}
	list := doc.Components.Schemas["EnterpriseRoleListRequest"].Value
	if err := list.VisitJSON(map[string]any{}); err != nil {
		t.Fatal("default role list page rejected", err)
	}
	if list.VisitJSON(map[string]any{"page": map[string]any{"limit": float64(0)}}) == nil {
		t.Fatal("explicit zero role page limit accepted")
	}
}

func TestRoleActualRouterContractsAndResponseExamples(t *testing.T) {
	doc, router := companyRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ path, success string }{
		{"/v1/orgs/{org_id}/roles/assign", "201"},
		{"/v1/orgs/{org_id}/roles/{assignment_id}/end", "200"},
		{"/v1/orgs/{org_id}/roles/{assignment_id}/change", "200"},
		{"/v1/orgs/{org_id}/roles/{assignment_id}/read", "200"},
		{"/v1/orgs/{org_id}/roles/list", "200"},
	} {
		op := doc.Paths.Value(route.path).Post
		request := op.RequestBody.Value.Content.Get("application/json")
		if !op.RequestBody.Value.Required || request.Example == nil {
			t.Fatal("missing exact role request body", route.path)
		}
		if err := request.Schema.Value.VisitJSON(enterpriseContractObject(t, request.Example)); err != nil {
			t.Fatal("invalid role request example", route.path, err)
		}
		response := op.Responses.Value(route.success).Value.Content.Get("application/json")
		if err := response.Schema.Value.VisitJSON(enterpriseContractObject(t, response.Example)); err != nil {
			t.Fatal("invalid role response example", route.path, err)
		}
		t.Run(route.path+"/unguarded", func(t *testing.T) {
			document, actual := companyRoutes(t)
			actual.Post(route.path, func(http.ResponseWriter, *http.Request) {})
			if ValidateOrganizationRoutes(actual, document) == nil {
				t.Fatal("unguarded role route accepted")
			}
		})
	}
	result := doc.Components.Schemas["EnterpriseRoleResult"].Value
	assignment := doc.Paths.Value("/v1/orgs/{org_id}/roles/{assignment_id}/read").Post.Responses.Value("200").Value.Content.Get("application/json").Example
	wire := enterpriseContractObject(t, assignment)
	wire["replayed"] = true
	if err := result.VisitJSON(wire); err != nil {
		t.Fatal("real top-level role replay field rejected", err)
	}
	wire["account_id"] = "7b0e2a54-0000-4000-8000-000000000201"
	if result.VisitJSON(wire) == nil {
		t.Fatal("extra account authority in role acknowledgement accepted")
	}
}
