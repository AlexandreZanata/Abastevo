// Package http exposes only the current member's own inbox and preferences.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgnotify/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const (
	MembershipPrefix      = "/v1/orgs/{org_id}/memberships/{membership_id}"
	ListPath              = MembershipPrefix + "/notifications/self/list"
	ReadPath              = MembershipPrefix + "/notifications/self/{notice_id}/read"
	PreferencesReadPath   = MembershipPrefix + "/notification-preferences/self/read"
	PreferencesUpdatePath = MembershipPrefix + "/notification-preferences/self/update"
	// WriteClass keys the dedicated self-notice write admission. It is a
	// route class, never an authorization catalog permission.
	WriteClass = "notif.self.write"
)

// WriteRule is the admission rule for own read marks and preference changes.
func WriteRule() orgapi.Rule {
	return orgapi.Rule{Permission: WriteClass, Write: true, Success: 200}
}

type Options struct {
	Enabled   bool
	Service   app.Service
	Cookies   company.Handler
	Gateway   orgapi.Gateway
	Admission orgapi.WriteAdmission
}

type endpoint struct {
	options Options
	action  string
}

func (e endpoint) SelfNoticeRule() orgapi.SelfNoticeRule {
	return orgapi.SelfNoticeRule{Action: e.action, Cookie: true, CSRF: true, Binding: true, Success: 200}
}

type listRequest struct {
	Limit      int    `json:"limit"`
	Cursor     string `json:"cursor"`
	UnreadOnly bool   `json:"unread_only"`
}
type readRequest struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
}
type preferenceRequest struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Kind          string `json:"kind"`
	Muted         *bool  `json:"muted"`
	Revision      *int64 `json:"revision"`
}

func secure(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Del("ETag")
}

func write(w http.ResponseWriter, r *http.Request, result any, err error) {
	if err != nil {
		orgapi.WriteError(w, r, err)
		return
	}
	body, err := json.Marshal(result)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func mapped(err error) error {
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{domain.ErrNotFound, orgapi.NotFound}, {domain.ErrSuspended, orgapi.MembershipSuspended},
		{domain.ErrInvalid, orgapi.Malformed}, {domain.ErrValidation, orgapi.ValidationFailed},
		{domain.ErrConflict, orgapi.Conflict}, {domain.ErrReused, orgapi.OperationReused},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	if code, ok := orgapi.Registered(err); ok {
		return code
	}
	return orgapi.Unavailable
}

// decode applies strict JSON plus the shared reserved-field rules: lower-case
// fields, no nulls, no duplicates and no unknown or credential fields.
func decode(raw []byte, dst any, required ...string) error {
	var fields map[string]json.RawMessage
	if httpapi.DecodeJSON(raw, dst) != nil || json.Unmarshal(raw, &fields) != nil {
		return orgapi.Malformed
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return orgapi.Malformed
		}
	}
	for _, name := range required {
		if fields[name] == nil {
			return orgapi.Malformed
		}
	}
	return nil
}

func pathID(r *http.Request, name string) (string, error) {
	raw := chi.URLParam(r, name)
	id, err := kernel.ParseOrgID(raw)
	if err != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}

func (e endpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	secure(w)
	if !e.options.Gateway.Enabled {
		write(w, r, nil, orgapi.NotFound)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 || r.URL.RawQuery != "" || r.URL.ForceQuery {
		write(w, r, nil, orgapi.Malformed)
		return
	}
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(15 * time.Second))
	raw, err := httpapi.ReadBody(r, company.MaxBodyBytes)
	if err != nil {
		write(w, r, nil, orgapi.Malformed)
		return
	}
	var list listRequest
	var read readRequest
	var preference preferenceRequest
	switch e.action {
	case "list":
		err = decode(raw, &list)
		if err == nil && (list.Limit < 0 || list.Limit > app.MaxLimit || len(list.Cursor) > 2048) {
			err = orgapi.Malformed
		}
	case "read":
		err = decode(raw, &read, "schema_version", "operation_id")
		if err == nil && read.SchemaVersion != 1 {
			err = orgapi.Malformed
		}
	case "preferences_read":
		err = decode(raw, &struct{}{})
	case "preferences_update":
		err = decode(raw, &preference, "schema_version", "operation_id", "kind", "muted")
		if err == nil && (preference.SchemaVersion != 1 || preference.Muted == nil || preference.Revision != nil && *preference.Revision < 1) {
			err = orgapi.Malformed
		}
	default:
		err = orgapi.Unavailable
	}
	if err != nil {
		write(w, r, nil, err)
		return
	}
	e.options.Cookies.RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := company.SessionFromContext(r.Context())
		if !ok {
			write(w, r, nil, orgapi.Unauthenticated)
			return
		}
		rawOrg, err := pathID(r, "org_id")
		if err != nil {
			write(w, r, nil, err)
			return
		}
		org, _ := kernel.ParseOrgID(rawOrg)
		scope, err := kernel.NewOrgScope(org)
		if err != nil {
			write(w, r, nil, orgapi.NotFound)
			return
		}
		member, err := pathID(r, "membership_id")
		if err != nil {
			write(w, r, nil, err)
			return
		}
		self := app.Self{AccountID: session.AccountID, MembershipID: member}
		principal := orgapi.Principal{Actor: orgapi.Actor{AccountID: session.AccountID, Channel: "web"}, Scope: scope, MembershipID: member}
		result, err := e.serve(r, scope, self, principal, list, read, preference)
		write(w, r, result, err)
	})).ServeHTTP(w, r)
}

