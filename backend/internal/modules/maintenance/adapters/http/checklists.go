// Package http exposes the checklist template commands and reads of
// EMP-CHECK-001 through orgapi (EDR-215).
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
	"unicode/utf8"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/domain/checklist"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	organization "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const ChecklistPrefix = "/v1/orgs/{org_id}/checklists"
const TemplateCreatePath = ChecklistPrefix + "/templates/create"
const TemplateListPath = ChecklistPrefix + "/templates/list"
const TemplateReadPath = ChecklistPrefix + "/templates/{template_id}/read"
const TemplatePublishPath = ChecklistPrefix + "/templates/{template_id}/publish"
const TemplatePolicyPath = ChecklistPrefix + "/templates/{template_id}/policy"
const TemplateArchivePath = ChecklistPrefix + "/templates/{template_id}/archive"
const ApplicablePath = ChecklistPrefix + "/assets/{asset_id}/applicable"

// ChecklistRecords accepts only this module's fixed policies: the template
// policies on the organization target, the asset read on an asset target.
type ChecklistRecords struct{}

func (ChecklistRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.Validate(scope) != nil || facts.OrgID != scope.OrgID() || len(rules) == 0 {
		return access.ErrUnavailable
	}
	root := facts.ID == scope.OrgID().String()
	switch policy {
	case app.Policy(app.TemplateRead), app.Policy(app.TemplateManage):
		if root {
			return nil
		}
	case app.Policy(app.ApplicableRead):
		if !root {
			return nil
		}
	}
	return access.ErrUnavailable
}

// templateRoot resolves the organization itself: no client-selected target.
type templateRoot struct{}

func (templateRoot) Resolve(_ context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}, nil
}

// assetResource resolves the path asset as the protected record.
type assetResource struct{}

func (assetResource) Resolve(ctx context.Context, scope kernel.OrgScope, _ access.Subject) (permissions.ResourceFacts, error) {
	if scope.Validate() != nil {
		return permissions.ResourceFacts{}, access.ErrUnavailable
	}
	id := chi.URLParamFromCtx(ctx, "asset_id")
	if !checklist.CanonicalID(id) {
		return permissions.ResourceFacts{}, access.ErrNotFound
	}
	return permissions.ResourceFacts{OrgID: scope.OrgID(), ID: id}, nil
}

type companyRequest struct{}

func (companyRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type itemRequest struct {
	Label        string `json:"label"`
	Critical     *bool  `json:"critical"`
	Optional     *bool  `json:"optional"`
	PhotoOnNotOK *bool  `json:"photo_on_not_ok"`
}

type policyRequest struct {
	Mode            string `json:"mode"`
	ValiditySeconds *int64 `json:"validity_seconds"`
}

type commandFields struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
}

func (v commandFields) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type createRequest struct {
	commandFields
	Context    string         `json:"context"`
	Name       string         `json:"name"`
	TargetKind string         `json:"target_kind"`
	TargetIDs  *[]string      `json:"target_ids"`
	Items      []itemRequest  `json:"items"`
	Policy     *policyRequest `json:"policy"`
}
type publishRequest struct {
	commandFields
	Name       string        `json:"name"`
	TargetKind string        `json:"target_kind"`
	TargetIDs  *[]string     `json:"target_ids"`
	Items      []itemRequest `json:"items"`
}
type policyChangeRequest struct {
	commandFields
	Mode            string `json:"mode"`
	ValiditySeconds *int64 `json:"validity_seconds"`
}
type archiveRequest struct{ commandFields }
type listRequest struct {
	companyRequest
	orgapi.Page
}
type readRequest struct {
	companyRequest
	Version *int64 `json:"version"`
}
type applicableRequest struct {
	companyRequest
	Context string `json:"context"`
}

