package orgapi

// SignedAttachmentSessionRule owns only the enrolled member's own attachment
// session (EMP-OFFL-005): declare a document, link it to a record and receive
// a short upload capability. It never grants content access; document and
// target permissions are rechecked by their owners on every request.
type SignedAttachmentSessionRule struct {
	AccountProof       bool `json:"account_proof"`
	Signed             bool `json:"signed"`
	CurrentEnrollment  bool `json:"current_enrollment"`
	CreatorBound       bool `json:"creator_bound"`
	OwnerLinked        bool `json:"owner_linked"`
	WholeBodyPreflight bool `json:"whole_body_preflight"`
	Success            int  `json:"success"`
}
type SignedAttachmentSessionDescriptor interface {
	SignedAttachmentSessionRule() SignedAttachmentSessionRule
}
