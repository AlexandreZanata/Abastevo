package http

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
	"strconv"
	"time"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	orgweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters/http"
	org "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const DeviceEnrollPath = "/v1/orgs/{org_id}/devices/enroll"
const DeviceCheckPath = "/v1/orgs/{org_id}/devices/check"
const DeviceContactPath = "/v1/orgs/{org_id}/devices/contact"
const DeviceListPath = MembershipAccessPrefix + "/devices/list"
const DeviceRevokePath = MembershipAccessPrefix + "/devices/{device_id}/revoke"
const SelfDeviceListPath = MembershipAccessPrefix + "/devices/self/list"
const SelfDeviceRevokePath = MembershipAccessPrefix + "/devices/self/{device_id}/revoke"

// DeviceRepository owns fresh scoped source checks, including the independent
// proof and member recheck after admission and before any mutation commits.
type DeviceRepository interface {
	PrepareDeviceEnrollment(context.Context, kernel.OrgScope, app.DeviceActor) (string, error)
	EnrollDevice(context.Context, kernel.OrgScope, app.DeviceActor, app.DeviceCommand) (app.DeviceResult, error)
	CheckDevice(context.Context, kernel.OrgScope, app.DeviceActor) (app.DeviceRecord, error)
	ContactDevice(context.Context, kernel.OrgScope, app.DeviceActor, app.DeviceContactRequest) (app.DeviceContact, error)
	ListDevices(context.Context, kernel.OrgScope, app.EmployeeActor, string, string, bool, string, int) (app.DevicePage, error)
	RevokeDevice(context.Context, kernel.OrgScope, app.EmployeeActor, string, string, bool, app.DeviceCommand) (app.DeviceResult, error)
}

type deviceListRequest struct {
	companyRequest
	orgapi.Page
}

func (v *deviceListRequest) UnmarshalJSON(raw []byte) error {
	type plain deviceListRequest
	return decodeRedemption(raw, (*plain)(v))
}

type deviceRevokeRequest struct {
	companyRequest
	SchemaVersion int               `json:"schema_version"`
	OperationID   string            `json:"operation_id"`
	Revision      *int64            `json:"revision"`
	Class         d.RevocationClass `json:"revocation_class"`
	Reason        string            `json:"reason"`
}

func (v *deviceRevokeRequest) UnmarshalJSON(raw []byte) error {
	type plain deviceRevokeRequest
	return decodeRedemption(raw, (*plain)(v), "schema_version", "operation_id", "revision", "revocation_class", "reason")
}
func (v deviceRevokeRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}
func (v deviceRevokeRequest) command(id string) app.DeviceCommand {
	return app.DeviceCommand{OperationID: v.OperationID, DeviceID: id, ExpectedRevision: *v.Revision, Class: v.Class, Reason: v.Reason}
}

type deviceEnrollRequest struct {
	privateDTO
	FamilyID      string `json:"family_id"`
	AccessToken   string `json:"access_token"`
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Label         string `json:"label"`
}

func (v *deviceEnrollRequest) UnmarshalJSON(raw []byte) error {
	type plain deviceEnrollRequest
	return decodeRedemption(raw, (*plain)(v), "family_id", "access_token", "schema_version", "operation_id", "label")
}

