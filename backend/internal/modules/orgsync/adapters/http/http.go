package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

const Path = "/v1/orgs/{org_id}/sync/operations"

type DeviceSource interface {
	CheckDevice(context.Context, kernel.OrgScope, member.DeviceActor) (member.DeviceRecord, error)
}

// LateDeviceSource proves the intake-only source of one exact owned period
// whose access ended (EMP-OFFL-009). Nil keeps late intake unmounted.
type LateDeviceSource interface {
	CheckLateIntakeDevice(context.Context, kernel.OrgScope, member.DeviceActor, string, string) (member.LateIntakeDevice, error)
}

// StateDeviceSource proves the current member's enrolled device of a
// SUSPENDED organization for the EMP-OFFL-012 state assessment only.
type StateDeviceSource interface {
	CheckSuspendedIntakeDevice(context.Context, kernel.OrgScope, member.DeviceActor) (member.DeviceRecord, error)
}
type Options struct {
	Enabled  bool
	Origin   string
	Accounts *account.Service
	Verifier auth.Verifier
	Devices  DeviceSource
	Late     LateDeviceSource
	State    StateDeviceSource
	Service  app.Service
	Limits   protocol.Limits
}
type endpoint struct{ Options }

func (endpoint) SignedBatchRule() orgapi.SignedBatchRule {
	return orgapi.SignedBatchRule{AccountProof: true, Signed: true, CurrentEnrollment: true, WholeBodyPreflight: true, PerOperation: true, Success: 200}
}

func Register(router chi.Router, o Options) error {
	if !o.Enabled {
		return nil
	}
	origin, e := url.Parse(o.Origin)
	if router == nil || e != nil || protocol.ValidateOrigin(o.Origin) != nil || origin.Host != o.Verifier.Authority || o.Devices == nil || o.Limits == (protocol.Limits{}) {
		return app.ErrUnavailable
	}
	router.Method(http.MethodPost, Path, endpoint{o})
	return nil
}
func mapped(e error) error {
	var limit *orgapi.RateLimitError
	if errors.As(e, &limit) {
		return limit
	}
	if code, ok := orgapi.Registered(e); ok {
		return code
	}
	for _, pair := range []struct{ from, to error }{
		{protocol.ErrInvalid, orgapi.Malformed}, {protocol.ErrBinding, orgapi.Malformed}, {protocol.ErrDigest, orgapi.Malformed}, {protocol.ErrLimit, orgapi.Malformed},
		{member.ErrEmployeeUnauthenticated, orgapi.Unauthenticated}, {access.ErrUnauthenticated, orgapi.Unauthenticated},
		{member.ErrEmployeeNotFound, orgapi.NotFound}, {access.ErrNoMembership, orgapi.NotFound},
		{member.ErrDeviceRevoked, orgapi.DeviceRevoked}, {member.ErrManagedRestricted, orgapi.ManagedAccountRestricted},
		{access.ErrMembershipSuspended, orgapi.MembershipSuspended}, {access.ErrOrgSuspended, orgapi.OrgSuspended}, {access.ErrOrgArchived, orgapi.OrgArchived},
		{access.ErrEntitlementRequired, orgapi.EntitlementRequired},
	} {
		if errors.Is(e, pair.from) {
			return pair.to
		}
	}
	return orgapi.Unavailable
}

// A producer may have committed before an unknown suffix or an admission wait.
// Reconfirm the captured period before exposing any buffered private result.
func confirmCurrentDevice(ctx context.Context, devices DeviceSource, s app.Source) error {
	v, e := devices.CheckDevice(ctx, s.Scope, s.Actor)
	if e != nil {
		return e
	}
	state, e := v.Enrollment.Snapshot(s.Scope)
	if e != nil || v.Validate(s.Scope) != nil {
		return app.ErrUnavailable
	}
	if state.State != d.DeviceEnrolled || state.ID != s.EnrollmentID || state.MembershipID != s.MembershipID || state.DeviceID != s.Actor.KeyID || state.Fingerprint != s.Actor.Fingerprint {
		return member.ErrEmployeeNotFound
	}
	return nil
}

