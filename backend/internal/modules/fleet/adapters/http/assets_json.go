// Package http exposes private organization fleet commands through orgapi.
package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"unicode/utf8"

	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

type privateRequest struct{}

func (privateRequest) String() string                 { return "EnterpriseFleetRequest(redacted)" }
func (v privateRequest) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, v.String()) }
func (v privateRequest) MarshalJSON() ([]byte, error) { return json.Marshal(v.String()) }
func (v privateRequest) LogValue() slog.Value         { return slog.StringValue(v.String()) }

type companyRequest struct{ privateRequest }

func (companyRequest) Credentials() orgapi.Session { return orgapi.Session{} }

type categoryFields struct {
	privateRequest
	ExistingID string `json:"existing_id,omitempty"`
	NewLabel   string `json:"new_label,omitempty"`
}
type tankFields struct {
	privateRequest
	Role          d.TankRole `json:"role"`
	CapacityMilli string     `json:"capacity_milli"`
	Products      []string   `json:"products"`
}
type expectedFields struct {
	privateRequest
	Product string  `json:"product"`
	Basis   d.Basis `json:"basis"`
	Value   string  `json:"value"`
	Source  string  `json:"source"`
	Date    string  `json:"date"`
}
type assetFields struct {
	privateRequest
	Kind                d.Kind           `json:"kind"`
	Name                string           `json:"name"`
	FleetNumber         string           `json:"fleet_number,omitempty"`
	Plate               string           `json:"plate,omitempty"`
	SerialNumber        string           `json:"serial_number,omitempty"`
	UsageBasis          d.Basis          `json:"usage_basis"`
	Category            categoryFields   `json:"category"`
	UnitID              string           `json:"unit_id,omitempty"`
	Tanks               []tankFields     `json:"tanks,omitempty"`
	ExpectedConsumption []expectedFields `json:"expected_consumption,omitempty"`
}
type assetCreateRequest struct {
	privateRequest
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	assetFields
}

func (v assetCreateRequest) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID}
}

type assetRevisionCommand struct {
	companyRequest
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	Revision      *int64 `json:"revision"`
}

func (v assetRevisionCommand) CommandFields() orgapi.Command {
	return orgapi.Command{SchemaVersion: v.SchemaVersion, OperationID: v.OperationID, Revision: v.Revision}
}

type assetChangeRequest struct {
	privateRequest
	assetRevisionCommand
	assetFields
}
type assetTransitionRequest struct{ assetRevisionCommand }
type assetReadRequest struct{ companyRequest }
type assetListRequest struct {
	companyRequest
	orgapi.Page
	State d.State `json:"state,omitempty"`
	Kind  d.Kind  `json:"kind,omitempty"`
}

// encoding/json otherwise replaces invalid UTF-8 or lone escaped surrogates.
// Refuse that loss before domain validation, while retaining a literal U+FFFD.
func losslessStrings(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	hex := func(raw []byte) (uint16, bool) {
		if len(raw) != 4 {
			return 0, false
		}
		var value uint16
		for _, b := range raw {
			value <<= 4
			switch {
			case b >= '0' && b <= '9':
				value += uint16(b - '0')
			case b >= 'a' && b <= 'f':
				value += uint16(b - 'a' + 10)
			case b >= 'A' && b <= 'F':
				value += uint16(b - 'A' + 10)
			default:
				return 0, false
			}
		}
		return value, true
	}
	quoted := false
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case '"':
			quoted = !quoted
		case '\\':
			if !quoted {
				continue
			}
			i++
			if i >= len(raw) {
				return false
			}
			if raw[i] != 'u' {
				continue
			}
			if i+4 >= len(raw) {
				return false
			}
			unit, ok := hex(raw[i+1 : i+5])
			if !ok {
				return false
			}
			i += 4
			if unit >= 0xd800 && unit <= 0xdbff {
				if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
					return false
				}
				low, ok := hex(raw[i+3 : i+7])
				if !ok || low < 0xdc00 || low > 0xdfff {
					return false
				}
				i += 6
			} else if unit >= 0xdc00 && unit <= 0xdfff {
				return false
			}
		}
	}
	return !quoted
}

func canonicalFields(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if len(key) == 0 || key[0] < 'a' || key[0] > 'z' {
				return false
			}
			for i := 1; i < len(key); i++ {
				b := key[i]
				if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_') {
					return false
				}
			}
			if !canonicalFields(item) {
				return false
			}
		}
	case []any:
		for _, item := range v {
			if !canonicalFields(item) {
				return false
			}
		}
	case nil:
		return false
	}
	return true
}

func decodeAsset[T any](raw []byte, target *T) error {
	if !losslessStrings(raw) {
		return orgapi.Malformed
	}
	if err := httpapi.DecodeJSON(raw, target); err != nil {
		return orgapi.Malformed
	}
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&fields) != nil || fields == nil || !canonicalFields(fields) {
		return orgapi.Malformed
	}
	return nil
}

func (v *assetCreateRequest) UnmarshalJSON(raw []byte) error {
	type plain assetCreateRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = assetCreateRequest(input)
	return nil
}
func (v *assetChangeRequest) UnmarshalJSON(raw []byte) error {
	type plain assetChangeRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = assetChangeRequest(input)
	return nil
}
func (v *assetTransitionRequest) UnmarshalJSON(raw []byte) error {
	type plain assetTransitionRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = assetTransitionRequest(input)
	return nil
}
func (v *assetReadRequest) UnmarshalJSON(raw []byte) error {
	type plain assetReadRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = assetReadRequest(input)
	return nil
}
func (v *assetListRequest) UnmarshalJSON(raw []byte) error {
	type plain assetListRequest
	var input plain
	if err := decodeAsset(raw, &input); err != nil {
		return err
	}
	*v = assetListRequest(input)
	return nil
}

var _ json.Unmarshaler = (*assetCreateRequest)(nil)
