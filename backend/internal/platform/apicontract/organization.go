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
		ownedSetups, setupsGuarded := handler.(orgapi.OwnedSetupsDescriptor)
		contact, contactGuarded := handler.(orgapi.DeviceContactDescriptor)
		batch, batchGuarded := handler.(orgapi.SignedBatchDescriptor)
		reviewResults, reviewResultsGuarded := handler.(orgapi.SignedReviewResultsDescriptor)
		selfNotice, selfNoticeGuarded := handler.(orgapi.SelfNoticeDescriptor)
		ingress, ingressGuarded := handler.(orgapi.DocumentIngressDescriptor)
		referenceFeed, referenceGuarded := handler.(orgapi.SignedReferenceFeedDescriptor)
		attachmentSession, attachmentGuarded := handler.(orgapi.SignedAttachmentSessionDescriptor)
		item := document.Paths.Value(path)
		if method != http.MethodPost || (!guarded && !companyGuarded && !creationGuarded && !personalGuarded && !selfGuarded && !signedDeviceGuarded && !selfDeviceGuarded && !contextGuarded && !setupsGuarded && !contactGuarded && !batchGuarded && !reviewResultsGuarded && !selfNoticeGuarded && !ingressGuarded && !referenceGuarded && !attachmentGuarded) || item == nil {
			return fmt.Errorf("unguarded or undocumented organization route %s %s", method, path)
		}
		operation := item.GetOperation(method)
		if operation == nil {
			return fmt.Errorf("missing organization operation %s %s", method, path)
		}
		if !selfNoticeGuarded && operation.Extensions["x-abastevo-self-notice"] != nil {
			return fmt.Errorf("unexpected self notice metadata for %s", path)
		}
		if !setupsGuarded && operation.Extensions["x-abastevo-owned-setups"] != nil {
			return fmt.Errorf("unexpected owned setups metadata for %s", path)
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
		if !contactGuarded && operation.Extensions["x-abastevo-device-contact"] != nil {
			return fmt.Errorf("unexpected device contact metadata for %s", path)
		}
		if !batchGuarded && operation.Extensions["x-abastevo-signed-batch"] != nil {
			return fmt.Errorf("unexpected batch authority metadata for %s", path)
		}
		if !reviewResultsGuarded && operation.Extensions["x-abastevo-signed-review-results"] != nil {
			return fmt.Errorf("unexpected author result authority metadata for %s", path)
		}
		if !ingressGuarded && operation.Extensions["x-abastevo-document-ingress"] != nil {
			return fmt.Errorf("unexpected document ingress metadata for %s", path)
		}
		if !referenceGuarded && operation.Extensions["x-abastevo-signed-reference-feed"] != nil {
			return fmt.Errorf("unexpected reference feed metadata for %s", path)
		}
		if !attachmentGuarded && operation.Extensions["x-abastevo-signed-attachment-session"] != nil {
			return fmt.Errorf("unexpected attachment session metadata for %s", path)
		}
		bodyType := "application/json"
		if attachmentGuarded {
			// EMP-OFFL-005: the enrolled member's own signed attachment session (EDR-270).
			expected := orgapi.SignedAttachmentSessionRule{AccountProof: true, Signed: true, CurrentEnrollment: true, CreatorBound: true, OwnerLinked: true, WholeBodyPreflight: true, Success: 200}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || signedDeviceGuarded || selfDeviceGuarded || contextGuarded || setupsGuarded || contactGuarded || batchGuarded || reviewResultsGuarded || selfNoticeGuarded || ingressGuarded || referenceGuarded ||
				path != "/v1/orgs/{org_id}/sync/attachments" || attachmentSession.SignedAttachmentSessionRule() != expected {
				return fmt.Errorf("invalid signed attachment session guard for %s", path)
			}
			for _, key := range []string{"x-abastevo-organization", "x-abastevo-company-session", "x-abastevo-organization-creation", "x-abastevo-personal-invitation", "x-abastevo-self-membership", "x-abastevo-signed-device", "x-abastevo-self-device", "x-abastevo-account-context", "x-abastevo-owned-setups", "x-abastevo-device-contact", "x-abastevo-signed-batch", "x-abastevo-signed-review-results", "x-abastevo-self-notice", "x-abastevo-document-ingress", "x-abastevo-signed-reference-feed"} {
				if operation.Extensions[key] != nil {
					return fmt.Errorf("unexpected authority metadata for attachment session %s", path)
				}
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-signed-attachment-session"])
			var recorded orgapi.SignedAttachmentSessionRule
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.DisallowUnknownFields()
			if e != nil || decoder.Decode(&recorded) != nil || recorded != expected {
				return fmt.Errorf("signed attachment session metadata differs for %s", path)
			}
			if e := validateSignedDeviceSecurity(operation, path); e != nil {
				return e
			}
			body := operation.RequestBody.Value.Content.Get("application/json").Schema
			if body.Ref != "#/components/schemas/EnterpriseAttachmentSessionRequest" || !exactReviewObject(body.Value, "schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id",
				"declaration_id", "document_id", "kind", "sha256", "bytes", "media_type", "link_id", "target_kind", "target_id") {
				return fmt.Errorf("invalid whole attachment session body for %s", path)
			}
			if operation.Responses == nil || operation.Responses.Value("200") == nil || operation.Responses.Value("200").Value == nil {
				return fmt.Errorf("missing attachment session response for %s", path)
			}
			media := operation.Responses.Value("200").Value.Content.Get("application/json")
			if media == nil || media.Schema == nil || media.Schema.Ref != "#/components/schemas/EnterpriseAttachmentSessionResponse" ||
				!exactReviewObject(media.Schema.Value, "schema_version", "request_id", "membership_id", "enrollment_id", "document_id", "storage", "storage_revision", "upload") {
				return fmt.Errorf("invalid attachment session response for %s", path)
			}
			declared.Success = 200
		} else if referenceGuarded {
			// EMP-OFFL-006: the member's signed, purpose-bound reference feed.
			expected := orgapi.SignedReferenceFeedRule{AccountProof: true, Signed: true, CurrentEnrollment: true, BoundCursor: true, WholeBodyPreflight: true, Success: 200}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || signedDeviceGuarded || selfDeviceGuarded || contextGuarded || setupsGuarded || contactGuarded || batchGuarded || reviewResultsGuarded || selfNoticeGuarded || ingressGuarded || path != "/v1/orgs/{org_id}/sync/reference" || referenceFeed.SignedReferenceFeedRule() != expected {
				return fmt.Errorf("invalid signed reference feed guard for %s", path)
			}
			for _, key := range []string{"x-abastevo-organization", "x-abastevo-company-session", "x-abastevo-organization-creation", "x-abastevo-personal-invitation", "x-abastevo-self-membership", "x-abastevo-signed-device", "x-abastevo-self-device", "x-abastevo-account-context", "x-abastevo-owned-setups", "x-abastevo-device-contact", "x-abastevo-signed-batch", "x-abastevo-signed-review-results", "x-abastevo-self-notice", "x-abastevo-document-ingress"} {
				if operation.Extensions[key] != nil {
					return fmt.Errorf("unexpected authority metadata for reference feed %s", path)
				}
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-signed-reference-feed"])
			var recorded orgapi.SignedReferenceFeedRule
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.DisallowUnknownFields()
			if e != nil || decoder.Decode(&recorded) != nil || recorded != expected {
				return fmt.Errorf("signed reference feed metadata differs for %s", path)
			}
			if e := validateSignedDeviceSecurity(operation, path); e != nil {
				return e
			}
			body := operation.RequestBody.Value.Content.Get("application/json").Schema
			if body == nil || body.Ref != "#/components/schemas/EnterpriseReferenceFeedRequest" ||
				!exactTripReferenceRequest(body.Value) {
				return fmt.Errorf("invalid whole reference feed body for %s", path)
			}
			if operation.Responses == nil || operation.Responses.Value("200") == nil || operation.Responses.Value("200").Value == nil {
				return fmt.Errorf("missing reference feed response for %s", path)
			}
			media := operation.Responses.Value("200").Value.Content.Get("application/json")
			if media == nil || media.Schema == nil || !exactReferenceResponses(media.Schema.Value) {
				return fmt.Errorf("invalid reference feed response for %s", path)
			}
			declared.Success = 200
		} else if ingressGuarded {
			if e := validateDocumentIngress(operation, path, ingress.DocumentIngressRule(), guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || signedDeviceGuarded || selfDeviceGuarded || contextGuarded || setupsGuarded || contactGuarded || batchGuarded || reviewResultsGuarded || selfNoticeGuarded); e != nil {
				return e
			}
			bodyType = "image/jpeg"
			declared.Success = 200
		} else if selfNoticeGuarded {
			rule := selfNotice.SelfNoticeRule()
			expected := orgapi.SelfNoticeRule{Action: rule.Action, Cookie: true, CSRF: true, Binding: true, Success: 200}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || signedDeviceGuarded || selfDeviceGuarded || contextGuarded || setupsGuarded || contactGuarded || batchGuarded || reviewResultsGuarded || ingressGuarded ||
				selfNoticePaths[rule.Action] == "" || path != selfNoticePaths[rule.Action] || rule != expected {
				return fmt.Errorf("invalid self notice guard for %s", path)
			}
			for _, key := range []string{"x-abastevo-organization", "x-abastevo-company-session", "x-abastevo-organization-creation", "x-abastevo-personal-invitation", "x-abastevo-self-membership", "x-abastevo-signed-device", "x-abastevo-self-device", "x-abastevo-account-context", "x-abastevo-owned-setups", "x-abastevo-device-contact", "x-abastevo-signed-batch", "x-abastevo-signed-review-results"} {
				if operation.Extensions[key] != nil {
					return fmt.Errorf("unexpected authority metadata for self notice %s", path)
				}
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-self-notice"])
			var recorded orgapi.SelfNoticeRule
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.DisallowUnknownFields()
			if e != nil || decoder.Decode(&recorded) != nil || recorded != expected {
				return fmt.Errorf("self notice metadata differs for %s", path)
			}
			if e := validateCompanySecurity(document, operation, path, orgapi.CompanySessionRule{Cookie: true, CSRF: true, Success: 200}); e != nil {
				return e
			}
			declared.Success = 200
		} else if reviewResultsGuarded {
			expected := orgapi.SignedReviewResultsRule{AccountProof: true, Signed: true, ExactEnrollment: true, OriginalAuthor: true, WholeBodyPreflight: true, Success: 200}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || signedDeviceGuarded || selfDeviceGuarded || contextGuarded || setupsGuarded || contactGuarded || batchGuarded || path != "/v1/orgs/{org_id}/sync/review-results" || reviewResults.SignedReviewResultsRule() != expected {
				return fmt.Errorf("invalid signed original-author result guard for %s", path)
			}
			for _, key := range []string{"x-abastevo-organization", "x-abastevo-company-session", "x-abastevo-organization-creation", "x-abastevo-personal-invitation", "x-abastevo-self-membership", "x-abastevo-signed-device", "x-abastevo-self-device", "x-abastevo-account-context", "x-abastevo-owned-setups", "x-abastevo-device-contact", "x-abastevo-signed-batch"} {
				if operation.Extensions[key] != nil {
					return fmt.Errorf("unexpected authority metadata for own review results %s", path)
				}
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-signed-review-results"])
			var recorded orgapi.SignedReviewResultsRule
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.DisallowUnknownFields()
			if e != nil || decoder.Decode(&recorded) != nil || recorded != expected {
				return fmt.Errorf("signed author result metadata differs for %s", path)
			}
			if e := validateSignedDeviceSecurity(operation, path); e != nil {
				return e
			}
			body := operation.RequestBody.Value.Content.Get("application/json").Schema
			if body.Ref != "#/components/schemas/EnterpriseReviewResultsRequest" || !exactReviewObject(body.Value, "schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id", "selectors") {
				return fmt.Errorf("invalid whole author result body for %s", path)
			}
			selectors := body.Value.Properties["selectors"]
			if selectors == nil || selectors.Value == nil || !selectors.Value.Type.Is("array") || selectors.Value.MaxItems == nil || *selectors.Value.MaxItems != 50 || selectors.Value.Items == nil || !exactReviewObject(selectors.Value.Items.Value, "operation_id", "payload_digest") {
				return fmt.Errorf("invalid bounded author selectors for %s", path)
			}
			if operation.Responses == nil || operation.Responses.Value("200") == nil || operation.Responses.Value("200").Value == nil {
				return fmt.Errorf("missing author results response for %s", path)
			}
			media := operation.Responses.Value("200").Value.Content.Get("application/json")
			if media == nil || media.Schema == nil || media.Schema.Ref != "#/components/schemas/EnterpriseReviewResultsResponse" {
				return fmt.Errorf("invalid complete author results response for %s", path)
			}
			declared.Success = 200
		} else if batchGuarded {
			expected := orgapi.SignedBatchRule{AccountProof: true, Signed: true, CurrentEnrollment: true, WholeBodyPreflight: true, PerOperation: true, Success: 200}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || signedDeviceGuarded || selfDeviceGuarded || contextGuarded || setupsGuarded || contactGuarded || path != "/v1/orgs/{org_id}/sync/operations" || batch.SignedBatchRule() != expected {
				return fmt.Errorf("invalid signed batch guard for %s", path)
			}
			for _, key := range []string{"x-abastevo-organization", "x-abastevo-company-session", "x-abastevo-organization-creation", "x-abastevo-personal-invitation", "x-abastevo-self-membership", "x-abastevo-signed-device", "x-abastevo-self-device", "x-abastevo-account-context", "x-abastevo-owned-setups", "x-abastevo-device-contact"} {
				if operation.Extensions[key] != nil {
					return fmt.Errorf("unexpected authority metadata for signed batch %s", path)
				}
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-signed-batch"])
			var recorded orgapi.SignedBatchRule
			if e != nil || json.Unmarshal(raw, &recorded) != nil || recorded != expected {
				return fmt.Errorf("signed batch metadata differs for %s", path)
			}
			if e := validateSignedDeviceSecurity(operation, path); e != nil {
				return e
			}
			body := operation.RequestBody.Value.Content.Get("application/json").Schema
			if body.Ref != "#/components/schemas/EnterpriseSyncBatch" || len(body.Value.Required) != 4 || len(body.Value.Properties) != 4 {
				return fmt.Errorf("invalid complete batch body for %s", path)
			}
			for _, name := range []string{"schema_version", "operations"} {
				found := false
				for _, required := range body.Value.Required {
					if required == name {
						found = true
					}
				}
				if !found || body.Value.Properties[name] == nil {
					return fmt.Errorf("missing batch wrapper field %s", name)
				}
			}
			declared.Success = 200
		} else if contactGuarded {
			expected := orgapi.DeviceContactRule{AccountProof: true, Signed: true, ExactEnrollment: true, TerminalRead: true, PolicyLease: true, Success: 200}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || signedDeviceGuarded || selfDeviceGuarded || contextGuarded || setupsGuarded || path != "/v1/orgs/{org_id}/devices/contact" || contact.DeviceContactRule() != expected {
				return fmt.Errorf("invalid device contact guard for %s", path)
			}
			for _, key := range []string{"x-abastevo-organization", "x-abastevo-company-session", "x-abastevo-organization-creation", "x-abastevo-personal-invitation", "x-abastevo-self-membership", "x-abastevo-signed-device", "x-abastevo-self-device", "x-abastevo-account-context", "x-abastevo-owned-setups"} {
				if operation.Extensions[key] != nil {
					return fmt.Errorf("unexpected authority metadata for device contact %s", path)
				}
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-device-contact"])
			var recorded orgapi.DeviceContactRule
			if e != nil || json.Unmarshal(raw, &recorded) != nil || recorded != expected {
				return fmt.Errorf("device contact metadata differs for %s", path)
			}
			if e := validateSignedDeviceSecurity(operation, path); e != nil {
				return e
			}
			schema := operation.RequestBody.Value.Content.Get("application/json").Schema.Value
			for _, name := range []string{"schema_version", "request_id", "membership_id", "enrollment_id"} {
				found := false
				for _, required := range schema.Required {
					if required == name {
						found = true
					}
				}
				if !found || schema.Properties[name] == nil {
					return fmt.Errorf("missing exact contact selector %s for %s", name, path)
				}
			}
			declared.Success = 200
		} else if setupsGuarded {
			expected := orgapi.OwnedSetupsRule{Cookie: true, CSRF: true, Binding: true, Success: 200}
			if guarded || companyGuarded || creationGuarded || personalGuarded || selfGuarded || signedDeviceGuarded || selfDeviceGuarded || contextGuarded || path != "/v1/org-access/organizations/setups/list" || ownedSetups.OwnedSetupsRule() != expected {
				return fmt.Errorf("invalid owned setups guard for %s", path)
			}
			for _, key := range []string{"x-abastevo-organization", "x-abastevo-company-session", "x-abastevo-organization-creation", "x-abastevo-personal-invitation", "x-abastevo-self-membership", "x-abastevo-signed-device", "x-abastevo-self-device", "x-abastevo-account-context"} {
				if operation.Extensions[key] != nil {
					return fmt.Errorf("unexpected authority metadata for owned setups %s", path)
				}
			}
			raw, e := json.Marshal(operation.Extensions["x-abastevo-owned-setups"])
			var recorded orgapi.OwnedSetupsRule
			if e != nil || json.Unmarshal(raw, &recorded) != nil || recorded != expected {
				return fmt.Errorf("owned setups metadata differs for %s", path)
			}
			if e := validateCompanySecurity(document, operation, path, orgapi.CompanySessionRule{Cookie: true, CSRF: true, Success: 200}); e != nil {
				return e
			}
			declared.Success = 200
		} else if contextGuarded {
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
			!operation.RequestBody.Value.Required || operation.RequestBody.Value.Content.Get(bodyType) == nil {
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

// selfNoticePaths pins each own-inbox action to its only allowed route.
var selfNoticePaths = map[string]string{
	"list":               "/v1/orgs/{org_id}/memberships/{membership_id}/notifications/self/list",
	"read":               "/v1/orgs/{org_id}/memberships/{membership_id}/notifications/self/{notice_id}/read",
	"preferences_read":   "/v1/orgs/{org_id}/memberships/{membership_id}/notification-preferences/self/read",
	"preferences_update": "/v1/orgs/{org_id}/memberships/{membership_id}/notification-preferences/self/update",
}

func exactTripReferenceRequest(schema *openapi3.Schema) bool {
	if schema == nil || len(schema.Properties) != 8 || schema.Properties["trip_id"] == nil ||
		schema.Properties["trip_id"].Ref != "#/components/schemas/EnterpriseUUID" {
		return false
	}
	assetSchema := *schema
	assetSchema.Properties = make(openapi3.Schemas, len(schema.Properties)-1)
	for key, value := range schema.Properties {
		if key != "trip_id" {
			assetSchema.Properties[key] = value
		}
	}
	return exactReviewObject(&assetSchema, "schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id", "cursor")
}

func exactReferenceResponses(schema *openapi3.Schema) bool {
	if schema == nil || len(schema.OneOf) != 2 {
		return false
	}
	asset, field := schema.OneOf[0], schema.OneOf[1]
	if asset == nil || asset.Ref != "#/components/schemas/EnterpriseReferenceFeedResponse" ||
		field == nil || field.Ref != "#/components/schemas/EnterpriseTripFieldResponse" ||
		!exactReviewObject(field.Value, "schema_version", "request_id", "membership_id", "enrollment_id", "field") {
		return false
	}
	return exactReviewObject(field.Value.Properties["field"].Value,
		"trip_id", "plan", "expired", "due_soon", "nearest_days", "open_maintenance", "voucher")
}

func exactReviewObject(schema *openapi3.Schema, names ...string) bool {
	if schema == nil || !schema.Type.Is("object") || len(schema.Properties) != len(names) || len(schema.Required) != len(names) || schema.AdditionalProperties.Has == nil || *schema.AdditionalProperties.Has {
		return false
	}
	seen := make(map[string]bool, len(names))
	for _, key := range schema.Required {
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	for _, key := range names {
		if !seen[key] || schema.Properties[key] == nil {
			return false
		}
	}
	return true
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

// validateDocumentIngress pins the single capability-bound byte boundary: no
// cookie, signed proof or JSON body, one required capability header and one
// exact binary JPEG body (EMP-DOC-002, EDR-153).
func validateDocumentIngress(operation *openapi3.Operation, path string, rule orgapi.DocumentIngressRule, other bool) error {
	expected := orgapi.DocumentIngressRule{Capability: true, MemberBound: true, CurrentAuthority: true, VerifiedBytes: true, Permission: "doc.document.attach", Success: 200}
	if other || path != "/v1/orgs/{org_id}/documents/{document_id}/content/upload" || rule != expected {
		return fmt.Errorf("invalid document ingress guard for %s", path)
	}
	for key := range operation.Extensions {
		if strings.HasPrefix(key, "x-abastevo-") && key != "x-abastevo-document-ingress" {
			return fmt.Errorf("unexpected authority metadata for document ingress %s", path)
		}
	}
	raw, e := json.Marshal(operation.Extensions["x-abastevo-document-ingress"])
	var recorded orgapi.DocumentIngressRule
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if e != nil || decoder.Decode(&recorded) != nil || recorded != expected {
		return fmt.Errorf("document ingress metadata differs for %s", path)
	}
	if operation.Security == nil || len(*operation.Security) != 0 {
		return fmt.Errorf("unexpected cookie authority for document ingress %s", path)
	}
	found := false
	for _, parameter := range operation.Parameters {
		if parameter.Value != nil && parameter.Value.In == "header" && parameter.Value.Name == orgapi.DocumentCapabilityHeader && parameter.Value.Required {
			found = true
		}
		if parameter.Value != nil && parameter.Value.In == "query" {
			return fmt.Errorf("document ingress query parameter for %s", path)
		}
	}
	if !found || operation.RequestBody == nil || operation.RequestBody.Value == nil || len(operation.RequestBody.Value.Content) != 1 {
		return fmt.Errorf("missing document ingress capability or body for %s", path)
	}
	media := operation.RequestBody.Value.Content.Get("image/jpeg")
	if media == nil || media.Schema == nil || media.Schema.Value == nil || !media.Schema.Value.Type.Is("string") || media.Schema.Value.Format != "binary" ||
		media.Schema.Value.MaxLength == nil || *media.Schema.Value.MaxLength != 5<<20 {
		return fmt.Errorf("invalid document ingress body for %s", path)
	}
	return nil
}
