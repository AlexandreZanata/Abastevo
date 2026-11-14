package http

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

// The Go decoder accepts case aliases for tagged fields. Company contracts use
// exact keys and distinguish omitted optional fields from explicit null.
func roleObject(raw []byte, keys ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if httpapi.DecodeJSON(raw, &fields) != nil || fields == nil {
		return nil, orgapi.Malformed
	}
	for key, value := range fields {
		if !slices.Contains(keys, key) || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, orgapi.Malformed
		}
	}
	return fields, nil
}
func (v *roleAssignRequest) UnmarshalJSON(raw []byte) error {
	if _, err := roleObject(raw, "schema_version", "operation_id", "membership_id", "role", "scope", "target_id", "valid_from", "valid_to", "reason"); err != nil {
		return err
	}
	type plain roleAssignRequest
	return httpapi.DecodeJSON(raw, (*plain)(v))
}
func (v *roleEndRequest) UnmarshalJSON(raw []byte) error {
	if _, err := roleObject(raw, "schema_version", "operation_id", "revision", "reason"); err != nil {
		return err
	}
	type plain roleEndRequest
	return httpapi.DecodeJSON(raw, (*plain)(v))
}
func (v *roleChangeRequest) UnmarshalJSON(raw []byte) error {
	if _, err := roleObject(raw, "schema_version", "operation_id", "revision", "role", "scope", "target_id", "valid_from", "valid_to", "reason"); err != nil {
		return err
	}
	type plain roleChangeRequest
	return httpapi.DecodeJSON(raw, (*plain)(v))
}
func (v *roleReadRequest) UnmarshalJSON(raw []byte) error { _, err := roleObject(raw); return err }
func (v *roleListRequest) UnmarshalJSON(raw []byte) error {
	fields, err := roleObject(raw, "page", "membership_id", "role", "state")
	if err != nil {
		return err
	}
	if page, ok := fields["page"]; ok {
		fields, err = roleObject(page, "limit", "cursor")
		if err != nil {
			return err
		}
		if limit, ok := fields["limit"]; ok {
			var number int
			if json.Unmarshal(limit, &number) != nil || number < 1 || number > 100 {
				return orgapi.Malformed
			}
		}
	}
	type plain roleListRequest
	return httpapi.DecodeJSON(raw, (*plain)(v))
}
