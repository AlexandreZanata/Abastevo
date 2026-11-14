//go:build integration

package adapters_test

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func TestMeterReviewActualCompanyHTTPKeepsOriginalAckAndOneDecision(t *testing.T) {
	fx, _, installed := meterEvidenceFixture(t)
	store := meterReviewStore(t, fx)
	observed := time.Now().UTC().Add(-time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	op, held, ack, err := storeMeterHeldReview(t, fx, store, meterObservedCommand(t, installed, observed, 7))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT,UPDATE ON identity_rate_windows TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{61}, 32), Pool: fx.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal("real review runtime composition", err)
	}
	reviewer := matrixNewPerson(t, fx.deviceFixtureState, router)
	matrixAssign(t, fx.deviceFixtureState, reviewer, "ADMIN", "ORG", "ACTIVE")
	base := "/v1/orgs/" + fx.scope.OrgID().String() + "/sync/reviews/meter-readings"
	call := func(path string, body any, want int, code string, edit func(*http.Request)) map[string]any {
		return matrixCall(t, fx.deviceFixtureState, router, reviewer, path, body, want, code, "", edit)
	}
	page := call(base+"/list", map[string]any{"limit": 1, "state": "OPEN"}, 200, "", nil)
	if len(page["items"].([]any)) != 1 || page["items"].([]any)[0].(map[string]any)["review_item_id"] != held.ReviewItemID {
		t.Fatal("actual held queue missing")
	}
	path := base + "/" + held.ReviewItemID
	view := call(path+"/read", map[string]any{}, 200, "", nil)
	if view["resolution"] != nil || view["state"] != "OPEN" || view["fact"].(map[string]any)["occurred_at"] != orgtime.FormatInstant(observed) || view["fact"].(map[string]any)["value"] != "7" {
		t.Fatal("held fact became accepted or changed")
	}
	decisionID := onboardingID(t)
	body := map[string]any{"schema_version": 1, "operation_id": decisionID, "revision": 1, "decision": "ACCEPT", "reason": ""}
	call(path+"/resolve", body, 401, "UNAUTHENTICATED", func(r *http.Request) { r.Header.Del("X-CSRF") })
	accepted := call(path+"/resolve", body, 200, "", nil)
	resolution := accepted["resolution"].(map[string]any)
	if accepted["replayed"] != false || resolution["outcome"] != "ACCEPTED" || resolution["operation_id"] != op.ID() || resolution["review_item_id"] != held.ReviewItemID {
		t.Fatal("actual decision metadata changed")
	}
	replay := call(path+"/resolve", body, 200, "", nil)
	if replay["replayed"] != true {
		t.Fatal("normal HTTP retry did not recover original decision")
	}
	body["reason"] = "Changed immutable decision"
	call(path+"/resolve", body, 409, "OPERATION_ID_REUSED", nil)
	body["operation_id"] = onboardingID(t)
	body["reason"] = ""
	call(path+"/resolve", body, 409, "CONFLICT", nil)
	call(base+"/"+onboardingID(t)+"/read", map[string]any{}, 404, "NOT_FOUND", nil)
	var intact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT canonical_ack=$3 AND result='ACCEPTED_FOR_REVIEW' AND(SELECT count(*) FROM org_sync_review_resolutions WHERE org_id=$1 AND review_item_id=$4)=1 AND(SELECT count(*) FROM org_sync_effects WHERE org_id=$1 AND operation_id=$2)=1 FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2`, fx.scope.OrgID().String(), op.ID(), ack.CanonicalBytes(), held.ReviewItemID).Scan(&intact); err != nil || !intact {
		t.Fatal("company HTTP rewrote original ACK or duplicated decision", err)
	}
}
