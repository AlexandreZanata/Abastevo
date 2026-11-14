package http

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestMeterDTORejectsUnprovedAuthorityAndLossyObservation(t *testing.T) {
	for name, raw := range map[string]string{"author": `{"author_membership_id":"private"}`, "source": `{"capture":{"kind":"WEB_SESSION","id":"private"}}`, "receipt": `{"received_at":"private"}`, "state": `{"state":"VALID"}`, "alias": `{"Label":"private"}`, "null": `{"label":null}`, "duplicate": `{"label":"a","label":"b"}`, "surrogate": `{"label":"\ud800"}`, "utf8": "{\"label\":\"\xff\"}", "numeric-value": `{"opening_value":100}`, "numeric-modulus": `{"modulus":100}`} {
		t.Run(name, func(t *testing.T) {
			var v meterInstallRequest
			if json.Unmarshal([]byte(raw), &v) == nil {
				t.Fatal("unproved/lossy meter input accepted")
			}
		})
	}
}
func TestMeterDTOOwnsExactOpeningAndRedactsDiagnostics(t *testing.T) {
	raw := `{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000008101","revision":1,"opening_reading_id":"7b0e2a54-0000-4000-8000-000000008403","kind":"ODOMETER","label":"Private display � 🚀","modulus":"1000000000001","opening_value":"1000000000000","occurred_at":"2026-11-14T00:00:00.123456789Z","entry_zone":"America/Cuiaba","device_offset_seconds":-14400,"method":"TYPED"}`
	var v meterInstallRequest
	if e := json.Unmarshal([]byte(raw), &v); e != nil {
		t.Fatal(e)
	}
	command, e := v.command("7b0e2a54-0000-4000-8000-000000008201")
	if e != nil {
		t.Fatal("real exact opening request", e)
	}
	in, e := command.Snapshot()
	if e != nil || in.Value.Count() != 1000000000000 || in.Declared.OccurredAt.Nanosecond() != 123456789 || in.Modulus == nil || *in.Modulus != 1000000000001 {
		t.Fatal("exact observed opening lost")
	}
	if strings.Contains(fmt.Sprintf("%+v", v), "Private display") {
		t.Fatal("private DTO diagnostic")
	}
	v.DeviceOffsetSeconds = nil
	if _, e := v.command(in.AssetID); e == nil {
		t.Fatal("omitted offset silently becamezero")
	}
}
