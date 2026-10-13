package application

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

func (s *Service) Reconcile(ctx context.Context) (int, error) {
	if !s.Configured() {
		return 0, domain.ErrUnavailable
	}
	store, ok := s.Store.(ReconciliationStore)
	if !ok {
		return 0, domain.ErrUnavailable
	}
	return store.ScheduleReconciliation(ctx, s.Environment, s.PackageName, 25)
}
