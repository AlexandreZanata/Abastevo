package kernel

import "errors"

// Shared refusals at personal-account/billing and private-access boundaries.
// They contain no organization IDs, names or provider/database causes.
var (
	ErrLastOrgOwner               = errors.New("enterprise: last owner protected")
	ErrOrgAccessPolicyUnavailable = errors.New("enterprise: access policy unavailable")
	ErrPendingOrgLimit            = errors.New("enterprise: pending organization limit reached")
	ErrOwnerRequiresPersonal      = errors.New("enterprise: owner requires personal account")
)
