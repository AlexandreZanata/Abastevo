package domain

import (
	"encoding/json"
	"testing"
)

func TestNormalizeVehicleAcceptsMHEVAsOwnKind(t *testing.T) {
	v := testVehicle()
	propulsion := PropulsionMHEV
	v.Propulsion = &propulsion
	got, err := NormalizeVehicle(v)
	if err != nil {
		t.Fatal("MHEV refused", err)
	}
	if got.Propulsion == nil || *got.Propulsion != PropulsionMHEV {
		t.Fatal("MHEV silently rewritten", got.Propulsion)
	}
	battery := int64(1000)
	v.BatteryWh = &battery
	if _, err := NormalizeVehicle(v); err == nil {
		t.Fatal("MHEV with external battery accepted")
	}
}

func TestPreserveUnknownVehicleFieldsCarriesNewerFields(t *testing.T) {
	stored := []byte(`{"id":"aaaaaaaa-1111-4111-8111-000000000001","display_name":"Old","future_energy":{"usable_wh":30000},"charge_connector":"TYPE2"}`)
	incoming := []byte(`{"id":"aaaaaaaa-1111-4111-8111-000000000001","display_name":"New","propulsion":"BEV","battery_wh":38000}`)
	merged, err := PreserveUnknownVehicleFields(stored, incoming)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatal(err)
	}
	if string(got["display_name"]) != `"New"` {
		t.Fatal("canonical field not taken from the update")
	}
	if string(got["future_energy"]) != `{"usable_wh":30000}` {
		t.Fatal("unknown newer object lost", string(merged))
	}
	if string(got["charge_connector"]) != `"TYPE2"` {
		t.Fatal("unknown newer field lost", string(merged))
	}
}

func TestPreserveUnknownVehicleFieldsRefusesMalformedAndDrift(t *testing.T) {
	if _, err := PreserveUnknownVehicleFields([]byte(`{`), []byte(`{}`)); err == nil {
		t.Fatal("malformed stored payload accepted")
	}
	if _, err := PreserveUnknownVehicleFields([]byte(`{}`), []byte(`{`)); err == nil {
		t.Fatal("malformed incoming payload accepted")
	}
	if _, err := PreserveUnknownVehicleFields(nil, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
}
