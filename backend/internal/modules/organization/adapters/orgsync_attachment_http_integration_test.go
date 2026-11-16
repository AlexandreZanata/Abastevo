//go:build integration

package adapters_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	mediapg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/adapters"
	syncweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// attachmentHTTPFixture is the real default enterprise runtime with a
// disposable private document bucket, a signed enrolled device and a meter
// whose readings are the C1 link targets (EMP-OFFL-005).
type attachmentHTTPFixture struct {
	meterIntakeHTTPFixture
	router chi.Router
	raw    []byte
	org    string
}

func attachmentHTTP(t *testing.T) attachmentHTTPFixture {
	t.Helper()
	required := func(name string) string {
		t.Helper()
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("explicit disposable %s required", name)
		}
		return value
	}
	fx := syncFixture(t)
	assets := fleetStore(t, fx.legalFixtureState)
	created, _, err := fleetWrite(t, fx.legalFixtureState, assets, fleetCommand(t, fleetCreateInput()))
	if err != nil {
		t.Fatal(err)
	}
	meters := meterStore(t, fx.legalFixtureState)
	installed, _, err := meterInstall(t, fx.legalFixtureState, meters, meterInstallCommand(t, meterInstallInput(t, created.AssetID, time.Now().UTC().Add(-time.Hour))))
	if err != nil {
		t.Fatal(err)
	}
	view, err := meters.ReadLatestMeter(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, created.AssetID, installed.MeterID)
	if err != nil {
		t.Fatal(err)
	}
	grantEmployeeTables(t, fx.legalFixtureState)
	documentGrants(t, fx.deviceFixtureState)
	role := pgx.Identifier{fx.f.RuntimeRole}.Sanitize()
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT(org_id,employee_id,asset_id,valid_from,valid_to,end_cause) ON org_asset_assignments TO "+role+"; GRANT SELECT,INSERT ON org_document_links TO "+role); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../evidence/adapters/media/testdata/valid-4x4.jpg")
	if err != nil {
		t.Fatal(err)
	}
	marker := []byte("SYNTHETIC-DEVICE-ATTACHMENT-ORIGINAL")
	raw = append(append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(marker) + 2)}, marker...), raw[2:]...)
	accounts := *fx.accounts
	accounts.Store, accounts.Clock = accountpg.NewPGStore(fx.f.Runtime), syncHTTPClock{}
	documents := mediapg.StorageConfig{Endpoint: required("ANPFUEL_TEST_STORAGE_ENDPOINT"), Bucket: required("ANPFUEL_TEST_STORAGE_BUCKET"), Region: required("ANPFUEL_TEST_STORAGE_REGION"),
		Credentials: storage.Credentials{AccessKeyID: required("ANPFUEL_TEST_STORAGE_ACCESS_KEY_ID"), SecretAccessKey: required("ANPFUEL_TEST_STORAGE_SECRET_ACCESS_KEY")}, AllowLoopback: true}
	router := chi.NewRouter()
	// No injected handler or store: the real default runtime with its document owner.
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{47}, 32), Pool: fx.f.Runtime, Accounts: &accounts, DocumentStorage: &documents}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(router)
	t.Cleanup(server.Close)
	server.Client().Timeout = 8 * time.Second
	return attachmentHTTPFixture{meterIntakeHTTPFixture: meterIntakeHTTPFixture{syncFixtureState: fx, server: server, asset: created.AssetID, meter: installed, installedAt: view.InstalledAt,
		path: "/v1/orgs/" + fx.scope.OrgID().String() + "/sync/operations"}, router: router, raw: raw, org: fx.scope.OrgID().String()}
}

// reading records one of the device person's own readings through the signed intake.
func (fx attachmentHTTPFixture) reading(t *testing.T, value string) string {
	t.Helper()
	op := fx.operation(t, value, nil)
	if ack := fx.dispatch(t, op, 200); ack.Result() != protocol.Accepted {
		t.Fatal("synthetic reading was not accepted", ack.Result(), ack.Code())
	}
	return op.Snapshot().Entity.ID
}