func (v *createRequest) UnmarshalJSON(raw []byte) error {
	type plain createRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = createRequest(in)
	return nil
}
func (v *publishRequest) UnmarshalJSON(raw []byte) error {
	type plain publishRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = publishRequest(in)
	return nil
}
func (v *policyChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain policyChangeRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = policyChangeRequest(in)
	return nil
}
func (v *archiveRequest) UnmarshalJSON(raw []byte) error {
	type plain archiveRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = archiveRequest(in)
	return nil
}
func (v *listRequest) UnmarshalJSON(raw []byte) error {
	type plain listRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = listRequest(in)
	return nil
}
func (v *readRequest) UnmarshalJSON(raw []byte) error {
	type plain readRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = readRequest(in)
	return nil
}
func (v *applicableRequest) UnmarshalJSON(raw []byte) error {
	type plain applicableRequest
	var in plain
	if err := decodeChecklist(raw, &in); err != nil {
		return err
	}
	*v = applicableRequest(in)
	return nil
}

func flag(v *bool) bool { return v != nil && *v }

// contentInput copies the exact wire content; validation is the domain's.
func contentInput(name, kind string, ids *[]string, items []itemRequest) (string, checklist.Target, []checklist.Item, error) {
	target := checklist.Target{Kind: checklist.TargetKind(kind)}
	if ids != nil {
		target.IDs = append([]string{}, (*ids)...)
	}
	if items == nil {
		return "", checklist.Target{}, nil, checklist.ErrInvalid
	}
	out := make([]checklist.Item, 0, len(items))
	for _, item := range items {
		out = append(out, checklist.Item{Label: item.Label, Critical: flag(item.Critical), Optional: flag(item.Optional), PhotoOnNotOK: flag(item.PhotoOnNotOK)})
	}
	return name, target, out, nil
}

// losslessStrings refuses invalid UTF-8 and lone escaped surrogates, which
// encoding/json would otherwise replace before the reason profile is checked.
func losslessStrings(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	unit := func(raw []byte) (int, bool) {
		if len(raw) != 4 {
			return 0, false
		}
		value := 0
		for _, b := range raw {
			value <<= 4
			switch {
			case b >= '0' && b <= '9':
				value += int(b - '0')
			case b >= 'a' && b <= 'f':
				value += int(b-'a') + 10
			case b >= 'A' && b <= 'F':
				value += int(b-'A') + 10
			default:
				return 0, false
			}
		}
		return value, true
	}
	quoted := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			quoted = !quoted
		case '\\':
			if !quoted || i+1 >= len(raw) {
				return false
			}
			i++
			if raw[i] != 'u' {
				continue
			}
			if i+4 >= len(raw) {
				return false
			}
			high, ok := unit(raw[i+1 : i+5])
			if !ok {
				return false
			}
			i += 4
			switch {
			case high >= 0xd800 && high <= 0xdbff:
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				low, ok := unit(raw[i+3 : i+7])
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			case high >= 0xdc00 && high <= 0xdfff:
				return false
			}
		}
	}
	return !quoted
}

// canonicalKeys admits only lower_snake keys and no JSON null.
func canonicalKeys(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if key == "" || key[0] < 'a' || key[0] > 'z' {
				return false
			}
			for i := 1; i < len(key); i++ {
				if b := key[i]; !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_') {
					return false
				}
			}
			if !canonicalKeys(item) {
				return false
			}
		}
	case []any:
		for _, item := range v {
			if !canonicalKeys(item) {
				return false
			}
		}
	case nil:
		return false
	}
	return true
}

func decodeChecklist[T any](raw []byte, target *T) error {
	if !losslessStrings(raw) || httpapi.DecodeJSON(raw, target) != nil {
		return orgapi.Malformed
	}
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&fields) != nil || fields == nil || !canonicalKeys(fields) {
		return orgapi.Malformed
	}
	return nil
}

