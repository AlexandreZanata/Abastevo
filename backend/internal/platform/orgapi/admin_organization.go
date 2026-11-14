package orgapi

// AdminOrganizationRule describes the separate ADR-023 operator boundary. It
// grants no organization role and never accepts the company-session cookie.
type AdminOrganizationRule struct {
	Action          string `json:"action"`
	Cookie          bool   `json:"cookie"`
	CSRF            bool   `json:"csrf"`
	Fresh           bool   `json:"fresh"`
	RequireRevision bool   `json:"require_revision"`
	RequireReason   bool   `json:"require_reason"`
	Success         int    `json:"success"`
}
type AdminOrganizationDescriptor interface{ AdminOrganizationRule() AdminOrganizationRule }
