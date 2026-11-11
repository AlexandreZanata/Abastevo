// Package application joins required follow-up to the existing audited command.
package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/application"
	auditdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
	"github.com/jackc/pgx/v5"
)

type Writer interface {
	Enqueue(context.Context, kernel.OrgScope, pgx.Tx, domain.Envelope, string) error
}
type Work func(context.Context, kernel.OrgScope, pgx.Tx) error
type Service struct {
	Enabled bool
	Catalog domain.Catalog
	Audit   audit.Runner
	Writer  Writer
}

func SafeError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		return domain.ErrInvalid
	case errors.Is(err, domain.ErrReused), errors.Is(err, auditdomain.ErrReused):
		return domain.ErrReused
	case errors.Is(err, domain.ErrDenied):
		return domain.ErrDenied
	default:
		return domain.ErrUnavailable
	}
}

// Record requires current write authority first. Every callback write uses tx;
// no external operation or transaction lifecycle belongs inside the callback.
func (s Service) Record(ctx context.Context, scope kernel.OrgScope, a auditdomain.Event, e domain.Event, consumers []string, work Work) (bool, error) {
	if !s.Enabled {
		return false, domain.ErrDisabled
	}
	plan, bound, err := s.Catalog.Prepare(scope, a, e, consumers)
	if err != nil || work == nil {
		return false, domain.ErrInvalid
	}
	if s.Audit == nil || s.Writer == nil {
		return false, domain.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	replay, err := s.Audit.RecordChange(ctx, scope, bound, func(ctx context.Context, tx pgx.Tx) error {
		if err := work(ctx, scope, tx); err != nil {
			return SafeError(err)
		}
		for _, consumer := range plan.Consumers {
			envelope := domain.Envelope{Version: domain.Version, OrgID: scope.OrgID().String(), Consumer: consumer, Event: plan.Event}
			if err := s.Writer.Enqueue(ctx, scope, tx, envelope, bound.RequestHash); err != nil {
				return SafeError(err)
			}
		}
		return nil
	})
	if err != nil {
		return false, SafeError(err)
	}
	if ctx.Err() != nil {
		return false, domain.ErrUnavailable
	}
	return replay, nil
}
