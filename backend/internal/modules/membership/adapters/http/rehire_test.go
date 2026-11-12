package http

import (
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"testing"
)

func TestRehireStrictIntentAndPrivateResponse(t *testing.T) {
	value := map[string]any{"schema_version": 1, "operation_id": "7b0e2a54-0000-4000-8000-000000001041", "revision": 2, "reason": "Synthetic rehire"}
	raw, _ := json.Marshal(value)
	var v rehireRequest
	if httpapi.DecodeJSON(raw, &v) != nil {
		t.Fatal("valid intent")
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
	for _, key := range []string{"account_id", "state", "roles", "membership_id", "employee_id", "session_id"} {
		value[key] = "synthetic"
		raw, _ = json.Marshal(value)
		if httpapi.DecodeJSON(raw, &v) == nil {
			t.Fatal("authority or target override", key)
		}
		delete(value, key)
	}
}
