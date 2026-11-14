package adapters

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	"github.com/jackc/pgx/v5"
)

type followupWriter struct{}

func (*followupWriter) Enqueue(context.Context, kernel.OrgScope, pgx.Tx, event.Envelope, string) error {
	return errors.New("planning must not enqueue")
}

func followupFixture(t *testing.T) (app.Source, protocol.Operation, app.Descriptor, Config) {
	t.Helper()
	const prefix = "7b0e2a54-0000-4000-8000-0000000000"
	org, err := kernel.ParseOrgID(prefix + "65")
	if err != nil {
		t.Fatal(err)
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(prefix+"01", "https://api.example.invalid", org.String())
	if err != nil {
		t.Fatal(err)
	}
	op, err := protocol.Seal([]byte(`{"schema_version":1,"operation_id":"`+prefix+`21","owner_account_id":"`+prefix+`01","api_origin":"https://api.example.invalid","org_id":"`+prefix+`65","membership_id":"`+prefix+`03","enrollment_id":"`+prefix+`04","type":"validation.setting","operation_schema":1,"entity":{"type":"setting","id":"`+prefix+`05","expected_revision":null},"occurred_at":"2026-11-13T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{}}`), binding)
	if err != nil {
		t.Fatal(err)
	}
	source := app.Source{Scope: scope}
	source.Actor.AccountID = prefix + "01"
	descriptor := app.Descriptor{Type: "validation.setting", Schema: 1, WritePolicy: access.Policy{Permission: "org.settings.manage", Capability: "organizations", Effect: access.ConfigurationWrite}, ReadPolicy: access.Policy{Permission: "org.settings.read", Capability: "organizations", Effect: access.Read}, TerminalResults: []string{protocol.Accepted, protocol.Rejected}}
	catalog, err := audit.NewCatalog(AuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	return source, op, descriptor, Config{AuditCatalog: catalog, NoAsyncPlans: map[app.OperationKey]map[string]app.FollowupMode{{Type: descriptor.Type, Schema: descriptor.Schema}: {protocol.Accepted: app.NoAsyncFollowup, protocol.Rejected: app.NoAsyncFollowup}}}
}

func TestExplicitNoAsyncRequiresExactFrozenSourceResult(t *testing.T) {
	source, op, descriptor, config := followupFixture(t)
	store := NewStore(config)
	for _, result := range []string{protocol.Accepted, protocol.Rejected} {
		plan, bound, err := store.plan(source, op, descriptor, result)
		if err != nil || len(plan.Consumers) != 0 || bound.RequestHash != strings.TrimPrefix(op.PayloadDigest(), "sha256:") || bound.Action != action(result) {
			t.Fatal("explicit atomic result required a fabricated asynchronous producer", err)
		}
	}
	for _, result := range []string{protocol.AcceptedForReview, protocol.Retry, "UNKNOWN"} {
		if _, _, err := store.plan(source, op, descriptor, result); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("undeclared terminal result acquired readiness")
		}
	}
	delete(config.NoAsyncPlans[app.OperationKey{Type: descriptor.Type, Schema: 1}], protocol.Accepted)
	if _, _, err := store.plan(source, op, descriptor, protocol.Accepted); err != nil {
		t.Fatal("caller mutated captured followup declaration", err)
	}
}

func TestNoAsyncCannotReplaceMissingOrReviewProducer(t *testing.T) {
	for _, name := range []string{"absent", "other_type", "other_schema", "invalid_type", "review", "unknown_mode", "unknown_result", "missing_result", "missing_audit", "default_results", "default_accepted", "conflicting_async"} {
		t.Run(name, func(t *testing.T) {
			source, op, descriptor, config := followupFixture(t)
			key := app.OperationKey{Type: descriptor.Type, Schema: 1}
			result := protocol.Accepted
			switch name {
			case "absent":
				config.NoAsyncPlans = nil
			case "other_type":
				config.NoAsyncPlans = map[app.OperationKey]map[string]app.FollowupMode{{Type: "other.setting", Schema: 1}: {result: app.NoAsyncFollowup}}
			case "other_schema":
				config.NoAsyncPlans = map[app.OperationKey]map[string]app.FollowupMode{{Type: descriptor.Type, Schema: 2}: {result: app.NoAsyncFollowup}}
			case "invalid_type":
				config.NoAsyncPlans = map[app.OperationKey]map[string]app.FollowupMode{{Type: "setting", Schema: 1}: {result: app.NoAsyncFollowup}}
			case "review":
				descriptor.TerminalResults = []string{protocol.AcceptedForReview, protocol.Rejected}
				result = protocol.AcceptedForReview
				config.NoAsyncPlans[key][result] = app.NoAsyncFollowup
			case "unknown_mode":
				config.NoAsyncPlans[key][result] = "NO_FOLLOWUP_TYPO"
			case "unknown_result":
				config.NoAsyncPlans[key]["UNKNOWN"] = app.NoAsyncFollowup
			case "missing_result":
				delete(config.NoAsyncPlans[key], result)
			case "missing_audit":
				config.AuditCatalog = audit.Catalog{}
			case "default_results":
				descriptor.TerminalResults = nil
				result = protocol.AcceptedForReview
			case "default_accepted":
				descriptor.TerminalResults = nil
			case "conflicting_async":
				config.Plans = map[string]map[string][]string{descriptor.Type: {result: {"validation.sync"}}}
			}
			if _, _, err := NewStore(config).plan(source, op, descriptor, result); !errors.Is(err, app.ErrUnavailable) {
				t.Fatal("missing or invalid followup producer became ready")
			}
		})
	}
}

func TestRequiredAsyncPlanKeepsConsumerAndWriterObligations(t *testing.T) {
	source, op, descriptor, config := followupFixture(t)
	config.NoAsyncPlans = nil
	catalog, err := event.NewCatalog(EventDefinitions([]string{"validation.sync"}))
	if err != nil {
		t.Fatal(err)
	}
	config.EventCatalog = catalog
	config.EventWriter = &followupWriter{}
	config.Plans = map[string]map[string][]string{descriptor.Type: {protocol.Accepted: {"validation.sync"}}}
	plan, bound, err := NewStore(config).plan(source, op, descriptor, protocol.Accepted)
	if err != nil || len(plan.Consumers) != 1 || plan.Consumers[0] != "validation.sync" || bound.RequestHash == strings.TrimPrefix(op.PayloadDigest(), "sha256:") {
		t.Fatal("required async plan changed", err)
	}
	config.EventWriter = nil
	if _, _, err := NewStore(config).plan(source, op, descriptor, protocol.Accepted); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("missing required writer counted as readiness")
	}
	var absent *followupWriter
	config.EventWriter = absent
	if _, _, err := NewStore(config).plan(source, op, descriptor, protocol.Accepted); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("typed nil required writer counted as readiness")
	}
}