func (fx attachmentHTTPFixture) declaration(t *testing.T, raw []byte, kind, target string) map[string]any {
	t.Helper()
	sum := sha256.Sum256(raw)
	return map[string]any{"schema_version": 1, "request_id": onboardingID(t), "family_id": fx.person.auth.Session.FamilyID, "access_token": fx.person.auth.Session.AccessToken,
		"membership_id": fx.source.MembershipID, "enrollment_id": fx.source.EnrollmentID, "declaration_id": onboardingID(t), "document_id": onboardingID(t),
		"kind": "RECEIPT", "sha256": hex.EncodeToString(sum[:]), "bytes": len(raw), "media_type": "image/jpeg", "link_id": onboardingID(t),
		"target_kind": kind, "target_id": target}
}

func again(t *testing.T, body map[string]any, edits ...func(map[string]any)) map[string]any {
	t.Helper()
	copy := map[string]any{}
	for key, value := range body {
		copy[key] = value
	}
	copy["request_id"] = onboardingID(t)
	for _, edit := range edits {
		edit(copy)
	}
	return copy
}

type attachmentAnswer struct {
	SchemaVersion   int    `json:"schema_version"`
	RequestID       string `json:"request_id"`
	MembershipID    string `json:"membership_id"`
	EnrollmentID    string `json:"enrollment_id"`
	DocumentID      string `json:"document_id"`
	Storage         string `json:"storage"`
	StorageRevision int64  `json:"storage_revision"`
	Upload          *struct {
		Capability string `json:"capability"`
		ExpiresAt  string `json:"expires_at"`
	} `json:"upload"`
}

// session sends one signed attachment session request for org (default the
// fixture organization) and returns its status, decoded answer and error code.
func (fx attachmentHTTPFixture) session(t *testing.T, org string, body map[string]any) (int, attachmentAnswer, string) {
	t.Helper()
	if org == "" {
		org = fx.org
	}
	path := strings.Replace(syncweb.AttachmentsPath, "{org_id}", org, 1)
	r := deviceSignedRequest(t, fx.registrar, fx.person.signer, path, body, time.Time{})
	endpoint, err := url.Parse(fx.server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	r.URL, r.RequestURI, r.Host = endpoint, "", deviceTestHost
	response, err := fx.server.Client().Do(r)
	if err != nil {
		t.Fatal("actual local TLS signed request failed")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 8193))
	if err != nil || len(raw) > 8192 || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("ETag") != "" {
		t.Fatal("attachment session answer unreadable or cacheable")
	}
	for _, private := range []string{fx.person.auth.Session.AccessToken, fx.person.auth.Session.FamilyID, fx.person.signer.keyID, fx.person.signer.fingerprint, fx.person.signer.contributor, string(fx.raw[6:30])} {
		if strings.Contains(string(raw), private) {
			t.Fatal("attachment session disclosed proof or content")
		}
	}
	var answer attachmentAnswer
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if response.StatusCode == 200 {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&answer) != nil || answer.SchemaVersion != 1 || answer.RequestID != body["request_id"] || answer.MembershipID != fx.source.MembershipID ||
			answer.EnrollmentID != fx.source.EnrollmentID || answer.DocumentID != body["document_id"] {
			t.Fatal("attachment session answer is not bound to its request and device")
		}
		if answer.Upload != nil && answer.Storage != "UPLOADING" {
			t.Fatal("capability answered for a settled document")
		}
	} else if json.Unmarshal(raw, &envelope) != nil || envelope.Error.Code == "" {
		t.Fatal("attachment refusal is not the private error envelope", response.StatusCode)
	}
	return response.StatusCode, answer, envelope.Error.Code
}

func (fx attachmentHTTPFixture) upload(t *testing.T, org, document, capability string, body io.Reader, length int64) (int, map[string]any, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, deviceTestOrigin+"/v1/orgs/"+org+"/documents/"+document+"/content/upload", body)
	r.ContentLength = length
	r.Header.Set("Content-Type", "image/jpeg")
	if capability != "" {
		r.Header.Set(orgapi.DocumentCapabilityHeader, capability)
	}
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
		t.Fatal("document ingress answer is cacheable")
	}
	var answer map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &answer)
	code := ""
	if failure, ok := answer["error"].(map[string]any); ok {
		code, _ = failure["code"].(string)
	}
	return w.Code, answer, code
}

