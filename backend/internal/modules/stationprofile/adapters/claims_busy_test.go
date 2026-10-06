package adapters

import (
	"net/http"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
)

func TestClaimBusyRetryDuringPublish(t *testing.T) {
	store := &fakeClaimStore{claims: map[string]application.ClaimRow{}}
	// Forge the publish window: claim row exists but its first
	// declaration is not published yet.
	store.claims["claim-1"] = application.ClaimRow{
		ID: "claim-1", AccountID: "acc-1", StationID: "d6c74c23-63db-4c24-a2e5-408cb23bad26",
		OperatorCNPJ: "04218406000104", OperatorSource: "registry",
		Role: "administrator", Scopes: []string{"profile.edit"},
		PolicyVersion: "profile-v1", State: "draft", ClientKey: "key-busy",
	}
	h := claimTestHandler(store)
	res := doClaimRequest(h, http.MethodPost, "/v1/stations/d6c74c23-63db-4c24-a2e5-408cb23bad26/claims", openBody("key-busy"))
	if res.Code != http.StatusConflict {
		t.Fatalf("busy = %d, want 409 (retryable, never a misleading 404)", res.Code)
	}
	if body := res.Body.String(); !strings.Contains(body, "claim.busy-retry") {
		t.Fatalf("busy body must carry claim.busy-retry: %s", body)
	}
}
