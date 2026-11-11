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
		item := document.Paths.Value(path)
		if method != http.MethodPost || !guarded || item == nil {
			return fmt.Errorf("unguarded or undocumented organization route %s %s", method, path)
		}
		operation := item.GetOperation(method)
		if operation == nil {
			return fmt.Errorf("missing organization operation %s %s", method, path)
		}
		raw, err := json.Marshal(operation.Extensions["x-abastevo-organization"])
		var declared orgapi.Rule
		if err != nil || json.Unmarshal(raw, &declared) != nil || declared != descriptor.OrganizationRule() {
			return fmt.Errorf("organization authority metadata differs for %s", path)
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
