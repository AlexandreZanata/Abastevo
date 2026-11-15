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
	"unicode/utf8"

	account "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	docdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
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

// AttachmentsPath is the signed attachment session (EMP-OFFL-005, EDR-270):
// declare one document, link it to its record and receive the upload capability.
const AttachmentsPath = "/v1/orgs/{org_id}/sync/attachments"

// AttachmentRequestBytes bounds the whole signed request body.
const AttachmentRequestBytes = 4096

const (
	attachmentKind      = "RECEIPT"
	attachmentMediaType = "image/jpeg"
	attachmentMaxBytes  = 5 * 1024 * 1024
	attachmentMaxGrant  = 4096
)

type AttachmentOptions struct {
	Enabled  bool
	Origin   string
	Accounts *account.Service
	Verifier auth.Verifier
	Devices  DeviceSource
	Sessions app.AttachmentSessions
}
type attachmentEndpoint struct{ AttachmentOptions }

func (attachmentEndpoint) SignedAttachmentSessionRule() orgapi.SignedAttachmentSessionRule {
	return orgapi.SignedAttachmentSessionRule{AccountProof: true, Signed: true, CurrentEnrollment: true, CreatorBound: true, OwnerLinked: true, WholeBodyPreflight: true, Success: 200}
}

// RegisterAttachments mounts the signed attachment session; disabled mounts nothing.
func RegisterAttachments(router chi.Router, options AttachmentOptions) error {
	if !options.Enabled {
		return nil
	}
	origin, err := url.Parse(options.Origin)
	if router == nil || err != nil || protocol.ValidateOrigin(options.Origin) != nil || origin.Host != options.Verifier.Authority || reviewMissing(options.Devices) || reviewMissing(options.Sessions) {
		return app.ErrUnavailable
	}
	router.Method(http.MethodPost, AttachmentsPath, attachmentEndpoint{options})
	return nil
}

type attachmentRequest struct {
	SchemaVersion int    `json:"schema_version"`
	RequestID     string `json:"request_id"`
	FamilyID      string `json:"family_id"`
	AccessToken   string `json:"access_token"`
	MembershipID  string `json:"membership_id"`
	EnrollmentID  string `json:"enrollment_id"`
	DeclarationID string `json:"declaration_id"`
	DocumentID    string `json:"document_id"`
	Kind          string `json:"kind"`
	SHA256        string `json:"sha256"`
	Bytes         int64  `json:"bytes"`
	MediaType     string `json:"media_type"`
	LinkID        string `json:"link_id"`
	TargetKind    string `json:"target_kind"`
	TargetID      string `json:"target_id"`
}

var attachmentDigits = regexp.MustCompile(`^[1-9][0-9]{0,6}$`)
var attachmentSHA = regexp.MustCompile(`^[0-9a-f]{64}$`)

func attachmentID(raw string) bool {
	id, err := kernel.ParseOrgID(raw)
	return err == nil && id.String() == raw
}

func attachmentGraphic(raw string, maximum int) bool {
	if len(raw) < 1 || len(raw) > maximum {
		return false
	}
	for _, c := range []byte(raw) {
		if c < 0x21 || c > 0x7e {
			return false
		}
	}
	return true
}

// parseAttachmentRequest is the strict whole-body preflight: exactly the
// fifteen keys once each, exact types, closed values and bounds, decided
// before any account, device or document source is read.
func parseAttachmentRequest(raw []byte) (attachmentRequest, error) {
	if len(raw) > AttachmentRequestBytes {
		return attachmentRequest{}, protocol.ErrLimit
	}
	if !utf8.Valid(raw) {
		return attachmentRequest{}, protocol.ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return attachmentRequest{}, protocol.ErrInvalid
	}
	var v attachmentRequest
	texts := map[string]*string{"request_id": &v.RequestID, "family_id": &v.FamilyID, "access_token": &v.AccessToken, "membership_id": &v.MembershipID,
		"enrollment_id": &v.EnrollmentID, "declaration_id": &v.DeclarationID, "document_id": &v.DocumentID, "kind": &v.Kind, "sha256": &v.SHA256,
		"media_type": &v.MediaType, "link_id": &v.LinkID, "target_kind": &v.TargetKind, "target_id": &v.TargetID}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return attachmentRequest{}, protocol.ErrInvalid
		}
		seen[key] = true
		var value any
		if decoder.Decode(&value) != nil {
			return attachmentRequest{}, protocol.ErrInvalid
		}
		switch key {
		case "schema_version":
			if n, ok := value.(json.Number); !ok || n.String() != "1" {
				return attachmentRequest{}, protocol.ErrInvalid
			}
			v.SchemaVersion = app.AttachmentSchema
		case "bytes":
			n, ok := value.(json.Number)
			if !ok || !attachmentDigits.MatchString(n.String()) {
				return attachmentRequest{}, protocol.ErrInvalid
			}
			v.Bytes, _ = strconv.ParseInt(n.String(), 10, 64)
		default:
			destination := texts[key]
			text, ok := value.(string)
			if destination == nil || !ok {
				return attachmentRequest{}, protocol.ErrInvalid
			}
			*destination = text
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return attachmentRequest{}, protocol.ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF || len(seen) != len(texts)+2 {
		return attachmentRequest{}, protocol.ErrInvalid
	}
	for _, id := range []string{v.RequestID, v.FamilyID, v.MembershipID, v.EnrollmentID, v.DeclarationID, v.DocumentID, v.LinkID, v.TargetID} {
		if !attachmentID(id) {
			return attachmentRequest{}, protocol.ErrInvalid
		}
	}
	if _, err := docdomain.ParseTargetKind(v.TargetKind); err != nil || v.Kind != attachmentKind || v.MediaType != attachmentMediaType ||
		!attachmentSHA.MatchString(v.SHA256) || v.Bytes < 1 || v.Bytes > attachmentMaxBytes || !attachmentGraphic(v.AccessToken, 512) ||
		v.DeclarationID == v.DocumentID || v.DeclarationID == v.LinkID || v.DocumentID == v.LinkID {
		return attachmentRequest{}, protocol.ErrInvalid
	}
	return v, nil
}

