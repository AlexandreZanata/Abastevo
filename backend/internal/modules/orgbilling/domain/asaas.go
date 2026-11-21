package domain

import (
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

const asaasOrganizationPrefix = "abv_org:"

func IsCompanyAsaasReference(reference string) bool {
	return strings.HasPrefix(reference, asaasOrganizationPrefix)
}

// AsaasReferenceScope binds a provider-retrieved external reference to the
// company side. The full reference must still match the reviewed charge.
func AsaasReferenceScope(reference string) (kernel.OrgScope, error) {
	raw, ok := strings.CutPrefix(reference, asaasOrganizationPrefix)
	if !ok || len(raw) != 73 {
		return kernel.OrgScope{}, ErrBillingBinding
	}
	org, checkout, _ := strings.Cut(raw, ":")
	id, orgErr := kernel.ParseOrgID(org)
	op, checkoutErr := kernel.ParseOrgID(checkout)
	if orgErr != nil || checkoutErr != nil || id.String() != org || op.String() != checkout {
		return kernel.OrgScope{}, ErrBillingBinding
	}
	return kernel.NewOrgScope(id)
}
