package application

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	d "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/membership/domain"
	permissions "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/domain"
	"github.com/jackc/pgx/v5"
	"time"
)

var (
	ErrTokenInvalid      = errors.New("membership: token invalid")
	ErrAlreadyMember     = errors.New("membership: already member")
	ErrManagedRestricted = errors.New("membership: managed account restricted")
	ErrConsentChanged    = errors.New("membership: consent changed")
)

const InvitationNoticeVersion = "enterprise-invitation-v1"
const InvitationNotice = "The organization sees your employee record, accepted account alias and actions within this organization. It cannot see your personal credentials, personal vehicles, private history or other organizations."

type RedemptionCommand struct {
	privateRegister
	Action, OperationID, Code, ConsentDigest string
	Consent                                  bool
}

func (c RedemptionCommand) Validate() error {
	switch c.Action {
	case "PREVIEW":
		if c.OperationID != "" || c.Consent || c.ConsentDigest != "" {
			return d.ErrInvalid
		}
	case "ACCEPT":
		if !canonicalEmployeeID(c.OperationID) || !c.Consent || len(c.ConsentDigest) != 64 {
			return d.ErrInvalid
		}
	case "DECLINE":
		if !canonicalEmployeeID(c.OperationID) || c.Consent || c.ConsentDigest != "" {
			return d.ErrInvalid
		}
	default:
		return d.ErrInvalid
	}
	if len(c.Code) > 160 {
		return d.ErrInvalid
	}
	return nil
}

type RedemptionFacts struct {
	privateRegister
	Assignments                             []permissions.Assignment
	Name, Alias                             string
	Verified, RequireConfirmation, Eligible bool
	RootRevision, SettingsRevision          int64
	Now                                     time.Time
}
type RedemptionAuthority interface {
	// BootstrapTx resolves only a locator, locks accounts in UUID order and then
	// verifies actual personal proof/capability/root; replay never bypasses proof.
	CheckPersonalTx(context.Context, pgx.Tx, EmployeeActor) error
	LockRedemptionTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string, string, bool) error
	CheckRedemptionTx(context.Context, pgx.Tx, kernel.OrgScope, EmployeeActor, string) (RedemptionFacts, error)
}
type RedemptionResult struct {
	privateRegister
	OrgID, InvitationID, MembershipID, OrganizationName, ConsentDigest string
	Verified, ConfirmationRequired, Replayed                           bool
	Grants                                                             []d.InvitationGrant
	State                                                              string
	MembershipState                                                    d.MembershipState
	AcceptedAt, ConfirmationDeadline                                   time.Time
}
type AdmissionView struct {
	privateRegister
	InvitationID, MembershipID, Alias string
	State                             d.AdmissionState
	Revision                          int64
	AcceptedAt, Deadline              time.Time
}
type RedemptionRepository interface {
	Redeem(context.Context, EmployeeActor, RedemptionCommand) (RedemptionResult, error)
	ManageAdmission(context.Context, kernel.OrgScope, EmployeeActor, string, string, string, int64, string) (AdmissionView, bool, error)
}
