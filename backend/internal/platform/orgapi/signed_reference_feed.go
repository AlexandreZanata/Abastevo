package orgapi

// SignedReferenceFeedRule owns only the requesting member's purpose-bound
// reference copy (EMP-OFFL-006); it never grants or caches authority.
type SignedReferenceFeedRule struct {
	AccountProof       bool `json:"account_proof"`
	Signed             bool `json:"signed"`
	CurrentEnrollment  bool `json:"current_enrollment"`
	BoundCursor        bool `json:"bound_cursor"`
	WholeBodyPreflight bool `json:"whole_body_preflight"`
	Success            int  `json:"success"`
}
type SignedReferenceFeedDescriptor interface {
	SignedReferenceFeedRule() SignedReferenceFeedRule
}
