package http

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestAssetDTORejectsAmbiguousAuthorityAndLossyUnicode(t *testing.T) {
	for name, raw := range map[string]string{
		"account proof":       `{"account_id":"synthetic"}`,
		"family proof":        `{"family_id":"synthetic"}`,
		"state authority":     `{"state":"ACTIVE"}`,
		"meter authority":     `{"meters":[{"in_service":true}]}`,
		"accepted history":    `{"accepted_operations":0}`,
		"field alias":         `{"Name":"alias"}`,
		"field null":          `{"name":null}`,
		"nested null":         `{"category":{"new_label":null}}`,
		"array null":          `{"tanks":[null]}`,
		"duplicate":           `{"name":"a","name":"b"}`,
		"nested alias":        `{"category":{"New_Label":"alias"}}`,
		"lone high surrogate": `{"name":"\ud800"}`,
		"lone low surrogate":  `{"name":"\udc00"}`,
		"bad surrogate pair":  `{"name":"\ud800\u0041"}`,
		"invalid utf8":        "{\"name\":\"\xff\"}",
		"capacity numeric":    `{"tanks":[{"capacity_milli":275000}]}`,
		"expected numeric":    `{"expected_consumption":[{"value":9007199254740993}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var input assetCreateRequest
			if json.Unmarshal([]byte(raw), &input) == nil {
				t.Fatal("ambiguous, authoritative or lossy asset input accepted")
			}
		})
	}
	for _, input := range []json.Unmarshaler{&assetCreateRequest{}, &assetChangeRequest{}, &assetTransitionRequest{}, &assetReadRequest{}, &assetListRequest{}} {
		for _, raw := range []string{`null`, `[]`, `{"account_id":"synthetic"}`, `{} {}`} {
			if json.Unmarshal([]byte(raw), input) == nil {
				t.Fatal("private fleet request boundary widened")
			}
		}
	}
}

func TestAssetDTOExactTextAndPrivateDiagnostics(t *testing.T) {
	raw := []byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","kind":"VEHICLE","name":"Synthetic private asset \ud83d\ude80 �","fleet_number":"PRIV-01","plate":"ABC1D23","serial_number":"PRIVATE-SERIAL","usage_basis":"DISTANCE","category":{"new_label":"Synthetic private category"},"tanks":[{"role":"MAIN","capacity_milli":"275000","products":["DIESEL_S10"]}],"expected_consumption":[{"product":"DIESEL_S10","basis":"DISTANCE","value":"9007199254740993","source":"Synthetic private manual","date":"2026-11-14"}]}`)
	var input assetCreateRequest
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal("canonical fleet request refused", err)
	}
	if input.Name != "Synthetic private asset 🚀 �" || input.Tanks[0].CapacityMilli != "275000" || input.ExpectedConsumption[0].Value != "9007199254740993" || input.CommandFields().SchemaVersion != 1 || input.Credentials().AccessToken != "" {
		t.Fatal("exact fleet input or proof boundary changed")
	}
	for _, v := range []any{input, input.assetFields, input.Category, input.Tanks[0], input.ExpectedConsumption[0], assetChangeRequest{assetFields: input.assetFields}} {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
			if strings.Contains(fmt.Sprintf(verb, v), "private") || strings.Contains(fmt.Sprintf(verb, v), "PRIVATE") {
				t.Fatal("private fleet formatting leaked")
			}
		}
		encoded, err := json.Marshal(v)
		if err != nil || strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "PRIVATE") {
			t.Fatal("private fleet JSON leaked")
		}
		if strings.Contains(slog.AnyValue(v).Resolve().String(), "private") {
			t.Fatal("private fleet structured log leaked")
		}
	}
}
