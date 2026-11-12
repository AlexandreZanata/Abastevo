package apicontract

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// ValidateOrganizationRoutes compares actual chi registrations and OpenAPI in
// both directions. Feature composition tests reuse this gate as routes land.
func ValidateOrganizationRoutes(router chi.Routes, document *openapi3.T) error {
	if router == nil || document == nil || document.Paths == nil {
		return fmt.Errorf("organization route gate requires router and contract")
	}
	registered := map[string]bool{}
	err := chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		if !orgapi.IsOrganizationPath(path) {
			return nil
		}
		descriptor, guarded := handler.(orgapi.Descriptor)
		company, companyGuarded := handler.(orgapi.CompanySessionDescriptor)
		creation, creationGuarded := handler.(orgapi.OrganizationCreationDescriptor)
		item := document.Paths.Value(path)
		if method != http.MethodPost || (!guarded && !companyGuarded && !creationGuarded) || item == nil {
			return fmt.Errorf("unguarded or undocumented organization route %s %s", method, path)
		}
		operation := item.GetOperation(method)
		if operation == nil {
			return fmt.Errorf("missing organization operation %s %s", method, path)
		}
		var declared orgapi.Rule
		if companyGuarded || creationGuarded {
			var rule orgapi.CompanySessionRule
			if creationGuarded {
				expected := orgapi.OrganizationCreationRule{Cookie: true, CSRF: true, Fresh: true, Success: 201}
				if companyGuarded || guarded || path != "/v1/org-access/organizations/create" || creation.OrganizationCreationRule() != expected || operation.Extensions["x-abastevo-organization"] != nil || operation.Extensions["x-abastevo-company-session"] != nil {
					return fmt.Errorf("invalid organization creation guard for %s", path)
				}
				raw, err := json.Marshal(operation.Extensions["x-abastevo-organization-creation"])
				var recorded orgapi.OrganizationCreationRule
				if err != nil || json.Unmarshal(raw, &recorded) != nil || recorded != expected {
					return fmt.Errorf("organization creation metadata differs for %s", path)
				}
				rule = orgapi.CompanySessionRule{Cookie: true, CSRF: true, Fresh: true, Success: 201}
			} else {
				rule = company.CompanySessionRule()
				expected := orgapi.CompanySessionRule{Action: rule.Action, AccountProof: rule.Action == "login", Cookie: rule.Action != "login", CSRF: rule.Action != "login", Fresh: rule.Action == "login", Success: 200}
				if (rule.Action != "login" && rule.Action != "read" && rule.Action != "logout") || path != "/v1/org-access/session/"+rule.Action ||
					rule != expected || guarded || operation.Extensions["x-abastevo-organization-creation"] != nil || operation.Extensions["x-abastevo-organization"] != nil {
					return fmt.Errorf("invalid company session guard for %s", path)
				}
				raw, err := json.Marshal(operation.Extensions["x-abastevo-company-session"])
				var recorded orgapi.CompanySessionRule
				if err != nil || json.Unmarshal(raw, &recorded) != nil || recorded != rule {
					return fmt.Errorf("company session metadata differs for %s", path)
				}
			}
			declared.Success = rule.Success
			if err := validateCompanySecurity(document, operation, path, rule); err != nil {
				return err
			}
		} else {
			raw, err := json.Marshal(operation.Extensions["x-abastevo-organization"])
			if err != nil || json.Unmarshal(raw, &declared) != nil || declared != descriptor.OrganizationRule() || operation.Extensions["x-abastevo-company-session"] != nil || operation.Extensions["x-abastevo-organization-creation"] != nil {
				return fmt.Errorf("organization authority metadata differs for %s", path)
			}
			if declared.CompanyOnly {
				if err := validateCompanySecurity(document, operation, path, orgapi.CompanySessionRule{Cookie: true, CSRF: true}); err != nil {
					return err
				}
			}
		}
		if operation.OperationID == "" || operation.RequestBody == nil || operation.RequestBody.Value == nil ||
			!operation.RequestBody.Value.Required || operation.RequestBody.Value.Content.Get("application/json") == nil {
			return fmt.Errorf("missing organization body contract for %s", path)
		}
		if strings.HasPrefix(path, "/v1/orgs/") {
			found := false
			for _, parameter := range append(append(openapi3.Parameters{}, item.Parameters...), operation.Parameters...) {
				if parameter.Value != nil && parameter.Value.Name == "org_id" && parameter.Value.In == "path" && parameter.Value.Required {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("missing organization path scope for %s", path)
			}
		}
		if operation.Responses == nil || operation.Responses.Value(strconv.Itoa(declared.Success)) == nil {
			return fmt.Errorf("organization success status differs for %s", path)
		}
		for _, required := range []string{"400", "401", "403", "404", "409", "422", "429", "503"} {
			if operation.Responses.Value(required) == nil {
				return fmt.Errorf("missing private error %s for %s", required, path)
			}
		}
		for _, response := range operation.Responses.Map() {
			if response.Value == nil {
				return fmt.Errorf("unresolved organization response for %s", path)
			}
			noStore := false
			for name, header := range response.Value.Headers {
				if strings.EqualFold(name, "ETag") {
					return fmt.Errorf("private organization ETag for %s", path)
				}
				if strings.EqualFold(name, "Cache-Control") && header.Value != nil && header.Value.Schema != nil && header.Value.Schema.Value != nil {
					values := header.Value.Schema.Value.Enum
					noStore = len(values) == 1 && values[0] == "no-store"
				}
			}
			if !noStore || response.Value.Content.Get("application/json") == nil {
				return fmt.Errorf("missing private response policy for %s", path)
			}
		}
		registered[method+" "+path] = true
		return nil
	})
	if err != nil {
		return err
	}
	for path, item := range document.Paths.Map() {
		if !orgapi.IsOrganizationPath(path) {
			continue
		}
		for method := range item.Operations() {
			if !registered[method+" "+path] {
				return fmt.Errorf("unregistered organization contract operation %s %s", method, path)
			}
		}
	}
	return nil
}

func validateCompanySecurity(document *openapi3.T, operation *openapi3.Operation, path string, rule orgapi.CompanySessionRule) error {
	if operation.Security == nil {
		return fmt.Errorf("missing company session security for %s", path)
	}
	if rule.Cookie {
		scheme := document.Components.SecuritySchemes["CompanySessionCookie"]
		if scheme == nil || scheme.Value == nil || scheme.Value.Type != "apiKey" || scheme.Value.In != "cookie" || scheme.Value.Name != "__Host-empresas-session" ||
			len(*operation.Security) != 1 || len((*operation.Security)[0]) != 1 {
			return fmt.Errorf("invalid company cookie contract for %s", path)
		}
		scopes, ok := (*operation.Security)[0]["CompanySessionCookie"]
		if !ok || len(scopes) != 0 {
			return fmt.Errorf("missing company cookie requirement for %s", path)
		}
	} else if len(*operation.Security) != 0 {
		return fmt.Errorf("unexpected cookie authority for company login")
	}
	for _, header := range []string{"Origin", "X-CSRF"} {
		if header == "X-CSRF" && !rule.CSRF {
			continue
		}
		found := false
		for _, parameter := range operation.Parameters {
			if parameter.Value != nil && parameter.Value.In == "header" && parameter.Value.Name == header && parameter.Value.Required {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("missing company session header %s", header)
		}
	}
	return nil
}
