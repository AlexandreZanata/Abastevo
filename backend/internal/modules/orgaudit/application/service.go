// Package application owns audit ports. Feature commands resolve current write
// authority before using Runner. Read authority is resolved on every List call.
package application

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

// Runner joins the business callback and exactly one event in a scoped transaction.
// A true result means replay and the callback was skipped. Callbacks use tx for
// every write and leave transaction lifecycle to the runner.
type Runner interface {
	RecordChange(context.Context, kernel.OrgScope, domain.Event, func(context.Context, pgx.Tx) error) (bool, error)
}

type Reader interface {
	List(context.Context, kernel.OrgScope, domain.Filter) ([]domain.Record, error)
}

// ReadAccess carries server-resolved actor and optional support-session references.
type ReadAccess struct {
	ActorID          string
	SupportSessionID string
}

// Authorizer verifies current audit.event.read authority covering the whole
// organization, or current approved support access for that actor and organization.
// Partial record scopes cannot authorize this organization-wide list.
type Authorizer interface {
	AuthorizeAuditRead(context.Context, kernel.OrgScope, ReadAccess) error
}

type Service struct {
	Reader     Reader
	Authorizer Authorizer
}

func (service Service) List(ctx context.Context, scope kernel.OrgScope, access ReadAccess, filter domain.Filter) ([]domain.Record, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if !domain.ValidID(access.ActorID) ||
		(access.SupportSessionID != "" && !domain.ValidID(access.SupportSessionID)) {
		return nil, domain.ErrInvalid
	}
	filter, err := filter.Normalize()
	if err != nil {
		return nil, err
	}
	if service.Authorizer == nil {
		return nil, domain.ErrDenied
	}
	if err := service.Authorizer.AuthorizeAuditRead(ctx, scope, access); err != nil {
		return nil, err
	}
	if service.Reader == nil {
		return nil, domain.ErrUnavailable
	}
	return service.Reader.List(ctx, scope, filter)
}
