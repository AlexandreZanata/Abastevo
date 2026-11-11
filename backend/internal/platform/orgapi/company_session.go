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
