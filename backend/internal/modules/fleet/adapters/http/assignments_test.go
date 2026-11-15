package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	company "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/companysession/adapters/http"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type unavailableAssignments struct{}

func (*unavailableAssignments) CreateAssignments(context.Context, kernel.OrgScope, app.FleetActor, string, []string, app.AssignmentCreate) (app.AssignmentResult, bool, error) {
	return app.AssignmentResult{}, false, app.ErrUnavailable
}
func (*unavailableAssignments) EndAssignment(context.Context, kernel.OrgScope, app.FleetActor, string, app.AssignmentEnd) (app.AssignmentResult, bool, error) {
	return app.AssignmentResult{}, false, app.ErrUnavailable
}
func (*unavailableAssignments) ListAssetAssignments(context.Context, kernel.OrgScope, app.FleetActor, string, string, app.AssignmentFilter) (app.AssignmentPage, error) {
	return app.AssignmentPage{}, app.ErrUnavailable
}
func (*unavailableAssignments) ListOwnAssignments(context.Context, kernel.OrgScope, app.FleetActor, string, app.OwnAssignmentFilter) (app.AssignmentPage, error) {
	return app.AssignmentPage{}, app.ErrUnavailable
}

type assignmentAdmission struct{}

func (assignmentAdmission) Validate(rule orgapi.Rule) error {
	if rule.Permission != "fleet.asset.assign" || !rule.Write || !rule.Fresh || !rule.CompanyOnly {
		return orgapi.Unavailable
	}
	return nil
}
func (assignmentAdmission) Check(context.Context, orgapi.Principal, orgapi.Rule) error {
	return orgapi.Unavailable
}

