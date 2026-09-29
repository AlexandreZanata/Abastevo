package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Handler serves community writes and owner reads. Authentication already
// happened upstream: Authenticate resolves the caller from the verified
// proof, so this package never sees keys or signatures.
type Handler struct {
	Authenticate func(r *http.Request) (application.Caller, error)
	Submit       func(ctx context.Context, caller application.Caller, key string, dto application.SubmitDTO, body []byte) (application.SubmitResult, bool, error)
	Status       func(ctx context.Context, caller application.Caller, id string) (application.StatusResult, error)
	History      func(ctx context.Context, caller application.Caller, limit int, after time.Time, afterID string, hasCursor bool) ([]application.HistoryItem, string, error)
	Secrets      []byte
}

// RegisterRoutes mounts community endpoints under /v1.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/observations", h.submit)
	r.Get("/v1/observations/{observation_id}", h.status)
	r.Get("/v1/contributors/me/observations", h.history)
}

func generatedNow() string { return time.Now().UTC().Format(time.RFC3339) }

func (h Handler) caller(w http.ResponseWriter, r *http.Request) (application.Caller, bool) {
	caller, err := h.Authenticate(r)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusUnauthorized, "community.auth-required", "authentication required", nil)
		return application.Caller{}, false
	}
	return caller, true
}

func writeAPIError(w http.ResponseWriter, r *http.Request, err error) {
	var denied *application.QuotaDeniedError
	if errors.As(err, &denied) {
		w.Header().Set("Retry-After", strconv.FormatInt(int64(denied.RetryAfter/time.Second), 10))
		httpapi.WriteError(w, r, http.StatusTooManyRequests, "community.quota-exceeded", "quota exhausted, retry later", nil)
		return
	}
	switch {
	case errors.Is(err, application.ErrUnauthorized):
		httpapi.WriteError(w, r, http.StatusUnauthorized, "community.auth-required", "authentication required", nil)
	case errors.Is(err, application.ErrConflict):
		httpapi.WriteError(w, r, http.StatusConflict, "community.conflict", "same key, different body", nil)
	default:
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.invalid", "invalid submission",
			[]httpapi.Detail{{Field: "body", Code: "invalid"}})
	}
}

// submitDTO is the strict wire parse: unknown fields rejected, amounts kept
// in canonical integer form (no float, no exponent).
type submitDTO struct {
	ClientSubmissionID string `json:"client_submission_id"`
	StationID          string `json:"station_id"`
	Product            string `json:"fuel_product"`
	Price              struct {
		Amount   json.Number `json:"amount_milli_brl"`
		Currency string      `json:"currency"`
		Unit     string      `json:"unit"`
	} `json:"price"`
	Condition struct {
		Kind        string  `json:"kind"`
		QualifierID *string `json:"qualifier_id"`
	} `json:"condition"`
	EvidenceID        *string `json:"evidence_id"`
	ClaimedCapturedAt *string `json:"claimed_captured_at"`
	SupersedesID      *string `json:"supersedes_observation_id"`
}

func parseSubmitBody(raw []byte) (application.SubmitDTO, error) {
	var in submitDTO
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return application.SubmitDTO{}, err
	}
	amountStr := in.Price.Amount.String()
	if amountStr == "" {
		return application.SubmitDTO{}, errors.New("amount required")
	}
	for _, r := range amountStr {
		if r < '0' || r > '9' {
			return application.SubmitDTO{}, errors.New("amount must be a canonical integer")
		}
	}
	amount, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil {
		return application.SubmitDTO{}, err
	}
	if in.Price.Currency != "" && in.Price.Currency != "BRL" {
		return application.SubmitDTO{}, errors.New("currency must be BRL")
	}
	out := application.SubmitDTO{
		ClientSubmissionID: in.ClientSubmissionID,
		StationID:          in.StationID,
		Product:            in.Product,
		Unit:               in.Price.Unit,
		AmountMilli:        amount,
		ConditionKind:      in.Condition.Kind,
	}
	if in.Condition.QualifierID != nil {
		out.Qualifier = *in.Condition.QualifierID
	}
	if in.EvidenceID != nil {
		out.EvidenceID = *in.EvidenceID
	}
	if in.SupersedesID != nil {
		out.SupersedesID = *in.SupersedesID
	}
	if in.ClaimedCapturedAt != nil {
		day, err := time.Parse(time.RFC3339, *in.ClaimedCapturedAt)
		if err != nil {
			return application.SubmitDTO{}, err
		}
		out.ClaimedCapturedAt = day
	}
	return out, nil
}

