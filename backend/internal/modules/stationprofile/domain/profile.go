package domain

// PolicyVersion freezes the profile/claim contract this module
// implements (see STATION_PROFILE_CONTRACT_FREEZE).
const PolicyVersion = "profile-v1"

// ValidOperatorSource reports whether the operator evidence source is
// permitted. Registry snapshots, DOU acts and audited review qualify;
// suggestions, pins and user claims never establish operation.
func ValidOperatorSource(source string) bool {
	switch source {
	case "registry", "dou", "review":
		return true
	default:
		return false
	}
}

// TerminalClaimState reports whether the claim state ends that claim.
// Appeals open linked new claims; terminal states never reopen.
func TerminalClaimState(state string) bool {
	switch state {
	case "approved", "denied", "cancelled", "expired":
		return true
	default:
		return false
	}
}

// PublicBusinessKeys bounds the business-provided projection (frozen
// P30-T01 field policy). Anything else — CPF, keys, passwords,
// precise GPS, prices — is never public here.
var PublicBusinessKeys = []string{
	"opening_hours", "services", "phone", "website", "description",
}
