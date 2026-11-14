package application_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
)

func TestSetupReadModelsKeepPrivateDiagnosticsRedacted(t *testing.T) {
	id := "7b0e2a54-0000-4000-8000-000000000601"
	name := "Synthetic private setup member"
	values := []any{app.SettingsMembersFilter{Limit: 1, AfterMembershipID: id}, app.SettingsMember{MembershipID: id, DisplayName: name}, app.SettingsContext{AccountID: id, MembershipID: id}, app.SettingsMembersPage{AccountID: id, OrganizationID: id, MembershipID: id}, app.CreatedSetup{ID: id, Name: name, MembershipID: id}, app.CreatedSetupPage{AccountID: id, Setups: []app.CreatedSetup{{ID: id, Name: name}}}}
	check := func(raw string) {
		t.Helper()
		if strings.Contains(raw, id) || strings.Contains(raw, name) {
			t.Fatal("setup read diagnostic exposed a private identifier or label")
		}
	}
	for _, value := range values {
		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
			check(fmt.Sprintf(format, value))
		}
		raw, e := json.Marshal(map[string]any{"nested": []any{value}})
		if e != nil {
			t.Fatal(e)
		}
		check(string(raw))
		var output bytes.Buffer
		slog.New(slog.NewJSONHandler(&output, nil)).Info("synthetic", "value", value)
		check(output.String())
	}
}
