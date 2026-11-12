package http

import (
	"encoding/json"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"strings"
	"testing"
)

func TestRedemptionDTORejectsAuthorityAndAmbiguousConsent(t *testing.T) {
	for _, raw := range []string{`{}`, `{"code":"synthetic","code":"other"}`, `{"Code":"synthetic"}`, `{"code":null}`, `{"code":"synthetic","account_id":"foreign"}`, `{"code":"synthetic","family_id":"foreign"}`} {
		var v previewRequest
		if httpapi.DecodeJSON([]byte(raw), &v) == nil {
			t.Fatal("ambiguous preview accepted")
		}
	}
	raw := []byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001010","code":"synthetic-private-code","consent":true,"consent_digest":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}`)
	var v acceptRequest
	if e := httpapi.DecodeJSON(raw, &v); e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(v)
	for _, value := range []string{string(encoded), fmt.Sprintf("%+v %#v", v, v)} {
		if strings.Contains(value, "synthetic-private-code") || strings.Contains(value, "dddddddd") {
			t.Fatal("credential formatting disclosure")
		}
	}
}
