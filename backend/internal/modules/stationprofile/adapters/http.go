package adapters

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Handler serves the public profile read. Anonymous: the DTO carries
// station identity, versioned business fields and the operator link
// only — no private proof, personal identity, prices or grants, and no
// badge without an actual grant record (grants arrive in P31).
type Handler struct {
	Store application.ProfileStore
	Read  application.StationReader
}

// RegisterRoutes mounts the additive profile path.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/stations/{station_id}/profile", h.profile)
}

func (h Handler) profile(w http.ResponseWriter, r *http.Request) {
	profile, err := application.ReadProfile(r.Context(), h.Store, h.Read, chi.URLParam(r, "station_id"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusNotFound, "profile.not-found", "station profile not found", nil)
		return
	}
	operator := map[string]any{"cnpj": profile.OperatorCNPJ, "source": profile.OperatorSource}
	if profile.OperatorCNPJ == "" {
		operator = map[string]any{}
	}
	var lat, lon any
	if profile.Latitude != nil {
		lat = *profile.Latitude
	}
	if profile.Longitude != nil {
		lon = *profile.Longitude
	}
	httpapi.WriteJSON(w, r, http.StatusOK, "public, max-age=30, s-maxage=60", mustJSON(map[string]any{
		"station_id":       profile.StationID,
		"display_name":     profile.DisplayName,
		"location_quality": profile.LocationQuality,
		"coordinates":      map[string]any{"lat": lat, "lon": lon},
		"policy_version":   profile.PolicyVersion,
		"revision":         profile.Revision,
		"business":         profile.Business,
		"operator":         operator,
		"has_badge":        profile.HasBadge,
	}))
}

func mustJSON(doc map[string]any) []byte {
	raw, err := json.Marshal(doc)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
