package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// CommerceHandler serves the station's own attributed commercial source.
// The public reads are anonymous and source-labelled; the managed writes
// recheck the live `price.publish` scope, account and operator server-side.
// All responses carry no-store for writes and public caching for reads.
type CommerceHandler struct {
	Ports    func(r *http.Request) application.CommercePorts
	Reads    CommerceStore
	Sessions func(ctx context.Context, familyID, accessToken string) (string, error)
	Now      func() time.Time
}

// RegisterPublicRoutes mounts the anonymous commercial reads.
func (h CommerceHandler) RegisterPublicRoutes(r chi.Router) {
	r.Get("/v1/stations/{station_id}/announced-prices", h.announcedRead)
	r.Get("/v1/stations/{station_id}/promotions", h.promotionsRead)
}

// RegisterManageRoutes mounts the session-authenticated commercial writes.
func (h CommerceHandler) RegisterManageRoutes(r chi.Router) {
	r.Post("/v1/stations/{station_id}/announced-prices/save", h.announce)
	r.Post("/v1/stations/{station_id}/announced-prices/withdraw", h.withdrawPrice)
	r.Post("/v1/stations/{station_id}/promotions/save", h.promote)
	r.Post("/v1/stations/{station_id}/promotions/withdraw", h.withdrawPromotion)
}

func (h CommerceHandler) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

type commerceSessionDTO struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

type announceDTO struct {
	commerceSessionDTO
	FuelProduct   string `json:"fuel_product"`
	Unit          string `json:"unit"`
	PriceMilliBRL int64  `json:"price_milli_brl"`
	Conditions    string `json:"conditions"`
	ValidFrom     string `json:"valid_from"`
	ValidUntil    string `json:"valid_until"`
}

type withdrawPriceDTO struct {
	commerceSessionDTO
	PriceID string `json:"price_id"`
}

type promoteDTO struct {
	commerceSessionDTO
	Label       string `json:"label"`
	Description string `json:"description"`
	FuelProduct string `json:"fuel_product"`
	ValidFrom   string `json:"valid_from"`
	ValidUntil  string `json:"valid_until"`
}

type withdrawPromotionDTO struct {
	commerceSessionDTO
	PromotionID string `json:"promotion_id"`
}

func (h CommerceHandler) sessionAccount(w http.ResponseWriter, r *http.Request, dto commerceSessionDTO) (string, bool) {
	accountID, err := h.Sessions(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "profile.session-invalid", "valid account session required", nil)
		return "", false
	}
	return accountID, true
}

func (h CommerceHandler) instant(w http.ResponseWriter, r *http.Request, raw string) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "commerce.bad-request", "invalid commercial input", nil)
		return time.Time{}, false
	}
	return at.UTC(), true
}

func (h CommerceHandler) announce(w http.ResponseWriter, r *http.Request) {
	var dto announceDTO
	if !readCommerce(w, r, &dto) {
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto.commerceSessionDTO)
	if !ok {
		return
	}
	from, ok := h.instant(w, r, dto.ValidFrom)
	if !ok {
		return
	}
	until, ok := h.instant(w, r, dto.ValidUntil)
	if !ok {
		return
	}
	stored, err := application.AnnouncePrice(r.Context(), h.Ports(r), accountID, chi.URLParam(r, "station_id"), domain.AnnouncedPrice{
		Product: dto.FuelProduct, Unit: dto.Unit, PriceMilliBRL: dto.PriceMilliBRL,
		Conditions: dto.Conditions, ValidFrom: from, ValidUntil: until,
	})
	if err != nil {
		writeCommerceError(w, r, err)
		return
	}
	writeCommerceJSON(w, r, http.StatusOK, "no-store", map[string]any{"announced_price": h.announcedProjection(stored)})
}

func (h CommerceHandler) withdrawPrice(w http.ResponseWriter, r *http.Request) {
	var dto withdrawPriceDTO
	if !readCommerce(w, r, &dto) {
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto.commerceSessionDTO)
	if !ok {
		return
	}
	stored, err := application.WithdrawAnnouncedPrice(r.Context(), h.Ports(r), accountID, chi.URLParam(r, "station_id"), dto.PriceID)
	if err != nil {
		writeCommerceError(w, r, err)
		return
	}
	writeCommerceJSON(w, r, http.StatusOK, "no-store", map[string]any{"announced_price": h.announcedProjection(stored)})
}

func (h CommerceHandler) promote(w http.ResponseWriter, r *http.Request) {
	var dto promoteDTO
	if !readCommerce(w, r, &dto) {
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto.commerceSessionDTO)
	if !ok {
		return
	}
	from, ok := h.instant(w, r, dto.ValidFrom)
	if !ok {
		return
	}
	until, ok := h.instant(w, r, dto.ValidUntil)
	if !ok {
		return
	}
	stored, err := application.PublishPromotion(r.Context(), h.Ports(r), accountID, chi.URLParam(r, "station_id"), domain.Promotion{
		Label: dto.Label, Description: dto.Description, Product: dto.FuelProduct, ValidFrom: from, ValidUntil: until,
	})
	if err != nil {
		writeCommerceError(w, r, err)
		return
	}
	writeCommerceJSON(w, r, http.StatusOK, "no-store", map[string]any{"promotion": h.promotionProjection(stored)})
}