type deviceCheckRequest struct {
	privateDTO
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

func (v *deviceCheckRequest) UnmarshalJSON(raw []byte) error {
	type plain deviceCheckRequest
	return decodeRedemption(raw, (*plain)(v), "family_id", "access_token")
}

type DeviceRecords struct{}

func (DeviceRecords) Check(_ context.Context, scope kernel.OrgScope, subject access.Subject, policy access.Policy, facts permissions.ResourceFacts, rules []permissions.Rule) error {
	if scope.Validate() != nil || subject.Scope() != scope || facts.OrgID != scope.OrgID() || facts.ID != scope.OrgID().String() || len(rules) == 0 || (policy != app.DevicePolicy(false) && policy != app.DevicePolicy(true)) {
		return access.ErrUnavailable
	}
	return nil
}
func deviceRule(write bool) orgapi.Rule {
	p := app.DevicePolicy(write)
	return orgapi.Rule{Permission: string(p.Permission), Write: write, Fresh: true, RequireRevision: write, CompanyOnly: true, Success: 200}
}
func deviceMapped(e error) error {
	switch {
	case errors.Is(e, app.ErrDeviceRevoked):
		return orgapi.DeviceRevoked
	case errors.Is(e, app.ErrDeviceLimit):
		return orgapi.LimitReached
	case errors.Is(e, app.ErrDeviceConflict), errors.Is(e, d.ErrRevoked):
		return orgapi.Conflict
	}
	return offboardingMapped(e)
}
func devicePathID(ctx context.Context) (string, error) {
	raw := chi.URLParamFromCtx(ctx, "device_id")
	id, e := kernel.ParseOrgID(raw)
	if e != nil || id.String() != raw {
		return "", orgapi.NotFound
	}
	return raw, nil
}
func deviceScope(r *http.Request) (kernel.OrgScope, error) {
	raw := chi.URLParam(r, "org_id")
	id, e := kernel.ParseOrgID(raw)
	if e != nil || id.String() != raw {
		return kernel.OrgScope{}, orgapi.NotFound
	}
	return kernel.NewOrgScope(id)
}
func deviceView(scope kernel.OrgScope, record app.DeviceRecord) (map[string]any, error) {
	if record.Validate(scope) != nil {
		return nil, orgapi.Unavailable
	}
	s, e := record.Enrollment.Snapshot(scope)
	if e != nil {
		return nil, orgapi.Unavailable
	}
	// Key/contributor IDs, fingerprints and personal families never cross this boundary.
	return map[string]any{"id": s.ID, "membership_id": s.MembershipID, "label": record.Label, "state": s.State, "revision": s.Revision, "revocation_class": s.Revocation, "enrolled_at": record.EnrolledAt, "last_sync_at": record.LastSyncAt, "revoked_at": record.RevokedAt, "data_mode": "SYNTHETIC"}, nil
}
func deviceResult(scope kernel.OrgScope, c app.DeviceCommand, result app.DeviceResult) (map[string]any, error) {
	if result.OperationID != c.OperationID || result.Reason != c.Reason {
		return nil, orgapi.Unavailable
	}
	s, e := result.Device.Enrollment.Snapshot(scope)
	if e != nil {
		return nil, orgapi.Unavailable
	}
	if c.DeviceID == "" {
		if s.State != d.DeviceEnrolled || s.Revision != 1 || result.Device.Label != c.Label {
			return nil, orgapi.Unavailable
		}
	} else if s.ID != c.DeviceID || s.State != d.DeviceRevoked || s.Revision != c.ExpectedRevision+1 || s.Revocation != c.Class {
		return nil, orgapi.Unavailable
	}
	view, e := deviceView(scope, result.Device)
	if e != nil {
		return nil, e
	}
	return map[string]any{"device": view, "operation_id": result.OperationID, "reason": result.Reason, "replayed": result.Replayed, "data_mode": "SYNTHETIC"}, nil
}
func devicePage(ctx context.Context, repository DeviceRepository, gateway orgapi.Gateway, p orgapi.Principal, a app.EmployeeActor, caller, target, path string, self bool, input deviceListRequest) (map[string]any, error) {
	page, e := input.Page.Normalize()
	if e != nil {
		return nil, orgapi.Malformed
	}
	filters := fmt.Sprintf("devices.v1:%s:%d", target, page.Limit)
	after, e := gateway.OpenCursor(p, path, filters, page.Cursor)
	if e != nil {
		return nil, orgapi.Malformed
	}
	out, e := repository.ListDevices(ctx, p.Scope, a, caller, target, self, after, page.Limit)
	if e != nil {
		return nil, deviceMapped(e)
	}
	if out.Devices == nil {
		return nil, orgapi.Unavailable
	}
	views := make([]map[string]any, 0, len(out.Devices))
	last := ""
	for _, v := range out.Devices {
		s, e := v.Enrollment.Snapshot(p.Scope)
		if e != nil || s.MembershipID != target || s.ID <= last {
			return nil, orgapi.Unavailable
		}
		view, e := deviceView(p.Scope, v)
		if e != nil {
			return nil, e
		}
		last = s.ID
		views = append(views, view)
	}
	if len(views) > page.Limit || out.NextID != "" && (last == "" || out.NextID != last) {
		return nil, orgapi.Unavailable
	}
	cursor := ""
	if out.NextID != "" {
		cursor, e = gateway.SealCursor(p, path, filters, out.NextID)
		if e != nil {
			return nil, orgapi.Unavailable
		}
	}
	return map[string]any{"devices": views, "next_cursor": cursor, "data_mode": "SYNTHETIC"}, nil
}

type signedDeviceEndpoint struct {
	repository DeviceRepository
	accounts   *account.Service
	verifier   auth.Verifier
	gateway    orgapi.Gateway
	action     string
}

func (e signedDeviceEndpoint) SignedDeviceRule() orgapi.SignedDeviceRule {
	return orgapi.SignedDeviceRule{Action: e.action, AccountProof: true, Signed: true, Fresh: e.action == "enroll", Enrollment: e.action == "check", Bootstrap: e.action == "enroll", Success: 200}
}
func deviceSecure(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Del("ETag")
}
func deviceBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	deviceSecure(w)
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || len(r.Header.Values("Content-Type")) != 1 || media != "application/json" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		return nil, orgapi.Malformed
	}
	raw, e := httpapi.ReadBody(r, company.MaxBodyBytes)
	if e != nil {
		return nil, orgapi.Malformed
	}
	return raw, nil
}
func deviceWrite(w http.ResponseWriter, r *http.Request, result any, e error) {
	if e != nil {
		orgapi.WriteError(w, r, e)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
func (e signedDeviceEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !e.gateway.Enabled {
		deviceSecure(w)
		deviceWrite(w, r, nil, orgapi.NotFound)
		return
	}
	raw, err := deviceBody(w, r)
	if err != nil {
		deviceWrite(w, r, nil, err)
		return
	}
	origin, err := url.Parse(e.gateway.Origin)
	if err != nil || r.Host != origin.Host || len(r.Header.Values("Origin")) > 1 || len(r.Header.Values("Origin")) == 1 && r.Header.Get("Origin") != e.gateway.Origin {
		deviceWrite(w, r, nil, orgapi.Unauthenticated)
		return
	}
	for _, name := range []string{"Cookie", "Authorization", "X-CSRF", "Signature-Input"} {
		if len(r.Header.Values(name)) > 0 {
			deviceWrite(w, r, nil, orgapi.Unauthenticated)
			return
		}
	}
	var family, token string
	var command app.DeviceCommand
	var contact app.DeviceContactRequest
	if e.action == "enroll" {
		var input deviceEnrollRequest
		if httpapi.DecodeJSON(raw, &input) != nil || input.SchemaVersion != 1 {
			deviceWrite(w, r, nil, orgapi.Malformed)
			return
		}
		family, token = input.FamilyID, input.AccessToken
		command = app.DeviceCommand{OperationID: input.OperationID, Label: input.Label}
		if err := command.Validate(true); err != nil {
			deviceWrite(w, r, nil, deviceMapped(err))
			return
		}
	} else if e.action == "contact" {
		var input deviceContactRequest
		if httpapi.DecodeJSON(raw, &input) != nil || input.SchemaVersion != 1 || input.selector().Validate() != nil {
			deviceWrite(w, r, nil, orgapi.Malformed)
			return
		}
		for _, b := range []byte(input.AccessToken) {
			if b < 0x21 || b > 0x7e {
				deviceWrite(w, r, nil, orgapi.Unauthenticated)
				return
			}
		}
		family, token, contact = input.FamilyID, input.AccessToken, input.selector()
	} else {
		var input deviceCheckRequest
		if httpapi.DecodeJSON(raw, &input) != nil {
			deviceWrite(w, r, nil, orgapi.Malformed)
			return
		}
		family, token = input.FamilyID, input.AccessToken
	}
	if id, err := kernel.ParseOrgID(family); err != nil || id.String() != family || len(token) < 1 || len(token) > 512 {
		deviceWrite(w, r, nil, orgapi.Unauthenticated)
		return
	}
	scope, err := deviceScope(r)
	if err != nil {
		deviceWrite(w, r, nil, orgapi.NotFound)
		return
	}
	if e.accounts == nil || e.accounts.Store == nil || e.accounts.Clock == nil || e.verifier.Pool == nil {
		deviceWrite(w, r, nil, orgapi.Unavailable)
		return
	}
	accountID, err := (guarded.AccountSessions{Service: e.accounts}).Confirm(r.Context(), family, token, e.action == "enroll")
	if err != nil {
		deviceWrite(w, r, nil, err)
		return
	}
	r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
	proof, err := e.verifier.Verify(r.Context(), r)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrAuthMissing), errors.Is(err, auth.ErrAuthMalformed), errors.Is(err, auth.ErrAuthDenied), errors.Is(err, auth.ErrAuthReplayed), errors.Is(err, auth.ErrAuthExpired):
			err = orgapi.Unauthenticated
		default:
			err = orgapi.Unavailable
		}
		deviceWrite(w, r, nil, err)
		return
	}
	owner, found, err := e.accounts.Store.FindBindingOwner(r.Context(), proof.ContributorID)
	if err != nil {
		deviceWrite(w, r, nil, orgapi.Unavailable)
		return
	}
	if !found || owner != accountID {
		deviceWrite(w, r, nil, orgapi.Unauthenticated)
		return
	}
	expires, err := strconv.ParseInt(r.Header.Get(auth.HeaderExpires), 10, 64)
	if err != nil {
		deviceWrite(w, r, nil, orgapi.Unauthenticated)
		return
	}
	a := app.DeviceActor{AccountID: accountID, FamilyID: family, AccessToken: token, ContributorID: proof.ContributorID, KeyID: proof.KeyID, Fingerprint: proof.Fingerprint, ProofExpires: time.Unix(expires, 0).UTC()}
	if a.Validate() != nil {
		deviceWrite(w, r, nil, orgapi.Unauthenticated)
		return
	}
	if e.action == "contact" {
		out, err := e.repository.ContactDevice(r.Context(), scope, a, contact)
		if err != nil {
			deviceWrite(w, r, nil, deviceMapped(err))
			return
		}
		view, err := deviceContactView(e.gateway.Origin, scope.OrgID().String(), a, contact, out)
		deviceWrite(w, r, view, err)
		return
	}
	if e.action == "enroll" {
		member, err := e.repository.PrepareDeviceEnrollment(r.Context(), scope, a)
		if err != nil {
			deviceWrite(w, r, nil, deviceMapped(err))
			return
		}
		p := orgapi.Principal{Actor: orgapi.Actor{AccountID: a.AccountID, ContributorID: a.ContributorID, DeviceKeyID: a.KeyID, Channel: "android"}, Scope: scope, MembershipID: member}
		if err := e.gateway.WriteAdmission.Check(r.Context(), p, deviceRule(true)); err != nil {
			deviceWrite(w, r, nil, err)
			return
		}
		out, err := e.repository.EnrollDevice(r.Context(), scope, a, command)
		if err != nil {
			deviceWrite(w, r, nil, deviceMapped(err))
			return
		}
		state, err := out.Device.Enrollment.Snapshot(scope)
		if err != nil || state.MembershipID != member || state.DeviceID != a.KeyID || state.Fingerprint != a.Fingerprint {
			deviceWrite(w, r, nil, orgapi.Unavailable)
			return
		}
		view, err := deviceResult(scope, command, out)
		deviceWrite(w, r, view, err)
		return
	}
	// Every installed signed request except enrolment goes through this actual
	// per-request source guard; it never trusts cached membership/enrolment.
	out, err := e.repository.CheckDevice(r.Context(), scope, a)
	if err != nil {
		deviceWrite(w, r, nil, deviceMapped(err))
		return
	}
	state, err := out.Enrollment.Snapshot(scope)
	if err != nil || state.State != d.DeviceEnrolled || state.DeviceID != a.KeyID || state.Fingerprint != a.Fingerprint {
		deviceWrite(w, r, nil, orgapi.Unavailable)
		return
	}
	view, err := deviceView(scope, out)
	deviceWrite(w, r, view, err)
}

