package portalgen

import (
	"strings"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

// A canonical public label is primary; an equal or unknown legal name is not
// rendered a second time. Legacy snapshots simply have no secondary fact.
func stationLegalName(station enterpriseexport.Station) string {
	legal := strings.TrimSpace(kernel.PublicStationLegalName(station.LegalName))
	if strings.EqualFold(strings.Join(strings.Fields(legal), " "), strings.Join(strings.Fields(station.DisplayName), " ")) {
		return ""
	}
	return legal
}