func mapped(err error) error {
	var denied *permissions.PermissionDenial
	if errors.As(err, &denied) {
		return orgapi.MissingPermission(string(denied.Permission()))
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrSurfaceDisabled, orgapi.NotFound}, {app.ErrNotFound, orgapi.NotFound}, {app.ErrUnauthenticated, orgapi.Unauthenticated},
		{app.ErrOperationReused, orgapi.OperationReused}, {app.ErrConflict, orgapi.Conflict}, {checklist.ErrInvalid, orgapi.ValidationFailed},
		{access.ErrNoMembership, orgapi.NotFound}, {access.ErrNotFound, orgapi.NotFound}, {access.ErrUnauthenticated, orgapi.Unauthenticated},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired}, {access.ErrMembershipSuspended, orgapi.MembershipSuspended}, {access.ErrOrgSuspended, orgapi.OrgSuspended}, {access.ErrOrgArchived, orgapi.OrgArchived}, {access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete}, {access.ErrEntitlementRequired, orgapi.EntitlementRequired},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}

func stamp(at time.Time) (string, error) {
	out := orgtime.FormatInstant(at)
	if _, err := orgtime.ParseInstant(out); err != nil || at.IsZero() {
		return "", orgapi.Unavailable
	}
	return out, nil
}

// withArchived adds archived_at only when the template is archived; absent
// fields are omitted, never null.
func withArchived(out map[string]any, at *time.Time) (map[string]any, error) {
	if at == nil {
		return out, nil
	}
	archived, err := stamp(*at)
	if err != nil {
		return nil, err
	}
	out["archived_at"] = archived
	return out, nil
}

func projectAck(v app.TemplateAck, replayed bool) map[string]any {
	return map[string]any{"template_id": v.TemplateID, "revision": v.Revision, "version": v.Version, "policy_revision": v.PolicyRevision, "state": v.State, "replayed": replayed, "data_mode": "SYNTHETIC"}
}

func projectItems(items []checklist.Item) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for i, item := range items {
		out = append(out, map[string]any{"position": i + 1, "label": item.Label, "critical": item.Critical, "optional": item.Optional, "photo_on_not_ok": item.PhotoOnNotOK})
	}
	return out
}

func projectPolicy(p app.PolicyView) (map[string]any, error) {
	effective, err := stamp(p.EffectiveAt)
	if err != nil {
		return nil, err
	}
	return map[string]any{"revision": p.Revision, "mode": p.Policy.Mode, "validity_seconds": p.Policy.ValiditySeconds, "effective_at": effective}, nil
}

func projectSummary(t app.TemplateSummary) (map[string]any, error) {
	created, err := stamp(t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return withArchived(map[string]any{"template_id": t.TemplateID, "context": t.Context, "state": t.State, "revision": t.Revision, "version": t.Version, "policy_revision": t.PolicyRevision, "name": t.Name, "target_kind": t.TargetKind, "created_at": created}, t.ArchivedAt)
}

func projectTemplate(v app.TemplateView) (map[string]any, error) {
	created, err := stamp(v.CreatedAt)
	if err != nil {
		return nil, err
	}
	published, err := stamp(v.Selected.PublishedAt)
	if err != nil {
		return nil, err
	}
	policy, err := projectPolicy(v.Policy)
	if err != nil {
		return nil, err
	}
	ids := append([]string{}, v.Selected.Target.IDs...)
	selected := map[string]any{"version": v.Selected.Version, "name": v.Selected.Name, "target_kind": v.Selected.Target.Kind, "target_ids": ids, "items": projectItems(v.Selected.Items), "published_at": published}
	return withArchived(map[string]any{"template_id": v.TemplateID, "context": v.Context, "state": v.State, "revision": v.Revision, "version": v.Version, "policy_revision": v.PolicyRevision, "created_at": created, "selected_version": selected, "policy": policy, "data_mode": "SYNTHETIC"}, v.ArchivedAt)
}

func projectApplicable(v app.ApplicableView) (map[string]any, error) {
	at, err := stamp(v.At)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"asset_id": v.AssetID, "context": v.Context, "required": v.Match != checklist.MatchNone, "match": v.Match, "evaluated_at": at, "data_mode": "SYNTHETIC"}
	if v.Template != nil && v.Policy != nil {
		published, err := stamp(v.Template.PublishedAt)
		if err != nil {
			return nil, err
		}
		policy, err := projectPolicy(*v.Policy)
		if err != nil {
			return nil, err
		}
		out["template"] = map[string]any{"template_id": v.Template.TemplateID, "name": v.Template.Name, "version": v.Template.Version, "published_at": published, "items": projectItems(v.Template.Items)}
		out["policy"] = policy
	}
	return out, nil
}

