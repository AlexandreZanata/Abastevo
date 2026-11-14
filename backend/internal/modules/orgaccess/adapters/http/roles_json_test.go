package http

import (
	"encoding/json"
	"testing"
)

func TestRoleRequestExactKeysAndNull(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		into      func() any
	}{
		{"assign case", `{"Schema_Version":1}`, func() any { return &roleAssignRequest{} }},
		{"assign null", `{"valid_to":null}`, func() any { return &roleAssignRequest{} }},
		{"change case", `{"ROLE":"ADMIN"}`, func() any { return &roleChangeRequest{} }},
		{"end null", `{"revision":null}`, func() any { return &roleEndRequest{} }},
		{"read extra", `{"account_id":"forged"}`, func() any { return &roleReadRequest{} }},
		{"list nested case", `{"page":{"Limit":20}}`, func() any { return &roleListRequest{} }},
		{"list nested null", `{"page":{"cursor":null}}`, func() any { return &roleListRequest{} }},
		{"list null", `{"page":null}`, func() any { return &roleListRequest{} }},
		{"list zero", `{"page":{"limit":0}}`, func() any { return &roleListRequest{} }},
		{"list duplicate", `{"page":{},"page":{}}`, func() any { return &roleListRequest{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if json.Unmarshal([]byte(tc.raw), tc.into()) == nil {
				t.Fatal("noncanonical request accepted")
			}
		})
	}
	for _, raw := range []string{`{}`, `{"page":{}}`, `{"page":{"limit":20,"cursor":""},"role":"ADMIN"}`} {
		var v roleListRequest
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Fatal("valid optional page rejected", err)
		}
	}
}
