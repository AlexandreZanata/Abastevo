package orgapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

const MaxBodyBytes = 64 << 10

var permissionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
var fieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type Session struct {
	FamilyID    string `json:"family_id,omitempty"`
	AccessToken string `json:"access_token,omitempty"`
}

func (session Session) Credentials() Session { return session }

type Request interface{ Credentials() Session }

type Command struct {
	Session
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision,omitempty"`
}

func (command Command) CommandFields() Command { return command }

type Rule struct {
	Permission      string `json:"permission"`
	Write           bool   `json:"write"`
	Fresh           bool   `json:"fresh"`
	RequireRevision bool   `json:"require_revision"`
	CompanyOnly     bool   `json:"company_only,omitempty"`
	Success         int    `json:"success"`
}

func (rule Rule) normalized() (Rule, error) {
	if rule.Success == 0 {
		rule.Success = 200
	}
	if len(rule.Permission) > 128 || !permissionPattern.MatchString(rule.Permission) ||
		(rule.RequireRevision && !rule.Write) || rule.Success < 200 || rule.Success > 299 || rule.Success == 204 {
		return Rule{}, fmt.Errorf("orgapi: invalid route rule")
	}
	return rule, nil
}

type Actor struct {
	AccountID     string
	ContributorID string
	DeviceKeyID   string
	Channel       string
}

type Membership struct {
	ID        string
	AccountID string
	OrgID     kernel.OrgID
}

type Principal struct {
	Actor
	Scope        kernel.OrgScope
	MembershipID string
}

// Gateway accepts narrow existing session/proof adapters. WebSession must check
// the dedicated cookie and CSRF. Resolve must recheck current membership,
// organization state, covering permission, fresh-auth and capability in order.
type Gateway struct {
	// Enabled comes from Config.EnterpriseEnabled. Zero hides every route.
	Enabled      bool
	Origin       string
	CursorSecret []byte
	Sessions     func(context.Context, string, string) (string, error)
	// SessionConfirm rechecks Android proof/freshness after current scope and
	// permission. Fresh Android routes cannot register without it.
	SessionConfirm func(context.Context, string, string, bool) (string, error)
	SignedOwner    func(*http.Request, string) (string, string, error)
	WebSession     func(*http.Request) (string, error)
	// WebSession peeks without renewing idle time. Confirm rechecks credentials
	// and declared freshness after current organization authority succeeds.
	WebSessionConfirm func(*http.Request, bool) (string, error)
	Resolve           func(context.Context, Actor, kernel.OrgScope, Rule) (Membership, error)
	// ConfirmAuthority completes independent capability/record gates after
	// proof confirmation, before admission or handler dispatch. AUTH binds it
	// to the current request's prepared authority.
	ConfirmAuthority func(context.Context, Principal, Rule) error
	Now              func() time.Time
	WriteAdmission   WriteAdmission
}

func validID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

func validOrigin(raw string) bool {
	origin, err := url.Parse(raw)
	return err == nil && origin.Scheme == "https" && origin.Host != "" &&
		origin.User == nil && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == ""
}

func IsOrganizationPath(path string) bool {
	return path == "/v1/orgs" || strings.HasPrefix(path, "/v1/orgs/") ||
		path == "/v1/org-access" || strings.HasPrefix(path, "/v1/org-access/")
}

// Descriptor allows the route/contract gate to verify that private routes use
// this guarded entry and exactly the authority requirements in OpenAPI.
type Descriptor interface{ OrganizationRule() Rule }

type endpoint[T Request] struct {
	gateway Gateway
	rule    Rule
	serve   func(context.Context, Principal, T) (any, error)
}

func (handler endpoint[T]) OrganizationRule() Rule { return handler.rule }

// AuthorityCompletion lets the route gate require the AUTH completion step:
// a resolver alone never applies current capability and record gates.
type AuthorityCompletion interface{ CompletesAuthority() bool }

func (handler endpoint[T]) CompletesAuthority() bool { return handler.gateway.ConfirmAuthority != nil }

