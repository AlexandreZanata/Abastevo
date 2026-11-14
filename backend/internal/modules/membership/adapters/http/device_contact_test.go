package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
)

func TestDeviceContactWireRejectsPolicyClaimsAndUnboundResults(t *testing.T) {
	const raw = `{"schema_version":1,"request_id":"10000000-0000-4000-8000-000000000001","family_id":"10000000-0000-4000-8000-000000000002","access_token":"synthetic-contact","membership_id":"10000000-0000-4000-8000-000000000003","enrollment_id":"10000000-0000-4000-8000-000000000004"}`
	var v deviceContactRequest
	if json.Unmarshal([]byte(raw), &v) != nil || v.selector().Validate() != nil {
		t.Fatal("canonical contact refused")
	}
	for _, extra := range []string{`,"account_id":"untrusted"`, `,"offline_validity_seconds":2592000`, `,"managed_unlock_seconds":300`, `,"revocation_class":"LOST_OR_COMPROMISED"`, `,"request_id":null`} {
		if json.Unmarshal([]byte(strings.TrimSuffix(raw, "}")+extra+"}"), &v) == nil {
			t.Fatal("client custody policy accepted")
		}
	}
	a := app.DeviceActor{AccountID: "10000000-0000-4000-8000-000000000005", FamilyID: v.FamilyID, ContributorID: "10000000-0000-4000-8000-000000000006", KeyID: "10000000-0000-4000-8000-000000000007", AccessToken: "synthetic", Fingerprint: "fp:" + strings.Repeat("a", 64), ProofExpires: time.Now().Add(time.Minute)}
	if json.Unmarshal([]byte(raw), &v) != nil {
		t.Fatal("reset fixture")
	}
	a.FamilyID = v.FamilyID
	r := v.selector()
	result := app.DeviceContact{RequestID: r.RequestID, AccountID: a.AccountID, FamilyID: a.FamilyID, MembershipID: r.MembershipID, EnrollmentID: r.EnrollmentID, Revision: 1, ServerTime: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC), Verdict: app.ContactActive, AccessModel: d.ExistingAccount, ValiditySeconds: 2592000}
	view, e := deviceContactView("https://company.example.invalid", "10000000-0000-4000-8000-000000000008", a, r, result)
	if e != nil || view["enrollment_revision"] != "1" || view["managed_unlock_seconds"] != nil || view["access_model"] != d.ExistingAccount {
		t.Fatal("active wire", e)
	}
	result.Verdict, result.RevocationClass, result.AccessModel, result.ValiditySeconds = app.ContactLock, d.AccessEnded, "", 0
	view, e = deviceContactView("https://company.example.invalid", "10000000-0000-4000-8000-000000000008", a, r, result)
	if e != nil {
		t.Fatal(e)
	}
	for _, key := range []string{"offline_validity_seconds", "managed_unlock_seconds", "access_model", "label", "key_id", "access_token"} {
		if _, ok := view[key]; ok {
			t.Fatal("terminal disclosed lease/proof", key)
		}
	}
	result.EnrollmentID = a.KeyID
	if _, e := deviceContactView("https://company.example.invalid", "10000000-0000-4000-8000-000000000008", a, r, result); e == nil {
		t.Fatal("unbound repository contact published")
	}
}
