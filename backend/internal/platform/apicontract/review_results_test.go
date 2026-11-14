package apicontract

import (
	"net/http"
	"testing"

	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	syncweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters/http"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

func ownReviewRoutes(t *testing.T) (*openapi3.T, chi.Router) {
	t.Helper()
	doc := loadSpec(t)
	item := doc.Paths.Value(syncweb.ReviewResultsPath)
	if item == nil || item.Post == nil {
		t.Fatalf("own-result contract missing at %s for %s", contractsDir(t), syncweb.ReviewResultsPath)
	}
	// Exercise the actual endpoint descriptor without I/O or a positive auth stub.
	doc.Paths = openapi3.NewPaths()
	doc.Paths.Set(syncweb.ReviewResultsPath, item)
	router := chi.NewRouter()
	if err := syncweb.RegisterReviewResults(router, syncweb.ReviewResultsOptions{Enabled: true, Origin: "https://company.example.invalid", Verifier: auth.Verifier{Authority: "company.example.invalid"}, Devices: &memberpg.DeviceStore{}, Repository: &fleetpg.MeterReviewStore{}}); err != nil {
		t.Fatal(err)
	}
	return doc, router
}
func TestOwnReviewResultsRequiresItsExactSignedAuthorBoundary(t *testing.T) {
	doc, router := ownReviewRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal("actual own-result endpoint rejected", err)
	}
	op := doc.Paths.Value(syncweb.ReviewResultsPath).Post
	metadata := op.Extensions["x-abastevo-signed-review-results"]
	for _, replacement := range []map[string]any{{"account_proof": true, "signed": true, "exact_enrollment": true, "original_author": false, "whole_body_preflight": true, "success": 200}, {"account_proof": true, "signed": true, "exact_enrollment": true, "original_author": true, "whole_body_preflight": false, "success": 200}, {"account_proof": true, "signed": true, "exact_enrollment": true, "original_author": true, "whole_body_preflight": true, "success": 200, "fallback": true}} {
		op.Extensions["x-abastevo-signed-review-results"] = replacement
		if ValidateOrganizationRoutes(router, doc) == nil {
			t.Fatal("weakened/unknown author boundary accepted")
		}
	}
	op.Extensions["x-abastevo-signed-review-results"] = metadata
	security := op.Security
	cookie := openapi3.SecurityRequirements{{"CompanySessionCookie": []string{}}}
	op.Security = &cookie
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("cookie fallback admitted")
	}
	op.Security = security
	op.Extensions["x-abastevo-signed-batch"] = map[string]any{"signed": true}
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("ambiguous transport authority accepted")
	}
	delete(op.Extensions, "x-abastevo-signed-batch")
	router.Post(syncweb.ReviewResultsPath, func(http.ResponseWriter, *http.Request) {})
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("unguarded private result route accepted")
	}
}
func TestOwnReviewResultsBodyAndProofCannotLoseCorrelationFields(t *testing.T) {
	doc, router := ownReviewRoutes(t)
	op := doc.Paths.Value(syncweb.ReviewResultsPath).Post
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	schema := op.RequestBody.Value.Content.Get("application/json").Schema.Value
	for _, field := range []string{"schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id", "selectors"} {
		before := schema.Required
		schema.Required = nil
		for _, key := range before {
			if key != field {
				schema.Required = append(schema.Required, key)
			}
		}
		if ValidateOrganizationRoutes(router, doc) == nil {
			t.Fatal("missing exact request correlation", field)
		}
		schema.Required = before
	}
	parameters := op.Parameters
	op.Parameters = nil
	for _, p := range parameters {
		if p.Value == nil || p.Value.Name != "Signature-Nonce" {
			op.Parameters = append(op.Parameters, p)
		}
	}
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("replayable unsigned result request accepted")
	}
	op.Parameters = parameters
	requestSchema := op.RequestBody.Value.Content.Get("application/json").Schema
	before := requestSchema.Ref
	requestSchema.Ref = "#/components/schemas/EnterpriseSyncBatch"
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("old batch body substituted for author-result read")
	}
	requestSchema.Ref = before
}

func TestOwnReviewResultsMalformedBodyDriftReturnsError(t *testing.T) {
	doc, router := ownReviewRoutes(t)
	op := doc.Paths.Value(syncweb.ReviewResultsPath).Post
	body := op.RequestBody
	bodyValue := body.Value
	media := bodyValue.Content.Get("application/json")
	schema := media.Schema
	schemaValue := schema.Value
	for _, test := range []struct {
		name            string
		mutate, restore func()
	}{
		{"missing-body", func() { op.RequestBody = nil }, func() { op.RequestBody = body }},
		{"unresolved-body", func() { body.Value = nil }, func() { body.Value = bodyValue }},
		{"missing-json-content", func() { delete(bodyValue.Content, "application/json") }, func() { bodyValue.Content["application/json"] = media }},
		{"missing-schema", func() { media.Schema = nil }, func() { media.Schema = schema }},
		{"unresolved-schema", func() { schema.Value = nil }, func() { schema.Value = schemaValue }},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.mutate()
			defer test.restore()
			if ValidateOrganizationRoutes(router, doc) == nil {
				t.Fatal("malformed body schema drift accepted")
			}
		})
	}
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal("restored actual contract no longer valid", err)
	}
}
