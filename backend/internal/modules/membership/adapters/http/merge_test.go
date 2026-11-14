package http

import (
	"encoding/json"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"strings"
	"testing"
)

func TestEmployeeMergeStrictReviewIntent(t *testing.T) {
	value := map[string]any{"schema_version": 1, "operation_id": "7b0e2a54-0000-4000-8000-000000001063", "revision": 1, "survivor_id": "7b0e2a54-0000-4000-8000-000000001062", "survivor_revision": 1, "review_digest": strings.Repeat("a", 64), "reviewed": true, "reason": "Synthetic reviewed duplicate"}
	raw, _ := json.Marshal(value)
	var v mergeRequest
	if httpapi.DecodeJSON(raw, &v) != nil {
		t.Fatal("valid review")
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
	for _, key := range []string{"account_id", "source_id", "state", "roles", "session_id", "team_effects"} {
		value[key] = "synthetic"
		raw, _ = json.Marshal(value)
		if httpapi.DecodeJSON(raw, &v) == nil {
			t.Fatal("authority override", key)
		}
		delete(value, key)
	}
}
