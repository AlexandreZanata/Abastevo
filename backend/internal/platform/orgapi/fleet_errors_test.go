package orgapi

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFleetRefusalsAreRegisteredBoundedAndCopied(t *testing.T) {
	t.Run("identifier", func(t *testing.T) {
		writer := httptest.NewRecorder()
		WriteError(writer, httptest.NewRequest("POST", "/", nil), Code("ASSET_IDENTIFIER_TAKEN"))
		if writer.Code != 409 || !strings.Contains(writer.Body.String(), `"code":"ASSET_IDENTIFIER_TAKEN"`) {
			t.Fatal("asset identifier refusal unavailable")
		}
	})
	t.Run("activation", func(t *testing.T) {
		fields := []string{"unit_id", "tanks", "meters"}
		err := IncompleteAsset(fields)
		fields[0] = "Synthetic private injected field"
		if !errors.Is(err, ValidationFailed) {
			t.Fatal("activation checklist lost registered refusal")
		}
		writer := httptest.NewRecorder()
		WriteError(writer, httptest.NewRequest("POST", "/", nil), err)
		var body struct {
			Error struct {
				Code    string                         `json:"code"`
				Details []struct{ Field, Code string } `json:"details"`
			} `json:"error"`
		}
		if json.Unmarshal(writer.Body.Bytes(), &body) != nil || writer.Code != 422 || body.Error.Code != "VALIDATION_FAILED" || len(body.Error.Details) != 3 {
			t.Fatal("activation checklist shape changed")
		}
		for i, field := range []string{"unit_id", "tanks", "meters"} {
			if body.Error.Details[i].Field != field || body.Error.Details[i].Code != "REQUIRED" {
				t.Fatal("activation details were mutable or unbounded")
			}
		}
		for _, value := range [][]string{nil, {"unit_id", "unit_id"}, {"unit_id", "tanks", "meters", "extra"}, {"Synthetic private injected field"}, {"asset_id"}} {
			if IncompleteAsset(value) != Unavailable {
				t.Fatal("private arbitrary activation detail accepted")
			}
		}
	})
}
