package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
)

func TestCreationRejectsNonCanonicalTransportBeforeDispatch(t *testing.T) {
	for _, body := range []string{"", `[]`, `null`, `{"schema_version":null}`, `{"schema_version":1,"Schema_version":1}`, `{"schema_version":1,"account_id":"synthetic"}`, `{"schema_version":1,"terms":{"Accepted":true}}`, `{"schema_version":1} {}`} {
		endpoint := creationEndpoint{service: app.Onboarding{Enabled: true}}
		r := httptest.NewRequest(http.MethodPost, CreationPath, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		endpoint.ServeHTTP(w, r)
		if w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "" {
			t.Fatal("malformed creation dispatched", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	(creationEndpoint{}).ServeHTTP(w, httptest.NewRequest(http.MethodPost, CreationPath, nil))
	if w.Code != 404 {
		t.Fatal("disabled entry exposed")
	}
}
func TestOnboardingErrorsRemainBoundedAndSourceRegistered(t *testing.T) {
	for _, test := range []struct {
		source error
		want   orgapi.Code
	}{
		{kernel.ErrPendingOrgLimit, orgapi.RateLimited}, {kernel.ErrOwnerRequiresPersonal, orgapi.OwnerRequiresPersonal},
		{app.ErrOperationReused, orgapi.OperationReused}, {d.ErrSetup, orgapi.OrgSetupIncomplete}, {d.ErrTerms, orgapi.ValidationFailed},
		{errors.New("private SQL or provider detail"), orgapi.Unavailable},
	} {
		if mapped(test.source) != test.want {
			t.Fatal("source refusal lost", test.source)
		}
	}
}

func TestPrivateCreationRequestRedactsEveryFormattingPath(t *testing.T) {
	input := creationRequest{Name: "synthetic-private-company", UnitName: "synthetic-private-unit"}
	for _, verb := range []string{"%v", "%+v", "%#v", "%d"} {
		if text := fmt.Sprintf(verb, input); strings.Contains(text, "synthetic-private") {
			t.Fatal("request formatting leaked private text")
		}
	}
	raw, err := json.Marshal(map[string]any{"input": input})
	if err != nil || strings.Contains(string(raw), "synthetic-private") {
		t.Fatal("nested request encoding leaked", err)
	}
}

func TestIncompleteSetupReportsOnlySourceChecklistFields(t *testing.T) {
	id, err := kernel.ParseOrgID("7b0e2a54-0000-4000-8000-000000000301")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(id)
	if err != nil {
		t.Fatal(err)
	}
	root, err := d.NewPending(scope, d.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = root.Transition(scope, 1, d.Active, d.Conditions{})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, ActivationPath, nil)
	orgapi.WriteError(w, r, mapped(err))
	var body struct {
		Error struct {
			Details []struct{ Field, Code string }
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 409 || len(body.Error.Details) != 5 {
		t.Fatal("missing setup checklist", w.Code, w.Body.String())
	}
	for _, detail := range body.Error.Details {
		if detail.Code != "REQUIRED" {
			t.Fatal("unbounded checklist detail")
		}
	}
}
