package application_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	"log/slog"
	"strings"
	"testing"
)

func TestAccountContextSourceObjectsRedactPrivateReferences(t *testing.T) {
	organization := app.AccountContext{ID: "7b0e2a54-0000-4000-8000-000000000001", Name: "Synthetic private context name", MembershipID: "7b0e2a54-0000-4000-8000-000000000601"}
	for _, value := range []any{organization, app.AccountContextPage{Organizations: []app.AccountContext{organization}, NextAfterOrgID: organization.ID}, app.AccountContextView{Organization: organization, InvitationValidityDays: 7}} {
		raw, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		var output bytes.Buffer
		slog.New(slog.NewJSONHandler(&output, nil)).Info("synthetic", "value", value)
		for _, encoded := range []string{string(raw), fmt.Sprintf("%v %+v %#v", value, value, value), output.String()} {
			for _, secret := range []string{organization.ID, organization.Name, organization.MembershipID} {
				if strings.Contains(encoded, secret) {
					t.Fatal("source context formatting leaked private metadata")
				}
			}
		}
	}
}
