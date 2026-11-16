package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/go-chi/chi/v5"
)

// Shared wire oracle of the signed attachment session (EMP-OFFL-005, EDR-270).
// The Kotlin client consumes the same file; this consumer pins the server side.
const (
	attachmentVectorPath = "../../../../../../contracts/testdata/enterprise/sync/attachment-session-v1.psv"
	attachmentVectorSHA  = "1996b45fb008044a04df5715315ac0d82f82f5d942f36a8c64b89dbf4f170b9c"
)

type attachmentVector struct {
	id, operation, outcome   string
	input, context, expected []byte
}

func loadAttachmentVectors(t *testing.T) []attachmentVector {
	t.Helper()
	raw, err := os.ReadFile(attachmentVectorPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != attachmentVectorSHA {
		t.Fatal("attachment vector file changed without updating its pinned SHA-256")
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) != 91 || lines[0] != referenceVectorHeader || lines[len(lines)-1] != "" {
		t.Fatal("attachment vector framing or count")
	}
	outcomes := map[string]map[string]bool{"REQUEST": {"OK": true, "INVALID": true, "LIMIT": true}, "RESPONSE": {"OK": true, "INVALID": true, "LIMIT": true, "BINDING": true}}
	counts, ids := map[string]int{}, map[string]bool{}
	var vectors []attachmentVector
	for _, line := range lines[1 : len(lines)-1] {
		parts := strings.Split(line, "|")
		if len(parts) != 6 || ids[parts[0]] || !outcomes[parts[1]][parts[4]] {
			t.Fatal("attachment vector row", line)
		}
		ids[parts[0]] = true
		v := attachmentVector{id: parts[0], operation: parts[1], outcome: parts[4]}
		if v.input, err = referenceHex(parts[2]); err != nil {
			t.Fatal(v.id, err)
		}
		if parts[3] != "-" {
			if v.context, err = referenceHex(parts[3]); err != nil {
				t.Fatal(v.id, err)
			}
		}
		if parts[5] != "-" {
			if v.expected, err = referenceHex(parts[5]); err != nil {
				t.Fatal(v.id, err)
			}
		}
		if (v.operation == "RESPONSE") != (v.context != nil) || (v.outcome == "OK") != (v.expected != nil) {
			t.Fatal("attachment vector context or expectation shape", v.id)
		}
		counts[v.operation+"/"+v.outcome]++
		vectors = append(vectors, v)
	}
	want := map[string]int{"REQUEST/OK": 15, "REQUEST/INVALID": 37, "REQUEST/LIMIT": 1, "RESPONSE/OK": 11, "RESPONSE/INVALID": 20, "RESPONSE/LIMIT": 1, "RESPONSE/BINDING": 4}
	if len(counts) != len(want) {
		t.Fatal("attachment vector outcome classes", counts)
	}
	for key, n := range want {
		if counts[key] != n {
			t.Fatal("attachment vector counts", key, counts[key])
		}
	}
	return vectors
}

// canonicalJSON re-encodes a decoded object with sorted keys, compact and
// without HTML escaping: the oracle's canonical spelling for ASCII values.
func canonicalJSON(t *testing.T, value map[string]any) []byte {
	t.Helper()
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out bytes.Buffer
	out.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			out.WriteByte(',')
		}
		k, _ := json.Marshal(key)
		out.Write(k)
		out.WriteByte(':')
		switch v := value[key].(type) {
		case map[string]any:
			out.Write(canonicalJSON(t, v))
		default:
			var b bytes.Buffer
			e := json.NewEncoder(&b)
			e.SetEscapeHTML(false)
			if err := e.Encode(v); err != nil {
				t.Fatal(err)
			}
			out.Write(bytes.TrimSuffix(b.Bytes(), []byte("\n")))
		}
	}
	out.WriteByte('}')
	return out.Bytes()
}

