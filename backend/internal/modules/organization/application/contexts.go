package application

import (
	"context"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
)

// Account contexts are current source metadata, never a cached authority grant.
// State and SuspensionReason carry the SCR-ORG-12 notice for readable
// SUSPENDED/ARCHIVED organizations (BR-ORG-030/033); CLOSED is never a context.
type AccountContext struct {
	privateOnboarding
	ID, Name, MembershipID  string
	State, SuspensionReason string
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
