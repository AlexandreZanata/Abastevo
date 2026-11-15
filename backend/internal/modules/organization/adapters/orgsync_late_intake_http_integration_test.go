//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	syncweb "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/adapters/http"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/jackc/pgx/v5"
)

// lateHTTPPost signs one batch (or a review-results request) for the fixture's
// person and returns the status, the bounded body and the error code.
func lateHTTPPost(t *testing.T, fx meterIntakeHTTPFixture, path string, body any) (int, []byte, string) {
	t.Helper()
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
	raw, err := io.ReadAll(io.LimitReader(response.Body, 131073))
	if err != nil || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("signed late response unreadable or cacheable")
	}
	for _, private := range []string{fx.person.auth.Session.AccessToken, fx.person.auth.Session.FamilyID, fx.person.signer.keyID, fx.person.signer.fingerprint, fx.person.signer.contributor} {
		if strings.Contains(string(raw), private) {
			t.Fatal("signed late response disclosed proof material")
		}
	}
	var wire struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &wire)
	return response.StatusCode, raw, wire.Error.Code
}

func lateHTTPBatch(t *testing.T, fx meterIntakeHTTPFixture, want int, ops ...protocol.Operation) ([]protocol.Acknowledgement, []byte, string) {
	t.Helper()
	raw := make([]json.RawMessage, len(ops))
	for i, op := range ops {
		raw[i] = op.CanonicalBytes()
	}
	before := ""
	if want >= 400 {
		before = matrixPrivateState(t, fx.deviceFixtureState)
	}
	status, body, code := lateHTTPPost(t, fx, fx.path, syncHTTPBody(fx.deviceFixtureState, raw...))
	if status != want {
		t.Fatalf("signed late batch got%d want%d (%s)", status, want, code)
	}
	if want >= 400 {
		if before != matrixPrivateState(t, fx.deviceFixtureState) {
			t.Fatal("refused late batch changed private state")
		}
		return nil, body, code
	}
	var result struct {
		Version          int               `json:"schema_version"`
		Acknowledgements []json.RawMessage `json:"acknowledgements"`
	}
	if json.Unmarshal(body, &result) != nil || result.Version != 1 || len(result.Acknowledgements) != len(ops) {
		t.Fatal("late batch returned an invalid bounded response")
	}
	acks := make([]protocol.Acknowledgement, len(ops))
	for i, op := range ops {
		ack, err := protocol.ParseAcknowledgement(result.Acknowledgements[i], op)
		if err != nil || !bytes.Equal(ack.CanonicalBytes(), result.Acknowledgements[i]) {
			t.Fatal("late response lost the canonical acknowledgement", err)
		}
		acks[i] = ack
	}
	return acks, body, ""
}

func (fx meterIntakeHTTPFixture) lateReading(t *testing.T, declared time.Time) protocol.Operation {
	t.Helper()
	return fx.operation(t, "7", func(raw map[string]any) { raw["occurred_at"] = orgtime.FormatInstant(declared) })
}

func (fx meterIntakeHTTPFixture) lateFueling(t *testing.T, declared time.Time) protocol.Operation {
	t.Helper()
	return syncHTTPOperation(t, fx.deviceFixtureState, fx.source.EnrollmentID, func(raw map[string]any) {
		raw["type"] = "fueling.submit"
		raw["entity"] = map[string]any{"type": "fueling", "id": onboardingID(t), "expected_revision": nil}
		raw["occurred_at"] = orgtime.FormatInstant(declared)
		raw["payload"] = map[string]any{"asset_id": fx.asset}
	})
}

