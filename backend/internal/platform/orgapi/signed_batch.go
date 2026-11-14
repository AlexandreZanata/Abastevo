package orgapi

// SignedBatchRule is a transport boundary, never a business permission. Each
// operation still requires its own current owning source/record policy.
type SignedBatchRule struct {
	AccountProof       bool `json:"account_proof"`
	Signed             bool `json:"signed"`
	CurrentEnrollment  bool `json:"current_enrollment"`
	WholeBodyPreflight bool `json:"whole_body_preflight"`
	PerOperation       bool `json:"per_operation"`
	Success            int  `json:"success"`
}
type SignedBatchDescriptor interface{ SignedBatchRule() SignedBatchRule }