func (fx attachmentHTTPFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := fx.f.Admin.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (fx attachmentHTTPFixture) storage(t *testing.T, document string) (string, int64) {
	t.Helper()
	var state string
	var revision int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT storage_state, storage_revision FROM org_documents WHERE org_id=$1 AND id=$2`, fx.org, document).Scan(&state, &revision); err != nil {
		t.Fatal(err)
	}
	return state, revision
}

type failingReader struct {
	data []byte
	read int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.read >= len(r.data)/2 {
		return 0, errors.New("synthetic connection reset")
	}
	n := copy(p, r.data[r.read:len(r.data)/2])
	r.read += n
	return n, nil
}

// BUC-EMP-211/212/213: the device declares, links and uploads its own
// attachment; only matching bytes become STORED, through private ingress.
func TestSignedAttachmentSessionDeclaresLinksAndStoresOnlyVerifiedBytes(t *testing.T) {
	fx := attachmentHTTP(t)
	reading := fx.reading(t, "1000")
	body := fx.declaration(t, fx.raw, "METER_READING", reading)
	document := body["document_id"].(string)

	status, first, code := fx.session(t, "", body)
	if status != 200 || first.Storage != "UPLOADING" || first.StorageRevision != 1 || first.Upload == nil || first.Upload.Capability == "" {
		t.Fatal("first attachment session", status, code)
	}
	expires, err := time.Parse(time.RFC3339Nano, first.Upload.ExpiresAt)
	if err != nil || expires.After(time.Now().Add(5*time.Minute+time.Second)) || !expires.After(time.Now()) {
		t.Fatal("device capability outlives five minutes", first.Upload.ExpiresAt)
	}
	// The device is the creator; the document is linked to the reading at once (BR-OFFL-015).
	if fx.count(t, `SELECT count(*) FROM org_documents WHERE org_id=$1 AND id=$2 AND created_by=$3 AND operation_id=$4`, fx.org, document, fx.source.MembershipID, body["declaration_id"]) != 1 ||
		fx.count(t, `SELECT count(*) FROM org_document_links WHERE org_id=$1 AND document_id=$2 AND target_kind='METER_READING' AND target_id=$3 AND operation_id=$4`, fx.org, document, reading, body["link_id"]) != 1 {
		t.Fatal("device declaration or link not stored as the creator's own")
	}
	events := `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2 AND action=$3`
	if fx.count(t, events, fx.org, document, "document.upload.created") != 1 || fx.count(t, events, fx.org, document, "document.link.created") != 1 {
		t.Fatal("device declaration and link are not audited once")
	}

	// A repeated request (lost answer, expired capability) renews the session and writes nothing.
	status, renewed, _ := fx.session(t, "", again(t, body))
	if status != 200 || renewed.Storage != "UPLOADING" || renewed.Upload == nil || fx.count(t, events, fx.org, document, "document.upload.created") != 1 ||
		fx.count(t, `SELECT count(*) FROM org_document_links WHERE org_id=$1 AND document_id=$2`, fx.org, document) != 1 {
		t.Fatal("session renewal wrote twice or lost the capability", status)
	}
	capability := renewed.Upload.Capability

	// Mismatched, truncated, interrupted and foreign transfers leave UPLOADING at revision 1.
	altered := append([]byte(nil), fx.raw...)
	altered[len(altered)/2] ^= 0xff
	for name, attempt := range map[string]func() (int, string){
		"mismatched": func() (int, string) {
			s, _, c := fx.upload(t, fx.org, document, capability, bytes.NewReader(altered), int64(len(altered)))
			return s, c
		},
		"truncated": func() (int, string) {
			s, _, c := fx.upload(t, fx.org, document, capability, bytes.NewReader(fx.raw[:len(fx.raw)/2]), int64(len(fx.raw)))
			return s, c
		},
		"interrupted": func() (int, string) {
			s, _, c := fx.upload(t, fx.org, document, capability, &failingReader{data: fx.raw}, int64(len(fx.raw)))
			return s, c
		},
		"forged capability": func() (int, string) {
			s, _, c := fx.upload(t, fx.org, document, capability+"x", bytes.NewReader(fx.raw), int64(len(fx.raw)))
			return s, c
		},
		"other organization": func() (int, string) {
			s, _, c := fx.upload(t, onboardingID(t), document, capability, bytes.NewReader(fx.raw), int64(len(fx.raw)))
			return s, c
		},
		"other document": func() (int, string) {
			s, _, c := fx.upload(t, fx.org, onboardingID(t), capability, bytes.NewReader(fx.raw), int64(len(fx.raw)))
			return s, c
		},
	} {
		status, code := attempt()
		if status < 400 {
			t.Fatal("refused transfer was accepted", name, status, code)
		}
		if state, revision := fx.storage(t, document); state != "UPLOADING" || revision != 1 {
			t.Fatal("refused transfer changed the document", name, state, revision)
		}
	}

	// The verified original is STORED; only then does the session say so.
	status, stored, code := fx.upload(t, fx.org, document, capability, bytes.NewReader(fx.raw), int64(len(fx.raw)))
	if status != 200 {
		t.Fatal("verified upload refused", status, code)
	}
	if fx.count(t, events, fx.org, document, "document.upload.received") != 1 {
		t.Fatal("verified bytes were not received once")
	}
	state, revision := fx.storage(t, document)
	if state != "STORED" || revision != 3 || stored["document"].(map[string]any)["statuses"].(map[string]any)["storage"] != "STORED" {
		t.Fatal("verified upload did not reach STORED inline", state, revision)
	}
	var digest string
	var size int64
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT original_sha256, original_bytes FROM org_documents WHERE org_id=$1 AND id=$2`, fx.org, document).Scan(&digest, &size); err != nil || digest != body["sha256"] || size != int64(len(fx.raw)) {
		t.Fatal("stored original differs from the declaration")
	}
	status, settled, _ := fx.session(t, "", again(t, body))
	if status != 200 || settled.Storage != "STORED" || settled.StorageRevision != 3 || settled.Upload != nil {
		t.Fatal("a verified document still offers an upload", status)
	}
	// Duplicate completion is a replay that reads and writes nothing.
	status, replay, _ := fx.upload(t, fx.org, document, capability, bytes.NewReader(fx.raw), int64(len(fx.raw)))
	if status != 200 || replay["replayed"] != true || fx.count(t, events, fx.org, document, "document.upload.stored") != 1 || fx.count(t, events, fx.org, document, "document.upload.received") != 1 {
		t.Fatal("duplicate completion wrote again", status)
	}
}

