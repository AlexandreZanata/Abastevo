package http

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

func exportID(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

// EDR-157: refusals map to registered codes; provider details stay private.
func TestExportErrorsMapToRegisteredCodes(t *testing.T) {
	for _, c := range []struct {
		err  error
		want orgapi.Code
	}{
		{app.ErrFormatUnavailable, orgapi.ExportFormatUnavailable},
		{app.ErrQueueFull, orgapi.ExportQueueFull},
		{app.ErrNotReady, orgapi.ExportNotReady},
		{app.ErrExpired, orgapi.ExportExpired},
		{app.ErrNotFound, orgapi.NotFound},
		{app.ErrSurfaceDisabled, orgapi.NotFound},
		{app.ErrOperationReused, orgapi.OperationReused},
		{app.ErrUnauthenticated, orgapi.Unauthenticated},
		{app.ErrUnavailable, orgapi.Unavailable},
		{domain.ErrInvalid, orgapi.ValidationFailed},
		{access.ErrNoMembership, orgapi.NotFound},
		{access.ErrMembershipSuspended, orgapi.MembershipSuspended},
		{access.ErrOrgSuspended, orgapi.OrgSuspended},
		{access.ErrOrgArchived, orgapi.OrgArchived},
		{access.ErrSetupIncomplete, orgapi.OrgSetupIncomplete},
		{access.ErrFreshAuthRequired, orgapi.FreshAuthRequired},
		{access.ErrEntitlementRequired, orgapi.EntitlementRequired},
		{orgapi.Malformed, orgapi.Malformed},
		{errors.New("synthetic private provider detail"), orgapi.Unavailable},
	} {
		if got, ok := orgapi.Registered(Mapped(c.err)); !ok || got != c.want {
			t.Fatalf("%v mapped to %v, want %v", c.err, got, c.want)
		}
	}
	denied := permissions.DenyPermission("rpt.export.run")
	if got := Mapped(denied); !errors.Is(got, orgapi.Forbidden) || !strings.Contains(fmt.Sprintf("%#v", got), "rpt.export.run") {
		t.Fatal("permission denial lost its registered key", got)
	}
	for code, status := range map[orgapi.Code]int{orgapi.ExportQueueFull: 429, orgapi.ExportNotReady: 409, orgapi.ExportExpired: 410, orgapi.ExportFormatUnavailable: 422} {
		w := httptest.NewRecorder()
		orgapi.WriteError(w, httptest.NewRequest("POST", "/", nil), code)
		if w.Code != status || !strings.Contains(w.Body.String(), `"code":"`+string(code)+`"`) {
			t.Fatal("export code status drifted", code, w.Code)
		}
	}
}

func TestExportRequestsDecodeStrictly(t *testing.T) {
	var request exportRequest
	if err := decodeExport([]byte(`{"schema_version":1,"operation_id":"`+exportID(1)+`","kind":"test.synthetic_rows","parameters":{"rows":2,"unit":"x"}}`), &request); err != nil || request.Kind != "test.synthetic_rows" || string(request.Parameters) != `{"rows":2,"unit":"x"}` {
		t.Fatal("valid request refused", err)
	}
	for _, raw := range []string{
		`{"schema_version":1,"operation_id":"` + exportID(1) + `","kind":"test.synthetic_rows"}`,
		`{"schema_version":1,"operation_id":"` + exportID(1) + `","kind":"test.synthetic_rows","parameters":{},"requester":"x"}`,
		`{"schema_version":1,"operation_id":"` + exportID(1) + `","kind":"test.synthetic_rows","parameters":{},"parameters":{}}`,
		`{"schema_version":1,"operation_id":"` + exportID(1) + `","kind":"test.synthetic_rows","parameters":null}`,
		`{"schema_version":1,"operation_id":"` + exportID(1) + `","kind":null,"parameters":{}}`,
		`[]`,
	} {
		var v exportRequest
		if decodeExport([]byte(raw), &v) == nil {
			t.Fatal("non-canonical request accepted", raw)
		}
	}
	var list exportListRequest
	if err := decodeExport([]byte(`{"limit":2,"state":"READY"}`), &list); err != nil || list.Limit != 2 || list.State != "READY" {
		t.Fatal("list request", err)
	}
	var read exportReadRequest
	if err := decodeExport([]byte(`{}`), &read); err != nil || decodeExport([]byte(`{"export_id":"`+exportID(1)+`"}`), &read) == nil {
		t.Fatal("read must take only its path selector", err)
	}
	var content exportContentRequest
	if err := decodeExport([]byte(`{"capability":"synthetic"}`), &content); err != nil || content.Capability != "synthetic" {
		t.Fatal("content request", err)
	}
	if decodeExport([]byte(`{}`), &content) == nil || decodeExport([]byte(`{"capability":""}`), &content) == nil {
		t.Fatal("content without capability accepted")
	}
}

func TestExportProjectionsAreExactAndFailClosed(t *testing.T) {
	at := time.Date(2026, 11, 14, 12, 0, 0, 123456789, time.UTC)
	ready := app.View{ExportID: exportID(50), Kind: "test.synthetic_rows", Parameters: []byte(`{"rows":2}`), ParametersDigest: strings.Repeat("a", 64), State: domain.Ready, Attempts: 1, MaxAttempts: 3,
		Rows: 2, Bytes: 34, SHA256: strings.Repeat("b", 64), MediaType: "text/csv", RequestedAt: at, AsOf: at, ReadyAt: at.Add(time.Minute), ExpiresAt: at.Add(domain.DefaultRetention + time.Minute)}
	out, err := projectView(ready)
	if err != nil || len(out) != 19 || out["export_id"] != exportID(50) || out["state"] != domain.Ready || out["row_count"] != int64(2) || out["byte_count"] != int64(34) ||
		out["sha256"] != strings.Repeat("b", 64) || out["requested_at"] != "2026-11-14T12:00:00.123456789Z" || out["failure_code"] != nil || out["expired_at"] != nil || out["data_mode"] != "SYNTHETIC" {
		t.Fatal("ready projection", out, err)
	}
	for _, forbidden := range []string{"key", "url", "org/", "lease", "requester_account_id", "requester_membership_id"} {
		for field := range out {
			if strings.Contains(field, forbidden) {
				t.Fatal("projection exposes a private field", field)
			}
		}
	}
	queued := app.View{ExportID: exportID(51), Kind: "test.synthetic_rows", Parameters: []byte(`{}`), ParametersDigest: strings.Repeat("a", 64), State: domain.Queued, MaxAttempts: 3, RequestedAt: at, AsOf: at}
	q, err := projectView(queued)
	if err != nil || q["sha256"] != nil || q["row_count"] != nil || q["ready_at"] != nil || q["expires_at"] != nil {
		t.Fatal("queued projection", q, err)
	}
	ready.SHA256 = ""
	if _, err := projectView(ready); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal("inconsistent view projected")
	}
	capability := app.Capability{Token: "synthetic-capability", ExpiresAt: at}
	c, err := projectCapability(exportID(50), capability)
	if err != nil || len(c) != 3 || c["capability"] != "synthetic-capability" || c["expires_at"] != "2026-11-14T12:00:00.123456789Z" || c["export_id"] != exportID(50) {
		t.Fatal("capability projection", c, err)
	}
	content := app.Content{ExportID: exportID(50), MediaType: "text/csv", SHA256: strings.Repeat("b", 64), Rows: 2, Bytes: []byte("a,b\n1,2\n")}
	body, err := projectContent(content)
	if err != nil || len(body) != 6 || body["content_base64"] != base64.StdEncoding.EncodeToString(content.Bytes) || body["byte_count"] != int64(8) || body["media_type"] != "text/csv" {
		t.Fatal("content projection", body, err)
	}
	if _, err := projectContent(app.Content{ExportID: exportID(50)}); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal("empty content projected")
	}
}

func TestExportRecordsAndRegistrationFailClosed(t *testing.T) {
	org, _ := kernel.ParseOrgID(exportID(1))
	scope, _ := kernel.NewOrgScope(org)
	facts := permissions.ResourceFacts{OrgID: scope.OrgID(), ID: scope.OrgID().String()}
	rules := []permissions.Rule{{}}
	if (ExportRecords{}).Check(context.Background(), scope, access.Subject{}, app.Policy(app.ActionRead), facts, rules) == nil {
		t.Fatal("record check accepted a subject outside the scope")
	}
	if (ExportRecords{}).Check(context.Background(), kernel.OrgScope{}, access.Subject{}, app.Policy(app.ActionRequest), facts, rules) == nil {
		t.Fatal("record check accepted an invalid scope")
	}
	if err := RegisterExports(chi.NewRouter(), app.Service{}, company.Handler{}, orgapi.Gateway{}, access.Authorization{}); err != nil {
		t.Fatal("disabled export surface must mount nothing", err)
	}
	if err := RegisterExports(chi.NewRouter(), app.Service{Enabled: true}, company.Handler{}, orgapi.Gateway{}, access.Authorization{}); err == nil {
		t.Fatal("incomplete export surface registered")
	}
}