func (e endpoint) serve(r *http.Request, scope kernel.OrgScope, self app.Self, p orgapi.Principal, list listRequest, read readRequest, preference preferenceRequest) (any, error) {
	ctx, service := r.Context(), e.options.Service
	switch e.action {
	case "list":
		return e.list(ctx, scope, self, p, list)
	case "preferences_read":
		preferences, err := service.Preferences(ctx, scope, self)
		if err != nil {
			return nil, mapped(err)
		}
		return map[string]any{"preferences": preferenceViews(preferences), "data_mode": "SYNTHETIC"}, nil
	}
	notice := ""
	if e.action == "read" {
		var err error
		if notice, err = pathID(r, "notice_id"); err != nil {
			return nil, err
		}
	}
	// Prove the actual own current membership before spending the selected
	// organization's shared admission budget; the command repeats the proof.
	if _, err := service.Preferences(ctx, scope, self); err != nil {
		return nil, mapped(err)
	}
	if err := e.options.Admission.Check(ctx, p, WriteRule()); err != nil {
		var limited *orgapi.RateLimitError
		if errors.As(err, &limited) && limited != nil && limited.RetryAfter > 0 {
			return nil, limited
		}
		return nil, orgapi.Unavailable
	}
	if e.action == "read" {
		out, err := service.MarkRead(ctx, scope, self, app.ReadCommand{OperationID: read.OperationID, NoticeID: notice})
		if err != nil {
			return nil, mapped(err)
		}
		if out.Notice.ID != notice || out.Notice.ReadAt.IsZero() {
			return nil, orgapi.Unavailable
		}
		return map[string]any{"notice": noticeView(out.Notice), "operation_id": read.OperationID, "replayed": out.Replayed, "data_mode": "SYNTHETIC"}, nil
	}
	command := app.PreferenceCommand{OperationID: preference.OperationID, Kind: preference.Kind, Muted: *preference.Muted}
	if preference.Revision != nil {
		command.Revision = *preference.Revision
	}
	out, err := service.SetPreference(ctx, scope, self, command)
	if err != nil {
		return nil, mapped(err)
	}
	if out.Preference.Kind != command.Kind || out.Preference.Revision != command.Revision+1 {
		return nil, orgapi.Unavailable
	}
	return map[string]any{"preference": preferenceViews([]app.Preference{out.Preference})[0], "operation_id": preference.OperationID, "replayed": out.Replayed, "data_mode": "SYNTHETIC"}, nil
}

func (e endpoint) list(ctx context.Context, scope kernel.OrgScope, self app.Self, p orgapi.Principal, input listRequest) (any, error) {
	limit := input.Limit
	if limit == 0 {
		limit = app.DefaultLimit
	}
	filters := fmt.Sprintf("notices.v1:%t:%d", input.UnreadOnly, limit)
	key, err := e.options.Gateway.OpenCursor(p, ListPath, filters, input.Cursor)
	if err != nil {
		return nil, orgapi.Malformed
	}
	var after int64
	if key != "" {
		if after, err = strconv.ParseInt(key, 10, 64); err != nil || after < 1 || strconv.FormatInt(after, 10) != key {
			return nil, orgapi.Malformed
		}
	}
	page, err := e.options.Service.List(ctx, scope, self, app.InboxQuery{UnreadOnly: input.UnreadOnly, After: after, Limit: limit})
	if err != nil {
		return nil, mapped(err)
	}
	views := make([]map[string]any, 0, len(page.Notices))
	for _, n := range page.Notices {
		views = append(views, noticeView(n))
	}
	cursor := ""
	if page.Next != 0 {
		if cursor, err = e.options.Gateway.SealCursor(p, ListPath, filters, strconv.FormatInt(page.Next, 10)); err != nil {
			return nil, orgapi.Unavailable
		}
	}
	return map[string]any{"notices": views, "next_cursor": cursor, "data_mode": "SYNTHETIC"}, nil
}

func noticeView(n app.Notice) map[string]any {
	params := n.Params
	if params == nil {
		params = map[string]string{}
	}
	view := map[string]any{"id": n.ID, "kind": n.Kind, "template": n.Template, "params": params, "link_path": nil, "created_at": n.CreatedAt.UTC(), "read_at": nil}
	if n.Link != "" {
		view["link_path"] = n.Link
	}
	if !n.ReadAt.IsZero() {
		view["read_at"] = n.ReadAt.UTC()
	}
	return view
}

func preferenceViews(preferences []app.Preference) []map[string]any {
	out := make([]map[string]any, 0, len(preferences))
	for _, p := range preferences {
		out = append(out, map[string]any{"kind": p.Kind, "muted": p.Muted, "mandatory": p.Mandatory, "revision": p.Revision})
	}
	return out
}

func validOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}

// Register mounts the four SELF_NOTICE routes. Disabled composition mounts none.
func Register(router chi.Router, o Options) error {
	if !o.Enabled {
		return nil
	}
	if router == nil || !validOrigin(o.Gateway.Origin) || len(o.Gateway.CursorSecret) < 32 || o.Cookies.Service == nil || !o.Cookies.Service.Enabled ||
		!o.Service.Enabled || o.Service.Repository == nil || o.Admission == nil || o.Admission.Validate(WriteRule()) != nil {
		return fmt.Errorf("orgnotify: incomplete self notice composition")
	}
	for path, action := range map[string]string{ListPath: "list", ReadPath: "read", PreferencesReadPath: "preferences_read", PreferencesUpdatePath: "preferences_update"} {
		router.Method(http.MethodPost, path, endpoint{options: o, action: action})
	}
	return nil
}
