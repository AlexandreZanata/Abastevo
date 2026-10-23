package domain

import "encoding/json"

// VehicleFields is the frozen allowlist for hosted vehicle payloads.
// Unknown keys are preserved only on updates (see
// PreserveUnknownVehicleFields), never admitted from a new write.
var VehicleFields = map[string]bool{
	"id":                       true,
	"display_name":             true,
	"capacity_milli":           true,
	"fuel_product":             true,
	"unit":                     true,
	"price_source":             true,
	"specific_station_cnpj":    true,
	"price_drop_alert_enabled": true,
	"sort_order":               true,
	"archived":                 true,
	"propulsion":               true,
	"battery_wh":               true,
}

const maxPreservedVehicleFields = 16

// MergeStoredVehiclePayload combines a stored record with an incoming
// partial update. Energy keys omitted by an older client (the sync wire
// normalizes absent propulsion to legacy ICE) are restored from storage,
// unknown newer fields are carried over, and the result is revalidated.
// Explicit incoming values still win.
func MergeStoredVehiclePayload(stored, incoming []byte) ([]byte, error) {
	if len(stored) == 0 {
		return incoming, nil
	}
	var previous map[string]json.RawMessage
	var next map[string]json.RawMessage
	if json.Unmarshal(stored, &previous) != nil || json.Unmarshal(incoming, &next) != nil {
		return nil, ErrInvalid
	}
	for _, key := range []string{"propulsion", "battery_wh"} {
		if _, exists := next[key]; exists {
			continue
		}
		if value, exists := previous[key]; exists {
			next[key] = value
		}
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return nil, ErrInvalid
	}
	var vehicle Vehicle
	if json.Unmarshal(raw, &vehicle) != nil {
		return nil, ErrInvalid
	}
	vehicle, err = NormalizeVehicle(vehicle)
	if err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(vehicle)
	if err != nil {
		return nil, ErrInvalid
	}
	return PreserveUnknownVehicleFields(stored, canonical)
}

// PreserveUnknownVehicleFields keeps fields written by a newer server or
// client when an older partial update is stored. The incoming payload is
// already typed/allowlisted; fields absent from the frozen allowlist are
// carried from the stored record only. Canonical fields always come from
// the incoming payload, and a malformed stored/incoming object refuses
// instead of losing data silently.
func PreserveUnknownVehicleFields(stored, incoming []byte) ([]byte, error) {
	if len(stored) == 0 || len(incoming) == 0 {
		return incoming, nil
	}
	var previous map[string]json.RawMessage
	var next map[string]json.RawMessage
	if json.Unmarshal(stored, &previous) != nil || json.Unmarshal(incoming, &next) != nil {
		return nil, ErrInvalid
	}
	preserved := 0
	for key, value := range previous {
		if VehicleFields[key] {
			continue
		}
		if _, exists := next[key]; exists {
			continue
		}
		if preserved >= maxPreservedVehicleFields {
			return nil, ErrInvalid
		}
		next[key] = value
		preserved++
	}
	if preserved == 0 {
		return incoming, nil
	}
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > 8192 {
		return nil, ErrInvalid
	}
	return raw, nil
}
