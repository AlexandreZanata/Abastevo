package orgapi

// CompanySessionRule describes the account-scoped cookie lifecycle entry,
// rather than organization resource permission. Only its three fixed paths
// may use this descriptor in the route/contract gate.
type CompanySessionRule struct {
	Action       string `json:"action"`
	AccountProof bool   `json:"account_proof"`
	Cookie       bool   `json:"cookie"`
	CSRF         bool   `json:"csrf"`
	Fresh        bool   `json:"fresh"`
	Success      int    `json:"success"`
}
type CompanySessionDescriptor interface{ CompanySessionRule() CompanySessionRule }

// OrganizationCreationRule is confined to the one account-scoped creation
// command, before an organization and its membership exist.
type OrganizationCreationRule struct {
	Cookie  bool `json:"cookie"`
	CSRF    bool `json:"csrf"`
	Fresh   bool `json:"fresh"`
	Success int  `json:"success"`
}
type OrganizationCreationDescriptor interface {
	OrganizationCreationRule() OrganizationCreationRule
}

// PersonalInvitationRule is confined to authenticated personal invitation
// bootstrap paths; it grants no organization permission or public discovery.
type PersonalInvitationRule struct {
	Action  string `json:"action"`
	Cookie  bool   `json:"cookie"`
	CSRF    bool   `json:"csrf"`
	Fresh   bool   `json:"fresh"`
	Success int    `json:"success"`
}
type PersonalInvitationDescriptor interface{ PersonalInvitationRule() PersonalInvitationRule }

// SelfMembershipRule permits only source-bound personal review/leave. It is
// neither an organization permission nor a general exception to the AUTH gate.
type SelfMembershipRule struct {
	Action  string `json:"action"`
	Cookie  bool   `json:"cookie"`
	CSRF    bool   `json:"csrf"`
	Fresh   bool   `json:"fresh"`
	Binding bool   `json:"binding"`
	Success int    `json:"success"`
}
type SelfMembershipDescriptor interface{ SelfMembershipRule() SelfMembershipRule }

// SignedDeviceRule is confined to the two actual device proof entry points.
// Only first enrolment may bootstrap without an existing scoped enrolment.
type SignedDeviceRule struct {
	Action       string `json:"action"`
	AccountProof bool   `json:"account_proof"`
	Signed       bool   `json:"signed"`
	Fresh        bool   `json:"fresh"`
	Enrollment   bool   `json:"enrollment"`
	Bootstrap    bool   `json:"bootstrap"`
	Success      int    `json:"success"`
}
type SignedDeviceDescriptor interface{ SignedDeviceRule() SignedDeviceRule }

// SelfDeviceRule authorizes only account-bound scoped enrolment management;
// it neither grants an organization permission nor exposes personal sessions.
type SelfDeviceRule struct {
	Action  string `json:"action"`
	Cookie  bool   `json:"cookie"`
	CSRF    bool   `json:"csrf"`
	Fresh   bool   `json:"fresh"`
	Binding bool   `json:"binding"`
	Success int    `json:"success"`
}
type SelfDeviceDescriptor interface{ SelfDeviceRule() SelfDeviceRule }

// AccountContextRule admits only current own-account discovery/selection;
// metadata is not a permission grant and grants no organization commands.
type AccountContextRule struct {
	Action  string `json:"action"`
	Cookie  bool   `json:"cookie"`
	CSRF    bool   `json:"csrf"`
	Binding bool   `json:"binding"`
	Success int    `json:"success"`
}
type AccountContextDescriptor interface{ AccountContextRule() AccountContextRule }

// OwnedSetupsRule describes only the own-created recovery directory. Every
// listed item still requires current scoped settings-read authority.
type OwnedSetupsRule struct {
	Cookie  bool `json:"cookie"`
	CSRF    bool `json:"csrf"`
	Binding bool `json:"binding"`
	Success int  `json:"success"`
}
type OwnedSetupsDescriptor interface{ OwnedSetupsRule() OwnedSetupsRule }
