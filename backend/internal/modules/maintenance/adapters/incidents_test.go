package adapters

import (
	"context"
	"errors"
	"testing"

	documentapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	documentdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/maintenance/application"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

// EDR-238: the owner composes only from real ports and has no held path.
func TestReportHandlerNeedsEveryPortAndHasNoHeldPath(t *testing.T) {
	if _, err := NewReportHandler(nil, nil, nil, nil, nil); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("handler composed without ports", err)
	}
	var store *IncidentStore
	if _, err := NewReportHandler(nil, nil, nil, nil, store); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("handler composed with a nil store", err)
	}
	d := (&reportHandler{}).Descriptor()
	if d.Validate() != nil || d.Type != "check.incident.report" || d.Schema != 1 || len(d.Results()) != 2 || d.Results()[0] != protocol.Accepted || d.Results()[1] != protocol.Rejected ||
		d.WritePolicy != app.ReportPolicy(app.ReportWrite) || d.ReadPolicy != app.ReportPolicy(app.ReportReceipt) {
		t.Fatal("descriptor drift")
	}
}

// One audit action per handling step on the incident_report target; the
// catalog accepts all four and nothing else (BR-AUD-004: no text).
func TestIncidentAuditActionsAreOnePerStep(t *testing.T) {
	definitions := IncidentAuditDefinitions()
	if len(definitions) != 4 {
		t.Fatal("audit definitions", len(definitions))
	}
	if _, err := audit.NewCatalog(definitions); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, d := range definitions {
		if d.TargetType != "incident_report" || seen[d.Action] || d.RequireReason {
			t.Fatal("audit definition", d.Action)
		}
		seen[d.Action] = true
	}
	for _, action := range []string{"check.report.acknowledged", "check.report.treated", "check.report.resolved", "check.report.dismissed"} {
		if !seen[action] {
			t.Fatal("missing audit action", action)
		}
	}
}

// EDR-239: the INCIDENT resolver answers only its kind, under the reporter's
// own check.incident.report, and refuses without a transaction.
func TestIncidentDocumentTargetsResolveOnlyTheirKind(t *testing.T) {
	r := IncidentDocumentTargets{}
	if p, err := r.DocumentTargetPermission(documentdomain.Incident); err != nil || p != "check.incident.report" {
		t.Fatal("permission", p, err)
	}
	if _, err := r.DocumentTargetPermission(documentdomain.Asset); err == nil {
		t.Fatal("another kind admitted")
	}
	targets := documentapp.Targets{documentdomain.Incident: r}
	for _, effect := range []access.Effect{access.OperationalWrite, access.Read} {
		policy, _, err := targets.Policy(documentdomain.Incident, effect)
		if err != nil || policy != (access.Policy{Permission: "check.incident.report", Capability: "organizations", Effect: effect}) {
			t.Fatal("owner policy", effect, err)
		}
	}
	if _, err := r.ResolveDocumentTargetTx(context.Background(), nil, kernel.OrgScope{}, documentdomain.Incident, "x"); !errors.Is(err, documentapp.ErrUnavailable) {
		t.Fatal("resolver without a transaction", err)
	}
	if b, ok := documentapp.BindIncident.Kind(); !ok || b != documentdomain.Incident {
		t.Fatal("binding")
	}
}

// Intake store methods refuse without a scoped borrowed transaction.
func TestIncidentStoreIntakeRefusesWithoutTransaction(t *testing.T) {
	s := &IncidentStore{}
	ctx := context.Background()
	if _, err := s.ReportOperationTx(ctx, nil, kernel.OrgScope{}, "x"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.TripReportsTx(ctx, nil, kernel.OrgScope{}, "x"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal(err)
	}
	if err := s.InsertReportTx(ctx, nil, kernel.OrgScope{}, syncapp.Source{}, app.ReportCommand{}, app.ReportFacts{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.ReadReportTx(ctx, nil, kernel.OrgScope{}, "x"); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal(err)
	}
	if _, _, err := s.DecideReport(ctx, kernel.OrgScope{}, app.IncidentActor{}, "x", app.DecisionCommand{}); err == nil {
		t.Fatal("decision without scope")
	}
}
