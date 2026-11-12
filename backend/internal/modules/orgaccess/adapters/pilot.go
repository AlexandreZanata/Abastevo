package adapters

import (
	"context"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

// CapabilityStates is a scoped read port, not the authorizing capability
// Service: using that Service here would recurse into AUTH again.
type CapabilityStates interface {
	State(context.Context, kernel.OrgScope, string) (capability.State, error)
}

// PilotEntitlements consumes current operator-provisioned flags. It neither
// provisions them nor reads account tiers, payments, client state or roles.
// Composition supplies the same default-off activation as the private gateway.
type PilotEntitlements struct {
	Enabled bool
	Catalog capability.Catalog
	States  CapabilityStates
}

var _ app.CapabilityGate = PilotEntitlements{}

func (p PilotEntitlements) Check(ctx context.Context, scope kernel.OrgScope, subject app.Subject, policy app.Policy) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if scope.Validate() != nil || subject.Scope() != scope || subject.AccountID() == "" || subject.MembershipID() == "" || policy.Validate() != nil || p.Catalog.Validate(policy.Capability) != nil {
		return app.ErrUnavailable
	}
	if !p.Enabled {
		return app.ErrEntitlementRequired
	}
	if p.States == nil {
		return app.ErrUnavailable
	}
	state, err := p.States.State(ctx, scope, policy.Capability)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return app.ErrUnavailable
	}
	if state.Revision < 0 || (state.Enabled && state.Revision == 0) {
		return app.ErrUnavailable
	}
	switch policy.Effect {
	case app.Read, app.ReduceAccess, app.RegularizePlan, app.RestoreOrganization:
		// Preserve already-authorized reads/exports and the existing safety paths.
		// Current proof/roles and record policy are still mandatory independent gates.
		return nil
	default:
		if !state.Enabled {
			return app.ErrEntitlementRequired
		}
		return nil
	}
}
