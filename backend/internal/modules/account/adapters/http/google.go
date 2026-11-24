package http

import (
	"errors"
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

func (h Handler) linkGoogleNative(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		FamilyID    string `json:"family_id"`
		AccessToken string `json:"access_token"`
		Provider    string `json:"provider"`
		IDToken     string `json:"id_token"`
		Nonce       string `json:"nonce"`
	}
	if err := read(r, &dto); err != nil || dto.FamilyID == "" || dto.AccessToken == "" ||
		dto.Provider != "google" || !domain.ValidProviderToken(dto.IDToken) || !domain.ValidNativeNonce(dto.Nonce) {
		bad(w, r)
		return
	}
	link, err := h.Service.LinkGoogleNative(r.Context(), dto.FamilyID, dto.AccessToken, application.ProviderLogin{
		Provider: dto.Provider, IDToken: dto.IDToken, Nonce: dto.Nonce,
	})
	if errors.Is(err, domain.ErrLinkCrossAccount) {
		httpapi.WriteError(w, r, http.StatusConflict, "account.link-cross-account-refused", "provider proof belongs to another account", nil)
		return
	}
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, http.StatusOK, map[string]any{"provider_link": providerLinkJSONOf(link)})
}