type attachmentUploadWire struct {
	Capability string `json:"capability"`
	ExpiresAt  string `json:"expires_at"`
}

// attachmentResponseWire is the fixed response field order shared with the
// Kotlin client (sync/attachment-session-v1.psv).
type attachmentResponseWire struct {
	SchemaVersion   int                   `json:"schema_version"`
	RequestID       string                `json:"request_id"`
	MembershipID    string                `json:"membership_id"`
	EnrollmentID    string                `json:"enrollment_id"`
	DocumentID      string                `json:"document_id"`
	Storage         string                `json:"storage"`
	StorageRevision int64                 `json:"storage_revision"`
	Upload          *attachmentUploadWire `json:"upload"`
}

var attachmentStorage = map[string]bool{"UPLOADING": true, "RECEIVED": true, "STORED": true, "QUARANTINED": true, "FAILED": true, "REPLACED": true, "DELETED": true}

// attachmentResponse writes only a session that belongs to the request and the
// verified source; a capability exists only for an UPLOADING document.
func attachmentResponse(in attachmentRequest, source app.Source, session app.AttachmentSession) ([]byte, error) {
	if source.MembershipID != in.MembershipID || source.EnrollmentID != in.EnrollmentID || session.DocumentID != in.DocumentID ||
		!attachmentStorage[session.Storage] || session.StorageRevision < 1 {
		return nil, app.ErrUnavailable
	}
	body := attachmentResponseWire{SchemaVersion: app.AttachmentSchema, RequestID: in.RequestID, MembershipID: in.MembershipID, EnrollmentID: in.EnrollmentID,
		DocumentID: in.DocumentID, Storage: session.Storage, StorageRevision: session.StorageRevision}
	if session.Capability != "" || !session.ExpiresAt.IsZero() {
		if session.Storage != "UPLOADING" || !attachmentGraphic(session.Capability, attachmentMaxGrant) || session.ExpiresAt.IsZero() {
			return nil, app.ErrUnavailable
		}
		body.Upload = &attachmentUploadWire{Capability: session.Capability, ExpiresAt: orgtime.FormatInstant(session.ExpiresAt)}
	}
	return json.Marshal(body)
}

func (e attachmentEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	raw, err := httpapi.ReadBody(r, AttachmentRequestBytes)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Malformed)
		return
	}
	in, err := parseAttachmentRequest(raw)
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
	if e.Accounts == nil || reviewMissing(e.Accounts.Store) || reviewMissing(e.Accounts.Clock) || e.Verifier.Pool == nil || reviewMissing(e.Devices) || reviewMissing(e.Sessions) {
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
	session, err := e.Sessions.OpenAttachment(r.Context(), source, app.AttachmentDeclaration{DeclarationID: in.DeclarationID, DocumentID: in.DocumentID, Kind: in.Kind,
		SHA256: in.SHA256, MediaType: in.MediaType, Bytes: in.Bytes, LinkID: in.LinkID, TargetKind: in.TargetKind, TargetID: in.TargetID})
	if err != nil {
		// The document owner answers bounded private envelopes; a permission
		// refusal keeps its registered key (BR-AUTH-013).
		if _, registered := orgapi.Registered(err); registered {
			orgapi.WriteError(w, r, err)
		} else {
			fail(err)
		}
		return
	}
	// A capability is never released to a device whose period changed meanwhile.
	if err := confirmCurrentDevice(r.Context(), e.Devices, source); err != nil {
		fail(err)
		return
	}
	encoded, err := attachmentResponse(in, source, session)
	if err != nil {
		orgapi.WriteError(w, r, orgapi.Unavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoded)
}
