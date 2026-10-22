package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/prepared"
	chargingdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

// CommandsHandler serves signed community EV observation commands. Authentication
// reuses account sessions (signed-in free accounts, no payment gate);
// reporters stay anonymous in reads (account IDs never leave writes).
// Forged, cross-owner and replayed bodies fail closed.
type CommandsHandler struct {
	Store    prepared.ObservationStore
	Sessions func(ctx context.Context, familyID, accessToken string) (string, error)
}

type session struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

// RegisterRoutes mounts observation commands without disturbing fuel,
// ANP, Directory or charging read routes.
func (h CommandsHandler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/charging/observations", h.submit)
	r.Put("/v1/charging/observations/{id}", h.correct)
	r.Delete("/v1/charging/observations/{id}", h.withdraw)
}

func (h CommandsHandler) principal(r *http.Request, s session) (string, error) {
	if len(s.FamilyID) != 36 || len(s.AccessToken) < 1 || len(s.AccessToken) > 512 {
		return "", chargingdomain.ErrNotOwner
	}
	if h.Sessions == nil {
		return "", chargingdomain.ErrNotOwner
	}
	owner, err := h.Sessions(r.Context(), s.FamilyID, s.AccessToken)
	if err != nil || owner == "" {
		return "", chargingdomain.ErrNotOwner
	}
	return owner, nil
}

func (h CommandsHandler) submit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session     session `json:"session"`
		ID          string  `json:"id"`
		PlaceID     *string `json:"place_id"`
		Connector   string  `json:"connector_code"`
		Price       *int64  `json:"price_milli_brl_per_kwh"`
		Condition   string  `json:"condition"`
		ObservedDay string  `json:"observed_day"`
		Note        string  `json:"note"`
	}
	if httpapi.DecodeJSON(mustRead(r), &body) != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-command", "command refused", nil)
		return
	}
	owner, err := h.principal(r, body.Session)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "charging.unauthorized", "authentication required", nil)
		return
	}
	id, err := h.Store.Submit(r.Context(), owner, chargingdomain.Observation{
		ID: body.ID, PlaceID: body.PlaceID, ConnectorCode: body.Connector,
		PriceMilliBRL: body.Price, Condition: body.Condition,
		ObservedDay: body.ObservedDay, Note: body.Note,
	})
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id})
}

func (h CommandsHandler) correct(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session     session `json:"session"`
		Connector   string  `json:"connector_code"`
		Price       *int64  `json:"price_milli_brl_per_kwh"`
		Condition   string  `json:"condition"`
		ObservedDay string  `json:"observed_day"`
		Note        string  `json:"note"`
		Expected    int64   `json:"expected_revision"`
	}
	if httpapi.DecodeJSON(mustRead(r), &body) != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-command", "command refused", nil)
		return
	}
	owner, err := h.principal(r, body.Session)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "charging.unauthorized", "authentication required", nil)
		return
	}
	revision, err := h.Store.Correct(r.Context(), owner, chargingdomain.Observation{
		ID: chi.URLParam(r, "id"), ConnectorCode: body.Connector,
		PriceMilliBRL: body.Price, Condition: body.Condition,
		ObservedDay: body.ObservedDay, Note: body.Note,
	}, body.Expected)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"revision": revision})
}

func (h CommandsHandler) withdraw(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session  session `json:"session"`
		Expected int64   `json:"expected_revision"`
	}
	if httpapi.DecodeJSON(mustRead(r), &body) != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-command", "command refused", nil)
		return
	}
	owner, err := h.principal(r, body.Session)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "charging.unauthorized", "authentication required", nil)
		return
	}
	revision, err := h.Store.Withdraw(r.Context(), owner, chi.URLParam(r, "id"), body.Expected)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"revision": revision})
}

func writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case chargingdomain.ErrNotOwner:
		httpapi.WriteError(w, r, http.StatusUnauthorized, "charging.unauthorized", "authentication required", nil)
	case chargingdomain.ErrQuotaExceeded:
		httpapi.WriteError(w, r, http.StatusTooManyRequests, "charging.quota", "quota exceeded", nil)
	case chargingdomain.ErrConflict:
		httpapi.WriteError(w, r, http.StatusConflict, "charging.conflict", "revision conflict", nil)
	default:
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-command", "command refused", nil)
	}
}

func mustRead(r *http.Request) []byte {
	raw := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			raw = append(raw, buf[:n]...)
			if len(raw) > 65536 {
				break
			}
		}
		if err != nil {
			break
		}
	}
	_ = json.Valid(raw)
	return raw
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, body map[string]any) {
	raw, _ := json.Marshal(body)
	httpapi.WriteJSON(w, r, status, "no-store", raw)
}