func Register[T Request](router chi.Router, path string, gateway Gateway, rule Rule,
	serve func(context.Context, Principal, T) (any, error)) error {
	normalized, err := rule.normalized()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(path, "/v1/orgs/{org_id}/") || strings.Count(path, "{org_id}") != 1 ||
		strings.ContainsAny(path, "?*#") || strings.Contains(path, "..") ||
		len(path) == len("/v1/orgs/{org_id}/") || !validOrigin(gateway.Origin) ||
		len(gateway.CursorSecret) < 32 || gateway.Resolve == nil || serve == nil ||
		(gateway.Sessions == nil && gateway.WebSession == nil) ||
		(gateway.Sessions != nil && gateway.SignedOwner == nil) {
		return fmt.Errorf("orgapi: incomplete organization route registration")
	}
	if gateway.WebSession != nil && gateway.WebSessionConfirm == nil {
		return fmt.Errorf("orgapi: missing company session confirmation")
	}
	if normalized.CompanyOnly && (gateway.Sessions != nil || gateway.WebSession == nil) {
		return fmt.Errorf("orgapi: company-only route requires the dedicated cookie plane")
	}
	if normalized.Fresh && gateway.Sessions != nil && gateway.SessionConfirm == nil {
		return fmt.Errorf("orgapi: missing Android session confirmation")
	}
	if normalized.Write && (gateway.WriteAdmission == nil || gateway.WriteAdmission.Validate(normalized) != nil) {
		return fmt.Errorf("orgapi: explicit write admission policy required")
	}
	router.Method(http.MethodPost, path, endpoint[T]{gateway: gateway, rule: normalized, serve: serve})
	return nil
}

func (handler endpoint[T]) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Del("ETag")
	if !handler.gateway.Enabled {
		WriteError(w, r, NotFound)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || len(r.Header.Values("Content-Type")) != 1 || mediaType != "application/json" || r.URL.RawQuery != "" {
		WriteError(w, r, Malformed)
		return
	}
	raw, err := httpapi.ReadBody(r, MaxBodyBytes)
	var input T
	validFields, credentialsPresent := canonicalFields(raw)
	if err != nil || httpapi.DecodeJSON(raw, &input) != nil || !validFields {
		WriteError(w, r, Malformed)
		return
	}
	if credentialsPresent && input.Credentials() == (Session{}) {
		WriteError(w, r, Unauthenticated)
		return
	}
	if paged, ok := any(input).(interface{ PageFields() Page }); ok {
		if _, err := paged.PageFields().Normalize(); err != nil {
			WriteError(w, r, Malformed)
			return
		}
	}
	if handler.rule.Write {
		command, ok := any(input).(interface{ CommandFields() Command })
		if !ok {
			WriteError(w, r, Malformed)
			return
		}
		fields := command.CommandFields()
		if fields.SchemaVersion != 1 || !validID(fields.OperationID) ||
			(handler.rule.RequireRevision && fields.Revision == nil) ||
			(fields.Revision != nil && *fields.Revision < 1) {
			WriteError(w, r, Malformed)
			return
		}
	}
	// Signed proof consumes the exact transmitted bytes, after strict decoding.
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	principal, err := handler.gateway.resolve(r, input.Credentials(), handler.rule)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if handler.rule.Write {
		err := handler.gateway.WriteAdmission.Check(r.Context(), principal, handler.rule)
		if err != nil || r.Context().Err() != nil {
			var limited *RateLimitError
			if errors.As(err, &limited) && limited != nil && limited.RetryAfter > 0 && r.Context().Err() == nil {
				WriteError(w, r, limited)
			} else {
				WriteError(w, r, Unavailable)
			}
			return
		}
	}
	result, err := handler.serve(r.Context(), principal, input)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	body, err := json.Marshal(result)
	if err != nil {
		WriteError(w, r, Unavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(handler.rule.Success)
	_, _ = w.Write(body)
}

// Common reserved scalars are omitted or carry their declared non-null value.
// Module DTOs use canonical lower-case JSON names and reject unknown fields.
func canonicalFields(raw []byte) (bool, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return false, false
	}
	for key, value := range fields {
		if !fieldPattern.MatchString(key) {
			return false, false
		}
		switch key {
		case "family_id", "access_token", "schema_version", "operation_id", "revision", "limit", "cursor":
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return false, false
			}
		}
		if key == "limit" {
			if _, err := httpapi.ParseLimit(string(value)); err != nil {
				return false, false
			}
		}
	}
	return true, fields["family_id"] != nil || fields["access_token"] != nil
}

