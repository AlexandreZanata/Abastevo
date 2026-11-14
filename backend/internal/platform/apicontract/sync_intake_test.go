package apicontract

import (
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

func TestSyncIntakeRequiresDedicatedSignedWholeBatchBoundary(t *testing.T) {
	doc, router := companyRoutes(t)
	const path = "/v1/orgs/{org_id}/sync/operations"
	item := doc.Paths.Value(path)
	if item == nil || item.Post == nil {
		t.Fatal("signed batch contract missing")
	}
	if e := ValidateOrganizationRoutes(router, doc); e != nil {
		t.Fatal(e)
	}
	var original http.Handler
	if e := chi.Walk(router, func(m, p string, h http.Handler, _ ...func(http.Handler) http.Handler) error {
		if m == http.MethodPost && p == path {
			original = h
		}
		return nil
	}); e != nil || original == nil {
		t.Fatal("actual batch route missing", e)
	}
	router.Post(path, func(http.ResponseWriter, *http.Request) {})
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("unguarded batch accepted")
	}
	router.Method(http.MethodPost, path, original)
	op := item.Post
	security := op.Security
	cookie := openapi3.SecurityRequirements{{"CompanySessionCookie": []string{}}}
	op.Security = &cookie
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("cookie batch fallback accepted")
	}
	op.Security = security
	ext := op.Extensions["x-abastevo-signed-batch"]
	op.Extensions["x-abastevo-signed-batch"] = map[string]any{"account_proof": true, "signed": true, "current_enrollment": true, "whole_body_preflight": false, "per_operation": true, "success": 200}
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("prefix effects descriptor accepted")
	}
	op.Extensions["x-abastevo-signed-batch"] = ext
	response := doc.Components.Schemas["EnterpriseSyncBatchResponse"].Value
	for _, input := range []map[string]any{{"schema_version": 1}, {"schema_version": 2, "acknowledgements": []any{}}, {"schema_version": 1, "acknowledgements": nil}, {"schema_version": 1, "acknowledgements": []any{}, "family_id": "untrusted"}} {
		if response.VisitJSON(enterpriseContractObject(t, input)) == nil {
			t.Fatal("invalid ACK transport accepted")
		}
	}
	if e := response.VisitJSON(enterpriseContractObject(t, map[string]any{"schema_version": 1, "acknowledgements": []any{}})); e != nil {
		t.Fatal("empty result invalid", e)
	}
}
