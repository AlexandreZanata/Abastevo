package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestCurrentSourceRegisteredRefusalsRemainSafeStaticTransportCodes(t *testing.T) {
	for _, code := range []orgapi.Code{orgapi.Forbidden, orgapi.NotFound, orgapi.MembershipSuspended, orgapi.FreshAuthRequired} {
		err := app.SourceDenied(fmt.Errorf("PRIVATE-SOURCE-DETAIL: %w", code))
		if got := mapped(err); got != code {
			t.Fatalf("registered refusal changed: got%v want%v", got, code)
		}
		if strings.Contains(mapped(err).Error(), "PRIVATE") {
			t.Fatal("wrapped source text exposed")
		}
	}
	if got := mapped(app.SourceDenied(orgapi.MissingPermission("idn.employee.manage"))); got != orgapi.Forbidden {
		t.Fatal("sourcepermission details escaped static mapping", got)
	}
}

type changingDevice struct {
	record member.DeviceRecord
	err    error
}

func (s *changingDevice) CheckDevice(context.Context, kernel.OrgScope, member.DeviceActor) (member.DeviceRecord, error) {
	return s.record, s.err
}

func TestFinalBatchSourceCannotFollowReplacementEnrollmentOrRehire(t *testing.T) {
	const account = "7b0e2a54-0000-4000-8000-000000000001"
	const memberID = "7b0e2a54-0000-4000-8000-000000000002"
	const enrollment = "7b0e2a54-0000-4000-8000-000000000003"
	const replacement = "7b0e2a54-0000-4000-8000-000000000004"
	id, _ := kernel.ParseOrgID(account)
	scope, _ := kernel.NewOrgScope(id)
	actor := member.DeviceActor{AccountID: account, FamilyID: memberID, ContributorID: account, KeyID: replacement, Fingerprint: "fp:" + strings.Repeat("a", 64), AccessToken: "synthetic", ProofExpires: time.Now().Add(time.Minute)}
	source := app.Source{Scope: scope, Actor: actor, Origin: "https://company.example.invalid", MembershipID: memberID, EnrollmentID: enrollment}
	state := d.DeviceSnapshot{OrgID: id, ID: enrollment, MembershipID: memberID, DeviceID: actor.KeyID, Fingerprint: actor.Fingerprint, State: d.DeviceEnrolled, Revision: 1}
	device := &changingDevice{}
	set := func() {
		var e error
		device.record.Enrollment, e = d.RehydrateDevice(scope, state)
		if e != nil {
			t.Fatal(e)
		}
		device.record.Label = "Synthetic"
		device.record.EnrolledAt = time.Now().UTC()
	}
	set()
	if e := confirmCurrentDevice(t.Context(), device, source); e != nil {
		t.Fatal("current proof refused", e)
	}
	for _, change := range []func(){func() { state.ID = replacement }, func() { state.ID = enrollment; state.MembershipID = replacement }} {
		change()
		set()
		if e := confirmCurrentDevice(t.Context(), device, source); e == nil {
			t.Fatal("buffered ACK followed a replacementperiod")
		}
	}
	device.err = member.ErrDeviceRevoked
	if e := confirmCurrentDevice(t.Context(), device, source); e != member.ErrDeviceRevoked {
		t.Fatal("source terminal denial lost", e)
	}
}

func TestSignedBatchRejectsAlternateAuthorityAndMalformedWholeBody(t *testing.T) {
	const body = `{"schema_version":1,"family_id":"7b0e2a54-0000-4000-8000-000000000001","access_token":"synthetic-batch-proof","operations":[]}`
	for _, edit := range []func(*http.Request){func(r *http.Request) { r.Header.Set("Cookie", "synthetic") }, func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") }, func(r *http.Request) { r.Header.Set("X-CSRF", "synthetic") }, func(r *http.Request) { r.Header.Set("Signature-Input", "synthetic") }, func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") }, func(r *http.Request) { r.Host = "foreign.example.invalid" }} {
		r := httptest.NewRequest(http.MethodPost, "https://company.example.invalid/v1/orgs/7b0e2a54-0000-4000-8000-000000000001/sync/operations", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		edit(r)
		w := httptest.NewRecorder()
		(endpoint{Options{Enabled: true, Origin: "https://company.example.invalid", Limits: protocol.DefaultLimits()}}).ServeHTTP(w, r)
		if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatal("alternate batch authority", w.Code)
		}
	}
	for _, raw := range []string{`{}`, strings.TrimSuffix(body, "}") + `,"schema_version":1}`, strings.Repeat(" ", 65537) + body, strings.Replace(body, `"operations":[]`, `"operations":[null,]`, 1)} {
		r := httptest.NewRequest(http.MethodPost, "https://company.example.invalid/v1/orgs/7b0e2a54-0000-4000-8000-000000000001/sync/operations", strings.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		(endpoint{Options{Enabled: true, Origin: "https://company.example.invalid", Limits: protocol.DefaultLimits()}}).ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("malformed batch reached proof/source", w.Code)
		}
	}
}