// confirmSuspendedDevice reconfirms the suspended-organization source before
// exposing any buffered private result (EMP-OFFL-012).
func confirmSuspendedDevice(ctx context.Context, devices StateDeviceSource, s app.Source) error {
	v, e := devices.CheckSuspendedIntakeDevice(ctx, s.Scope, s.Actor)
	if e != nil {
		return e
	}
	state, e := v.Enrollment.Snapshot(s.Scope)
	if e != nil || v.Validate(s.Scope) != nil {
		return app.ErrUnavailable
	}
	if state.State != d.DeviceEnrolled || state.ID != s.EnrollmentID || state.MembershipID != s.MembershipID || state.DeviceID != s.Actor.KeyID || state.Fingerprint != s.Actor.Fingerprint {
		return member.ErrEmployeeNotFound
	}
	return nil
}

// sealedPeriod reads the membership and enrollment sealed into the batch's
// first operation with the complete frozen parser; Preflight later requires
// every operation to carry the same period.
func sealedPeriod(batch protocol.Batch, account, origin string, scope kernel.OrgScope) (string, string, bool) {
	candidates := batch.Candidates()
	if len(candidates) == 0 {
		return "", "", false
	}
	binding, err := protocol.NewBinding(account, origin, scope.OrgID().String())
	if err != nil {
		return "", "", false
	}
	op, err := candidates[0].Parse(binding)
	if err != nil {
		return "", "", false
	}
	v := op.Snapshot()
	return v.MembershipID, v.EnrollmentID, true
}

