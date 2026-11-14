package protocol_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

func reviewRequestInput() protocol.ReviewResultsRequestSnapshot {
	return protocol.ReviewResultsRequestSnapshot{RequestID: reviewID("0901"), MembershipID: reviewID("0003"), EnrollmentID: reviewID("0004"), Proof: protocol.DeliveryProof{FamilyID: reviewID("0009"), AccessToken: `private-"proof`}, Selectors: []protocol.ReviewSelector{{OperationID: reviewID("0101"), PayloadDigest: "sha256:" + strings.Repeat("a", 64)}, {OperationID: reviewID("0104"), PayloadDigest: "sha256:" + strings.Repeat("b", 64)}}}
}
func reviewSource(t *testing.T) protocol.SourceBinding {
	t.Helper()
	v, err := protocol.NewSourceBinding(reviewID("0001"), "https://api.example.invalid", reviewID("0002"), reviewID("0003"), reviewID("0004"))
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestReviewResultsRequestCapturesExactPrivateSelectorsAndProof(t *testing.T) {
	in := reviewRequestInput()
	request, err := protocol.NewReviewResultsRequest(in)
	if err != nil {
		t.Fatal("valid review request refused", err)
	}
	in.Selectors[0].OperationID = reviewID("9999")
	copy := request.Snapshot()
	copy.Selectors[0].PayloadDigest = "changed"
	if request.Snapshot().Selectors[0].OperationID != reviewID("0101") || request.Snapshot().Selectors[0].PayloadDigest != "sha256:"+strings.Repeat("a", 64) {
		t.Fatal("request selectors alias input/output")
	}
	parsed, err := protocol.ParseReviewResultsRequest(request.CanonicalBytes())
	if err != nil || parsed.ValidateSource(reviewSource(t)) != nil || !bytes.Equal(parsed.CanonicalBytes(), request.CanonicalBytes()) {
		t.Fatal("exact request failed round trip", err)
	}
	raw, _ := json.Marshal(request)
	if strings.Contains(fmt.Sprintf("%#v %s %s", request, request.Snapshot(), raw), "private-") {
		t.Fatal("proof leaked from diagnostics")
	}
	foreign, _ := protocol.NewSourceBinding(reviewID("0001"), "https://api.example.invalid", reviewID("0002"), reviewID("0003"), reviewID("9999"))
	if !errors.Is(request.ValidateSource(foreign), protocol.ErrBinding) {
		t.Fatal("request substituted current enrollment")
	}
}
func reviewResponseFixture(t *testing.T) (protocol.ReviewResultsRequest, protocol.ReviewResultsResponse, []protocol.ReviewExpectation) {
	t.Helper()
	request, err := protocol.NewReviewResultsRequest(reviewRequestInput())
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := protocol.NewReviewResolution(reviewSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	selectors := request.Snapshot().Selectors
	response, err := protocol.NewReviewResultsResponse(request, reviewSource(t), []protocol.ReviewResult{{Selector: selectors[0], Resolution: &resolution}, {Selector: selectors[1]}})
	if err != nil {
		t.Fatal("valid full response refused", err)
	}
	expected := []protocol.ReviewExpectation{{Selector: selectors[0], ReviewItemID: reviewID("0102")}, {Selector: selectors[1], ReviewItemID: reviewID("0105")}}
	return request, response, expected
}
func TestReviewResultsResponsePreflightsAllSlotsAndOriginalReviewBeforePublication(t *testing.T) {
	request, response, expected := reviewResponseFixture(t)
	parsed, err := protocol.ParseReviewResultsResponse(response.CanonicalBytes(), request, reviewSource(t), expected)
	if err != nil || len(parsed.Results()) != 2 || parsed.Results()[0].Resolution == nil || parsed.Results()[1].Resolution != nil {
		t.Fatal("valid complete ordered results refused", err)
	}
	for name, edit := range map[string]func(map[string]any){
		"late-slot-digest": func(v map[string]any) {
			v["results"].([]any)[1].(map[string]any)["payload_digest"] = "sha256:" + strings.Repeat("c", 64)
		},
		"late-slot-id": func(v map[string]any) { v["results"].([]any)[1].(map[string]any)["operation_id"] = reviewID("9999") },
		"wrong-original-review": func(v map[string]any) {
			v["results"].([]any)[0].(map[string]any)["resolution"].(map[string]any)["review_item_id"] = reviewID("9999")
		},
		"missing-slot":  func(v map[string]any) { v["results"] = v["results"].([]any)[:1] },
		"wrong-source":  func(v map[string]any) { v["owner_account_id"] = reviewID("9999") },
		"wrong-request": func(v map[string]any) { v["request_id"] = reviewID("9999") },
		"extra-field":   func(v map[string]any) { v["access_token"] = "secret" },
	} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			_ = json.Unmarshal(response.CanonicalBytes(), &value)
			edit(value)
			raw, _ := json.Marshal(value)
			got, err := protocol.ParseReviewResultsResponse(raw, request, reviewSource(t), expected)
			if err == nil || len(got.CanonicalBytes()) != 0 || len(got.Results()) != 0 {
				t.Fatal("invalid suffix/source exposed buffered private prefix")
			}
		})
	}
	got := parsed.Results()
	got[0].Selector.OperationID = reviewID("9999")
	*got[0].Resolution = protocol.ReviewResolution{}
	if parsed.Results()[0].Resolution == nil || len(parsed.Results()[0].Resolution.CanonicalBytes()) == 0 || parsed.Results()[0].Selector.OperationID != reviewID("0101") {
		t.Fatal("response values alias exported slice/pointer")
	}
}
func TestReviewResultsNewBoundsDoNotExtendFrozenBatchProfile(t *testing.T) {
	in := reviewRequestInput()
	in.Selectors = nil
	results := make([]protocol.ReviewResult, 50)
	expected := make([]protocol.ReviewExpectation, 50)
	for i := range 50 {
		v := reviewSnapshot()
		v.OperationID = fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", i+1)
		v.Reason = strings.Repeat("😀", 280)
		r, err := protocol.NewReviewResolution(v)
		if err != nil {
			t.Fatal(err)
		}
		s := protocol.ReviewSelector{OperationID: v.OperationID, PayloadDigest: v.PayloadDigest}
		in.Selectors = append(in.Selectors, s)
		results[i] = protocol.ReviewResult{Selector: s, Resolution: &r}
		expected[i] = protocol.ReviewExpectation{Selector: s, ReviewItemID: v.ReviewItemID}
	}
	request, err := protocol.NewReviewResultsRequest(in)
	if err != nil {
		t.Fatal(err)
	}
	response, err := protocol.NewReviewResultsResponse(request, reviewSource(t), results)
	if err != nil {
		t.Fatal("50 complete Unicode outcomes refused", err)
	}
	if len(response.CanonicalBytes()) <= protocol.MaxBatchBytes || len(response.CanonicalBytes()) > protocol.MaxReviewResultsResponseBytes {
		t.Fatal("worst-case proof did not exercise separate 128KiB response profile")
	}
	if _, err := protocol.ParseReviewResultsResponse(response.CanonicalBytes(), request, reviewSource(t), expected); err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.ParseBatch(response.CanonicalBytes()); !errors.Is(err, protocol.ErrLimit) {
		t.Fatal("new response silently widened V1 batch")
	}
}
