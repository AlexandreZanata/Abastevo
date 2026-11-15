package orgapi

// DocumentCapabilityHeader carries the short upload capability of the private
// document ingress. It is never a query parameter or a cookie.
const DocumentCapabilityHeader = "X-Abastevo-Upload-Capability"

// DocumentIngressRule is a private byte transport boundary (EMP-DOC-002). A
// member- and document-bound capability is verified before any byte is read;
// the owning module still rechecks the creator and the current permission
// before verified storage. It never grants content-read access.
type DocumentIngressRule struct {
	Capability       bool   `json:"capability"`
	MemberBound      bool   `json:"member_bound"`
	CurrentAuthority bool   `json:"current_authority"`
	VerifiedBytes    bool   `json:"verified_bytes"`
	Permission       string `json:"permission"`
	Success          int    `json:"success"`
}
type DocumentIngressDescriptor interface{ DocumentIngressRule() DocumentIngressRule }
