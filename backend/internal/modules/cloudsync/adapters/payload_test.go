package adapters

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"strings"
	"testing"
)

func TestPortablePreferenceAllowlistRejectsDeviceCredentialsAndVersions(t *testing.T) {
	good := `{"version":1,"locale_tag":"pt-BR","nearest_station_radius_km":3,"show_price_history":true,"preferred_state":"MT","preferred_municipality":"5103403"}`
	if _, err := NormalizePayload("preferences", domain.PreferencesID, []byte(good)); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"access_token":"synthetic"`, `"private_key":"synthetic"`, `"location_permission":true`, `"auto_sync_on_wifi":true`, `"latitude":1`, `"version":2`} {
		bad := strings.TrimSuffix(good, "}") + "," + field + "}"
		if _, err := NormalizePayload("preferences", domain.PreferencesID, []byte(bad)); err == nil {
			t.Fatal("excluded field accepted", field)
		}
	}
	for _, raw := range []string{strings.Replace(good, `"version":1`, `"version":2`, 1), strings.Replace(good, `"preferred_state":"MT"`, `"preferred_state":"XX"`, 1), strings.Replace(good, "5103403", "not-code", 1)} {
		if _, err := NormalizePayload("preferences", domain.PreferencesID, []byte(raw)); err == nil {
			t.Fatal("invalid preference accepted")
		}
	}
}
