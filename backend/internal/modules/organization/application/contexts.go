package application

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// Account contexts are current source metadata, never a cached authority grant.
type AccountContext struct {
	privateOnboarding
	ID, Name, MembershipID string
}
type AccountContextPage struct {
	privateOnboarding
	Organizations  []AccountContext
	NextAfterOrgID string
}
type AccountContextView struct {
	privateOnboarding
	Organization           AccountContext
	Permissions            []permissions.Permission
	InvitationGrants       []permissions.ContextGrant
	InvitationValidityDays int
}
type AccountContextReader interface {
	ListAccountContexts(context.Context, CompanyActor, int, string) (AccountContextPage, error)
	ReadAccountContext(context.Context, CompanyActor, kernel.OrgScope, string) (AccountContextView, error)
}