func attachmentRequestMap(in attachmentRequest) map[string]any {
	return map[string]any{"schema_version": json.Number("1"), "request_id": in.RequestID, "family_id": in.FamilyID, "access_token": in.AccessToken,
		"membership_id": in.MembershipID, "enrollment_id": in.EnrollmentID, "declaration_id": in.DeclarationID, "document_id": in.DocumentID,
		"kind": in.Kind, "sha256": in.SHA256, "bytes": json.Number(strconv.FormatInt(in.Bytes, 10)), "media_type": in.MediaType,
		"link_id": in.LinkID, "target_kind": in.TargetKind, "target_id": in.TargetID}
}

// The server parser accepts exactly the requests the client oracle accepts,
// with the same values, and refuses the others before any proof is read.
func TestAttachmentRequestVectorsMatchTheServerParser(t *testing.T) {
	for _, v := range loadAttachmentVectors(t) {
		if v.operation != "REQUEST" {
			continue
		}
		in, err := parseAttachmentRequest(v.input)
		switch v.outcome {
		case "OK":
			if err != nil {
				t.Fatal("server refused a client request", v.id, err)
			}
			if got := canonicalJSON(t, attachmentRequestMap(in)); !bytes.Equal(got, v.expected) {
				t.Fatalf("server parsed other values %s:\n%s\n%s", v.id, got, v.expected)
			}
		case "LIMIT":
			if !errors.Is(err, protocol.ErrLimit) {
				t.Fatal("oversized request not refused as a limit", v.id, err)
			}
		default:
			if !errors.Is(err, protocol.ErrInvalid) {
				t.Fatal("server accepted a request the client refuses", v.id, err)
			}
		}
	}
}

func wireRequest(t *testing.T, raw []byte) attachmentRequest {
	t.Helper()
	in, err := parseAttachmentRequest(raw)
	if err != nil {
		t.Fatal("vector context is not a valid request", err)
	}
	return in
}

// sessionOf rebuilds the owner's session from a decoded wire; an expiry that
// is not an instant cannot come from a server session.
func sessionOf(w attachmentResponseWire) (app.AttachmentSession, bool) {
	session := app.AttachmentSession{DocumentID: w.DocumentID, Storage: w.Storage, StorageRevision: w.StorageRevision}
	if w.Upload != nil {
		at, err := orgtime.ParseInstant(w.Upload.ExpiresAt)
		if err != nil {
			return app.AttachmentSession{}, false
		}
		session.Capability, session.ExpiresAt = w.Upload.Capability, at
	}
	return session, true
}

// Every response the client accepts is byte for byte what the server
// produces for that request and session; every response it refuses as
// invalid or oversized is not server-producible.
func TestAttachmentResponseVectorsAreExactlyTheServerWire(t *testing.T) {
	for _, v := range loadAttachmentVectors(t) {
		if v.operation != "RESPONSE" {
			continue
		}
		in := wireRequest(t, v.context)
		var wire attachmentResponseWire
		decoder := json.NewDecoder(bytes.NewReader(v.input))
		decoder.DisallowUnknownFields()
		decoded := decoder.Decode(&wire) == nil && !decoder.More()
		producible := false
		if decoded {
			source := app.Source{MembershipID: wire.MembershipID, EnrollmentID: wire.EnrollmentID}
			request := in
			request.RequestID, request.MembershipID, request.EnrollmentID, request.DocumentID = wire.RequestID, wire.MembershipID, wire.EnrollmentID, wire.DocumentID
			if session, ok := sessionOf(wire); ok {
				if raw, err := attachmentResponse(request, source, session); err == nil && bytes.Equal(raw, v.input) {
					producible = true
				}
			}
		}
		switch v.outcome {
		case "OK", "BINDING":
			if !producible {
				t.Fatal("client-accepted response is not the server wire", v.id)
			}
			bound := wire.RequestID == in.RequestID && wire.MembershipID == in.MembershipID && wire.EnrollmentID == in.EnrollmentID && wire.DocumentID == in.DocumentID
			if bound != (v.outcome == "OK") {
				t.Fatal("response binding class differs", v.id)
			}
		default:
			if producible {
				t.Fatal("server can produce a response the client refuses", v.id)
			}
		}
	}
}

type attachmentSessionsFake struct{ calls int }

