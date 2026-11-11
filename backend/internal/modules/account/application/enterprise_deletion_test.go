package application

import (
	"context"
	"errors"
	"fmt"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"testing"
)

func TestEnterpriseBillingGuardRefusesBeforePersonalMutation(t *testing.T) {
	for _, tc := range []struct{ source, want error }{{kernel.ErrLastOrgOwner, domain.ErrEnterpriseLastOwner}, {kernel.ErrOrgAccessPolicyUnavailable, domain.ErrEnterprisePolicyUnavailable}} {
		svc, _, rec := deletionService()
		auth := signupBound(t, svc, "synthetic-enterprise@example.com", "482916", "synthetic-enterprise-contributor")
		svc.RequestBillingExit = func(context.Context, string) error { return fmt.Errorf("synthetic private cause: %w", tc.source) }
		if receipt, err := svc.DeleteAccount(context.Background(), auth.Account.ID); !errors.Is(err, tc.want) || err != tc.want || receipt.ErasureRequested {
			t.Fatal("enterprise refusal not bare or acknowledged", err)
		}
		if account, ok, err := svc.Store.GetAccount(context.Background(), auth.Account.ID); err != nil || !ok || account.Status != domain.StatusActive {
			t.Fatal("refusal changed account", err)
		}
		if len(rec.accountIDs) != 0 {
			t.Fatal("refusal dispatched erasure")
		}
	}
}
