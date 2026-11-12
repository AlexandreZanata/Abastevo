package adapters_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

func TestSettingsCodecPreservesExplicitFalseZeroAndInt64WithoutGuessing(t *testing.T) {
	v, e := d.DefaultSettingsValues("UTC", "")
	if e != nil {
		t.Fatal(e)
	}
	v.ReviewRequired.Trips = false
	v.Policies.IntakeWindowDays = 0
	v.Policies.ClockSkewLimitSeconds = 9223372036854775807
	raw, e := adapters.EncodeSettingsValues(v)
	if e != nil {
		t.Fatal(e)
	}
	got, e := adapters.DecodeSettingsValues(raw)
	if e != nil || got != v {
		t.Fatal("codec changed exact values", e)
	}
	if !strings.Contains(string(raw), `"9223372036854775807"`) {
		t.Fatal("clock precision lost")
	}
	var tree map[string]any
	if e := json.Unmarshal(raw, &tree); e != nil {
		t.Fatal(e)
	}
	for _, field := range []string{"time_zone", "review_required", "responsibilities", "policies"} {
		copy := map[string]any{}
		for k, v := range tree {
			copy[k] = v
		}
		delete(copy, field)
		bad, _ := json.Marshal(copy)
		if _, e := adapters.DecodeSettingsValues(bad); e == nil {
			t.Fatal("missing field accepted", field)
		}
	}
	for _, raw := range []string{strings.Replace(string(raw), `"trips":false`, `"trips":null`, 1), strings.Replace(string(raw), `"trips":false`, `"Trips":false`, 1), strings.Replace(string(raw), `"trips":false`, `"trips":false,"trips":true`, 1), strings.Replace(string(raw), `"9223372036854775807"`, `"09223372036854775807"`, 1)} {
		if _, e := adapters.DecodeSettingsValues([]byte(raw)); e == nil {
			t.Fatal("ambiguous body accepted")
		}
	}
}