func (e endpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Del("ETag")
	fail := func(err error) { orgapi.WriteError(w, r, mapped(err)) }
	if !e.Enabled {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	origin, err := url.Parse(e.Origin)
	if err != nil || r.Host != origin.Host || len(r.Header.Values("Origin")) > 1 || len(r.Header.Values("Origin")) == 1 && r.Header.Get("Origin") != e.Origin {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	for _, name := range []string{"Cookie", "Authorization", "X-CSRF", "Signature-Input"} {
		if len(r.Header.Values(name)) > 0 {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
			return
		}
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || len(r.Header.Values("Content-Type")) != 1 || media != "application/json" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	raw, err := httpapi.ReadBody(r, 65536)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	batch, err := protocol.ParseBatch(raw, e.Limits)
	if err != nil {
		fail(err)
		return
	}
	proofInput := batch.Proof()
	id, err := kernel.ParseOrgID(chi.URLParam(r, "org_id"))
	if err != nil || id.String() != chi.URLParam(r, "org_id") {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	if e.Accounts == nil || e.Accounts.Store == nil || e.Accounts.Clock == nil || e.Verifier.Pool == nil || e.Devices == nil {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	accountID, err := (guarded.AccountSessions{Service: e.Accounts}).Confirm(r.Context(), proofInput.FamilyID, proofInput.AccessToken, false)
	if err != nil {
		orgapi.WriteError(w, r, err)
		return
	}
	r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(raw)), int64(len(raw))
	proof, err := e.Verifier.Verify(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrAuthMissing) || errors.Is(err, auth.ErrAuthMalformed) || errors.Is(err, auth.ErrAuthDenied) || errors.Is(err, auth.ErrAuthReplayed) || errors.Is(err, auth.ErrAuthExpired) {
			orgapi.WriteError(w, r, orgapi.Unauthenticated)
		} else {
			orgapi.WriteError(w, r, orgapi.Unavailable)
		}
		return
	}
	owner, found, err := e.Accounts.Store.FindBindingOwner(r.Context(), proof.ContributorID)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	if !found || owner != accountID {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	expiry, err := strconv.ParseInt(r.Header.Get(auth.HeaderExpires), 10, 64)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	actor := member.DeviceActor{AccountID: accountID, FamilyID: proofInput.FamilyID, AccessToken: proofInput.AccessToken, ContributorID: proof.ContributorID, KeyID: proof.KeyID, Fingerprint: proof.Fingerprint, ProofExpires: time.Unix(expiry, 0).UTC()}
	if actor.Validate() != nil {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	sealedMember, sealedEnrollment, sealed := sealedPeriod(batch, accountID, e.Origin, scope)
	device, err := e.Devices.CheckDevice(r.Context(), scope, actor)
	// EMP-OFFL-012: while SUSPENDED, a batch with operations from the current
	// member's enrolled device reaches only the per-operation state assessment.
	suspended := false
	if errors.Is(err, access.ErrOrgSuspended) && sealed && e.State != nil {
		if device, err = e.State.CheckSuspendedIntakeDevice(r.Context(), scope, actor); err == nil {
			suspended = true
		}
	}
	late := false
	var source app.Source
	if err != nil {
		// EMP-OFFL-009: a revoked or ended device may deliver its own sealed
		// period intake-only; any other late refusal keeps the current one.
		if !sealed || e.Late == nil || !errors.Is(err, member.ErrDeviceRevoked) && !errors.Is(err, member.ErrEmployeeNotFound) {
			fail(err)
			return
		}
		if _, lateErr := e.Late.CheckLateIntakeDevice(r.Context(), scope, actor, sealedMember, sealedEnrollment); lateErr != nil {
			if errors.Is(lateErr, member.ErrEmployeeNotFound) {
				fail(err)
			} else {
				fail(lateErr)
			}
			return
		}
		late = true
		source = app.Source{Scope: scope, Origin: e.Origin, Actor: actor, MembershipID: sealedMember, EnrollmentID: sealedEnrollment}
	} else {
		state, err := device.Enrollment.Snapshot(scope)
		if err != nil || device.Validate(scope) != nil || state.State != d.DeviceEnrolled || state.DeviceID != actor.KeyID || state.Fingerprint != actor.Fingerprint {
			orgapi.WriteError(w, r, orgapi.Unavailable)
			return
		}
		source = app.Source{Scope: scope, Origin: e.Origin, Actor: actor, MembershipID: state.MembershipID, EnrollmentID: state.ID}
		// A rehired member's device may still hold an earlier, ended period.
		if sealed && e.Late != nil && (sealedMember != state.MembershipID || sealedEnrollment != state.ID) {
			if _, lateErr := e.Late.CheckLateIntakeDevice(r.Context(), scope, actor, sealedMember, sealedEnrollment); lateErr == nil {
				late = true
				source = app.Source{Scope: scope, Origin: e.Origin, Actor: actor, MembershipID: sealedMember, EnrollmentID: sealedEnrollment}
			} else if !errors.Is(lateErr, member.ErrEmployeeNotFound) {
				fail(lateErr)
				return
			}
		}
	}
	var acks []protocol.Acknowledgement
	if late {
		acks, err = e.Service.IntakeLate(r.Context(), source, batch)
	} else {
		acks, err = e.Service.Intake(r.Context(), source, batch)
	}
	if err != nil {
		fail(err)
		return
	}
	if len(acks) != len(batch.Candidates()) {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	if late {
		if _, err := e.Late.CheckLateIntakeDevice(r.Context(), scope, actor, sealedMember, sealedEnrollment); err != nil {
			fail(err)
			return
		}
	} else if suspended {
		if err := confirmSuspendedDevice(r.Context(), e.State, source); err != nil {
			fail(err)
			return
		}
	} else if err := confirmCurrentDevice(r.Context(), e.Devices, source); err != nil {
		fail(err)
		return
	}
	values := make([]json.RawMessage, 0, len(acks))
	for _, ack := range acks {
		raw := ack.CanonicalBytes()
		if len(raw) == 0 || !json.Valid(raw) {
			orgapi.WriteError(w, r, orgapi.Unavailable)
			return
		}
		values = append(values, json.RawMessage(raw))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "acknowledgements": values})
}
