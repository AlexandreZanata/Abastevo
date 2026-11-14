package http

import (
	"net/http"
	"strconv"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

type deviceContactRequest struct {
	privateDTO
	SchemaVersion int    `json:"schema_version"`
	RequestID     string `json:"request_id"`
	FamilyID      string `json:"family_id"`
	AccessToken   string `json:"access_token"`
	MembershipID  string `json:"membership_id"`
	EnrollmentID  string `json:"enrollment_id"`
}

func (v *deviceContactRequest) UnmarshalJSON(raw []byte) error {
	type plain deviceContactRequest
	return decodeRedemption(raw, (*plain)(v), "schema_version", "request_id", "family_id", "access_token", "membership_id", "enrollment_id")
}
func (v deviceContactRequest) selector() app.DeviceContactRequest {
	return app.DeviceContactRequest{RequestID: v.RequestID, MembershipID: v.MembershipID, EnrollmentID: v.EnrollmentID}
}

// The wrapper deliberately does not implement SignedDeviceDescriptor: terminal
// custody is an exact owned observation, not an active-enrollment business read.
type deviceContactEndpoint struct{ endpoint signedDeviceEndpoint }

func (e deviceContactEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.endpoint.ServeHTTP(w, r)
}
func (deviceContactEndpoint) DeviceContactRule() orgapi.DeviceContactRule {
	return orgapi.DeviceContactRule{AccountProof: true, Signed: true, ExactEnrollment: true, TerminalRead: true, PolicyLease: true, Success: 200}
}

func deviceContactView(origin, organization string, a app.DeviceActor, r app.DeviceContactRequest, v app.DeviceContact) (map[string]any, error) {
	if v.Validate(a, r) != nil {
		return nil, orgapi.Unavailable
	}
	out := map[string]any{"schema_version": 1, "request_id": v.RequestID, "account_id": v.AccountID, "api_origin": origin, "org_id": organization, "membership_id": v.MembershipID, "enrollment_id": v.EnrollmentID, "family_id": v.FamilyID, "enrollment_revision": strconv.FormatInt(v.Revision, 10), "server_time": orgtime.FormatInstant(v.ServerTime), "verdict": v.Verdict}
	if v.Verdict == app.ContactActive {
		out["access_model"], out["offline_validity_seconds"], out["managed_unlock_seconds"] = v.AccessModel, v.ValiditySeconds, nil
	} else {
		out["revocation_class"] = v.RevocationClass
	}
	return out, nil
}
