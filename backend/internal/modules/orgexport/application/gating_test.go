package application_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/domain"
)

// exportableReads is an independent literal of every org-permissions-v5 key a
// kind may declare as its data permission: the registered read keys outside
// location. Content, location, export and every write key are absent.
var exportableReads = []permissions.Permission{
	"audit.event.read", "auth.role.read", "budg.budget.read", "data.usage.read", "doc.document.read", "exp.expense.read", "find.finding.read", "fin.payable.read",
	"fleet.asset.read", "fuel.fueling.read", "idn.employee.read", "kpi.dashboard.read", "org.settings.read", "stn.metrics.read", "subs.plan.read", "trip.trip.read",
}

// B-BR-EMP-248, EDR-167: the closed declaration rule over the whole catalog.
func TestExportableReadAdmitsOnlyRegisteredDataReads(t *testing.T) {
	catalog := permissions.SystemCatalog()
	if _, version := catalog.Versions(); version != "org-permissions-v7" || len(catalog.Permissions()) != 85 {
		t.Fatal("catalog drift: revisit the exportable read literal", version)
	}
	admitted := 0
	for _, key := range catalog.Permissions() {
		if app.ExportableRead(key) != slices.Contains(exportableReads, key) {
			t.Fatal("exportable read classification drift", key)
		}
		if app.ExportableRead(key) {
			admitted++
		}
	}
	if admitted != len(exportableReads) {
		t.Fatal("exportable read count", admitted)
	}
	for _, key := range []permissions.Permission{"", "rpt.export.run", "doc.document.view_content", "gps.track.read", "gps.location.read_live", "gps.geofence.manage",
		"fleet.asset.manage", "exp.expense.record", "fin.period.close", "org.ownership.respond", "stock.reconcile", "invented.record.read", "Fleet.asset.read", "fleet.asset.read "} {
		if app.ExportableRead(key) {
			t.Fatal("non-data permission admitted as an export read", key)
		}
	}
}

// B-BR-EMP-248: a kind is admitted only with an exportable read declaration,
// and the registry answers the declared key for an admitted kind only.
func TestRegistryRequiresAnExportableReadPermission(t *testing.T) {
	for _, read := range []permissions.Permission{"", "rpt.export.run", "doc.document.view_content", "gps.track.read", "gps.location.read_live", "fleet.asset.manage", "invented.record.read"} {
		format := syntheticFormat()
		format.read = read
		if _, err := app.NewRegistry(format); !errors.Is(err, domain.ErrInvalid) {
			t.Fatal("kind with a non-data read permission admitted", read)
		}
	}
	plan := &rowsFormat{kind: "test.synthetic_plan_rows", read: "subs.plan.read", limits: domain.DefaultLimits()}
	r := registry(t, syntheticFormat(), plan)
	for kind, want := range map[string]permissions.Permission{"test.synthetic_rows": "exp.expense.read", "test.synthetic_plan_rows": "subs.plan.read"} {
		if got, ok := r.ReadPermission(kind); !ok || got != want {
			t.Fatal("declared read permission not returned", kind, got)
		}
	}
	if got, ok := r.ReadPermission("test.unknown_rows"); ok || got != "" {
		t.Fatal("unregistered kind has a read permission", got)
	}
	if got, ok := (app.Registry{}).ReadPermission("test.synthetic_rows"); ok || got != "" {
		t.Fatal("empty registry has a read permission", got)
	}
}