func (f *attachmentSessionsFake) OpenAttachment(context.Context, app.Source, app.AttachmentDeclaration) (app.AttachmentSession, error) {
	f.calls++
	return app.AttachmentSession{}, app.ErrUnavailable
}

const attachmentRequestBody = `{"schema_version":1,"request_id":"7b0e2a54-0000-4000-8000-000000041501","family_id":"7b0e2a54-0000-4000-8000-000000041502","access_token":"synthetic-private-proof","membership_id":"7b0e2a54-0000-4000-8000-000000041503","enrollment_id":"7b0e2a54-0000-4000-8000-000000041504","declaration_id":"7b0e2a54-0000-4000-8000-000000041505","document_id":"7b0e2a54-0000-4000-8000-000000041506","kind":"RECEIPT","sha256":"f116935c2e2a3b64a8c45a692d7a1f45f9f837bc2751cea3a2c60ee9c59d2fc5","bytes":1024,"media_type":"image/jpeg","link_id":"7b0e2a54-0000-4000-8000-000000041507","target_kind":"METER_READING","target_id":"7b0e2a54-0000-4000-8000-000000041508"}`

func attachmentPost(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "https://company.example.invalid/v1/orgs/7b0e2a54-0000-4000-8000-000000041509/sync/attachments", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

// Alternate authority, a foreign origin or host and malformed transport are
// refused before any account, device or document source is consulted.
func TestAttachmentSessionRefusesAlternateAuthorityAndMalformedTransportFirst(t *testing.T) {
	cases := []struct {
		name string
		edit func(*http.Request)
		body string
		want int
	}{
		{"cookie", func(r *http.Request) { r.Header.Set("Cookie", "synthetic") }, attachmentRequestBody, 401},
		{"bearer", func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, attachmentRequestBody, 401},
		{"csrf", func(r *http.Request) { r.Header.Set("X-CSRF", "synthetic") }, attachmentRequestBody, 401},
		{"signature input", func(r *http.Request) { r.Header.Set("Signature-Input", "synthetic") }, attachmentRequestBody, 401},
		{"foreign origin", func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, attachmentRequestBody, 401},
		{"double origin", func(r *http.Request) {
			r.Header.Add("Origin", "https://company.example.invalid")
			r.Header.Add("Origin", "https://company.example.invalid")
		}, attachmentRequestBody, 401},
		{"foreign host", func(r *http.Request) { r.Host = "foreign.example.invalid" }, attachmentRequestBody, 401},
		{"query", func(r *http.Request) { r.URL.RawQuery = "x=1" }, attachmentRequestBody, 400},
		{"form", func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") }, attachmentRequestBody, 400},
		{"oversized", func(*http.Request) {}, strings.TrimSuffix(attachmentRequestBody, "}") + strings.Repeat(" ", AttachmentRequestBytes) + "}", 400},
		{"empty object", func(*http.Request) {}, `{}`, 400},
		{"duplicate key", func(*http.Request) {}, strings.TrimSuffix(attachmentRequestBody, "}") + `,"kind":"RECEIPT"}`, 400},
		{"png", func(*http.Request) {}, strings.Replace(attachmentRequestBody, "image/jpeg", "image/png", 1), 400},
		{"null token", func(*http.Request) {}, strings.Replace(attachmentRequestBody, `"synthetic-private-proof"`, "null", 1), 400},
	}
	for _, test := range cases {
		sessions := &attachmentSessionsFake{}
		r := attachmentPost(test.body)
		test.edit(r)
		w := httptest.NewRecorder()
		w.Header().Set("ETag", "old")
		attachmentEndpoint{AttachmentOptions{Enabled: true, Origin: "https://company.example.invalid", Sessions: sessions}}.ServeHTTP(w, r)
		if w.Code != test.want || sessions.calls != 0 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" || strings.Contains(w.Body.String(), "synthetic-private") {
			t.Fatal("attachment session transport refusal", test.name, w.Code)
		}
	}
	// A disabled endpoint is absent; a well-formed request without its
	// account source is unavailable, still before any document work.
	sessions := &attachmentSessionsFake{}
	w := httptest.NewRecorder()
	attachmentEndpoint{AttachmentOptions{Enabled: false, Sessions: sessions}}.ServeHTTP(w, attachmentPost(attachmentRequestBody))
	if w.Code != 404 || sessions.calls != 0 {
		t.Fatal("disabled attachment session served", w.Code)
	}
	w = httptest.NewRecorder()
	routed := chi.NewRouter()
	routed.Method(http.MethodPost, AttachmentsPath, attachmentEndpoint{AttachmentOptions{Enabled: true, Origin: "https://company.example.invalid", Sessions: sessions}})
	routed.ServeHTTP(w, attachmentPost(attachmentRequestBody))
	if w.Code != 503 || sessions.calls != 0 {
		t.Fatal("attachment session without proof sources", w.Code)
	}
}

func TestAttachmentSessionRegistrationRequiresEveryCoherentSource(t *testing.T) {
	var absent *attachmentSessionsFake
	valid := AttachmentOptions{Enabled: true, Origin: "https://company.example.invalid", Verifier: auth.Verifier{Authority: "company.example.invalid"}, Devices: &changingDevice{}, Sessions: &attachmentSessionsFake{}}
	for _, edit := range []func(*AttachmentOptions){func(v *AttachmentOptions) { v.Sessions = nil }, func(v *AttachmentOptions) { v.Sessions = absent },
		func(v *AttachmentOptions) { v.Devices = (*changingDevice)(nil) }, func(v *AttachmentOptions) { v.Verifier.Authority = "foreign" },
		func(v *AttachmentOptions) { v.Origin = "http://company.example.invalid" }} {
		v := valid
		edit(&v)
		if RegisterAttachments(chi.NewRouter(), v) == nil {
			t.Fatal("incoherent attachment session registered")
		}
	}
	router := chi.NewRouter()
	if RegisterAttachments(router, valid) != nil {
		t.Fatal("coherent attachment session failed registration")
	}
	mounted := false
	_ = chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodPost && path == AttachmentsPath {
			_, mounted = handler.(attachmentEndpoint)
		}
		return nil
	})
	if !mounted {
		t.Fatal("attachment session not mounted at its path")
	}
	disabled := chi.NewRouter()
	if RegisterAttachments(disabled, AttachmentOptions{}) != nil || len(disabled.Routes()) != 0 {
		t.Fatal("disabled attachment session mounted")
	}
}

