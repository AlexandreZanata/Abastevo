package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	documentweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	mediaapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/application"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

const (
	origin    = "https://api.example.invalid"
	orgID     = "7b0e2a54-0000-4000-8000-000000020521"
	docID     = "7b0e2a54-0000-4000-8000-000000020522"
	accountID = "7b0e2a54-0000-4000-8000-000000020523"
	memberID  = "7b0e2a54-0000-4000-8000-000000020524"
	digest    = "f116935c2e2a3b64a8c45a692d7a1f45f9f837bc2751cea3a2c60ee9c59d2fc5"
	token     = "v1.synthetic-upload-capability"
)

func documentView(storage domain.State) app.View {
	return app.View{ID: docID, Kind: app.KindReceipt, DataMode: app.DataMode, OperationID: docID, CreatedBy: memberID,
		Original:  app.Declaration{Kind: app.KindReceipt, SHA256: digest, Bytes: 4, MediaType: app.MediaTypeJPEG},
		Statuses:  domain.Statuses{Storage: storage, Extraction: domain.ExtractionNone, Structure: domain.NotApplicable, Conference: domain.Unconferred, Reconciliation: domain.Unallocated, Fiscal: domain.NotConsulted},
		Revisions: app.Revisions{Storage: 3, Extraction: 1, Structure: 1, Conference: 1, Fiscal: 1}, CreatedAt: time.Unix(1_800_000_000, 0).UTC(), UpdatedAt: time.Unix(1_800_000_001, 0).UTC()}
}

type repository struct{ pendingErr error }

func (repository) Create(context.Context, kernel.OrgScope, app.Actor, app.Create) (app.View, bool, error) {
	return app.View{}, false, app.ErrUnavailable
}
func (r repository) Pending(context.Context, kernel.OrgScope, app.Actor, string) (app.View, error) {
	return documentView(domain.Uploading), r.pendingErr
}
func (repository) MarkStored(context.Context, kernel.OrgScope, app.Actor, string, string) (app.View, bool, error) {
	return documentView(domain.Stored), false, nil
}
func (repository) Facts(context.Context, kernel.OrgScope, string) (app.Facts, error) {
	return app.Facts{TargetID: docID, AuthoredBy: memberID}, nil
}

type originals struct {
	err  error
	read int
}

func (o *originals) Upload(_ context.Context, _ kernel.OrgScope, _ media.Member, _ string, body io.Reader) error {
	raw, _ := io.ReadAll(body)
	o.read += len(raw)
	return o.err
}
func (o *originals) Issue(context.Context, kernel.OrgScope, media.Member, string) (mediaapp.Access, error) {
	return mediaapp.Access{}, media.ErrUnavailable
}
func (o *originals) Open(context.Context, kernel.OrgScope, media.Member, string, string) (mediaapp.Content, error) {
	return mediaapp.Content{}, media.ErrUnavailable
}

type uploadTokens struct{ err error }

func (uploadTokens) Issue(kernel.OrgScope, app.Actor, string, time.Time) (app.UploadAccess, error) {
	return app.UploadAccess{}, app.ErrUnavailable
}
func (u uploadTokens) Verify(_ kernel.OrgScope, document, raw string, _ time.Time) (app.Actor, error) {
	if u.err != nil || document != docID || raw != token {
		return app.Actor{}, errors.New("synthetic refused capability")
	}
	return app.Actor{AccountID: accountID, MembershipID: memberID, Channel: "web"}, nil
}

type trackedBody struct {
	io.Reader
	reads int
}

func (b *trackedBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }

func ingress(t *testing.T, service app.Service) http.Handler {
	t.Helper()
	handler, err := documentweb.Ingress(service, origin)
	if err != nil {
		t.Fatal(err)
	}
	router := chi.NewRouter()
	router.Method(http.MethodPost, documentweb.UploadPath, handler)
	return router
}

