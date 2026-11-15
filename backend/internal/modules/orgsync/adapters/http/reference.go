package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	guarded "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

// ReferencePath serves the signed reference feed (EMP-OFFL-006, EDR-217).
const ReferencePath = "/v1/orgs/{org_id}/sync/reference"

const referenceRequestBytes = 4096

type ReferenceOptions struct {
	Enabled    bool
	Origin     string
	Secret     []byte
	Accounts   *account.Service
	Verifier   auth.Verifier
	Devices    DeviceSource
	Repository app.ReferenceRepository
}
type referenceEndpoint struct{ ReferenceOptions }

func (referenceEndpoint) SignedReferenceFeedRule() orgapi.SignedReferenceFeedRule {
	return orgapi.SignedReferenceFeedRule{AccountProof: true, Signed: true, CurrentEnrollment: true, BoundCursor: true, WholeBodyPreflight: true, Success: 200}
}

func RegisterReference(router chi.Router, options ReferenceOptions) error {
	if !options.Enabled {
		return nil
	}
	origin, err := url.Parse(options.Origin)
	if router == nil || err != nil || protocol.ValidateOrigin(options.Origin) != nil || origin.Host != options.Verifier.Authority || reviewMissing(options.Devices) || reviewMissing(options.Repository) || len(options.Secret) < 32 {
		return app.ErrUnavailable
	}
	router.Method(http.MethodPost, ReferencePath, referenceEndpoint{options})
	return nil
}

var referenceUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type referenceRequest struct {
	SchemaVersion int     `json:"schema_version"`
	RequestID     string  `json:"request_id"`
	FamilyID      string  `json:"family_id"`
	AccessToken   string  `json:"access_token"`
	MembershipID  string  `json:"membership_id"`
	EnrollmentID  string  `json:"enrollment_id"`
	Cursor        *string `json:"cursor"`
}

func parseReferenceRequest(raw []byte) (referenceRequest, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 7 {
		return referenceRequest{}, protocol.ErrInvalid
	}
	for _, key := range []string{"schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id", "cursor"} {
		if _, ok := fields[key]; !ok {
			return referenceRequest{}, protocol.ErrInvalid
		}
	}
	var v referenceRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&v) != nil || decoder.More() || v.SchemaVersion != app.ReferenceSchema || !referenceUUID.MatchString(v.RequestID) || !referenceUUID.MatchString(v.FamilyID) || !referenceUUID.MatchString(v.MembershipID) || !referenceUUID.MatchString(v.EnrollmentID) || len(v.AccessToken) < 1 || len(v.AccessToken) > 512 || v.Cursor != nil && (len(*v.Cursor) < 1 || len(*v.Cursor) > 1024) {
		return referenceRequest{}, protocol.ErrInvalid
	}
	for _, c := range []byte(v.AccessToken) {
		if c < 0x21 || c > 0x7e {
			return referenceRequest{}, protocol.ErrInvalid
		}
	}
	return v, nil
}

type referenceChangeWire struct {
	Revision   int64               `json:"revision"`
	EntityType string              `json:"entity_type"`
	EntityID   string              `json:"entity_id"`
	Op         string              `json:"op"`
	Asset      *app.ReferenceAsset `json:"asset"`
}
type referenceResponseWire struct {
	SchemaVersion int                   `json:"schema_version"`
	RequestID     string                `json:"request_id"`
	MembershipID  string                `json:"membership_id"`
	EnrollmentID  string                `json:"enrollment_id"`
	Mode          string                `json:"mode"`
	Reason        *string               `json:"reason"`
	FromRevision  *int64                `json:"from_revision"`
	Revision      int64                 `json:"revision"`
	ConfirmedAt   string                `json:"confirmed_at"`
	Cursor        string                `json:"cursor"`
	EntityTypes   []string              `json:"entity_types"`
	Changes       []referenceChangeWire `json:"changes"`
}

func (e referenceEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	raw, err := httpapi.ReadBody(r, referenceRequestBytes)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	in, err := parseReferenceRequest(raw)
	if err != nil {
		fail(err)
		return
	}
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
	if e.Accounts == nil || reviewMissing(e.Accounts.Store) || reviewMissing(e.Accounts.Clock) || e.Verifier.Pool == nil || reviewMissing(e.Devices) || reviewMissing(e.Repository) {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	accountID, err := (guarded.AccountSessions{Service: e.Accounts}).Confirm(r.Context(), in.FamilyID, in.AccessToken, false)
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
	actor := member.DeviceActor{AccountID: accountID, FamilyID: in.FamilyID, AccessToken: in.AccessToken, ContributorID: proof.ContributorID, KeyID: proof.KeyID, Fingerprint: proof.Fingerprint, ProofExpires: time.Unix(expiry, 0).UTC()}
	if actor.Validate() != nil {
		orgapi.WriteError(w, r, orgapi.Unauthenticated)
		return
	}
	device, err := e.Devices.CheckDevice(r.Context(), scope, actor)
	if err != nil {
		fail(err)
		return
	}
	state, err := device.Enrollment.Snapshot(scope)
	if err != nil || device.Validate(scope) != nil || state.State != d.DeviceEnrolled || state.DeviceID != actor.KeyID || state.Fingerprint != actor.Fingerprint {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	if state.MembershipID != in.MembershipID || state.ID != in.EnrollmentID {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	source := app.Source{Scope: scope, Origin: e.Origin, Actor: actor, MembershipID: state.MembershipID, EnrollmentID: state.ID}
	var cursor *app.ReferenceCursor
	if in.Cursor != nil {
		opened, err := app.OpenReferenceCursor(e.Secret, *in.Cursor)
		if err != nil || opened.OrgID != scope.OrgID().String() || opened.MembershipID != source.MembershipID || opened.EnrollmentID != source.EnrollmentID {
			orgapi.WriteError(w, r, orgapi.NotFound)
			return
		}
		cursor = &opened
	}
	feed, err := e.Repository.PullReference(r.Context(), source, cursor)
	if errors.Is(err, app.ErrReferenceCursor) {
		orgapi.WriteError(w, r, orgapi.NotFound)
		return
	}
	if err != nil {
		fail(err)
		return
	}
	if err := confirmCurrentDevice(r.Context(), e.Devices, source); err != nil {
		fail(err)
		return
	}
	body := referenceResponseWire{SchemaVersion: app.ReferenceSchema, RequestID: in.RequestID, MembershipID: source.MembershipID, EnrollmentID: source.EnrollmentID, Mode: feed.Mode, FromRevision: feed.FromRevision, Revision: feed.Revision, ConfirmedAt: orgtime.FormatInstant(feed.ConfirmedAt), Cursor: feed.Cursor, EntityTypes: []string{app.ReferenceEntityAsset}, Changes: []referenceChangeWire{}}
	if feed.Reason != "" {
		reason := feed.Reason
		body.Reason = &reason
	}
	for _, change := range feed.Changes {
		body.Changes = append(body.Changes, referenceChangeWire{Revision: change.Revision, EntityType: change.EntityType, EntityID: change.EntityID, Op: change.Op, Asset: change.Asset})
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoded)
}
