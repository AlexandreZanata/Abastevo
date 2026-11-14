package orgapi

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
)

func TestAssetOpenOperationsNamesOnlyRegisteredDistinctKinds(t *testing.T) {
	kinds := []string{"TRIP", "WORK_ORDER"}
	err := AssetOpenOperations(kinds)
	kinds[0] = "Synthetic private injected kind"
	if !errors.Is(err, AssetHasOpenOperations) {
		t.Fatal("open work refusal lost its registered code", err)
	}
	writer := httptest.NewRecorder()
	WriteError(writer, httptest.NewRequest("POST", "/", nil), err)
	var body struct {
		Error struct {
			Code    string                         `json:"code"`
			Details []struct{ Field, Code string } `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(writer.Body.Bytes(), &body) != nil || writer.Code != 409 || body.Error.Code != "ASSET_HAS_OPEN_OPERATIONS" || len(body.Error.Details) != 2 {
		t.Fatal("open work refusal shape changed", writer.Code, writer.Body.String())
	}
	for i, kind := range []string{"TRIP", "WORK_ORDER"} {
		if body.Error.Details[i].Field != "open_operations" || body.Error.Details[i].Code != kind {
			t.Fatal("open work details were mutable or unbounded")
		}
	}
	bare := httptest.NewRecorder()
	WriteError(bare, httptest.NewRequest("POST", "/", nil), AssetHasOpenOperations)
	if bare.Code != 409 {
		t.Fatal("bare open work code is not a bounded conflict", bare.Code)
	}
	for _, value := range [][]string{nil, {}, {"TRIP", "TRIP"}, {"TRIP", "WORK_ORDER", "TRIP"}, {"RESERVATION"}, {"trip"}, {"Synthetic private trip id"}} {
		if AssetOpenOperations(value) != Unavailable {
			t.Fatal("private arbitrary open work detail accepted", value)
		}
	}
}
