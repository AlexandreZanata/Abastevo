package orgapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsValidationDetailsExposeOnlyFrozenRanges(t *testing.T) {
	for field, code := range map[string]string{"closure_period_days": "RANGE_7_90", "policies.clock_skew_limit_seconds": "RANGE_0_9223372036854775807", "responsibilities.billing.substitute": "DISTINCT_SCOPED_MEMBER_OR_EMPTY", "time_zone": "IANA_TIME_ZONE_REQUIRED"} {
		w := httptest.NewRecorder()
		WriteError(w, httptest.NewRequest("POST", "/", nil), SettingsValidation(field))
		var result map[string]any
		if w.Code != 422 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal("range status")
		}
		detail := result["error"].(map[string]any)["details"].([]any)[0].(map[string]any)
		if detail["field"] != field || detail["code"] != code {
			t.Fatal("range detail")
		}
	}
	raw := "private settings value"
	err := SettingsValidation(raw)
	w := httptest.NewRecorder()
	WriteError(w, httptest.NewRequest("POST", "/", nil), err)
	if w.Code != 503 || strings.Contains(w.Body.String(), raw) {
		t.Fatal("unregistered field exposed")
	}
}
