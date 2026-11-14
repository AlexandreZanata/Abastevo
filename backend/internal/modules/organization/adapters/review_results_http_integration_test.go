//go:build integration

package adapters_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	accountpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	fleetpg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	member "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	memberdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/enterprisesuite"
	"github.com/go-chi/chi/v5"
)

type ownReviewTLS struct {
	syncFixtureState
	server       *httptest.Server
	reviews      *fleetpg.MeterReviewStore
	operations   []protocol.Operation
	references   []app.ReviewReference
	originalACKs []protocol.Acknowledgement
	path         string
}

func ownReviewTLSFixture(t *testing.T, count int) ownReviewTLS {
	t.Helper()
	fx, _, installed := meterEvidenceFixture(t)
	reviews := meterReviewStore(t, fx)
	result := ownReviewTLS{syncFixtureState: fx, reviews: reviews, path: "/v1/orgs/" + fx.scope.OrgID().String() + "/sync/review-results"}
	for i := range count {
		// Actual source-owned held facts/proof/ACK, not a fake authority or guessed
		// historical classification. Separate disposition tests prove admission.
		op, ref, ack, err := storeMeterHeldReview(t, fx, reviews, meterObservedCommand(t, installed, time.Now().UTC().Add(-time.Hour-time.Duration(i)*time.Minute), 7))
		if err != nil {
			t.Fatal("actual typed held source fixture", err)
		}
		result.operations = append(result.operations, op)
		result.references = append(result.references, ref)
		result.originalACKs = append(result.originalACKs, ack)
	}
	accounts := *fx.accounts
	accounts.Store = accountpg.NewPGStore(fx.f.Runtime)
	accounts.Clock = syncHTTPClock{}
	router := chi.NewRouter()
	if err := enterprisesuite.Register(router, enterprisesuite.Options{Enabled: true, Origin: deviceTestOrigin, CursorSecret: bytes.Repeat([]byte{95}, 32), Pool: fx.f.Runtime, Accounts: &accounts}); err != nil {
		t.Fatal("actual default enterprise runtime", err)
	}
	result.server = httptest.NewTLSServer(router)
	t.Cleanup(result.server.Close)
	result.server.Client().Timeout = 8 * time.Second
	return result
}
func (fx ownReviewTLS) request(t *testing.T, selectors []protocol.ReviewSelector) protocol.ReviewResultsRequest {
	t.Helper()
	v, err := protocol.NewReviewResultsRequest(protocol.ReviewResultsRequestSnapshot{RequestID: onboardingID(t), MembershipID: fx.source.MembershipID, EnrollmentID: fx.source.EnrollmentID, Proof: protocol.DeliveryProof{FamilyID: fx.person.auth.Session.FamilyID, AccessToken: fx.person.auth.Session.AccessToken}, Selectors: selectors})
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func (fx ownReviewTLS) query(t *testing.T, request protocol.ReviewResultsRequest, expected []protocol.ReviewExpectation, want int, edit func(*http.Request)) protocol.ReviewResultsResponse {
	t.Helper()
	r := deviceSignedRequest(t, fx.registrar, fx.person.signer, fx.path, json.RawMessage(request.CanonicalBytes()), time.Time{})
	endpoint, err := url.Parse(fx.server.URL + fx.path)
	if err != nil {
		t.Fatal(err)
	}
	r.URL = endpoint
	r.RequestURI = ""
	r.Host = deviceTestHost
	if edit != nil {
		edit(r)
	}
	before := ""
	if want >= 400 {
		before = matrixPrivateState(t, fx.deviceFixtureState)
	}
	response, err := fx.server.Client().Do(r)
	if err != nil {
		t.Fatal("actual local signed TLS own-result request")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, protocol.MaxReviewResultsResponseBytes+1))
	if err != nil || len(body) > protocol.MaxReviewResultsResponseBytes || response.StatusCode != want || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("ETag") != "" {
		t.Fatalf("signed own-results got%d want%d", response.StatusCode, want)
	}
	for _, proof := range []string{fx.person.auth.Session.AccessToken, fx.person.auth.Session.FamilyID, fx.person.signer.keyID, fx.person.signer.fingerprint, fx.person.signer.contributor} {
		if strings.Contains(string(body), proof) {
			t.Fatal("minimal outcome leaked proof material")
		}
	}
	if want >= 400 {
		if matrixPrivateState(t, fx.deviceFixtureState) != before {
			t.Fatal("refused own-result read changed company state")
		}
		var wire struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
			Results []json.RawMessage `json:"results"`
		}
		if json.Unmarshal(body, &wire) != nil || wire.Error.Code == "" || len(wire.Results) != 0 {
			t.Fatal("foreign/source suffix exposed buffered result prefix")
		}
		return protocol.ReviewResultsResponse{}
	}
	bound, err := protocol.NewSourceBinding(fx.person.auth.Account.ID, deviceTestOrigin, fx.scope.OrgID().String(), fx.source.MembershipID, fx.source.EnrollmentID)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := protocol.ParseReviewResultsResponse(body, request, bound, expected)
	if err != nil || !bytes.Equal(parsed.CanonicalBytes(), body) {
		t.Fatal("whole response lost exact source/request/original-review correlation", err)
	}
	return parsed
}
func TestOwnReviewResultsActualSignedTLSAcceptedRefusedUnresolvedWithoutBusinessPermission(t *testing.T) {
	fx := ownReviewTLSFixture(t, 3)
	var accepted protocol.ReviewResolution
	for i, kind := range []app.MeterReviewDecisionKind{app.MeterReviewAccept, app.MeterReviewRefuse} {
		reason := ""
		if kind == app.MeterReviewRefuse {
			reason = "Synthetic reviewer refusal 😀"
		}
		command, err := app.NewMeterReviewDecision(app.MeterReviewDecisionInput{OperationID: onboardingID(t), ReviewItemID: fx.references[i].ReviewItemID, ExpectedRevision: 1, Decision: kind, Reason: reason})
		if err != nil {
			t.Fatal(err)
		}
		result, replay, err := fx.reviews.ResolveMeterReview(t.Context(), fx.scope, fleetActor(fx.legalFixtureState), fx.member, command)
		if err != nil || replay {
			t.Fatal("actual company reviewer decision", err)
		}
		if i == 0 {
			accepted = result
		}
	}
	matrixAssign(t, fx.deviceFixtureState, matrixPerson{member: fx.person.member}, "ADMIN", "ORG", "NONE")
	_, err := fx.flags.Set(t.Context(), fx.scope, capability.Actor{AccountID: fx.actor.AccountID}, capability.Change{OperationID: onboardingID(t), Capability: "organizations", Enabled: false, ExpectedRevision: 1})
	if err != nil {
		t.Fatal("actual write capability switch", err)
	}
	selectors := make([]protocol.ReviewSelector, 3)
	expected := make([]protocol.ReviewExpectation, 3)
	for i, op := range fx.operations {
		selectors[i] = protocol.ReviewSelector{OperationID: op.ID(), PayloadDigest: op.PayloadDigest()}
		expected[i] = protocol.ReviewExpectation{Selector: selectors[i], ReviewItemID: fx.references[i].ReviewItemID}
	}
	request := fx.request(t, selectors)
	response := fx.query(t, request, expected, http.StatusOK, nil)
	values := response.Results()
	if len(values) != 3 || values[0].Resolution == nil || !bytes.Equal(values[0].Resolution.CanonicalBytes(), accepted.CanonicalBytes()) || values[1].Resolution == nil || values[1].Resolution.Snapshot().Outcome != protocol.ReviewRefused || values[1].Resolution.Snapshot().Reason != "Synthetic reviewer refusal 😀" || values[2].Resolution != nil {
		t.Fatal("minimal own results lost accepted/refused/unresolved source facts")
	}
	next := fx.request(t, selectors[:1])
	again := fx.query(t, next, expected[:1], http.StatusOK, nil)
	if next.Snapshot().RequestID == request.Snapshot().RequestID || !bytes.Equal(again.Results()[0].Resolution.CanonicalBytes(), accepted.CanonicalBytes()) {
		t.Fatal("fresh request correlation replaced original immutable decision")
	}
	for i, op := range fx.operations {
		var raw []byte
		if err := fx.f.Admin.QueryRow(t.Context(), `SELECT canonical_ack FROM org_sync_acknowledgements WHERE org_id=$1 AND operation_id=$2`, fx.scope.OrgID().String(), op.ID()).Scan(&raw); err != nil || !bytes.Equal(raw, fx.originalACKs[i].CanonicalBytes()) {
			t.Fatal("result read rewrote original REVIEW acknowledgement", err)
		}
	}
	var exact bool
	if err := fx.f.Admin.QueryRow(t.Context(), `SELECT(SELECT count(*) FROM org_meter_readings WHERE org_id=$1)=2 AND(SELECT count(*) FROM org_sync_effects WHERE org_id=$1)=1 AND(SELECT count(*) FROM org_sync_review_resolutions WHERE org_id=$1)=2 AND(SELECT count(*) FROM org_audit_events WHERE org_id=$1 AND action='offl.review.resolved')=2`, fx.scope.OrgID().String()).Scan(&exact); err != nil || !exact {
		t.Fatal("minimal result read changed reading/review/effect multiplicity", err)
	}
}
func TestOwnReviewResultsActualSignedTLSWithholdsForeignSuffixAndExpiredOrRevokedSource(t *testing.T) {
	fx := ownReviewTLSFixture(t, 1)
	op := fx.operations[0]
	selector := protocol.ReviewSelector{OperationID: op.ID(), PayloadDigest: op.PayloadDigest()}
	expected := []protocol.ReviewExpectation{{Selector: selector, ReviewItemID: fx.references[0].ReviewItemID}}
	fx.query(t, fx.request(t, []protocol.ReviewSelector{selector}), expected, http.StatusOK, nil)
	foreign := protocol.ReviewSelector{OperationID: onboardingID(t), PayloadDigest: op.PayloadDigest()}
	fx.query(t, fx.request(t, []protocol.ReviewSelector{selector, foreign}), nil, http.StatusNotFound, nil)
	mismatch := selector
	mismatch.PayloadDigest = "sha256:" + strings.Repeat("b", 64)
	fx.query(t, fx.request(t, []protocol.ReviewSelector{mismatch}), nil, http.StatusNotFound, nil)
	changed := fx.request(t, []protocol.ReviewSelector{selector}).Snapshot()
	changed.EnrollmentID = onboardingID(t)
	request, err := protocol.NewReviewResultsRequest(changed)
	if err != nil {
		t.Fatal(err)
	}
	fx.query(t, request, nil, http.StatusNotFound, nil)
	fx.query(t, fx.request(t, []protocol.ReviewSelector{selector}), nil, http.StatusUnauthorized, func(r *http.Request) { r.Header.Set("Cookie", "synthetic-alternate") })
	fx.query(t, fx.request(t, []protocol.ReviewSelector{selector}), nil, http.StatusUnauthorized, func(r *http.Request) {
		r.Body = io.NopCloser(strings.NewReader(string(fx.request(t, []protocol.ReviewSelector{selector}).CanonicalBytes())))
		r.ContentLength = -1
	})
	// A real expired identity-v1 proof cannot read a retained private outcome.
	request = fx.request(t, []protocol.ReviewSelector{selector})
	expired := deviceSignedRequest(t, fx.registrar, fx.person.signer, fx.path, json.RawMessage(request.CanonicalBytes()), time.Now().Add(-time.Second))
	fx.query(t, request, nil, http.StatusUnauthorized, func(r *http.Request) { r.Header = expired.Header.Clone() })
	command := member.DeviceCommand{OperationID: onboardingID(t), DeviceID: fx.source.EnrollmentID, ExpectedRevision: 1, Class: memberdomain.LostOrCompromised, Reason: "Synthetic lost device"}
	if _, err := fx.devices.RevokeDevice(t.Context(), fx.scope, employeeActor(fx.legalFixtureState), fx.member, fx.person.member, false, command); err != nil {
		t.Fatal("actual device revocation", err)
	}
	fx.query(t, fx.request(t, []protocol.ReviewSelector{selector}), nil, http.StatusForbidden, nil)
}