func (h Handler) submit(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-body", "unreadable body",
			[]httpapi.Detail{{Field: "body", Code: "unreadable"}})
		return
	}
	dto, err := parseSubmitBody(raw)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-body", "malformed submission",
			[]httpapi.Detail{{Field: "body", Code: "malformed"}})
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.key-required", "Idempotency-Key required",
			[]httpapi.Detail{{Field: "Idempotency-Key", Code: "required"}})
		return
	}
	result, _, err := h.Submit(r.Context(), caller, key, dto, raw)
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"id":               result.ObservationID,
		"validation_state": result.State,
		"received_at":      result.ReceivedAt.Format(time.RFC3339),
		"status_url":       "/v1/observations/" + result.ObservationID,
	})
	httpapi.WriteJSON(w, r, http.StatusCreated, "no-store", body)
}

func (h Handler) status(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "observation_id")
	res, err := h.Status(r.Context(), caller, id)
	if err != nil {
		if errors.Is(err, application.ErrUnauthorized) {
			httpapi.WriteError(w, r, http.StatusNotFound, "community.not-found", "observation not found", nil)
			return
		}
		writeAPIError(w, r, err)
		return
	}
	decisions := make([]map[string]any, 0, len(res.Decisions))
	for _, d := range res.Decisions {
		decisions = append(decisions, map[string]any{
			"sequence": d.Sequence, "to_state": d.ToState,
			"reason_codes": d.ReasonCodes,
			"occurred_at":  d.OccurredAt.Format(time.RFC3339),
			"actor":        d.Actor,
		})
	}
	body, _ := json.Marshal(map[string]any{
		"observation":      wireObservation(res.Observation, res.State),
		"validation_state": res.State,
		"decisions":        decisions,
	})
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

func (h Handler) history(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, err := httpapi.ParseLimit(q.Get("limit"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-limit", "limit must be 1..100",
			[]httpapi.Detail{{Field: "limit", Code: "range"}})
		return
	}
	fh := httpapi.FilterHash("own-history", caller.Token, strconv.Itoa(limit))
	lastKey, err := httpapi.ParseCursor(h.Secrets, q.Get("cursor"), fh, time.Now())
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-cursor", "cursor invalid, expired or changed",
			[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
		return
	}
	var after time.Time
	var afterID string
	var hasCursor bool
	if lastKey != "" {
		date, id, cut := splitCursor(lastKey)
		if !cut {
			httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-cursor", "cursor invalid, expired or changed",
				[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
			return
		}
		day, derr := time.Parse(time.RFC3339Nano, date)
		if derr != nil {
			httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-cursor", "cursor invalid, expired or changed",
				[]httpapi.Detail{{Field: "cursor", Code: "invalid"}})
			return
		}
		after, afterID, hasCursor = day, id, true
	}
	rows, nextKey, err := h.History(r.Context(), caller, limit, after, afterID, hasCursor)
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, it := range rows {
		items = append(items, wireObservation(it.Observation, it.State))
	}
	var next *string
	if nextKey != "" {
		sealed := httpapi.Seal(h.Secrets, fh, nextKey, time.Now())
		next = &sealed
	}
	body, _ := json.Marshal(map[string]any{
		"items": items, "next_cursor": next, "generated_at": generatedNow(),
	})
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

// splitCursor separates the RFC3339 instant from the UUID tail at the last
// colon: timestamps contain colons, UUIDs never do, so the split is stable
// for any zone rendering ("Z" or "+00:00").
func splitCursor(key string) (string, string, bool) {
	idx := strings.LastIndex(key, ":")
	if idx <= 0 || idx+1 >= len(key) {
		return "", "", false
	}
	return key[:idx], key[idx+1:], true
}

func wireObservation(o domain.Observation, state string) map[string]any {
	var qualifier any
	if o.QualifierKey != "" {
		qualifier = o.QualifierKey
	}
	return map[string]any{
		"id":               o.ID,
		"station_id":       o.StationID,
		"fuel_product":     o.Product,
		"unit":             o.Unit,
		"amount_milli_brl": o.AmountMilli,
		"condition":        map[string]any{"kind": o.ConditionKind, "qualifier_id": qualifier},
		"validation_state": state,
		"received_at":      o.ReceivedAt.Format(time.RFC3339),
		"policy_version":   o.PolicyVersion,
	}
}
