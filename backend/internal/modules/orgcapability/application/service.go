// Package application checks current authority independently of activation.
package application

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
	"github.com/jackc/pgx/v5"
)

// Authorizer resolves current membership, organization state, exact permission
// and resource coverage on every entry. Manage requires operator authority.
// AUTH owns its concrete implementation; a capability or payment grants none.
type Authorizer interface {
	Authorize(context.Context, kernel.OrgScope, domain.Actor, string, domain.Intent) error
}
type Repository interface {
	State(context.Context, kernel.OrgScope, string) (domain.State, error)
	Set(context.Context, kernel.OrgScope, domain.Actor, domain.Change) (bool, error)
	Run(context.Context, kernel.OrgScope, string, domain.Intent, func(context.Context, pgx.Tx) error) error
	GuardWrite(context.Context, pgx.Tx, kernel.OrgScope, string) error
}
type Service struct {
	Enabled    bool
	Catalog    domain.Catalog
	Authorizer Authorizer
	Repository Repository
}

func (service Service) authorize(ctx context.Context, scope kernel.OrgScope, actor domain.Actor, code string, intent domain.Intent) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if err := actor.Validate(); err != nil {
		return err
	}
	if err := service.Catalog.Validate(code); err != nil {
		return err
	}
	if service.Authorizer == nil {
		return domain.ErrDenied
	}
	if err := service.Authorizer.Authorize(ctx, scope, actor, code, intent); err != nil {
		return err
	}
	if service.Repository == nil {
		return domain.ErrUnavailable
	}
	return nil
}

func (service Service) State(ctx context.Context, scope kernel.OrgScope, actor domain.Actor, code string) (domain.State, error) {
	if !service.Enabled {
		return domain.State{}, domain.ErrSurfaceDisabled
	}
	if err := service.authorize(ctx, scope, actor, code, domain.Read); err != nil {
		return domain.State{}, err
	}
	return service.Repository.State(ctx, scope, code)
}

// Set is operator provisioning, independent of the product's global switch.
// Replays still require current operator authority.
func (service Service) Set(ctx context.Context, scope kernel.OrgScope, actor domain.Actor, change domain.Change) (bool, error) {
	if err := change.Validate(service.Catalog); err != nil {
		return false, err
	}
	if err := service.authorize(ctx, scope, actor, change.Capability, domain.Manage); err != nil {
		return false, err
	}
	return service.Repository.Set(ctx, scope, actor, change)
}

// Run is the transaction boundary for an already defined module operation.
// Every business query uses the supplied tx. Audited feature commands instead
// compose Service.GuardWrite inside their existing audit runner. The module also
// checks authority before that runner, including replay. Never nest runners.
func (service Service) Run(ctx context.Context, scope kernel.OrgScope, actor domain.Actor, code string, intent domain.Intent, work func(context.Context, pgx.Tx) error) error {
	if !service.Enabled {
		return domain.ErrSurfaceDisabled
	}
	if work == nil || (intent != domain.Read && intent != domain.Export && intent != domain.Write) {
		return domain.ErrInvalid
	}
	if err := service.authorize(ctx, scope, actor, code, intent); err != nil {
		return err
	}
	return service.Repository.Run(ctx, scope, code, intent, work)
}

// GuardWrite holds the capability row through the caller's existing scoped
// transaction. It rechecks current authority, independently of a prior decision.
func (service Service) GuardWrite(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, actor domain.Actor, code string) error {
	if !service.Enabled {
		return domain.ErrSurfaceDisabled
	}
	if tx == nil {
		return domain.ErrInvalid
	}
	if err := service.authorize(ctx, scope, actor, code, domain.Write); err != nil {
		return err
	}
	return service.Repository.GuardWrite(ctx, tx, scope, code)
}