type selfDeviceEndpoint struct {
	repository DeviceRepository
	cookies    company.Handler
	gateway    orgapi.Gateway
	action     string
}

func (e selfDeviceEndpoint) SelfDeviceRule() orgapi.SelfDeviceRule {
	return orgapi.SelfDeviceRule{Action: e.action, Cookie: true, CSRF: true, Fresh: true, Binding: true, Success: 200}
}
func (e selfDeviceEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !e.gateway.Enabled {
		deviceSecure(w)
		deviceWrite(w, r, nil, orgapi.NotFound)
		return
	}
	raw, err := deviceBody(w, r)
	if err != nil {
		deviceWrite(w, r, nil, err)
		return
	}
	var list deviceListRequest
	var revoke deviceRevokeRequest
	if e.action == "list" {
		err = httpapi.DecodeJSON(raw, &list)
	} else if httpapi.DecodeJSON(raw, &revoke) != nil || revoke.SchemaVersion != 1 || revoke.Revision == nil || *revoke.Revision < 1 {
		err = orgapi.Malformed
	}
	if err != nil {
		deviceWrite(w, r, nil, orgapi.Malformed)
		return
	}
	e.cookies.RequireFreshSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := company.SessionFromContext(r.Context())
		if !ok {
			deviceWrite(w, r, nil, orgapi.Unauthenticated)
			return
		}
		scope, err := deviceScope(r)
		if err != nil {
			deviceWrite(w, r, nil, orgapi.NotFound)
			return
		}
		target, err := membershipAccessID(r.Context())
		if err != nil {
			deviceWrite(w, r, nil, err)
			return
		}
		a := app.EmployeeActor{AccountID: session.AccountID, SessionID: session.ID}
		p := orgapi.Principal{Actor: orgapi.Actor{AccountID: a.AccountID, Channel: "web"}, Scope: scope, MembershipID: target}
		if e.action == "list" {
			view, err := devicePage(r.Context(), e.repository, e.gateway, p, a, "", target, SelfDeviceListPath, true, list)
			deviceWrite(w, r, view, err)
			return
		}
		id, err := devicePathID(r.Context())
		if err != nil {
			deviceWrite(w, r, nil, err)
			return
		}
		command := revoke.command(id)
		if err := command.Validate(false); err != nil {
			deviceWrite(w, r, nil, deviceMapped(err))
			return
		}
		// Prove actual own-membership binding before spending a selected scope's
		// budget. The write repeats that source proof in its retained transaction.
		if _, err := e.repository.ListDevices(r.Context(), scope, a, "", target, true, "", 1); err != nil {
			deviceWrite(w, r, nil, deviceMapped(err))
			return
		}
		if err := e.gateway.WriteAdmission.Check(r.Context(), p, deviceRule(true)); err != nil {
			deviceWrite(w, r, nil, err)
			return
		}
		out, err := e.repository.RevokeDevice(r.Context(), scope, a, "", target, true, command)
		if err != nil {
			deviceWrite(w, r, nil, deviceMapped(err))
			return
		}
		if state, err := out.Device.Enrollment.Snapshot(scope); err != nil || state.MembershipID != target {
			deviceWrite(w, r, nil, orgapi.Unavailable)
			return
		}
		view, err := deviceResult(scope, command, out)
		deviceWrite(w, r, view, err)
	})).ServeHTTP(w, r)
}

