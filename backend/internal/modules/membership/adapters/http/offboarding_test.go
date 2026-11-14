package http

import (
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"strings"
	"testing"
)

func TestOffboardingStrictIntentRequiresExplicitReviewAndEmployeeChoice(t *testing.T) {
	value := map[string]any{"schema_version": 1, "operation_id": "7b0e2a54-0000-4000-8000-000000001031", "revision": 2, "employee_revision": 1, "reason": "Synthetic leave", "handover_digest": strings.Repeat("a", 64), "flags": []string{}, "terminate_employee": false}
	raw, _ := json.Marshal(value)
	var v offboardingRequest
	if httpapi.DecodeJSON(raw, &v) != nil {
		t.Fatal("valid command")
	}
	for key, original := range value {
		delete(value, key)
		raw, _ = json.Marshal(value)
		if httpapi.DecodeJSON(raw, &v) == nil {
			t.Fatal("missing intent", key)
		}
		value[key] = nil
		raw, _ = json.Marshal(value)
		if httpapi.DecodeJSON(raw, &v) == nil {
			t.Fatal("null intent", key)
		}
		value[key] = original
	}
	for _, key := range []string{"account_id", "session_id", "state", "self", "employee_state"} {
		value[key] = "synthetic"
		raw, _ = json.Marshal(value)
		if httpapi.DecodeJSON(raw, &v) == nil {
			t.Fatal("authority selector", key)
		}
		delete(value, key)
	}
}
