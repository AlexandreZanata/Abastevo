package application

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/organization/domain"
)

type StructureUsageError struct {
	privateOnboarding
	usage d.StructureUsage
}

func (StructureUsageError) Error() string             { return d.ErrStructureInUse.Error() }
func (StructureUsageError) Unwrap() error             { return d.ErrStructureInUse }
func (e StructureUsageError) Usage() d.StructureUsage { return e.usage }
func StructureInUse(usage d.StructureUsage) error {
	if usage.Validate() != nil || !usage.InUse() {
		return ErrUnavailable
	}
	return StructureUsageError{usage: usage}
}

func StructurePolicy(write bool) access.Policy {
	effect := access.Read
	if write {
		effect = access.ConfigurationWrite
	}
	return access.Policy{Permission: "org.structure.manage", Capability: "organizations", Effect: effect, Fresh: write}
}
func structureDecision(ctx context.Context, scope kernel.OrgScope, actor CompanyActor, decision access.Decision, target string, write bool) error {
	if scope.Validate() != nil {
		return d.ErrScope
	}
	if actor.Validate() != nil || decision.Policy() != StructurePolicy(write) || decision.Check(ctx, scope, actor.AccountID, "org.structure.manage", target) != nil {
		return ErrUnauthenticated
	}
	return nil
}
