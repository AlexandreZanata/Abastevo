// Package http binds the private document routes to the shared organization
// boundary: guarded company routes plus one capability-bound byte ingress.
package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

const (
	Prefix     = "/v1/orgs/{org_id}/documents"
	CreatePath = Prefix + "/uploads/create"
	UploadPath = Prefix + "/{document_id}/content/upload"
	AccessPath = Prefix + "/{document_id}/content/access"
	ReadPath   = Prefix + "/{document_id}/content/read"
)

type privateRequest struct{}

func (privateRequest) String() string                 { return "EnterpriseDocumentRequest(redacted)" }
func (v privateRequest) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, v.String()) }
func (v privateRequest) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func (v privateRequest) LogValue() slog.Value         { return slog.StringValue(v.String()) }

type companyRequest struct{ privateRequest }

func (companyRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type originalFields struct {
	SHA256    string `json:"sha256"`
	Bytes     *int64 `json:"bytes"`
	MediaType string `json:"media_type"`
}
type createRequest struct {
	companyRequest
	SchemaVersion int             `json:"schema_version"`
	OperationID   string          `json:"operation_id"`
	DocumentID    string          `json:"document_id"`
	Kind          string          `json:"kind"`
	Original      *originalFields `json:"original"`
}

func (v createRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type accessRequest struct{ companyRequest }
type readRequest struct {
	companyRequest
	Capability string `json:"capability"`
}

// mapped keeps the bounded private envelope; forbidden carries only the
// route's registered permission key (BR-AUTH-013).
func mapped(err error, permission string) error {
	var denied *permissions.PermissionDenial
	if errors.As(err, &denied) {
		return orgapi.MissingPermission(string(denied.Permission()))
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	if errors.Is(err, media.ErrForbidden) {
		return orgapi.MissingPermission(permission)
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrDisabled, orgapi.NotFound}, {app.ErrNotFound, orgapi.NotFound}, {media.ErrNotFound, orgapi.NotFound}, {media.ErrDisabled, orgapi.NotFound},
		{access.ErrNoMembership, orgapi.NotFound}, {access.ErrNotFound, orgapi.NotFound}, {permissions.ErrResourceNotFound, orgapi.NotFound},
		{app.ErrInvalid, orgapi.ValidationFailed}, {media.ErrIntegrity, orgapi.ValidationFailed}, {media.ErrTooLarge, orgapi.ValidationFailed},
		{media.ErrUnsupported, orgapi.ValidationFailed}, {media.ErrUnsafe, orgapi.ValidationFailed}, {media.ErrInvalid, orgapi.ValidationFailed},
		{app.ErrConflict, orgapi.Conflict}, {app.ErrOperationReused, orgapi.OperationReused},
		{access.ErrMembershipSuspended, orgapi.MembershipSuspended}, {access.ErrOrgSuspended, orgapi.OrgSuspended}, {access.ErrOrgArchived, orgapi.OrgArchived},
		{access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete}, {access.ErrUnauthenticated, orgapi.Unauthenticated}, {access.ErrFreshAuthRequired, orgapi.FreshAuthRequired},
		{access.ErrEntitlementRequired, orgapi.EntitlementRequired}, {access.ErrLimitReached, orgapi.LimitReached},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}

func instant(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }

// projectView exposes the creator's container only: no storage key, URL,
// creator membership or content.
func projectView(v app.View) map[string]any {
	out := map[string]any{"id": v.ID, "kind": v.Kind, "data_mode": v.DataMode,
		"original": map[string]any{"sha256": v.Original.SHA256, "bytes": v.Original.Bytes, "media_type": v.Original.MediaType},
		"statuses": map[string]any{"storage": v.Statuses.Storage, "extraction": v.Statuses.Extraction, "structure": v.Statuses.Structure,
			"conference": v.Statuses.Conference, "reconciliation": v.Statuses.Reconciliation, "fiscal": v.Statuses.Fiscal},
		"revisions": map[string]any{"storage": v.Revisions.Storage, "extraction": v.Revisions.Extraction, "structure": v.Revisions.Structure,
			"conference": v.Revisions.Conference, "fiscal": v.Revisions.Fiscal},
		"created_at": instant(v.CreatedAt), "updated_at": instant(v.UpdatedAt)}
	if v.FiscalConsultedAt != nil {
		out["fiscal_consulted_at"] = instant(*v.FiscalConsultedAt)
	}
	return out
}

func validID(raw string) bool { id, e := kernel.ParseOrgID(raw); return e == nil && id.String() == raw }

func documentPathID(ctx context.Context) (string, bool) {
	raw := chi.URLParamFromCtx(ctx, "document_id")
	return raw, validID(raw)
}

func principalActor(p orgapi.Principal) app.Actor {
	return app.Actor{AccountID: p.AccountID, MembershipID: p.MembershipID, Channel: p.Channel, ContributorID: p.ContributorID, DeviceKeyID: p.DeviceKeyID}
}

// DocumentRecords accepts only this module's fixed source policies: a new
// document is the subject's own record on the organization root, and content
// targets an existing document.
type DocumentRecords struct{}

func (DocumentRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	root := scope.OrgID().String()
	switch policy {
	case app.AttachPolicy():
		if facts.ID != root || facts.AuthoredBy != subject.MembershipID() {
			return access.ErrUnavailable
		}
	case app.ContentPolicy():
		if facts.ID == root {
			return access.ErrUnavailable
		}
	default:
		return access.ErrUnavailable
	}
	return nil
}

type createResource struct{}

func (createResource) Resolve(_ context.Context, scope kernel.OrgScope, subject access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil || !validID(subject.MembershipID()) {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String(), AuthoredBy: subject.MembershipID()}, nil
}

type documentResource struct{ repository app.Repository }

func (r documentResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	id, ok := documentPathID(ctx)
	if !ok || scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	facts, err := r.repository.Facts(ctx, scope, id)
	if errors.Is(err, app.ErrNotFound) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	if err != nil || facts.TargetID != id {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id, AuthoredBy: facts.AuthoredBy}, nil
}

func rule(policy access.Policy, success int) orgapi.Rule {
	return orgapi.Rule{Permission: string(policy.Permission), Write: policy.Effect.Write(), Fresh: policy.Fresh, CompanyOnly: true, Success: success}
}

// Register mounts nothing while the service is disabled.
func Register(router chi.Router, service app.Service, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	if router == nil || service.Repository == nil || service.Tokens == nil || authorization.Source == nil || authorization.Capabilities == nil || authorization.Records == nil || authorization.Now == nil {
		return app.ErrUnavailable
	}
	ingress, err := Ingress(service, gateway.Origin)
	if err != nil {
		return err
	}
	owned, entry, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	attach, content := app.AttachPolicy(), app.ContentPolicy()
	if err := guarded.Register(owned, CreatePath, entry, rule(attach, http.StatusCreated), authorization, attach, createResource{}, func(ctx context.Context, p orgapi.Principal, _ access.Decision, input createRequest) (any, error) {
		if input.Original == nil || input.Original.Bytes == nil {
			return nil, orgapi.Malformed
		}
		created, err := service.Create(ctx, p.Scope, principalActor(p), app.Create{OperationID: input.OperationID, DocumentID: input.DocumentID,
			Original: app.Declaration{Kind: input.Kind, SHA256: input.Original.SHA256, Bytes: *input.Original.Bytes, MediaType: input.Original.MediaType}})
		if err != nil {
			return nil, mapped(err, string(attach.Permission))
		}
		out := map[string]any{"document": projectView(created.View), "replayed": created.Replayed}
		if created.Upload != nil {
			out["upload"] = map[string]any{"capability": created.Upload.Token, "expires_at": instant(created.Upload.ExpiresAt)}
		}
		return out, nil
	}); err != nil {
		return err
	}
	resource := documentResource{repository: service.Repository}
	if err := guarded.Register(owned, AccessPath, entry, rule(content, http.StatusOK), authorization, content, resource, func(ctx context.Context, p orgapi.Principal, _ access.Decision, _ accessRequest) (any, error) {
		id, ok := documentPathID(ctx)
		if !ok {
			return nil, orgapi.NotFound
		}
		granted, err := service.IssueContent(ctx, p.Scope, principalActor(p), id)
		if err != nil {
			return nil, mapped(err, string(content.Permission))
		}
		return map[string]any{"document_id": id, "capability": granted.Token, "expires_at": instant(granted.ExpiresAt)}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, ReadPath, entry, rule(content, http.StatusOK), authorization, content, resource, func(ctx context.Context, p orgapi.Principal, _ access.Decision, input readRequest) (any, error) {
		id, ok := documentPathID(ctx)
		if !ok {
			return nil, orgapi.NotFound
		}
		if input.Capability == "" || len(input.Capability) > 2048 {
			return nil, orgapi.NotFound
		}
		original, err := service.ReadContent(ctx, p.Scope, principalActor(p), id, input.Capability)
		if err != nil {
			return nil, mapped(err, string(content.Permission))
		}
		return map[string]any{"document_id": id, "data_mode": app.DataMode, "media_type": original.MIME, "bytes": len(original.Bytes),
			"content_base64": base64.StdEncoding.EncodeToString(original.Bytes)}, nil
	}); err != nil {
		return err
	}
	if err := registerLinks(owned, entry, service, authorization); err != nil {
		return err
	}
	router.Method(http.MethodPost, UploadPath, ingress)
	return nil
}

// ingress is the DOCUMENT_INGRESS boundary (EDR-153): transport and
// capability first, then the creator/authority recheck and verified upload.
type ingress struct {
	service app.Service
	origin  string
}

// Ingress is the capability-bound byte boundary for one document upload.
func Ingress(service app.Service, origin string) (http.Handler, error) {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || strings.ToLower(parsed.Host) != parsed.Host {
		return nil, app.ErrUnavailable
	}
	return ingress{service: service, origin: origin}, nil
}

func (ingress) DocumentIngressRule() orgapi.DocumentIngressRule {
	return orgapi.DocumentIngressRule{Capability: true, MemberBound: true, CurrentAuthority: true, VerifiedBytes: true, Permission: string(app.AttachPolicy().Permission), Success: http.StatusOK}
}

func (h ingress) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("ETag")
	if !h.service.Enabled {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	if origins := r.Header.Values("Origin"); len(origins) > 0 && (len(origins) != 1 || origins[0] != h.origin) {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	capabilities := r.Header.Values(orgapi.DocumentCapabilityHeader)
	if len(capabilities) != 1 || capabilities[0] == "" {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	rawOrg, id := chi.URLParam(r, "org_id"), chi.URLParam(r, "document_id")
	org, err := kernel.ParseOrgID(rawOrg)
	if err != nil || org.String() != rawOrg || !validID(id) {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	actor, err := h.service.VerifyUpload(scope, id, capabilities[0])
	if err != nil {
		orgapi.WriteError(w, r, mapped(err, string(app.AttachPolicy().Permission)))
		return
	}
	limit, _ := media.ByteLimit(app.MediaTypeJPEG)
	mediaType, parameters, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || len(r.Header.Values("Content-Type")) != 1 || mediaType != app.MediaTypeJPEG || len(parameters) != 0 || r.ContentLength > limit {
		orgapi.WriteError(w, r, orgapi.ValidationFailed)
		return
	}
	// Context alone cannot interrupt an arbitrary reader (EMP-PLAT-006).
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(15 * time.Second))
	stored, err := h.service.Upload(r.Context(), scope, actor, id, http.MaxBytesReader(w, r.Body, limit+1))
	if err != nil {
		orgapi.WriteError(w, r, mapped(err, string(app.AttachPolicy().Permission)))
		return
	}
	body, err := json.Marshal(map[string]any{"document": projectView(stored.View), "replayed": stored.Replayed})
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
