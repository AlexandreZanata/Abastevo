package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestDeviceWireIntentRejectsAuthorityAndMissingFields(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"family_id":null,"access_token":"synthetic"}`,
		`{"family_id":"synthetic","access_token":"synthetic","key_id":"untrusted"}`,
		`{"Family_id":"synthetic","access_token":"synthetic"}`,
	} {
		var v deviceCheckRequest
		if json.Unmarshal([]byte(raw), &v) == nil {
			t.Fatal("invalid signed device intent accepted")
		}
	}
	const valid = `{"family_id":"10000000-0000-4000-8000-000000000001","access_token":"synthetic","schema_version":1,"operation_id":"10000000-0000-4000-8000-000000000002","label":"Synthetic work phone"}`
	for _, extra := range []string{`,"proof_confirmed":true`, `,"device_limit":5`, `,"account_id":"untrusted"`, `,"device_id":"untrusted"`, `,"family_id":null`} {
		var v deviceEnrollRequest
		if json.Unmarshal([]byte(strings.TrimSuffix(valid, "}")+extra+"}"), &v) == nil {
			t.Fatal("client device authority accepted")
		}
	}
	var v deviceEnrollRequest
	if json.Unmarshal([]byte(valid), &v) != nil || v.Label != "Synthetic work phone" {
		t.Fatal("canonical enrollment intent refused")
	}
	for _, raw := range []string{`{}`, `{"schema_version":1,"operation_id":"synthetic","revision":null,"revocation_class":"ACCESS_ENDED","reason":"Synthetic ending"}`, `{"schema_version":1,"operation_id":"synthetic","revision":1,"revocation_class":"ACCESS_ENDED","reason":"Synthetic ending","account_id":"untrusted"}`} {
		var v deviceRevokeRequest
		if json.Unmarshal([]byte(raw), &v) == nil {
			t.Fatal("invalid revoke intent accepted")
		}
	}
}

func TestDeviceViewOmitsPersonalProofAndInventedSync(t *testing.T) {
	org, _ := kernel.ParseOrgID("10000000-0000-4000-8000-000000000001")
	scope, _ := kernel.NewOrgScope(org)
	state := d.DeviceSnapshot{OrgID: org, ID: "10000000-0000-4000-8000-000000000002", MembershipID: "10000000-0000-4000-8000-000000000003", DeviceID: "10000000-0000-4000-8000-000000000004", Fingerprint: "fp:" + strings.Repeat("a", 64), State: d.DeviceEnrolled, Revision: 1}
	enrollment, err := d.RehydrateDevice(scope, state)
	if err != nil {
		t.Fatal(err)
	}
	record := app.DeviceRecord{Enrollment: enrollment, Label: "Synthetic work phone", EnrolledAt: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)}
	view, err := deviceView(scope, record)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(view)
	if view["last_sync_at"] != (*time.Time)(nil) || strings.Contains(string(raw), state.DeviceID) || strings.Contains(string(raw), state.Fingerprint) {
		t.Fatal("private key or invented sync metadata exposed")
	}
	for _, field := range []string{"account_id", "family_id", "session_id", "access_token", "contributor_id", "key_id", "fingerprint"} {
		if _, exists := view[field]; exists {
			t.Fatal("personal authority field exposed", field)
		}
	}
	forgedSync := record.EnrolledAt
	record.LastSyncAt = &forgedSync
	if _, err := deviceView(scope, record); err == nil {
		t.Fatal("uninstalled sync producer accepted")
	}
	result := app.DeviceResult{Device: record, OperationID: "10000000-0000-4000-8000-000000000005"}
	if _, err := deviceResult(scope, app.DeviceCommand{OperationID: result.OperationID, Label: "Synthetic work phone"}, result); err == nil {
		t.Fatal("invalid repository acknowledgement accepted")
	}
}

func TestSignedDevicePlaneRefusesCookieFallbackAndForeignAuthority(t *testing.T) {
	for _, edit := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Cookie", "__Host-empresas-session=synthetic") },
		func(r *http.Request) { r.Header.Set("Authorization", "Bearer synthetic") },
		func(r *http.Request) { r.Header.Set("Origin", "https://foreign.example.invalid") },
		func(r *http.Request) { r.Host = "foreign.example.invalid" },
	} {
		request := httptest.NewRequest(http.MethodPost, "https://company.example.invalid/v1/orgs/10000000-0000-4000-8000-000000000001/devices/check", strings.NewReader(`{"family_id":"10000000-0000-4000-8000-000000000002","access_token":"synthetic"}`))
		request.Header.Set("Content-Type", "application/json")
		edit(request)
		response := httptest.NewRecorder()
		endpoint := signedDeviceEndpoint{gateway: orgapi.Gateway{Enabled: true, Origin: "https://company.example.invalid"}, action: "check"}
		endpoint.ServeHTTP(response, request)
		if response.Code != 401 || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("ETag") != "" {
			t.Fatal("alternate signed device authority accepted", response.Code)
		}
	}
}
