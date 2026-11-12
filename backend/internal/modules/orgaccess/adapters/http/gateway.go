// Package http binds the current AUTH service to the shared proof/HTTP boundary.
package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/go-chi/chi/v5"
)

type stateKey struct{}
type requestState struct {
	prepared app.Prepared
	decision app.Decision
}
type confirmedProof struct {
	principal orgapi.Principal
	rule      orgapi.Rule
}

func (p confirmedProof) Confirm(_ context.Context, s app.Subject, policy app.Policy) error {
	if p.principal.AccountID != s.AccountID() || p.principal.MembershipID != s.MembershipID() || p.principal.Scope != s.Scope() || p.principal.Channel != s.Channel() || p.principal.ContributorID != s.ContributorID() || p.principal.DeviceKeyID != s.DeviceKeyID() || p.rule.Permission != string(policy.Permission) || p.rule.Fresh != policy.Fresh || p.rule.Write != policy.Effect.Write() {
		return app.ErrUnauthenticated
	}
	return nil
}
func mapped(err error) error {
	var denied *d.PermissionDenial
	if errors.As(err, &denied) {
		return orgapi.MissingPermission(string(denied.Permission()))
	}
	for _, pair := range []struct {
		source error
		code   orgapi.Code
	}{
		{app.ErrNoMembership, orgapi.NotFound}, {d.ErrResourceNotFound, orgapi.NotFound},
		{app.ErrMembershipSuspended, orgapi.MembershipSuspended}, {app.ErrOrgSuspended, orgapi.OrgSuspended}, {app.ErrOrgArchived, orgapi.OrgArchived}, {app.ErrSetupIncomplete, orgapi.OrgSetupIncomplete},
		{app.ErrUnauthenticated, orgapi.Unauthenticated}, {app.ErrFreshAuthRequired, orgapi.FreshAuthRequired},
		{app.ErrEntitlementRequired, orgapi.EntitlementRequired}, {app.ErrLimitReached, orgapi.LimitReached},
		{app.ErrSelfApproval, orgapi.SelfApproval}, {app.ErrPeriodLocked, orgapi.PeriodLocked},
	} {
		if errors.Is(err, pair.source) {
			return pair.code
		}
	}
	return orgapi.Unavailable
}

// Register pins one source policy/resource resolver to one guarded route. The
// gateway authenticates normal proof, Prepare resolves current authority,
// gateway confirmation rechecks proof/freshness, and Complete applies plan and
// record gates before admission/dispatch. No client DTO carries authority.
func Register[T orgapi.Request](router chi.Router, path string, gateway orgapi.Gateway, rule orgapi.Rule, service app.Authorization, policy app.Policy, resource app.ResourceResolver, serve func(context.Context, orgapi.Principal, app.Decision, T) (any, error)) error {
	if policy.Validate() != nil || rule.Permission != string(policy.Permission) || rule.Write != policy.Effect.Write() || rule.Fresh != policy.Fresh || service.Source == nil || service.Capabilities == nil || service.Records == nil || service.Now == nil || resource == nil || serve == nil {
		return fmt.Errorf("orgaccess: incomplete guarded route policy")
	}
	if gateway.Sessions != nil && gateway.SessionConfirm == nil {
		return fmt.Errorf("orgaccess: current Android confirmation required")
	}
	if gateway.Resolve != nil || gateway.ConfirmAuthority != nil {
		return fmt.Errorf("orgaccess: existing authority callback cannot be replaced")
	}
	gateway.Resolve = func(ctx context.Context, actor orgapi.Actor, scope kernel.OrgScope, _ orgapi.Rule) (orgapi.Membership, error) {
		state, _ := ctx.Value(stateKey{}).(*requestState)
		if state == nil {
			return orgapi.Membership{}, orgapi.Unavailable
		}
		prepared, err := service.Prepare(ctx, scope, app.Actor{AccountID: actor.AccountID, Channel: actor.Channel, ContributorID: actor.ContributorID, DeviceKeyID: actor.DeviceKeyID}, policy, resource)
		if err != nil {
			return orgapi.Membership{}, mapped(err)
		}
		state.prepared = prepared
		s := prepared.Subject()
		return orgapi.Membership{ID: s.MembershipID(), AccountID: s.AccountID(), OrgID: s.Scope().OrgID()}, nil
	}
	gateway.ConfirmAuthority = func(ctx context.Context, principal orgapi.Principal, confirmed orgapi.Rule) error {
		state, _ := ctx.Value(stateKey{}).(*requestState)
		if state == nil {
			return orgapi.Unavailable
		}
		decision, err := service.Complete(ctx, state.prepared, confirmedProof{principal, confirmed})
		if err != nil {
			return mapped(err)
		}
		state.decision = decision
		return nil
	}
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, finish := app.BeginRequest(r.Context())
			defer finish()
			ctx = context.WithValue(ctx, stateKey{}, &requestState{})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	return orgapi.Register(router.With(middleware), path, gateway, rule, func(ctx context.Context, p orgapi.Principal, input T) (any, error) {
		state, _ := ctx.Value(stateKey{}).(*requestState)
		if state == nil {
			return nil, orgapi.Unavailable
		}
		if err := state.decision.Check(ctx, p.Scope, p.AccountID, policy.Permission, state.decision.TargetID()); err != nil {
			return nil, orgapi.Unavailable
		}
		return serve(ctx, p, state.decision, input)
	})
}