func (h CommerceHandler) withdrawPromotion(w http.ResponseWriter, r *http.Request) {
	var dto withdrawPromotionDTO
	if !readCommerce(w, r, &dto) {
		return
	}
	accountID, ok := h.sessionAccount(w, r, dto.commerceSessionDTO)
	if !ok {
		return
	}
	stored, err := application.WithdrawPromotion(r.Context(), h.Ports(r), accountID, chi.URLParam(r, "station_id"), dto.PromotionID)
	if err != nil {
		writeCommerceError(w, r, err)
		return
	}
	writeCommerceJSON(w, r, http.StatusOK, "no-store", map[string]any{"promotion": h.promotionProjection(stored)})
}

func (h CommerceHandler) announcedRead(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Reads.ListAnnouncedPrices(r.Context(), chi.URLParam(r, "station_id"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "commerce.unavailable", "commerce service unavailable", nil)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, stored := range rows {
		out = append(out, h.announcedProjection(stored))
	}
	writeCommerceJSON(w, r, http.StatusOK, "public, max-age=30, s-maxage=60", map[string]any{"announced_prices": out, "source": "STATION_ANNOUNCED"})
}

func (h CommerceHandler) promotionsRead(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Reads.ListPromotions(r.Context(), chi.URLParam(r, "station_id"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "commerce.unavailable", "commerce service unavailable", nil)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, stored := range rows {
		out = append(out, h.promotionProjection(stored))
	}
	writeCommerceJSON(w, r, http.StatusOK, "public, max-age=30, s-maxage=60", map[string]any{"promotions": out, "source": "STATION_PROMOTION"})
}

// announcedProjection is the bounded public DTO: product, unit, price,
// conditions, author attribution window and the derived display state
// (EMP-STN-003: every displayed price carries source, condition and age).
func (h CommerceHandler) announcedProjection(stored application.StoredAnnouncedPrice) map[string]any {
	return map[string]any{
		"id": stored.ID, "fuel_product": stored.Product, "unit": stored.Unit,
		"price_milli_brl": stored.PriceMilliBRL, "conditions": stored.Conditions,
		"valid_from": stored.ValidFrom.Format(time.RFC3339Nano), "valid_until": stored.ValidUntil.Format(time.RFC3339Nano),
		"state": stored.State, "display_state": AnnouncedDisplayState(stored.State, h.now(), stored.ValidFrom, stored.ValidUntil),
	}
}

func (h CommerceHandler) promotionProjection(stored application.StoredPromotion) map[string]any {
	return map[string]any{
		"id": stored.ID, "label": stored.Label, "description": stored.Description,
		"valid_from": stored.ValidFrom.Format(time.RFC3339Nano), "valid_until": stored.ValidUntil.Format(time.RFC3339Nano),
		"state": stored.State, "display_state": PromotionDisplayState(stored.State, h.now(), stored.ValidFrom, stored.ValidUntil),
	}
}

func readCommerce(w http.ResponseWriter, r *http.Request, dst any) bool {
	raw, err := httpapi.ReadBody(r, 8<<10)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "commerce.bad-request", "invalid commercial input", nil)
		return false
	}
	if httpapi.DecodeJSON(raw, dst) != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "commerce.bad-request", "invalid commercial input", nil)
		return false
	}
	return true
}

func writeCommerceJSON(w http.ResponseWriter, r *http.Request, status int, cacheControl string, body any) {
	raw, err := json.Marshal(body)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "commerce.unavailable", "commerce service unavailable", nil)
		return
	}
	httpapi.WriteJSON(w, r, status, cacheControl, raw)
}

func writeCommerceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, application.ErrCommerceGrant), errors.Is(err, application.ErrCommerceStale), errors.Is(err, application.ErrAccountGone):
		httpapi.WriteError(w, r, http.StatusForbidden, "commerce.grant-required", "active price.publish scope required", nil)
	case errors.Is(err, application.ErrCommerceNotFound), errors.Is(err, application.ErrClaimNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "commerce.not-found", "commercial record not found", nil)
	case errors.Is(err, application.ErrCommerceInvalid):
		httpapi.WriteError(w, r, http.StatusBadRequest, "commerce.bad-request", "invalid commercial input", nil)
	default:
		if code := domain.CommerceErrorCode(err); code != "" {
			httpapi.WriteError(w, r, http.StatusBadRequest, "commerce."+code, "invalid commercial input", nil)
			return
		}
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "commerce.unavailable", "commerce service unavailable", nil)
	}
}