// BUC-EMP-127: the command carries the kind's read permission to the
// repository, which checks it in the request transaction.
func TestRequestCarriesTheKindReadPermission(t *testing.T) {
	plan := &rowsFormat{kind: "test.synthetic_plan_rows", read: "subs.plan.read", limits: domain.DefaultLimits()}
	r := registry(t, syntheticFormat(), plan)
	s := scopeOf(t, 1)
	for kind, want := range map[string]permissions.Permission{"test.synthetic_rows": "exp.expense.read", "test.synthetic_plan_rows": "subs.plan.read"} {
		c, err := app.NewCommand(s, r, app.CommandInput{OperationID: id(30), Kind: kind, Parameters: []byte(`{}`)})
		if err != nil || !c.Valid() || c.ReadPermission() != want {
			t.Fatal("command lost the declared read permission", kind, err)
		}
	}
	if (app.Command{}).ReadPermission() != "" {
		t.Fatal("zero command carries a read permission")
	}
	ctx, scope, actor, d, _, repo, _, _, _, service, _, _ := serviceFixture(t, app.ActionRequest)
	service.Formats = r
	in := app.CommandInput{OperationID: id(31), Kind: "test.synthetic_plan_rows", Parameters: []byte(`{}`)}
	c, _ := app.NewCommand(scope, r, in)
	at := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	repo.view = app.View{Kind: in.Kind, Parameters: c.Parameters(), ParametersDigest: c.ParametersDigest(), State: domain.Queued, MaxAttempts: 3, RequestedAt: at, AsOf: at}
	if _, _, err := service.Request(ctx, scope, actor, d, in); err != nil {
		t.Fatal(err)
	}
	if repo.requested.ReadPermission() != "subs.plan.read" {
		t.Fatal("repository did not receive the kind's read permission", repo.requested.ReadPermission())
	}
}

// B-BR-EMP-249: every delivery check receives the service's registry, so the
// repository resolves the stored kind's read permission in its transaction.
func TestDeliveryResolvesTheStoredKindThroughTheRegistry(t *testing.T) {
	ctx, s, actor, d, _, repo, _, _, _, service, object, _ := serviceFixture(t, app.ActionRead)
	capability, err := service.Access(ctx, s, actor, d, object.ExportID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Open(ctx, s, actor, d, object.ExportID, capability.Token); err != nil {
		t.Fatal(err)
	}
	if len(repo.formats) != 4 {
		t.Fatal("every delivery check must receive the registry", len(repo.formats))
	}
	for _, formats := range repo.formats {
		if read, ok := formats.ReadPermission("test.synthetic_rows"); !ok || read != "exp.expense.read" {
			t.Fatal("delivery check received another registry", read)
		}
	}
	// A refusal of the data permission is returned unchanged (FORBIDDEN with
	// the read key at the HTTP boundary) and mints nothing.
	ctx, s, actor, d, e, repo, _, tokens, recorder, service, object, _ := serviceFixture(t, app.ActionRead)
	repo.deliverErr = []error{permissions.DenyPermission("exp.expense.read")}
	_, err = service.Access(ctx, s, actor, d, object.ExportID)
	var denied *permissions.PermissionDenial
	if !errors.As(err, &denied) || denied.Permission() != "exp.expense.read" || tokens.issued != "" || len(recorder.records) != 0 || e.String() != "deliverable" {
		t.Fatal("data permission refusal minted or recorded", err, e)
	}
}

// B-BR-EMP-249: each worker attempt re-checks the declared read permission of
// the claimed kind; an unadmitted kind fails before any requester check.
func TestWorkerRechecksTheKindReadPermission(t *testing.T) {
	s, e, store, _, format, w := workerFixture(t, `{"rows":1}`)
	format.read = "audit.event.read"
	w.Formats = registry(t, format)
	if err := w.Build(t.Context(), s, id(50)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(store.checked, []permissions.Permission{"audit.event.read"}) || store.state != domain.Ready {
		t.Fatal("worker did not re-check the kind's read permission", store.checked, e)
	}
	s, e, store, _, _, w = workerFixture(t, `{}`)
	store.rightsErr = app.ErrRightsLost
	if err := w.Build(t.Context(), s, id(50)); err != nil || store.failure != domain.RightsLost || !slices.Equal(store.checked, []permissions.Permission{"exp.expense.read"}) {
		t.Fatal("lost data permission did not fail the job", err, store.failure, e)
	}
	s, e, store, _, _, w = workerFixture(t, `{}`)
	w.Formats = registry(t, &rowsFormat{kind: "test.other_rows", read: "exp.expense.read", limits: domain.DefaultLimits()})
	if err := w.Build(t.Context(), s, id(50)); err != nil || store.failure != domain.FormatUnavailable || len(store.checked) != 0 || e.String() != "claim,fail:"+string(domain.FormatUnavailable) {
		t.Fatal("unadmitted kind reached the requester check", err, e)
	}
}