func TestAssignmentRegistrationPinsFourCompanyPolicies(t *testing.T) {
	_, cookies, gateway, auth := registrationFixture(t)
	gateway.WriteAdmission = assignmentAdmission{}
	service := app.AssignmentService{Enabled: true, Repository: &unavailableAssignments{}, NewID: func() (string, error) { return "", app.ErrUnavailable }}
	router := chi.NewRouter()
	if err := RegisterAssetAssignments(router, service, cookies, gateway, auth); err != nil {
		t.Fatal("complete assignment registration refused", err)
	}
	read := orgapi.Rule{Permission: "fleet.asset.read", CompanyOnly: true, Success: 200}
	want := map[string]orgapi.Rule{
		AssignmentCreatePath:  {Permission: "fleet.asset.assign", Write: true, Fresh: true, CompanyOnly: true, Success: 201},
		AssignmentEndPath:     {Permission: "fleet.asset.assign", Write: true, Fresh: true, CompanyOnly: true, Success: 200},
		AssignmentListPath:    read,
		OwnAssignmentListPath: read,
	}
	seen := 0
	if err := chi.Walk(router, func(method, path string, handler http.Handler, _ ...func(http.Handler) http.Handler) error {
		descriptor, ok := handler.(orgapi.Descriptor)
		if method != http.MethodPost || !ok || descriptor.OrganizationRule() != want[path] {
			t.Fatal("assignment source policy drift", path)
		}
		if completion, ok := handler.(orgapi.AuthorityCompletion); !ok || !completion.CompletesAuthority() {
			t.Fatal("assignment route lacks complete current authority", path)
		}
		seen++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if seen != len(want) {
		t.Fatal("assignment route count changed", seen)
	}
	for name, change := range map[string]func(*app.AssignmentService, *access.Authorization){
		"repository":       func(s *app.AssignmentService, _ *access.Authorization) { s.Repository = nil },
		"typed repository": func(s *app.AssignmentService, _ *access.Authorization) { s.Repository = (*unavailableAssignments)(nil) },
		"identifiers":      func(s *app.AssignmentService, _ *access.Authorization) { s.NewID = nil },
		"source":           func(_ *app.AssignmentService, a *access.Authorization) { a.Source = nil },
		"records":          func(_ *app.AssignmentService, a *access.Authorization) { a.Records = (*unavailableRecords)(nil) },
		"capabilities":     func(_ *app.AssignmentService, a *access.Authorization) { a.Capabilities = nil },
		"clock":            func(_ *app.AssignmentService, a *access.Authorization) { a.Now = nil },
	} {
		s, a := service, auth
		change(&s, &a)
		if RegisterAssetAssignments(chi.NewRouter(), s, cookies, gateway, a) == nil {
			t.Fatal("incomplete assignment registration accepted", name)
		}
	}
	if RegisterAssetAssignments(nil, service, cookies, gateway, auth) == nil {
		t.Fatal("assignment registration without router accepted")
	}
	hidden := chi.NewRouter()
	if RegisterAssetAssignments(hidden, app.AssignmentService{}, company.Handler{}, orgapi.Gateway{}, access.Authorization{}) != nil || len(hidden.Routes()) != 0 {
		t.Fatal("disabled assignments exposed routes or required live sources")
	}
}

const (
	assignmentOperation = "7b0e2a54-0000-4000-8000-000000009001"
	assignmentAsset     = "7b0e2a54-0000-4000-8000-000000009101"
	assignmentEmployee  = "7b0e2a54-0000-4000-8000-000000009201"
	assignmentOther     = "7b0e2a54-0000-4000-8000-000000009202"
	assignmentID        = "7b0e2a54-0000-4000-8000-000000009301"
)

func TestAssignmentRequestsAreStrictAndExact(t *testing.T) {
	var create assignmentCreateRequest
	raw := `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"TEMPORARY","employee_ids":["` + assignmentEmployee + `","` + assignmentOther + `"],"valid_from":"2026-12-01T03:00:00.123456Z","valid_to":"2026-12-08T03:00:00Z","reason":"Synthetic holiday cover"}`
	if err := json.Unmarshal([]byte(raw), &create); err != nil || create.Credentials().AccessToken != "" {
		t.Fatal("canonical create refused", err)
	}
	command, err := create.command(assignmentAsset)
	if err != nil {
		t.Fatal("canonical create command refused", err)
	}
	in, _ := command.Snapshot()
	if in.Kind != d.TemporaryAssignment || len(in.EmployeeIDs) != 2 || in.Start == nil || !in.Start.Equal(time.Date(2026, 12, 1, 3, 0, 0, 123456000, time.UTC)) || in.End == nil || in.Reason != "Synthetic holiday cover" || in.ReplacePrimary || in.AssetID != assignmentAsset {
		t.Fatal("create command lost exact intent", in)
	}
	var primary assignmentCreateRequest
	if err := json.Unmarshal([]byte(`{"schema_version":1,"operation_id":"`+assignmentOperation+`","kind":"PRIMARY","employee_ids":["`+assignmentEmployee+`"],"replace_primary":true}`), &primary); err != nil {
		t.Fatal(err)
	}
	if c, err := primary.command(assignmentAsset); err != nil {
		t.Fatal("replace primary refused", err)
	} else if v, _ := c.Snapshot(); !v.ReplacePrimary || v.Start != nil || v.End != nil {
		t.Fatal("replace primary intent", v)
	}
	for name, body := range map[string]string{
		"unknown key":     `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"PRIMARY","employee_ids":["` + assignmentEmployee + `"],"unit_id":"x"}`,
		"state chosen":    `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"PRIMARY","employee_ids":["` + assignmentEmployee + `"],"state":"ACTIVE"}`,
		"alias":           `{"schema_version":1,"operation_id":"` + assignmentOperation + `","Kind":"PRIMARY","employee_ids":["` + assignmentEmployee + `"]}`,
		"duplicate key":   `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"PRIMARY","kind":"ADDITIONAL","employee_ids":["` + assignmentEmployee + `"]}`,
		"scalar employee": `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"PRIMARY","employee_ids":"` + assignmentEmployee + `"}`,
		"null":            `null`,
		"trailing":        `{} {}`,
	} {
		var v assignmentCreateRequest
		if err := v.UnmarshalJSON([]byte(body)); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("ambiguous create accepted", name, err)
		}
	}
	for name, body := range map[string]string{
		"lower kind":        `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"primary","employee_ids":["` + assignmentEmployee + `"]}`,
		"local start":       `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"PRIMARY","employee_ids":["` + assignmentEmployee + `"],"valid_from":"2026-12-01T03:00:00"}`,
		"empty reason":      `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"PRIMARY","employee_ids":["` + assignmentEmployee + `"],"reason":""}`,
		"temporary no end":  `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"TEMPORARY","employee_ids":["` + assignmentEmployee + `"]}`,
		"no employees":      `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"ADDITIONAL","employee_ids":[]}`,
		"replace false set": `{"schema_version":1,"operation_id":"` + assignmentOperation + `","kind":"ADDITIONAL","employee_ids":["` + assignmentEmployee + `"],"replace_primary":true}`,
	} {
		var v assignmentCreateRequest
		if err := v.UnmarshalJSON([]byte(body)); err != nil {
			t.Fatal("well-formed create refused at decoding", name, err)
		}
		if _, err := v.command(assignmentAsset); !errors.Is(err, d.ErrInvalid) {
			t.Fatal("invalid create accepted", name, err)
		}
	}
	var end assignmentEndRequest
	if err := json.Unmarshal([]byte(`{"schema_version":1,"operation_id":"`+assignmentOperation+`","reason":"Synthetic return"}`), &end); err != nil || end.Reason == nil || *end.Reason != "Synthetic return" {
		t.Fatal("canonical end refused", err)
	}
	for name, body := range map[string]string{"valid_to chosen": `{"schema_version":1,"operation_id":"` + assignmentOperation + `","valid_to":"2026-12-01T03:00:00Z"}`, "revision": `{"schema_version":1,"operation_id":"` + assignmentOperation + `","revision":2}`, "array": `[]`} {
		var v assignmentEndRequest
		if err := v.UnmarshalJSON([]byte(body)); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("ambiguous end accepted", name, err)
		}
	}
	var list assignmentListRequest
	if err := json.Unmarshal([]byte(`{"limit":5,"cursor":"opaque"}`), &list); err != nil || list.Limit != 5 {
		t.Fatal("canonical page refused", err)
	}
	for name, body := range map[string]string{"employee filter": `{"employee_id":"x"}`, "instant": `{"at":"2026-12-01T03:00:00Z"}`, "state": `{"state":"ACTIVE"}`} {
		var v assignmentListRequest
		if err := v.UnmarshalJSON([]byte(body)); !errors.Is(err, orgapi.Malformed) {
			t.Fatal("caller-selected list filter accepted", name, err)
		}
	}
}

