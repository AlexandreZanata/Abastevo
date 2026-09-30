package http

import "testing"

func TestWriteParsersRejectTrailingAndDuplicateJSON(t *testing.T) {
	valid := `{"client_submission_id":"cfm-1"}`
	for _, raw := range []string{valid + ` {"ignored":true}`, `{"client_submission_id":"safe","client_submission_id":"substituted"}`, valid + ` trailing`} {
		if _, err := parseConfirmBody([]byte(raw)); err == nil {
			t.Error("accepted ambiguous JSON")
		}
	}
}
