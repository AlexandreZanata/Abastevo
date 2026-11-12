package adapters

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"

	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

func TestUnitUsageDecodingRefusesUnknownIncompleteOrNonIntegerCounts(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{}`, `{"assets":0,"employees":0,"budgets":0}`, `{"assets":0,"employees":0,"budgets":0,"operations":0,"private":"ignored"}`, `{"assets":null,"employees":0,"budgets":0,"operations":0}`, `{"assets":-1,"employees":0,"budgets":0,"operations":0}`, `{"assets":1.5,"employees":0,"budgets":0,"operations":0}`, `{"assets":"1","employees":0,"budgets":0,"operations":0}`, `{"assets":9223372036854775808,"employees":0,"budgets":0,"operations":0}`} {
		if _, err := decodeUnitUsage([]byte(raw)); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("invalid source usage accepted")
		}
	}
	value, err := decodeUnitUsage([]byte(`{"assets":9223372036854775807,"employees":2,"budgets":0,"operations":1}`))
	if err != nil || value != (d.UnitUsage{Assets: 9223372036854775807, Employees: 2, Operations: 1}) {
		t.Fatal("exact usage counts lost", err)
	}
}
func TestUnitAuditChangeReasonsAreExactSourceRegistrations(t *testing.T) {
	catalog, err := audit.NewCatalog(UnitAuditDefinitions())
	if err != nil {
		t.Fatal(err)
	}
	event := audit.Event{OperationID: "7b0e2a54-0000-4000-8000-000000001005", ActorID: "7b0e2a54-0000-4000-8000-000000000201", TargetID: "7b0e2a54-0000-4000-8000-000000000801", TargetType: "unit", Action: "org.unit.changed", RequestHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	for _, reason := range []string{"metadata", "restored"} {
		event.ReasonCode = reason
		if err := catalog.Validate(event); err != nil {
			t.Fatal("registered unit reason refused", err)
		}
	}
	for _, reason := range []string{"", "unknown", "private free text"} {
		event.ReasonCode = reason
		if catalog.Validate(event) == nil {
			t.Fatal("unregistered unit audit reason accepted")
		}
	}
}

func TestUnitErrorsKeepDuplicateFieldsPrivateAndCancellationCurrent(t *testing.T) {
	for _, test := range []struct{ code, name, field string }{{"P7701", "", "name"}, {"P7702", "", "code"}, {"23505", "org_units_active_name", "name"}, {"23505", "org_units_active_code", "code"}} {
		err := unitError(context.Background(), &pgconn.PgError{Code: test.code, ConstraintName: test.name, Detail: "Synthetic private row"})
		var duplicate app.UnitDuplicateError
		if !errors.As(err, &duplicate) || duplicate.Field() != test.field {
			t.Fatal("duplicate field not bounded")
		}
	}
	if !errors.Is(unitError(context.Background(), &pgconn.PgError{Code: "23505", ConstraintName: "unregistered_private_index"}), app.ErrUnavailable) {
		t.Fatal("unknown unique failure misreported as a declared identifier")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(unitError(ctx, d.ErrLastUnit), context.Canceled) {
		t.Fatal("cancellation lost")
	}
}