func pathTemplate(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "template_id")
	if !checklist.CanonicalID(raw) {
		return "", orgapi.NotFound
	}
	return raw, nil
}

func currentActor(ctx context.Context) (app.ChecklistActor, error) {
	actor, err := organization.CurrentCompanyActor(ctx)
	if err != nil {
		return app.ChecklistActor{}, err
	}
	return app.ChecklistActor{AccountID: actor.AccountID, SessionID: actor.SessionID}, nil
}

// Rule returns the exact orgapi rule of an action; success is 201 only for
// creation.
func Rule(action app.ChecklistAction, revision bool, success int) orgapi.Rule {
	policy := app.Policy(action)
	write := action == app.TemplateManage
	return orgapi.Rule{Permission: string(policy.Permission), Write: write, Fresh: policy.Fresh, RequireRevision: revision, CompanyOnly: true, Success: success}
}

func missing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan:
		return v.IsNil()
	}
	return false
}

// RegisterChecklists mounts the six template routes on the organization
// target and the applicable read on the asset. The template authorization
// carries no structure; the applicable authorization carries the asset
// structure and assignee sources.
func RegisterChecklists(router chi.Router, service app.ChecklistService, cookies company.Handler, gateway orgapi.Gateway, templates, assets access.Authorization) error {
	if !service.Enabled {
		return nil
	}
	for _, a := range []access.Authorization{templates, assets} {
		if missing(a.Source) || missing(a.Capabilities) || missing(a.Records) || a.Now == nil {
			return app.ErrUnavailable
		}
	}
	if router == nil || missing(service.Repository) || service.NewID == nil || missing(assets.Structure) || missing(assets.Assignee) {
		return app.ErrUnavailable
	}
	owned, gateway, err := organization.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	root := templateRoot{}
	command := func(ctx context.Context, p orgapi.Principal, decision access.Decision, in app.TemplateCommandInput) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		c, err := app.NewTemplateCommand(in)
		if err != nil {
			return nil, mapped(err)
		}
		ack, replayed, err := service.Command(ctx, p.Scope, actor, decision, c)
		if err != nil {
			return nil, mapped(err)
		}
		return projectAck(ack, replayed), nil
	}
	if err := guarded.Register(owned, TemplateCreatePath, gateway, Rule(app.TemplateManage, false, 201), templates, app.Policy(app.TemplateManage), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input createRequest) (any, error) {
		if input.Revision != nil {
			return nil, orgapi.Malformed
		}
		name, target, items, err := contentInput(input.Name, input.TargetKind, input.TargetIDs, input.Items)
		if err != nil {
			return nil, mapped(err)
		}
		in := app.TemplateCommandInput{Kind: app.CommandCreate, OperationID: input.OperationID, Context: checklist.Context(input.Context), Name: name, Target: target, Items: items}
		if input.Policy != nil {
			if input.Policy.ValiditySeconds == nil {
				return nil, mapped(checklist.ErrInvalid)
			}
			in.Policy = checklist.Policy{Mode: checklist.Mode(input.Policy.Mode), ValiditySeconds: *input.Policy.ValiditySeconds}
			if in.Policy == (checklist.Policy{}) {
				return nil, mapped(checklist.ErrInvalid)
			}
		}
		return command(ctx, p, decision, in)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, TemplatePublishPath, gateway, Rule(app.TemplateManage, true, 200), templates, app.Policy(app.TemplateManage), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input publishRequest) (any, error) {
		template, err := pathTemplate(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		name, target, items, err := contentInput(input.Name, input.TargetKind, input.TargetIDs, input.Items)
		if err != nil {
			return nil, mapped(err)
		}
		return command(ctx, p, decision, app.TemplateCommandInput{Kind: app.CommandPublish, OperationID: input.OperationID, TemplateID: template, ExpectedRevision: *input.Revision, Name: name, Target: target, Items: items})
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, TemplatePolicyPath, gateway, Rule(app.TemplateManage, true, 200), templates, app.Policy(app.TemplateManage), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input policyChangeRequest) (any, error) {
		template, err := pathTemplate(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		if input.ValiditySeconds == nil || input.Mode == "" {
			return nil, mapped(checklist.ErrInvalid)
		}
		return command(ctx, p, decision, app.TemplateCommandInput{Kind: app.CommandPolicy, OperationID: input.OperationID, TemplateID: template, ExpectedRevision: *input.Revision, Policy: checklist.Policy{Mode: checklist.Mode(input.Mode), ValiditySeconds: *input.ValiditySeconds}})
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, TemplateArchivePath, gateway, Rule(app.TemplateManage, true, 200), templates, app.Policy(app.TemplateManage), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input archiveRequest) (any, error) {
		template, err := pathTemplate(ctx)
		if err != nil {
			return nil, err
		}
		if input.Revision == nil {
			return nil, orgapi.Malformed
		}
		return command(ctx, p, decision, app.TemplateCommandInput{Kind: app.CommandArchive, OperationID: input.OperationID, TemplateID: template, ExpectedRevision: *input.Revision})
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, TemplateListPath, gateway, Rule(app.TemplateRead, false, 200), templates, app.Policy(app.TemplateRead), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input listRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		page, err := input.Page.Normalize()
		if err != nil {
			return nil, orgapi.Malformed
		}
		const binding = "checklists.templates.v1"
		filter := app.TemplateFilter{Limit: page.Limit}
		if filter.AfterID, err = gateway.OpenCursor(p, TemplateListPath, fmt.Sprintf("%s:%d", binding, page.Limit), page.Cursor); err != nil || filter.Validate() != nil {
			return nil, orgapi.Malformed
		}
		result, err := service.List(ctx, p.Scope, actor, decision, filter)
		if err != nil {
			return nil, mapped(err)
		}
		items := make([]map[string]any, 0, len(result.Templates))
		for _, t := range result.Templates {
			item, err := projectSummary(t)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		next := ""
		if result.NextID != "" {
			if next, err = gateway.SealCursor(p, TemplateListPath, fmt.Sprintf("%s:%d", binding, page.Limit), result.NextID); err != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, TemplateReadPath, gateway, Rule(app.TemplateRead, false, 200), templates, app.Policy(app.TemplateRead), root, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input readRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		template, err := pathTemplate(ctx)
		if err != nil {
			return nil, err
		}
		version := int64(0)
		if input.Version != nil {
			if *input.Version < 1 {
				return nil, orgapi.Malformed
			}
			version = *input.Version
		}
		view, err := service.Read(ctx, p.Scope, actor, decision, template, version)
		if err != nil {
			return nil, mapped(err)
		}
		return projectTemplate(view)
	}); err != nil {
		return err
	}
	return guarded.Register(owned, ApplicablePath, gateway, Rule(app.ApplicableRead, false, 200), assets, app.Policy(app.ApplicableRead), assetResource{}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input applicableRequest) (any, error) {
		actor, err := currentActor(ctx)
		if err != nil {
			return nil, err
		}
		asset := chi.URLParamFromCtx(ctx, "asset_id")
		if !checklist.CanonicalID(asset) {
			return nil, orgapi.NotFound
		}
		moment, err := checklist.ParseContext(input.Context)
		if err != nil {
			return nil, mapped(err)
		}
		view, err := service.Applicable(ctx, p.Scope, actor, decision, asset, moment)
		if err != nil {
			return nil, mapped(err)
		}
		return projectApplicable(view)
	})
}