// Wrong organization or record, reused identifiers, lost authority and
// invalid declarations are refused without a capability.
func TestSignedAttachmentSessionRefusesWrongRecordsAuthorityAndReuse(t *testing.T) {
	fx := attachmentHTTP(t)
	reading := fx.reading(t, "1000")
	documents := `SELECT count(*) FROM org_documents WHERE org_id=$1`
	links := `SELECT count(*) FROM org_document_links WHERE org_id=$1`

	// Another organization's route: the device has no enrollment there.
	if status, _, code := fx.session(t, onboardingID(t), fx.declaration(t, fx.raw, "METER_READING", reading)); status != 404 || code != "NOT_FOUND" {
		t.Fatal("foreign organization route served", status, code)
	}
	// The body must name the enrolled membership and enrollment.
	foreign := fx.declaration(t, fx.raw, "METER_READING", reading)
	foreign["enrollment_id"] = onboardingID(t)
	if status, _, code := fx.session(t, "", foreign); status != 404 || code != "NOT_FOUND" {
		t.Fatal("foreign enrollment in body accepted", status, code)
	}
	if fx.count(t, documents, fx.org) != 0 {
		t.Fatal("refused source declared a document")
	}
	// A missing record: NOT_FOUND, no link, no capability.
	missing := fx.declaration(t, fx.raw, "METER_READING", onboardingID(t))
	if status, _, code := fx.session(t, "", missing); status != 404 || code != "NOT_FOUND" {
		t.Fatal("missing record linked", status, code)
	}
	// A kind without a registered owner is refused by the DOC-003 registry
	// (FUELING has one since EMP-FUEL-011; EXPENSE still has none).
	if status, _, code := fx.session(t, "", fx.declaration(t, fx.raw, "EXPENSE", onboardingID(t))); status != 422 || code != "VALIDATION_FAILED" {
		t.Fatal("unregistered link kind accepted", status, code)
	}
	if fx.count(t, links, fx.org) != 0 {
		t.Fatal("a refused link was stored")
	}
	// Invalid declarations never reach the document owner.
	for _, edit := range []func(map[string]any){func(v map[string]any) { v["media_type"] = "image/png" }, func(v map[string]any) { v["bytes"] = 5*1024*1024 + 1 }, func(v map[string]any) { v["link_id"] = v["declaration_id"] }} {
		v := fx.declaration(t, fx.raw, "METER_READING", reading)
		edit(v)
		if status, _, _ := fx.session(t, "", v); status != 400 {
			t.Fatal("invalid declaration not refused as malformed", status)
		}
	}
	// Reused identifiers.
	body := fx.declaration(t, fx.raw, "METER_READING", reading)
	if status, _, _ := fx.session(t, "", body); status != 200 {
		t.Fatal("valid session refused", status)
	}
	if status, _, code := fx.session(t, "", again(t, body, func(v map[string]any) { v["sha256"] = strings.Repeat("ab", 32) })); status != 409 || code != "OPERATION_ID_REUSED" {
		t.Fatal("declaration reused with other facts", status, code)
	}
	if status, _, code := fx.session(t, "", again(t, body, func(v map[string]any) { v["declaration_id"] = onboardingID(t) })); status != 409 || code != "CONFLICT" {
		t.Fatal("document identifier taken by another declaration", status, code)
	}
	if status, _, code := fx.session(t, "", again(t, body, func(v map[string]any) { v["target_id"] = fx.meter.OpeningReadingID })); status != 409 || code != "OPERATION_ID_REUSED" {
		t.Fatal("link identifier reused with another target", status, code)
	}
	// Another member's record: a SELF driver may not document the manager's opening reading.
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.person.member}, "DRIVER", "SELF", "ACTIVE")
	before := fx.count(t, links, fx.org)
	if status, _, code := fx.session(t, "", fx.declaration(t, fx.raw, "METER_READING", fx.meter.OpeningReadingID)); status != 403 || code != "FORBIDDEN" {
		t.Fatal("another member's record linked", status, code)
	}
	// Its own reading stays linkable under SELF coverage.
	if status, own, _ := fx.session(t, "", fx.declaration(t, fx.raw, "METER_READING", reading)); status != 200 || own.Upload == nil {
		t.Fatal("own record refused under SELF coverage", status)
	}
	if fx.count(t, links, fx.org) != before+1 {
		t.Fatal("refused link counted")
	}
	// Lost authority: no attach permission, no session (the capability is never minted).
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.person.member}, "AUDITOR", "ORG", "ACTIVE")
	if status, answer, code := fx.session(t, "", again(t, body)); status != 403 || code != "FORBIDDEN" || answer.Upload != nil {
		t.Fatal("session served without doc.document.attach", status, code)
	}
}

