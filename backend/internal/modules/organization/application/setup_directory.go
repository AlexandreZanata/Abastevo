package application

import "context"

// CreatedSetup is current settings-read metadata, discovered through an owned
// creation receipt. It is not an operational context or permission grant.
type CreatedSetup struct {
	privateOnboarding
	ID, Name, MembershipID, State string
	RootRevision                  int64
}
type CreatedSetupPage struct {
	privateOnboarding
	AccountID      string
	Setups         []CreatedSetup
	NextAfterOrgID string
}
type CreatedSetupReader interface {
	ListCreatedSetups(context.Context, CompanyActor, int, string) (CreatedSetupPage, error)
}