func TestAssignmentProjectionAndRefusalsAreExact(t *testing.T) {
	scope := projectionScope(t, "7b0e2a54-0000-4000-8000-000000008001")
	at := time.Date(2026, 11, 14, 12, 0, 0, 5000, time.UTC)
	from := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 10, 3, 0, 0, 0, time.UTC)
	closed, _ := d.NewAssignmentPeriod(from, &end)
	open, _ := d.NewAssignmentPeriod(from, nil)
	ended := end.Add(-time.Hour)
	page := app.AssignmentPage{At: at, Assignments: []app.AssignmentView{
		{ID: assignmentID, AssetID: assignmentAsset, EmployeeID: assignmentEmployee, Kind: d.PrimaryAssignment, Period: closed, State: d.AssignmentEnded, Reason: "Synthetic", RecordedAt: from, EndCause: app.EndedByTermination, EndedAt: &ended},
		{ID: assignmentOperation, AssetID: assignmentAsset, EmployeeID: assignmentOther, Kind: d.AdditionalAssignment, Period: open, State: d.AssignmentActive, RecordedAt: from},
	}}
	items, err := assignmentItems(scope, page)
	if err != nil || len(items) != 2 {
		t.Fatal("valid assignment projection refused", err)
	}
	raw, _ := json.Marshal(items)
	want := `[{"asset_id":"` + assignmentAsset + `","assignment_id":"` + assignmentID + `","employee_id":"` + assignmentEmployee + `","end_cause":"EMPLOYEE_TERMINATED","ended_at":"2026-11-10T02:00:00Z","kind":"PRIMARY","reason":"Synthetic","recorded_at":"2026-11-01T03:00:00Z","state":"ENDED","valid_from":"2026-11-01T03:00:00Z","valid_to":"2026-11-10T03:00:00Z"},` +
		`{"asset_id":"` + assignmentAsset + `","assignment_id":"` + assignmentOperation + `","employee_id":"` + assignmentOther + `","kind":"ADDITIONAL","reason":"","recorded_at":"2026-11-01T03:00:00Z","state":"ACTIVE","valid_from":"2026-11-01T03:00:00Z"}]`
	if string(raw) != want {
		t.Fatal("assignment projection changed", string(raw))
	}
	for name, edit := range map[string]func(*app.AssignmentView){
		"state drift": func(v *app.AssignmentView) { v.State = d.AssignmentActive },
		"bad id":      func(v *app.AssignmentView) { v.EmployeeID = "Synthetic driver" },
		"cause alone": func(v *app.AssignmentView) { v.EndedAt = nil },
	} {
		bad := page
		bad.Assignments = append([]app.AssignmentView(nil), page.Assignments...)
		edit(&bad.Assignments[0])
		if _, err := assignmentItems(scope, bad); !errors.Is(err, orgapi.Unavailable) {
			t.Fatal("invalid assignment projected", name)
		}
	}
	if _, err := assignmentItems(kernel.OrgScope{}, page); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal("projection without scope")
	}
	ack, err := assignmentAck(app.AssignmentResult{AssetID: assignmentAsset, AssignmentIDs: []string{assignmentID}, ReplacedID: assignmentOperation}, true)
	if err != nil || ack["replaced_assignment_id"] != assignmentOperation || ack["replayed"] != true || ack["data_mode"] != "SYNTHETIC" || len(ack["assignment_ids"].([]string)) != 1 {
		t.Fatal("assignment acknowledgement", ack, err)
	}
	if plain, err := assignmentAck(app.AssignmentResult{AssetID: assignmentAsset, AssignmentIDs: []string{assignmentID}}, false); err != nil || plain["replaced_assignment_id"] != nil || len(plain) != 4 {
		t.Fatal("plain acknowledgement carries a replacement", plain, err)
	}
	if _, err := assignmentAck(app.AssignmentResult{AssetID: assignmentAsset}, false); !errors.Is(err, orgapi.Unavailable) {
		t.Fatal("empty acknowledgement published")
	}
	for source, want := range map[error]orgapi.Code{
		app.ErrAssignmentOverlap: orgapi.AssignmentOverlap, d.ErrAssignmentEnded: orgapi.Conflict, app.ErrNotFound: orgapi.NotFound,
		d.ErrInvalid: orgapi.ValidationFailed, app.ErrOperationReused: orgapi.OperationReused, app.ErrSurfaceDisabled: orgapi.NotFound,
		errors.New("Synthetic private SQL"): orgapi.Unavailable,
	} {
		if code, _ := orgapi.Registered(assignmentMapped(source)); code != want {
			t.Fatal("assignment refusal mapping changed", source, want)
		}
	}
	writer := httptest.NewRecorder()
	orgapi.WriteError(writer, httptest.NewRequest(http.MethodPost, "/", nil), assignmentMapped(app.ErrAssignmentOverlap))
	if writer.Code != 409 || !strings.Contains(writer.Body.String(), `"code":"ASSIGNMENT_OVERLAP"`) {
		t.Fatal("ASSIGNMENT_OVERLAP status", writer.Code, writer.Body.String())
	}
	if !strings.HasPrefix(OwnAssignmentListPath, "/v1/orgs/{org_id}/") || strings.Contains(OwnAssignmentListPath, "{membership_id}") {
		t.Fatal("own list selects a membership instead of the requester")
	}
}

func TestOwnAssignmentsResourceIsTheRequestingMembership(t *testing.T) {
	scope := projectionScope(t, "7b0e2a54-0000-4000-8000-000000008001")
	if _, err := (ownAssignmentsResource{}).Resolve(t.Context(), scope, access.Subject{}); err == nil {
		t.Fatal("own list resolved without a subject")
	}
	if _, err := (ownAssignmentsResource{}).Resolve(t.Context(), kernel.OrgScope{}, access.Subject{}); err == nil {
		t.Fatal("own list resolved without a scope")
	}
}