// The response is built only from the verified source and the owner's
// session; an unknown state, a capability on a settled document or an
// invalid revision is never written.
func TestAttachmentResponseRefusesInconsistentSessions(t *testing.T) {
	in := wireRequest(t, []byte(attachmentRequestBody))
	source := app.Source{MembershipID: in.MembershipID, EnrollmentID: in.EnrollmentID}
	at := time.Date(2026, 11, 15, 12, 5, 0, 0, time.UTC)
	good := app.AttachmentSession{DocumentID: in.DocumentID, Storage: "UPLOADING", StorageRevision: 1, Capability: "v1.synthetic", ExpiresAt: at}
	if _, err := attachmentResponse(in, source, good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []app.AttachmentSession{
		{DocumentID: in.DocumentID, Storage: "STORED", StorageRevision: 3, Capability: "v1.synthetic", ExpiresAt: at},
		{DocumentID: in.DocumentID, Storage: "VERIFIED", StorageRevision: 3},
		{DocumentID: in.DocumentID, Storage: "STORED", StorageRevision: 0},
		{DocumentID: "7b0e2a54-0000-4000-8000-000000041599", Storage: "STORED", StorageRevision: 3},
		{DocumentID: in.DocumentID, Storage: "UPLOADING", StorageRevision: 1, Capability: "with space", ExpiresAt: at},
		{DocumentID: in.DocumentID, Storage: "UPLOADING", StorageRevision: 1, Capability: "v1.synthetic"},
	} {
		if _, err := attachmentResponse(in, source, bad); err == nil {
			t.Fatal("inconsistent session written", bad.Storage)
		}
	}
	if _, err := attachmentResponse(in, app.Source{MembershipID: in.EnrollmentID, EnrollmentID: in.EnrollmentID}, good); err == nil {
		t.Fatal("response bound to another membership")
	}
}
