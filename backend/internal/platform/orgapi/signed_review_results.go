package orgapi

// SignedReviewResultsRule owns only a current author's minimal outcome read;
// it never grants a business-record permission or restores removed access.
type SignedReviewResultsRule struct {
	AccountProof       bool `json:"account_proof"`
	Signed             bool `json:"signed"`
	ExactEnrollment    bool `json:"exact_enrollment"`
	OriginalAuthor     bool `json:"original_author"`
	WholeBodyPreflight bool `json:"whole_body_preflight"`
	Success            int  `json:"success"`
}
type SignedReviewResultsDescriptor interface {
	SignedReviewResultsRule() SignedReviewResultsRule
}