// Concurrent identical requests of one device converge on one declaration and one link.
func TestSignedAttachmentSessionRacesConvergeOnOneDocumentAndLink(t *testing.T) {
	fx := attachmentHTTP(t)
	reading := fx.reading(t, "1000")
	body := fx.declaration(t, fx.raw, "METER_READING", reading)
	path := strings.Replace(syncweb.AttachmentsPath, "{org_id}", fx.org, 1)
	endpoint, err := url.Parse(fx.server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	// Signed before the barrier on the test goroutine; only transport runs concurrently.
	requests := make([]*http.Request, 6)
	for i := range requests {
		requests[i] = deviceSignedRequest(t, fx.registrar, fx.person.signer, path, again(t, body), time.Time{})
		requests[i].URL, requests[i].RequestURI, requests[i].Host = endpoint, "", deviceTestHost
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	statuses := make([]int, len(requests))
	for i := range requests {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			response, err := fx.server.Client().Do(requests[i])
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			statuses[i] = response.StatusCode
		}(i)
	}
	close(start)
	wg.Wait()
	for _, status := range statuses {
		if status != 200 {
			t.Fatal("racing identical session refused", statuses)
		}
	}
	document := body["document_id"].(string)
	if fx.count(t, `SELECT count(*) FROM org_documents WHERE org_id=$1 AND id=$2`, fx.org, document) != 1 ||
		fx.count(t, `SELECT count(*) FROM org_document_links WHERE org_id=$1 AND document_id=$2`, fx.org, document) != 1 ||
		fx.count(t, `SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND target_id=$2`, fx.org, document) != 2 {
		t.Fatal("racing sessions wrote more than one declaration or link")
	}
}