func RegisterDevices(router chi.Router, enabled bool, repository DeviceRepository, cookies company.Handler, gateway orgapi.Gateway, authorization access.Authorization, roots org.Repository, accounts *account.Service, verifier auth.Verifier) error {
	if !enabled {
		return nil
	}
	origin, err := url.Parse(gateway.Origin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || verifier.Authority != origin.Host {
		return app.ErrRegisterUnavailable
	}
	if repository == nil || roots == nil || gateway.WriteAdmission == nil || gateway.WriteAdmission.Validate(deviceRule(true)) != nil {
		return app.ErrRegisterUnavailable
	}
	owned, companyGateway, err := orgweb.CompanyEntry(router, cookies, gateway)
	if err != nil {
		return err
	}
	facts := orgweb.RootFacts{Roots: roots}
	if err := guarded.Register(owned, DeviceListPath, companyGateway, deviceRule(false), authorization, app.DevicePolicy(false), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input deviceListRequest) (any, error) {
		a, err := actor(ctx)
		if err != nil {
			return nil, err
		}
		target, err := membershipAccessID(ctx)
		if err != nil {
			return nil, err
		}
		return devicePage(ctx, repository, gateway, p, a, decision.Subject().MembershipID(), target, DeviceListPath, false, input)
	}); err != nil {
		return err
	}
	if err := guarded.Register(owned, DeviceRevokePath, companyGateway, deviceRule(true), authorization, app.DevicePolicy(true), facts, func(ctx context.Context, p orgapi.Principal, decision access.Decision, input deviceRevokeRequest) (any, error) {
		a, err := actor(ctx)
		if err != nil {
			return nil, err
		}
		target, err := membershipAccessID(ctx)
		if err != nil {
			return nil, err
		}
		id, err := devicePathID(ctx)
		if err != nil {
			return nil, err
		}
		command := input.command(id)
		if err := command.Validate(false); err != nil {
			return nil, deviceMapped(err)
		}
		out, err := repository.RevokeDevice(ctx, p.Scope, a, decision.Subject().MembershipID(), target, false, command)
		if err != nil {
			return nil, deviceMapped(err)
		}
		if state, err := out.Device.Enrollment.Snapshot(p.Scope); err != nil || state.MembershipID != target {
			return nil, orgapi.Unavailable
		}
		return deviceResult(p.Scope, command, out)
	}); err != nil {
		return err
	}
	router.Method(http.MethodPost, SelfDeviceListPath, selfDeviceEndpoint{repository, cookies, gateway, "list"})
	router.Method(http.MethodPost, SelfDeviceRevokePath, selfDeviceEndpoint{repository, cookies, gateway, "revoke"})
	router.Method(http.MethodPost, DeviceEnrollPath, signedDeviceEndpoint{repository, accounts, verifier, gateway, "enroll"})
	router.Method(http.MethodPost, DeviceCheckPath, signedDeviceEndpoint{repository, accounts, verifier, gateway, "check"})
	router.Method(http.MethodPost, DeviceContactPath, deviceContactEndpoint{signedDeviceEndpoint{repository, accounts, verifier, gateway, "contact"}})
	return nil
}
