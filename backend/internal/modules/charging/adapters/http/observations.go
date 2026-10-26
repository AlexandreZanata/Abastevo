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
	r.Post("/v1/charging/observations/mine", h.mine)
	r.Post("/v1/charging/observations/{id}/helpfulness", h.vote)
	r.Post("/v1/charging/observations/{id}/report", h.report)
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
		Session     session                          `json:"session"`
		ID          string                           `json:"id"`
		PlaceID     *string                          `json:"place_id"`
		Connector   string                           `json:"connector_code"`
		Price       *int64                           `json:"price_milli_brl_per_kwh"`
		Condition   string                           `json:"condition"`
		ObservedDay string                           `json:"observed_day"`
		Note        string                           `json:"note"`
		Operational *chargingdomain.OperationalEvent `json:"operational"`
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
		ObservedDay: body.ObservedDay, Note: body.Note, Operational: body.Operational,
	})
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id})
}

func (h CommandsHandler) correct(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session     session                          `json:"session"`
		Connector   string                           `json:"connector_code"`
		Price       *int64                           `json:"price_milli_brl_per_kwh"`
		Condition   string                           `json:"condition"`
		ObservedDay string                           `json:"observed_day"`
		Note        string                           `json:"note"`
		Operational *chargingdomain.OperationalEvent `json:"operational"`
		Expected    int64                            `json:"expected_revision"`
		PlaceID     *string                          `json:"place_id"`
		ID          string                           `json:"id"`
		OperationID string                           `json:"operation_id"`
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
	revision, err := h.Store.CorrectCommand(r.Context(), owner, chargingdomain.Observation{
		ID: chi.URLParam(r, "id"), PlaceID: body.PlaceID, ConnectorCode: body.Connector,
		PriceMilliBRL: body.Price, Condition: body.Condition,
		ObservedDay: body.ObservedDay, Note: body.Note, Operational: body.Operational,
	}, body.Expected, body.OperationID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"revision": revision})
}

func (h CommandsHandler) withdraw(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session     session `json:"session"`
		Expected    int64   `json:"expected_revision"`
		OperationID string  `json:"operation_id"`
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
	revision, err := h.Store.WithdrawCommand(r.Context(), owner, chi.URLParam(r, "id"), body.Expected, body.OperationID)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"revision": revision})
}

func (h CommandsHandler) mine(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session session `json:"session"`
		Limit   int     `json:"limit"`
		PlaceID string  `json:"place_id"`
	}
	if httpapi.DecodeJSON(mustRead(r), &body) != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-command", "command refused", nil)
		return
	}
	if body.Limit == 0 {
		body.Limit = 20
	}
	if body.Limit < 1 || body.Limit > 100 {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-limit", "limit must be 1..100", nil)
		return
	}
	owner, err := h.principal(r, body.Session)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "charging.unauthorized", "authentication required", nil)
		return
	}
	items, err := prepared.MineForPlace(r.Context(), h.Store.Pool, owner, body.PlaceID, body.Limit)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	wire := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := map[string]any{"id": item.ID, "connector_code": item.ConnectorCode,
			"condition": item.Condition, "observed_day": item.ObservedDay,
			"note": item.Note, "revision": item.Revision}
		if item.PlaceID != nil {
			entry["place_id"] = *item.PlaceID
		}
		if item.PriceMilliBRL != nil {
			entry["price_milli_brl_per_kwh"] = *item.PriceMilliBRL
		}
		if item.Operational != nil {
			entry["operational"] = item.Operational
		}
		wire = append(wire, entry)
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"items": wire})
}

func (h CommandsHandler) targetOwner(w http.ResponseWriter, r *http.Request, session session) (string, string, bool) {
	owner, err := h.principal(r, session)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "charging.unauthorized", "authentication required", nil)
		return "", "", false
	}
	target := chi.URLParam(r, "id")
	author, err := prepared.ObservationAccount(r.Context(), h.Store.Pool, target)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-target", "target unavailable", nil)
		return "", "", false
	}
	if author == owner {
		httpapi.WriteError(w, r, http.StatusForbidden, "charging.self-action", "cannot act on your own report", nil)
		return "", "", false
	}
	return owner, target, true
}

func (h CommandsHandler) vote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session      session `json:"session"`
		Choice       string  `json:"choice"`
		SubmissionID string  `json:"client_submission_id"`
	}
	if httpapi.DecodeJSON(mustRead(r), &body) != nil || len(body.SubmissionID) < 1 || len(body.SubmissionID) > 128 {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-command", "command refused", nil)
		return
	}
	owner, target, ok := h.targetOwner(w, r, body.Session)
	if !ok {
		return
	}
	if err := prepared.Vote(r.Context(), h.Store.Pool, owner, target, body.Choice, body.SubmissionID); err != nil {
		writeStoreError(w, r, err)
		return
	}
	helpful, notHelpful, err := prepared.Tally(r.Context(), h.Store.Pool, target)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{
		"observation_id": target, "choice": body.Choice,
		"helpful": helpful, "not_helpful": notHelpful,
	})
}

func (h CommandsHandler) report(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Session session `json:"session"`
		Reason  string  `json:"reason"`
		Detail  string  `json:"detail"`
	}
	if httpapi.DecodeJSON(mustRead(r), &body) != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "charging.bad-command", "command refused", nil)
		return
	}
	owner, target, ok := h.targetOwner(w, r, body.Session)
	if !ok {
		return
	}
	if err := prepared.Report(r.Context(), h.Store.Pool, owner, target, body.Reason, body.Detail); err != nil {
		writeStoreError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"observation_id": target, "status": "received"})
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
