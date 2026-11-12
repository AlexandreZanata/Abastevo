package http

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	codec "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const SettingsPrefix = "/v1/orgs/{org_id}/settings"
const SettingsReadPath = SettingsPrefix + "/read"
const SettingsRevisionPath = SettingsPrefix + "/revision/read"
const SettingsHistoryPath = SettingsPrefix + "/history"
const SettingsChangePath = SettingsPrefix + "/change"

type settingsReadRequest struct{ companyRequest }
type settingsRevisionRequest struct {
	companyRequest
	Revision int64 `json:"revision"`
}
type settingsHistoryRequest struct {
	companyRequest
	orgapi.Page
}
type settingsChangeRequest struct {
	unitRevisionCommand
	Values json.RawMessage `json:"values"`
	Reason string          `json:"reason"`
}

func (v *settingsReadRequest) UnmarshalJSON(raw []byte) error {
	type plain settingsReadRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	*v = settingsReadRequest(input)
	return nil
}
func (v *settingsRevisionRequest) UnmarshalJSON(raw []byte) error {
	type plain settingsRevisionRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	*v = settingsRevisionRequest(input)
	return nil
}
func (v *settingsHistoryRequest) UnmarshalJSON(raw []byte) error {
	type plain settingsHistoryRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	*v = settingsHistoryRequest(input)
	return nil
}
func (v *settingsChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain settingsChangeRequest
	var input plain
	if e := decodeLegal(raw, &input); e != nil {
		return e
	}
	*v = settingsChangeRequest(input)
	return nil
}

type SettingsRecords struct{}

func (SettingsRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 || policy != app.SettingsPolicy(false) && policy != app.SettingsPolicy(true) {
		return access.ErrUnavailable
	}
	return nil
}
func settingsMapped(err error) error {
	if field := d.SettingsField(err); field != "" {
		return orgapi.SettingsValidation(field)
	}
	return legalMapped(err, "")
}
func settingsRule(write bool) orgapi.Rule {
	return orgapi.Rule{Permission: string(app.SettingsPolicy(write).Permission), Write: write, Fresh: write, RequireRevision: write, CompanyOnly: true, Success: 200}
}
func settingsView(scope kernel.OrgScope, value d.Settings) (map[string]any, error) {
	v, e := value.Snapshot(scope)
	if e != nil {
		return nil, orgapi.Unavailable
	}
	raw, e := codec.EncodeSettingsValues(v.Values)
	if e != nil {
		return nil, orgapi.Unavailable
	}
	result := map[string]any{"revision": v.Revision, "effective_at": v.EffectiveAt, "values": json.RawMessage(raw), "source": v.Source, "reason": v.Reason, "ownership_transfer_validity_days": d.OwnershipTransferValidityDays, "data_mode": "SYNTHETIC"}
	if v.Author.AccountID != "" {
		result["author"] = map[string]any{"account_id": v.Author.AccountID, "membership_id": v.Author.MembershipID}
	}
	if v.OperationID != "" {
		result["operation_id"] = v.OperationID
	}
	if v.SourceMembershipID != "" {
		result["source_membership"] = map[string]any{"membership_id": v.SourceMembershipID, "revision": v.SourceMembershipRevision}
		result["notice_state"] = "PENDING_NOTIFICATION_CORE"
	}
	return result, nil
}
func RegisterSettings(router chi.Router, service app.SettingsService, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots app.Repository) error {
	if !service.Enabled {
		return nil
	}
	if service.Repository == nil || roots == nil {
		return app.ErrUnavailable
	}
	owned, gateway, e := companyEntry(router, cookies, gateway)
	if e != nil {
		return e
	}
	read := func(ctx context.Context, p orgapi.Principal, decision access.Decision, revision int64) (any, error) {
		actor, e := companyActor(ctx)
		if e != nil {
			return nil, e
		}
		v, e := service.Read(ctx, p.Scope, actor, decision, revision)
		if e != nil {
			return nil, settingsMapped(e)
		}
		view, e := settingsView(p.Scope, v.Settings)
		if e != nil {
			return nil, e
		}
		view["root_revision"] = v.RootRevision
		return view, nil
	}
	if e := guarded.Register(owned, SettingsReadPath, gateway, settingsRule(false), authorization, app.SettingsPolicy(false), RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, d access.Decision, _ settingsReadRequest) (any, error) {
		return read(ctx, p, d, 0)
	}); e != nil {
		return e
	}
	if e := guarded.Register(owned, SettingsRevisionPath, gateway, settingsRule(false), authorization, app.SettingsPolicy(false), RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, d access.Decision, input settingsRevisionRequest) (any, error) {
		if input.Revision < 1 {
			return nil, orgapi.ValidationFailed
		}
		return read(ctx, p, d, input.Revision)
	}); e != nil {
		return e
	}
	if e := guarded.Register(owned, SettingsChangePath, gateway, settingsRule(true), authorization, app.SettingsPolicy(true), RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input settingsChangeRequest) (any, error) {
		actor, e := companyActor(ctx)
		if e != nil {
			return nil, e
		}
		values, e := codec.DecodeSettingsValues(input.Values)
		if e != nil {
			return nil, settingsMapped(e)
		}
		command, e := app.NewSettingsWrite(app.SettingsWriteInput{OperationID: input.OperationID, ExpectedRevision: *input.Revision, Values: values, Reason: input.Reason})
		if e != nil {
			return nil, settingsMapped(e)
		}
		result, replay, e := service.Change(ctx, p.Scope, actor, decision, command)
		if e != nil {
			return nil, settingsMapped(e)
		}
		return map[string]any{"revision": result.Revision, "root_revision": result.RootRevision, "effective_at": result.EffectiveAt, "data_mode": "SYNTHETIC", "replayed": replay}, nil
	}); e != nil {
		return e
	}
	return guarded.Register(owned, SettingsHistoryPath, gateway, settingsRule(false), authorization, app.SettingsPolicy(false), RootFacts{roots}, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input settingsHistoryRequest) (any, error) {
		actor, e := companyActor(ctx)
		if e != nil {
			return nil, e
		}
		page, e := input.Page.Normalize()
		if e != nil {
			return nil, orgapi.Malformed
		}
		filters := fmt.Sprintf("settings.v1:%d", page.Limit)
		before, e := gateway.OpenCursor(p, SettingsHistoryPath, filters, page.Cursor)
		if e != nil {
			return nil, orgapi.Malformed
		}
		filter := app.SettingsFilter{Limit: page.Limit}
		if before != "" {
			filter.BeforeRevision, e = strconv.ParseInt(before, 10, 64)
			if e != nil || filter.BeforeRevision < 1 || strconv.FormatInt(filter.BeforeRevision, 10) != before {
				return nil, orgapi.Malformed
			}
		}
		result, e := service.List(ctx, p.Scope, actor, decision, filter)
		if e != nil {
			return nil, settingsMapped(e)
		}
		items := make([]map[string]any, 0, len(result.Versions))
		for _, v := range result.Versions {
			view, e := settingsView(p.Scope, v)
			if e != nil {
				return nil, e
			}
			items = append(items, view)
		}
		next := ""
		if result.NextRevision > 0 {
			next, e = gateway.SealCursor(p, SettingsHistoryPath, filters, strconv.FormatInt(result.NextRevision, 10))
			if e != nil {
				return nil, orgapi.Unavailable
			}
		}
		return map[string]any{"items": items, "next_cursor": next}, nil
	})
}