func upload(handler http.Handler, edit func(*http.Request)) (*httptest.ResponseRecorder, *trackedBody) {
	body := &trackedBody{Reader: strings.NewReader("\xff\xd8\xff\xd9")}
	r := httptest.NewRequest(http.MethodPost, origin+"/v1/orgs/"+orgID+"/documents/"+docID+"/content/upload", body)
	r.Header.Set("Content-Type", app.MediaTypeJPEG)
	r.Header.Set(orgapi.DocumentCapabilityHeader, token)
	if edit != nil {
		edit(r)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w, body
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) (string, []any) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Details []any  `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(w.Body.Bytes(), &envelope) != nil {
		t.Fatal("refusal is not the private error envelope", w.Body.String())
	}
	return envelope.Error.Code, envelope.Error.Details
}

func service(media *originals, tokens uploadTokens) app.Service {
	return app.Service{Enabled: true, Repository: repository{}, Media: media, Tokens: tokens, Now: func() time.Time { return time.Unix(1_800_000_000, 0) },
		NewID: func() (string, error) { return "7b0e2a54-0000-4000-8000-000000020525", nil }}
}

func TestIngressVerifiesTheCapabilityAndTransportBeforeReadingBytes(t *testing.T) {
	stored := &originals{}
	handler := ingress(t, service(stored, uploadTokens{}))
	for _, refusal := range []struct {
		name, code string
		status     int
		edit       func(*http.Request)
	}{
		{"missing-capability", "UNAUTHENTICATED", 401, func(r *http.Request) { r.Header.Del(orgapi.DocumentCapabilityHeader) }},
		{"two-capabilities", "UNAUTHENTICATED", 401, func(r *http.Request) { r.Header.Add(orgapi.DocumentCapabilityHeader, token) }},
		{"foreign-origin", "UNAUTHENTICATED", 401, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }},
		{"query-token", "MALFORMED", 400, func(r *http.Request) { r.URL.RawQuery = "token=" + token }},
		{"invalid-capability", "NOT_FOUND", 404, func(r *http.Request) { r.Header.Set(orgapi.DocumentCapabilityHeader, "v1.forged") }},
		{"other-document", "NOT_FOUND", 404, func(r *http.Request) {
			r.URL.Path = strings.Replace(r.URL.Path, docID, "7b0e2a54-0000-4000-8000-000000020599", 1)
		}},
		{"png", "VALIDATION_FAILED", 422, func(r *http.Request) { r.Header.Set("Content-Type", "image/png") }},
		{"json", "VALIDATION_FAILED", 422, func(r *http.Request) { r.Header.Set("Content-Type", "application/json") }},
		{"parameters", "VALIDATION_FAILED", 422, func(r *http.Request) { r.Header.Set("Content-Type", "image/jpeg; name=receipt.jpg") }},
		{"missing-type", "VALIDATION_FAILED", 422, func(r *http.Request) { r.Header.Del("Content-Type") }},
		{"declared-over-budget", "VALIDATION_FAILED", 422, func(r *http.Request) { r.ContentLength = 5<<20 + 1 }},
	} {
		t.Run(refusal.name, func(t *testing.T) {
			w, body := upload(handler, refusal.edit)
			if w.Code != refusal.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
				t.Fatal("ingress refusal status/policy", w.Code, w.Body.String())
			}
			if code, _ := errorCode(t, w); code != refusal.code {
				t.Fatal("ingress refusal code", code)
			}
			if body.reads != 0 || stored.read != 0 {
				t.Fatal("ingress read bytes before verifying the capability and transport")
			}
			if strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), digest) {
				t.Fatal("ingress refusal echoed private values")
			}
		})
	}
	w, _ := upload(ingress(t, app.Service{}), nil)
	if w.Code != 404 {
		t.Fatal("default-off ingress exposed", w.Code)
	}
}

func TestIngressStoresVerifiedBytesAndMapsOriginalRefusals(t *testing.T) {
	stored := &originals{}
	w, body := upload(ingress(t, service(stored, uploadTokens{})), func(r *http.Request) { r.Header.Set("Origin", origin) })
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" || body.reads == 0 || stored.read != 4 {
		t.Fatal("verified bytes not stored", w.Code, w.Body.String())
	}
	var result map[string]any
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result["replayed"] != false {
		t.Fatal("ingress response", w.Body.String())
	}
	document, _ := result["document"].(map[string]any)
	statuses, _ := document["statuses"].(map[string]any)
	if document["id"] != docID || statuses["storage"] != "STORED" || statuses["fiscal"] != "NOT_CONSULTED" || document["data_mode"] != "SYNTHETIC" {
		t.Fatal("stored document view", w.Body.String())
	}
	if strings.Contains(w.Body.String(), token) || strings.Contains(w.Body.String(), "/doc/") {
		t.Fatal("ingress response carries a capability or storage key")
	}
	for _, refusal := range []struct {
		err        error
		status     int
		code, perm string
	}{
		{media.ErrIntegrity, 422, "VALIDATION_FAILED", ""}, {media.ErrTooLarge, 422, "VALIDATION_FAILED", ""}, {media.ErrUnsupported, 422, "VALIDATION_FAILED", ""},
		{media.ErrNotFound, 404, "NOT_FOUND", ""}, {media.ErrForbidden, 403, "FORBIDDEN", "doc.document.attach"}, {media.ErrUnavailable, 503, "UNAVAILABLE", ""},
	} {
		w, _ := upload(ingress(t, service(&originals{err: refusal.err}, uploadTokens{})), nil)
		code, details := errorCode(t, w)
		if w.Code != refusal.status || code != refusal.code || (refusal.perm != "") != (len(details) == 1) {
			t.Fatal("original refusal mapping", refusal.err, w.Code, w.Body.String())
		}
		if refusal.perm != "" && !strings.Contains(w.Body.String(), `"code":"`+refusal.perm+`"`) {
			t.Fatal("forbidden upload omitted the attach permission key")
		}
	}
	s := service(&originals{}, uploadTokens{})
	s.Repository = repository{pendingErr: permissions.DenyPermission("doc.document.attach")}
	if w, _ := upload(ingress(t, s), nil); w.Code != 403 {
		t.Fatal("creator without attach permission uploaded", w.Code)
	}
	s.Repository = repository{pendingErr: access.ErrMembershipSuspended}
	if w, _ := upload(ingress(t, s), nil); w.Code != 403 || !strings.Contains(w.Body.String(), "MEMBERSHIP_SUSPENDED") {
		t.Fatal("suspended creator uploaded", w.Code)
	}
	s.Repository = repository{pendingErr: app.ErrNotFound}
	if w, _ := upload(ingress(t, s), nil); w.Code != 404 {
		t.Fatal("non-creator uploaded", w.Code)
	}
	descriptor, ok := ingressHandler(t).(orgapi.DocumentIngressDescriptor)
	if !ok || descriptor.DocumentIngressRule() != (orgapi.DocumentIngressRule{Capability: true, MemberBound: true, CurrentAuthority: true, VerifiedBytes: true, Permission: "doc.document.attach", Success: 200}) {
		t.Fatal("ingress descriptor drift")
	}
	if _, err := documentweb.Ingress(service(&originals{}, uploadTokens{}), "http://api.example.invalid"); err == nil {
		t.Fatal("insecure ingress origin accepted")
	}
}

func ingressHandler(t *testing.T) http.Handler {
	t.Helper()
	handler, err := documentweb.Ingress(service(&originals{}, uploadTokens{}), origin)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func TestDocumentRecordsAcceptOnlyOwnDeclarationsAndDocumentTargets(t *testing.T) {
	id, _ := kernel.ParseOrgID(orgID)
	scope, _ := kernel.NewOrgScope(id)
	rules := []permissions.Rule{{}}
	records := documentweb.DocumentRecords{}
	for name, check := range map[string]struct {
		policy access.Policy
		facts  permissions.ResourceFacts
		rules  []permissions.Rule
	}{
		"unknown-policy":     {access.Policy{Permission: "fleet.asset.read", Capability: "organizations", Effect: access.Read}, permissions.ResourceFacts{OrgID: id, ID: docID}, rules},
		"no-rules":           {app.ContentPolicy(), permissions.ResourceFacts{OrgID: id, ID: docID}, nil},
		"content-on-root":    {app.ContentPolicy(), permissions.ResourceFacts{OrgID: id, ID: orgID}, rules},
		"attach-on-document": {app.AttachPolicy(), permissions.ResourceFacts{OrgID: id, ID: docID, AuthoredBy: memberID}, rules},
		"foreign-facts":      {app.ContentPolicy(), permissions.ResourceFacts{ID: docID}, rules},
	} {
		if records.Check(t.Context(), scope, access.Subject{}, check.policy, check.facts, check.rules) == nil {
			t.Fatal("document record gate admitted", name)
		}
	}
}
