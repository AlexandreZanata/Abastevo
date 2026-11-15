package apicontract

import (
	"context"
	"net/http"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	memberpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/adapters"
	syncweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters/http"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
)

type attachmentSessionsStub struct{}

func (attachmentSessionsStub) OpenAttachment(_ context.Context, _ syncapp.Source, _ syncapp.AttachmentDeclaration) (syncapp.AttachmentSession, error) {
	return syncapp.AttachmentSession{}, syncapp.ErrUnavailable
}

func attachmentSessionRoutes(t *testing.T) (*openapi3.T, chi.Router) {
	t.Helper()
	doc := loadSpec(t)
	item := doc.Paths.Value(syncweb.AttachmentsPath)
	if item == nil || item.Post == nil {
		t.Fatalf("attachment session contract missing at %s", syncweb.AttachmentsPath)
	}
	doc.Paths = openapi3.NewPaths()
	doc.Paths.Set(syncweb.AttachmentsPath, item)
	router := chi.NewRouter()
	if err := syncweb.RegisterAttachments(router, syncweb.AttachmentOptions{Enabled: true, Origin: "https://company.example.invalid", Verifier: auth.Verifier{Authority: "company.example.invalid"}, Devices: &memberpg.DeviceStore{}, Sessions: attachmentSessionsStub{}}); err != nil {
		t.Fatal(err)
	}
	return doc, router
}

// EMP-OFFL-005 (EDR-270): the signed attachment session is its own special
// boundary; no weakened, ambiguous or cookie variant passes the gate.
func TestAttachmentSessionRequiresItsExactSignedBoundary(t *testing.T) {
	doc, router := attachmentSessionRoutes(t)
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal("actual attachment session endpoint rejected", err)
	}
	op := doc.Paths.Value(syncweb.AttachmentsPath).Post
	metadata := op.Extensions["x-abastevo-signed-attachment-session"]
	full := map[string]any{"account_proof": true, "signed": true, "current_enrollment": true, "creator_bound": true, "owner_linked": true, "whole_body_preflight": true, "success": 200}
	for _, key := range []string{"account_proof", "signed", "current_enrollment", "creator_bound", "owner_linked", "whole_body_preflight"} {
		weakened := map[string]any{}
		for k, v := range full {
			weakened[k] = v
		}
		weakened[key] = false
		op.Extensions["x-abastevo-signed-attachment-session"] = weakened
		if ValidateOrganizationRoutes(router, doc) == nil {
			t.Fatal("weakened attachment boundary accepted", key)
		}
	}
	extra := map[string]any{"fallback": true}
	for k, v := range full {
		extra[k] = v
	}
	op.Extensions["x-abastevo-signed-attachment-session"] = extra
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("unknown attachment boundary field accepted")
	}
	op.Extensions["x-abastevo-signed-attachment-session"] = metadata
	security := op.Security
	cookie := openapi3.SecurityRequirements{{"CompanySessionCookie": []string{}}}
	op.Security = &cookie
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("cookie fallback admitted")
	}
	op.Security = security
	for _, other := range []string{"x-abastevo-signed-batch", "x-abastevo-signed-reference-feed", "x-abastevo-document-ingress", "x-abastevo-organization"} {
		op.Extensions[other] = map[string]any{"signed": true}
		if ValidateOrganizationRoutes(router, doc) == nil {
			t.Fatal("ambiguous transport authority accepted", other)
		}
		delete(op.Extensions, other)
	}
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
	router.Post(syncweb.AttachmentsPath, func(http.ResponseWriter, *http.Request) {})
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("unguarded attachment session route accepted")
	}
}

func TestAttachmentSessionBodyProofAndResponseCannotDrift(t *testing.T) {
	doc, router := attachmentSessionRoutes(t)
	op := doc.Paths.Value(syncweb.AttachmentsPath).Post
	schema := op.RequestBody.Value.Content.Get("application/json").Schema.Value
	for _, field := range []string{"schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id", "declaration_id", "document_id", "kind", "sha256", "bytes", "media_type", "link_id", "target_kind", "target_id"} {
		before := schema.Required
		schema.Required = nil
		for _, key := range before {
			if key != field {
				schema.Required = append(schema.Required, key)
			}
		}
		if ValidateOrganizationRoutes(router, doc) == nil {
			t.Fatal("attachment request lost a required field", field)
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
		t.Fatal("replayable unsigned attachment request accepted")
	}
	op.Parameters = parameters
	request := op.RequestBody.Value.Content.Get("application/json").Schema
	before := request.Ref
	request.Ref = "#/components/schemas/EnterpriseDocumentCreateRequest"
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("company declaration body substituted for the signed session")
	}
	request.Ref = before
	response := op.Responses.Value("200").Value.Content.Get("application/json").Schema
	beforeResponse := response.Ref
	response.Ref = "#/components/schemas/EnterpriseDocumentCreated"
	if ValidateOrganizationRoutes(router, doc) == nil {
		t.Fatal("company declaration response substituted for the signed session")
	}
	response.Ref = beforeResponse
	if err := ValidateOrganizationRoutes(router, doc); err != nil {
		t.Fatal(err)
	}
}
