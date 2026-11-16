package adapters_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	expensepg "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/adapters"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/expense/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
)

type authority struct{}

func (*authority) CheckExpenseTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, app.Target, app.Action) error {
	return nil
}
func (*authority) ClassifyExpenseTx(context.Context, syncapp.DBTX, kernel.OrgScope, syncapp.Source, string, time.Time) (app.Disposition, error) {
	return app.Apply, nil
}

type lifecycle struct{}

func (lifecycle) MeterAssetDisposalTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (*time.Time, error) {
	return nil, errors.New("unexpected")
}

// The owner registers only with real ports, declares no review receipt and
// the catalog permissions of exp.expense (EDR-289); stores without their
// ports answer unavailable before any database call.
func TestIntakeHandlerNeedsRealPortsAndDeclaresItsOutcomes(t *testing.T) {
	var nilAuthority *authority
	var nilStore *expensepg.Store
	for name, build := range map[string]func() (syncapp.Handler, error){
		"nil-authority": func() (syncapp.Handler, error) {
			return expensepg.NewIntakeHandler(nil, lifecycle{}, &expensepg.Store{})
		},
		"typed-nil-auth": func() (syncapp.Handler, error) {
			return expensepg.NewIntakeHandler(nilAuthority, lifecycle{}, &expensepg.Store{})
		},
		"nil-lifecycle": func() (syncapp.Handler, error) {
			return expensepg.NewIntakeHandler(&authority{}, nil, &expensepg.Store{})
		},
		"typed-nil-store": func() (syncapp.Handler, error) {
			return expensepg.NewIntakeHandler(&authority{}, lifecycle{}, nilStore)
		},
	} {
		if handler, err := build(); handler != nil || !errors.Is(err, syncapp.ErrUnavailable) {
			t.Fatal("incomplete expense owner entered the registry", name)
		}
	}
	handler, err := expensepg.NewIntakeHandler(&authority{}, lifecycle{}, &expensepg.Store{})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := handler.Descriptor()
	if descriptor.Type != app.OperationType || descriptor.Schema != 1 || descriptor.Validate() != nil || descriptor.WritePolicy.Permission != "exp.expense.record" ||
		descriptor.ReadPolicy.Permission != "exp.expense.read" || !slices.Equal(descriptor.Results(), []string{protocol.Accepted, protocol.Rejected}) {
		t.Fatal("expense descriptor drift")
	}
	if _, err := handler.Apply(t.Context(), nil, syncapp.Source{}, protocol.Operation{}); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("handler applied without a transaction", err)
	}
	if err := handler.Admit(t.Context(), nil, syncapp.Source{}, protocol.Operation{}, syncapp.Write); !errors.Is(err, syncapp.ErrUnavailable) {
		t.Fatal("handler admitted without a transaction", err)
	}
	if _, err := nilStore.AuthorEmployeeTx(t.Context(), nil, kernel.OrgScope{}, ""); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("nil store answered")
	}
	if err := (&expensepg.Store{}).ProjectFuelingTx(t.Context(), nil, kernel.OrgScope{}, "", 1); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("store without a port projected")
	}
	if _, err := (&expensepg.Store{}).ProjectPendingFuelingsTx(t.Context(), nil, kernel.OrgScope{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("store without a port caught up")
	}
	if _, err := (&expensepg.Store{}).PrepareExpenseTx(t.Context(), nil, kernel.OrgScope{}, syncapp.Source{}, app.Command{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("store without a port prepared")
	}
	if err := (&expensepg.Store{}).InsertDirectTx(t.Context(), nil, kernel.OrgScope{}, syncapp.Source{}, app.Command{}, app.Prepared{}); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("store without a port inserted")
	}
	if _, err := (&expensepg.Store{}).ReadExpenseTx(t.Context(), nil, kernel.OrgScope{}, ""); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("store without a port read")
	}
}
