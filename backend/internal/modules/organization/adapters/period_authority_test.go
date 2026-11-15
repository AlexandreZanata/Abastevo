package adapters_test

import (
	"errors"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/adapters"
	period "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/review/application"
)

func TestPeriodAuthorityCannotRunWithoutRetainedTransactionAndSource(t *testing.T) {
	var absent *adapters.PeriodAuthority
	for _, guard := range []*adapters.PeriodAuthority{absent, {}} {
		for _, action := range []period.PeriodAction{period.PeriodRead, period.PeriodClose, period.PeriodReopen, 0, 255} {
			err := guard.CheckPeriodTx(t.Context(), nil, kernel.OrgScope{}, period.PeriodActor{}, "", action)
			if !errors.Is(err, period.ErrUnavailable) {
				t.Fatal("missing retained source admitted a period action", err)
			}
		}
	}
}
