package adapters_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	exports "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgexport/application"
)

func TestExportAuthorityCannotRunWithoutRetainedTransactionAndSource(t *testing.T) {
	var absent *adapters.ExportAuthority
	for _, guard := range []*adapters.ExportAuthority{absent, {}} {
		for _, action := range []exports.Action{exports.ActionRequest, exports.ActionRead, 0, 255} {
			if err := guard.CheckExportTx(t.Context(), nil, kernel.OrgScope{}, exports.Actor{}, "", action); !errors.Is(err, exports.ErrUnavailable) {
				t.Fatal("missing retained source admitted an export action", err)
			}
		}
		if err := guard.CheckRequesterTx(t.Context(), nil, kernel.OrgScope{}, exports.Requester{}, "test.synthetic_rows", "exp.expense.read"); !errors.Is(err, exports.ErrUnavailable) {
			t.Fatal("missing retained source re-evaluated a requester", err)
		}
		// EMP-AUTH-011: the data permission check has no source either.
		if err := guard.CheckDataTx(t.Context(), nil, kernel.OrgScope{}, exports.Actor{}, "", "exp.expense.read"); !errors.Is(err, exports.ErrUnavailable) {
			t.Fatal("missing retained source admitted a data permission", err)
		}
	}
}