func (gateway Gateway) resolve(r *http.Request, session Session, rule Rule) (Principal, error) {
	actor := Actor{}
	origins := r.Header.Values("Origin")
	if len(origins) > 0 && (len(origins) != 1 || origins[0] != gateway.Origin) {
		return Principal{}, Unauthenticated
	}
	if session.FamilyID != "" || session.AccessToken != "" {
		if !validID(session.FamilyID) || len(session.AccessToken) < 1 || len(session.AccessToken) > 512 ||
			gateway.Sessions == nil || gateway.SignedOwner == nil {
			return Principal{}, Unauthenticated
		}
		account, err := gateway.Sessions(r.Context(), session.FamilyID, session.AccessToken)
		if err != nil || !validID(account) {
			return Principal{}, authenticationError(err)
		}
		contributor, key, err := gateway.SignedOwner(r, account)
		if err != nil || !validID(contributor) || !validID(key) {
			return Principal{}, authenticationError(err)
		}
		actor = Actor{AccountID: account, ContributorID: contributor, DeviceKeyID: key, Channel: "android"}
	} else {
		// A failed or partial Android proof cannot become a browser session.
		for _, name := range []string{"Signature", "Signature-Created", "Signature-Expires", "Signature-Keyid", "Signature-Nonce", "Signature-Input"} {
			if _, present := r.Header[http.CanonicalHeaderKey(name)]; present {
				return Principal{}, Unauthenticated
			}
		}
		if len(origins) != 1 || gateway.WebSession == nil {
			return Principal{}, Unauthenticated
		}
		account, err := gateway.WebSession(r)
		if err != nil || !validID(account) {
			return Principal{}, authenticationError(err)
		}
		actor = Actor{AccountID: account, Channel: "web"}
	}
	rawOrg := chi.URLParam(r, "org_id")
	id, err := kernel.ParseOrgID(rawOrg)
	if err != nil || id.String() != rawOrg {
		return Principal{}, NotFound
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		return Principal{}, NotFound
	}
	membership, err := gateway.Resolve(r.Context(), actor, scope, rule)
	if err != nil {
		return Principal{}, err
	}
	if !validID(membership.ID) || membership.AccountID != actor.AccountID || membership.OrgID != id {
		return Principal{}, NotFound
	}
	if actor.Channel == "web" {
		account, err := gateway.WebSessionConfirm(r, rule.Fresh)
		if errors.Is(err, FreshAuthRequired) {
			return Principal{}, FreshAuthRequired
		}
		if err != nil {
			return Principal{}, authenticationError(err)
		}
		if account != actor.AccountID {
			return Principal{}, Unauthenticated
		}
	} else if gateway.SessionConfirm != nil {
		account, err := gateway.SessionConfirm(r.Context(), session.FamilyID, session.AccessToken, rule.Fresh)
		if errors.Is(err, FreshAuthRequired) {
			return Principal{}, FreshAuthRequired
		}
		if err != nil {
			return Principal{}, authenticationError(err)
		}
		if account != actor.AccountID {
			return Principal{}, Unauthenticated
		}
	}
	principal := Principal{Actor: actor, Scope: scope, MembershipID: membership.ID}
	if gateway.ConfirmAuthority != nil {
		if err := gateway.ConfirmAuthority(r.Context(), principal, rule); err != nil {
			return Principal{}, err
		}
	}
	return principal, nil
}

func authenticationError(err error) error {
	if errors.Is(err, Unavailable) {
		return Unavailable
	}
	return Unauthenticated
}
