package application

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
)

func TestDeviceContactIsExactAndNeverPromotesTerminalStateToLease(t *testing.T) {
	a := deviceTestActor()
	r := DeviceContactRequest{RequestID: deviceTestID, MembershipID: deviceTestMember, EnrollmentID: deviceTestEnrollment}
	v := DeviceContact{RequestID: r.RequestID, AccountID: a.AccountID, FamilyID: a.FamilyID, MembershipID: r.MembershipID, EnrollmentID: r.EnrollmentID, Revision: 1, ServerTime: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC), Verdict: ContactActive, AccessModel: d.ExistingAccount, ValiditySeconds: 2592000}
	if r.Validate() != nil || v.Validate(a, r) != nil {
		t.Fatal("exact current contact refused")
	}
	for _, change := range []func(*DeviceContact){
		func(v *DeviceContact) { v.RequestID = deviceTestEnrollment },
		func(v *DeviceContact) { v.AccountID = deviceTestEnrollment },
		func(v *DeviceContact) { v.FamilyID = deviceTestEnrollment },
		func(v *DeviceContact) { v.MembershipID = deviceTestEnrollment },
		func(v *DeviceContact) { v.EnrollmentID = deviceTestMember },
		func(v *DeviceContact) { v.Revision = 0 },
		func(v *DeviceContact) { v.ServerTime = time.Time{} },
		func(v *DeviceContact) { v.ValiditySeconds = 900 },
		func(v *DeviceContact) { v.AccessModel = d.ManagedAccess },
		func(v *DeviceContact) { v.RevocationClass = d.AccessEnded },
		func(v *DeviceContact) { v.Verdict = "UNKNOWN" },
	} {
		bad := v
		change(&bad)
		if bad.Validate(a, r) == nil {
			t.Fatal("invalid contact accepted")
		}
	}
	for _, terminal := range []struct {
		verdict ContactVerdict
		class   d.RevocationClass
	}{{ContactLock, d.AccessEnded}, {ContactErase, d.LostOrCompromised}} {
		x := v
		x.Verdict, x.RevocationClass, x.AccessModel, x.ValiditySeconds = terminal.verdict, terminal.class, "", 0
		if x.Validate(a, r) != nil {
			t.Fatal("exact terminal refused")
		}
		x.ValiditySeconds = 2592000
		if x.Validate(a, r) == nil {
			t.Fatal("terminal contact granted lease")
		}
		x.ValiditySeconds, x.RevocationClass = 0, ""
		if x.Validate(a, r) == nil {
			t.Fatal("unclassified terminal accepted")
		}
	}
	for _, x := range []any{a, r, v} {
		raw, _ := json.Marshal(x)
		for _, diagnostic := range []string{fmt.Sprintf("%v %+v %#v", x, x, x), string(raw)} {
			if strings.Contains(diagnostic, a.FamilyID) || strings.Contains(diagnostic, r.RequestID) {
				t.Fatal("contact diagnostic leaked identity")
			}
		}
	}
}
