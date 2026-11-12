package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"strings"
	"testing"
)

func TestMembershipAccessIntentAndPrivateFormatting(t *testing.T) {
	for _, raw := range []string{`{"schema_version":1,"reason":null}`, `{"SchemaVersion":1}`, `{"reason":"synthetic","reason":"different"}`, `{"reason":"synthetic","actor_id":"foreign"}`, `{"state":"ACTIVE"}`} {
		var request membershipAccessRequest
		if httpapi.DecodeJSON([]byte(raw), &request) == nil {
			t.Fatal("ambiguous/private authority input accepted")
		}
	}
	var request membershipAccessRequest
	if e := httpapi.DecodeJSON([]byte(`{"schema_version":1,"operation_id":"7b0e2a54-0000-4000-8000-000000001021","revision":2,"reason":"Synthetic private membership reason"}`), &request); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(request)
	if strings.Contains(string(raw), "Synthetic private membership reason") || strings.Contains(fmt.Sprintf("%+v %#v", request, request), "Synthetic private membership reason") {
		t.Fatal("reason leaked by generic formatting")
	}
	for _, tc := range []struct {
		e    error
		code orgapi.Code
	}{{d.ErrSelfSuspension, orgapi.Forbidden}, {kernel.ErrLastOrgOwner, orgapi.LastOwner}, {app.ErrAccessOperationReused, orgapi.OperationReused}, {d.ErrConditions, orgapi.Conflict}, {d.ErrEnded, orgapi.Conflict}} {
		if !errors.Is(membershipAccessMapped(tc.e), tc.code) {
			t.Fatal("bounded error mapping")
		}
	}
}