func (fx meterIntakeHTTPFixture) endPeriod(t *testing.T) time.Time {
	t.Helper()
	fx.f.GrantLateIntake(t)
	// The held-review owner's own fixture grants (as meterReviewStore).
	if _, err := fx.f.Admin.Exec(t.Context(), "GRANT SELECT,INSERT ON org_sync_review_items,org_meter_held_readings,org_sync_review_resolutions TO "+pgx.Identifier{fx.f.RuntimeRole}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	if err := lateEndMembership(t.Context(), fx.syncFixtureState); err != nil {
		t.Fatal(err)
	}
	var ended time.Time
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT ended_at FROM org_memberships WHERE org_id=$1 AND id=$2 AND state='ENDED'`, fx.scope.OrgID().String(), fx.source.MembershipID).Scan(&ended); err != nil {
		t.Fatal(err)
	}
	return ended.UTC()
}

// UC-OFFL-07 through the actual signed route and default runtime composition:
// after offboarding the device delivers intake-only, resends replay, owners
// without a held path stay unavailable, and nothing is readable.
func TestLateIntakeSignedRouteHoldsAndRefusesThroughTheDefaultRuntime(t *testing.T) {
	fx := meterIntakeHTTP(t)
	accepted := fx.operation(t, "7", nil)
	if ack := fx.dispatch(t, accepted, 200); ack.Result() != protocol.Accepted {
		t.Fatal("current reading not accepted before the end")
	}
	ended := fx.endPeriod(t)
	// No operations means no sealed period: the ended device stays refused.
	if _, _, code := lateHTTPBatch(t, fx, 404); code != "NOT_FOUND" {
		t.Fatal("an ended device without a sealed period was admitted", code)
	}
	pre := fx.lateReading(t, ended.Add(-30*time.Second))
	after := fx.lateReading(t, ended.Add(time.Second))
	fuelingPre := fx.lateFueling(t, ended.Add(-30*time.Second))
	fuelingAfter := fx.lateFueling(t, ended.Add(time.Second))
	acks, body, _ := lateHTTPBatch(t, fx, 200, pre, after, fuelingPre, fuelingAfter, accepted)
	if acks[0].Result() != protocol.AcceptedForReview || acks[0].Code() != syncapp.LateHeldCode {
		t.Fatal("pre-end reading was not quarantined", acks[0].Result(), acks[0].Code())
	}
	if acks[1].Result() != protocol.Rejected || acks[1].Code() != syncapp.LateMembershipEndedCode {
		t.Fatal("post-end reading was not refused", acks[1].Code())
	}
	if acks[2].Result() != protocol.Retry || acks[2].Code() != "UNAVAILABLE" {
		t.Fatal("a fueling without a held owner was not retained", acks[2].Result(), acks[2].Code())
	}
	if acks[3].Result() != protocol.Rejected || acks[3].Code() != syncapp.LateMembershipEndedCode {
		t.Fatal("a post-end fueling was not refused", acks[3].Code())
	}
	if acks[4].Result() != protocol.Retry || acks[4].Code() != "UNAVAILABLE" {
		t.Fatal("a normal pre-end receipt was reopened", acks[4].Result(), acks[4].Code())
	}
	var claimed bool
	var intakeReadings, held, decisions, fuelings int
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM org_sync_operations WHERE org_id=$1 AND operation_id=$2),
 (SELECT count(*) FROM org_meter_readings r WHERE r.org_id=$1 AND EXISTS(SELECT 1 FROM org_sync_operations o WHERE o.org_id=r.org_id AND o.operation_id=r.operation_id)),
 (SELECT count(*) FROM org_meter_held_readings WHERE org_id=$1),(SELECT count(*) FROM org_sync_late_intakes WHERE org_id=$1),(SELECT count(*) FROM org_fuelings WHERE org_id=$1)`, fx.scope.OrgID().String(), fuelingPre.ID()).Scan(&claimed, &intakeReadings, &held, &decisions, &fuelings); err != nil || claimed || intakeReadings != 1 || held != 1 || decisions != 3 || fuelings != 0 {
		t.Fatal("late route stored an unexpected fact or claim", err, claimed, intakeReadings, held, decisions, fuelings)
	}
	_, again, _ := lateHTTPBatch(t, fx, 200, pre, after, fuelingPre, fuelingAfter, accepted)
	if !bytes.Equal(body, again) {
		t.Fatal("late resend did not replay every acknowledgement byte for byte")
	}

	// Intake-only access gives no reading right: the signed review-results read
	// of the ended period stays refused, before and after the late hold.
	request, err := protocol.NewReviewResultsRequest(protocol.ReviewResultsRequestSnapshot{RequestID: onboardingID(t), MembershipID: fx.source.MembershipID, EnrollmentID: fx.source.EnrollmentID, Proof: protocol.DeliveryProof{FamilyID: fx.person.auth.Session.FamilyID, AccessToken: fx.person.auth.Session.AccessToken}, Selectors: []protocol.ReviewSelector{{OperationID: pre.ID(), PayloadDigest: pre.PayloadDigest()}}})
	if err != nil {
		t.Fatal(err)
	}
	path := strings.Replace(syncweb.ReviewResultsPath, "{org_id}", fx.scope.OrgID().String(), 1)
	if status, raw, _ := lateHTTPPost(t, fx, path, json.RawMessage(request.CanonicalBytes())); status < 400 || bytes.Contains(raw, []byte(acks[0].Snapshot().ReviewItemID)) {
		t.Fatal("an ended member read a review result", status)
	}
}

// A device revoked as lost or compromised is refused as a whole on the signed
// route, before any claim, receipt, audit or fact (BR-OFFL-012).
func TestLateIntakeSignedRouteRefusesALostDeviceAsAWhole(t *testing.T) {
	fx := meterIntakeHTTP(t)
	record, err := fx.devices.CheckDevice(t.Context(), fx.scope, fx.source.Actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.devices.RevokeDevice(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, fx.person.member, false, deviceRevokeCommand(t, fx.scope, record, d.LostOrCompromised)); err != nil {
		t.Fatal(err)
	}
	ended := fx.endPeriod(t)
	pre := fx.lateReading(t, ended.Add(-30*time.Second))
	if _, _, code := lateHTTPBatch(t, fx, 403, pre); code != "DEVICE_REVOKED" {
		t.Fatal("lost device not refused as a whole", code)
	}
	syncHTTPAssertNoIntake(t, fx.deviceFixtureState)
}
