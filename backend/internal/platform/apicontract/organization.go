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
		personal, personalGuarded := handler.(orgapi.PersonalInvitationDescriptor)
		self, selfGuarded := handler.(orgapi.SelfMembershipDescriptor)
		signedDevice, signedDeviceGuarded := handler.(orgapi.SignedDeviceDescriptor)
		selfDevice, selfDeviceGuarded := handler.(orgapi.SelfDeviceDescriptor)
		accountContext, contextGuarded := handler.(orgapi.AccountContextDescriptor)
		item := document.Paths.Value(path)
		if method != http.MethodPost || (!guarded && !companyGuarded && !creationGuarded && !personalGuarded && !selfGuarded && !signedDeviceGuarded && !selfDeviceGuarded && !contextGuarded) || item == nil {
			return fmt.Errorf("unguarded or undocumented organization route %s %s", method, path)
		}
		operation := item.GetOperation(method)
		if operation == nil {
			return fmt.Errorf("missing organization operation %s %s", method, path)
		}
		if !contextGuarded && operation.Extensions["x-abastevo-account-context"] != nil {
			return fmt.Errorf("unexpected account context metadata for %s", path)
		}
		if !selfGuarded && operation.Extensions["x-abastevo-self-membership"] != nil {
			return fmt.Errorf("unexpected self membership metadata for %s", path)
		}
		if !personalGuarded && operation.Extensions["x-abastevo-personal-invitation"] != nil {
			return fmt.Errorf("unexpected personal invitation metadata for %s", path)
		}
		if !signedDeviceGuarded && operation.Extensions["x-abastevo-signed-device"] != nil || !selfDeviceGuarded && operation.Extensions["x-abastevo-self-device"] != nil {
			return fmt.Errorf("unexpected device authority metadata for %s", path)
		}
		var declared orgapi.Rule
		if contextGuarded {
			rule := accountContext.AccountContextRule()
			expected := orgapi.AccountContextRule{Action: rule.Action, Cookie: true, CSRF: true, Binding: true, Success: 200}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || signedDeviceGuarded || selfDeviceGuarded || (rule.Action != "list" && rule.Action != "read") || path != "/v1/org-access/contexts/"+rule.Action || rule != expected {
				return fmt.Errorf("invalid account context guard for %s", path)
			}
			for _, key := range []string{"x-abastevo-organization", "x-abastevo-company-session", "x-abastevo-organization-creation", "x-abastevo-personal-invitation", "x-abastevo-self-membership", "x-abastevo-signed-device", "x-abastevo-self-device"} {
				if operation.Extensions[key] != nil {
					return fmt.Errorf("unexpected authority metadata for context %s", path)
				}
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-account-context"])
			var recorded orgapi.AccountContextRule
			if e != nil || json.Unmarshal(raw, &recorded) != nil || recorded != expected {
				return fmt.Errorf("account context metadata differs for %s", path)
			}
			if e := validateCompanySecurity(document, operation, path, orgapi.CompanySessionRule{Cookie: true, CSRF: true, Success: 200}); e != nil {
				return e
			}
			declared.Success = 200
		} else if signedDeviceGuarded {
			rule := signedDevice.SignedDeviceRule()
			expected := orgapi.SignedDeviceRule{Action: rule.Action, AccountProof: true, Signed: true, Fresh: rule.Action == "enroll", Enrollment: rule.Action == "check", Bootstrap: rule.Action == "enroll", Success: 200}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || selfDeviceGuarded || (rule.Action != "enroll" && rule.Action != "check") || path != "/v1/orgs/{org_id}/devices/"+rule.Action || rule != expected || operation.Extensions["x-abastevo-organization"] != nil || operation.Extensions["x-abastevo-company-session"] != nil || operation.Extensions["x-abastevo-organization-creation"] != nil || operation.Extensions["x-abastevo-self-membership"] != nil || operation.Extensions["x-abastevo-personal-invitation"] != nil {
				return fmt.Errorf("invalid signed device guard for %s", path)
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-signed-device"])
			var recorded orgapi.SignedDeviceRule
			if e != nil || json.Unmarshal(raw, &recorded) != nil || recorded != expected {
				return fmt.Errorf("signed device metadata differs for %s", path)
			}
			if e := validateSignedDeviceSecurity(operation, path); e != nil {
				return e
			}
			declared.Success = 200
		} else if selfDeviceGuarded {
			rule := selfDevice.SelfDeviceRule()
			expected := orgapi.SelfDeviceRule{Action: rule.Action, Cookie: true, CSRF: true, Fresh: true, Binding: true, Success: 200}
			expectedPath := "/v1/orgs/{org_id}/memberships/{membership_id}/devices/self/list"
			if rule.Action == "revoke" {
				expectedPath = "/v1/orgs/{org_id}/memberships/{membership_id}/devices/self/{device_id}/revoke"
			}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || (rule.Action != "list" && rule.Action != "revoke") || path != expectedPath || rule != expected || operation.Extensions["x-abastevo-organization"] != nil || operation.Extensions["x-abastevo-company-session"] != nil || operation.Extensions["x-abastevo-organization-creation"] != nil || operation.Extensions["x-abastevo-self-membership"] != nil || operation.Extensions["x-abastevo-personal-invitation"] != nil {
				return fmt.Errorf("invalid self device guard for %s", path)
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-self-device"])
			var recorded orgapi.SelfDeviceRule
			if e != nil || json.Unmarshal(raw, &recorded) != nil || recorded != expected {
				return fmt.Errorf("self device metadata differs for %s", path)
			}
			if e := validateCompanySecurity(document, operation, path, orgapi.CompanySessionRule{Cookie: true, CSRF: true, Fresh: true, Success: 200}); e != nil {
				return e
			}
			declared.Success = 200
		} else if selfGuarded {
			rule := self.SelfMembershipRule()
			expected := orgapi.SelfMembershipRule{Action: rule.Action, Cookie: true, CSRF: true, Fresh: true, Binding: true, Success: 200}
			expectedPath := "/v1/orgs/{org_id}/memberships/{membership_id}/leave"
			if rule.Action == "read" {
				expectedPath += "/read"
			}
			if guarded || companyGuarded || creationGuarded || personalGuarded || (rule.Action != "read" && rule.Action != "leave") || path != expectedPath || rule != expected || operation.Extensions["x-abastevo-organization"] != nil || operation.Extensions["x-abastevo-company-session"] != nil || operation.Extensions["x-abastevo-personal-invitation"] != nil || operation.Extensions["x-abastevo-organization-creation"] != nil {
				return fmt.Errorf("invalid self membership guard for %s", path)
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-self-membership"])
			var recorded orgapi.SelfMembershipRule
			if e != nil || json.Unmarshal(raw, &recorded) != nil || recorded != expected {
				return fmt.Errorf("self membership metadata differs for %s", path)
			}
			if e := validateCompanySecurity(document, operation, path, orgapi.CompanySessionRule{Cookie: true, CSRF: true, Fresh: true, Success: 200}); e != nil {
				return e
			}
			declared.Success = 200
		} else if personalGuarded {
			rule := personal.PersonalInvitationRule()
			expected := orgapi.PersonalInvitationRule{Action: rule.Action, Cookie: true, CSRF: true, Fresh: true, Success: 200}
			if guarded || companyGuarded || creationGuarded || (rule.Action != "preview" && rule.Action != "accept" && rule.Action != "decline") || path != "/v1/org-access/invitations/"+rule.Action || rule != expected || operation.Extensions["x-abastevo-organization"] != nil || operation.Extensions["x-abastevo-company-session"] != nil || operation.Extensions["x-abastevo-organization-creation"] != nil {
				return fmt.Errorf("invalid personal invitation guard for %s", path)
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-personal-invitation"])
			var recorded orgapi.PersonalInvitationRule
			if e != nil || json.Unmarshal(raw, &recorded) != nil || recorded != expected {
				return fmt.Errorf("personal invitation metadata differs for %s", path)
			}
			if e := validateCompanySecurity(document, operation, path, orgapi.CompanySessionRule{Cookie: true, CSRF: true, Fresh: true, Success: 200}); e != nil {
				return e
			}
			declared.Success = 200
		} else if companyGuarded || creationGuarded {
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
			if completion, ok := handler.(orgapi.AuthorityCompletion); !ok || !completion.CompletesAuthority() {
				return fmt.Errorf("organization route %s does not complete current authority", path)
			}
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
		// Every target selector belongs to the contract, including nested parent
		// and establishment identities. Checking only org_id misses route drift.
		for _, segment := range strings.Split(path, "/") {
			if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
				continue
			}
			key := strings.TrimSuffix(strings.TrimPrefix(segment, "{"), "}")
			found := false
			for _, parameter := range append(append(openapi3.Parameters{}, item.Parameters...), operation.Parameters...) {
				if parameter.Value != nil && parameter.Value.Name == key && parameter.Value.In == "path" && parameter.Value.Required {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("missing organization path selector %s for %s", key, path)
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

func validateSignedDeviceSecurity(operation *openapi3.Operation, path string) error {
	if operation.Security == nil || len(*operation.Security) != 0 {
		return fmt.Errorf("unexpected cookie authority for signed device %s", path)
	}
	for _, header := range []string{"Signature", "Signature-Created", "Signature-Expires", "Signature-Keyid", "Signature-Nonce"} {
		found := false
		for _, p := range operation.Parameters {
			if p.Value != nil && p.Value.In == "header" && p.Value.Name == header && p.Value.Required {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("missing signed device proof header %s for %s", header, path)
		}
	}
	if operation.RequestBody == nil || operation.RequestBody.Value == nil || operation.RequestBody.Value.Content.Get("application/json") == nil {
		return fmt.Errorf("missing signed device body for %s", path)
	}
	ref := operation.RequestBody.Value.Content.Get("application/json").Schema
	if ref == nil || ref.Value == nil {
		return fmt.Errorf("unresolved signed device body for %s", path)
	}
	for _, name := range []string{"family_id", "access_token"} {
		found := false
		for _, required := range ref.Value.Required {
			if required == name {
				found = true
			}
		}
		if !found || ref.Value.Properties[name] == nil {
			return fmt.Errorf("missing signed device account proof field %s for %s", name, path)
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
