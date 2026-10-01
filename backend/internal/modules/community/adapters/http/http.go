package http

import (
	"context"
	"encoding/json"
	"errors"
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
	Confirm      func(ctx context.Context, caller application.Caller, observationID string, dto application.ConfirmDTO, body []byte) (application.ConfirmResult, error)
	Dispute      func(ctx context.Context, caller application.Caller, observationID string, dto application.DisputeDTO, body []byte) (application.DisputeResult, error)
	Secrets      []byte
}

// RegisterRoutes mounts community endpoints under /v1.
func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/observations", h.submit)
	r.Get("/v1/observations/{observation_id}", h.status)
	r.Get("/v1/contributors/me/observations", h.history)
	r.Post("/v1/observations/{observation_id}/confirmations", h.confirm)
	r.Post("/v1/observations/{observation_id}/disputes", h.dispute)
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
	case errors.Is(err, application.ErrAccountBlocked):
		httpapi.WriteError(w, r, http.StatusForbidden, "community.account-blocked", "account suspended or deleted", nil)
	case errors.Is(err, application.ErrTargetNotFound):
		httpapi.WriteError(w, r, http.StatusNotFound, "community.not-found", "observation not found", nil)
	case errors.Is(err, domain.ErrSelfConfirmation):
		httpapi.WriteError(w, r, http.StatusForbidden, "community.self-confirmation", "contributors cannot confirm their own observations", nil)
	case errors.Is(err, application.ErrIneligibleTarget):
		httpapi.WriteError(w, r, http.StatusConflict, "community.ineligible-target", "target not eligible for this command", nil)
	case errors.Is(err, domain.ErrAlreadyConfirmed):
		httpapi.WriteError(w, r, http.StatusConflict, "community.already-confirmed", "observation already confirmed by contributor", nil)
	case errors.Is(err, application.ErrConflict):
		httpapi.WriteError(w, r, http.StatusConflict, "community.conflict", "same key, different body", nil)
	case errors.Is(err, application.ErrLocationForged):
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.location-forged", "location claim contradicts its metadata",
			[]httpapi.Detail{{Field: "location", Code: "forged"}})
	case errors.Is(err, domain.ErrUnknownReason),
		errors.Is(err, domain.ErrInvalidDispute),
		errors.Is(err, domain.ErrInvalidConfirmation):
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.invalid", "invalid vote or report",
			[]httpapi.Detail{{Field: "body", Code: "invalid"}})
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
	EvidenceID        *string      `json:"evidence_id"`
	ClaimedCapturedAt *string      `json:"claimed_captured_at"`
	SupersedesID      *string      `json:"supersedes_observation_id"`
	Location          *locationDTO `json:"location"`
}

// locationDTO is the strict wire parse for the optional device
// location claim: unknown fields rejected, numbers in canonical
// decimal form, verdict semantics checked by the application layer
// (a lying verdict refuses as forged, not malformed).
type locationDTO struct {
	ClaimedVerdict    string       `json:"verdict"`
	PermissionGranted bool         `json:"permission_granted"`
	HasFix            bool         `json:"has_fix"`
	SourceInfoPresent bool         `json:"source_info_present"`
	Simulated         bool         `json:"simulated"`
	AccuracyMeters    *json.Number `json:"accuracy_m"`
	ClockSkewSeconds  *json.Number `json:"clock_skew_s"`
	Manual            bool         `json:"manual"`
	CapturedAt        *string      `json:"captured_at"`
	Latitude          *json.Number `json:"lat"`
	Longitude         *json.Number `json:"lon"`
}

// parseLocationNumber parses an optional canonical JSON number: plain
// decimal form only (no NaN/Infinity/exponent tricks), range-checked
// by the caller.
func parseLocationNumber(raw *json.Number, field string) (*float64, error) {
	if raw == nil {
		return nil, nil
	}
	text := raw.String()
	if text == "" {
		return nil, errors.New(field + " required")
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, errors.New(field + " must be a number")
	}
	return &value, nil
}

func parseSubmitBody(raw []byte) (application.SubmitDTO, error) {
	var in submitDTO
	if err := httpapi.DecodeJSON(raw, &in); err != nil {
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
	if in.Location != nil {
		loc, err := parseLocationEvidence(in.Location)
		if err != nil {
			return application.SubmitDTO{}, err
		}
		out.Location = loc
	}
	return out, nil
}

// parseLocationEvidence validates the optional location claim block.
// Shapes refuse here (400 malformed); verdict semantics refuse later
// as forged claims, so a lying verdict is distinguishable from a
// malformed body.
func parseLocationEvidence(in *locationDTO) (*application.LocationEvidence, error) {
	if strings.TrimSpace(in.ClaimedVerdict) == "" {
		return nil, errors.New("location verdict required")
	}
	out := &application.LocationEvidence{
		ClaimedVerdict:    strings.TrimSpace(in.ClaimedVerdict),
		PermissionGranted: in.PermissionGranted,
		HasFix:            in.HasFix,
		SourceInfoPresent: in.SourceInfoPresent,
		Simulated:         in.Simulated,
		Manual:            in.Manual,
	}
	accuracy, err := parseLocationNumber(in.AccuracyMeters, "accuracy_m")
	if err != nil {
		return nil, err
	}
	if accuracy != nil && *accuracy < 0 {
		return nil, errors.New("accuracy_m must be non-negative")
	}
	out.AccuracyMeters = accuracy
	skew, err := parseLocationNumber(in.ClockSkewSeconds, "clock_skew_s")
	if err != nil {
		return nil, err
	}
	if skew != nil {
		whole := int64(*skew)
		if float64(whole) != *skew {
			return nil, errors.New("clock_skew_s must be whole seconds")
		}
		out.ClockSkewSeconds = &whole
	}
	if in.CapturedAt != nil {
		at, err := time.Parse(time.RFC3339, *in.CapturedAt)
		if err != nil {
			return nil, err
		}
		out.CapturedAt = &at
	}
	lat, err := parseLocationNumber(in.Latitude, "lat")
	if err != nil {
		return nil, err
	}
	if lat != nil && (*lat < -90 || *lat > 90) {
		return nil, errors.New("lat out of range")
	}
	out.Latitude = lat
	lon, err := parseLocationNumber(in.Longitude, "lon")
	if err != nil {
		return nil, err
	}
	if lon != nil && (*lon < -180 || *lon > 180) {
		return nil, errors.New("lon out of range")
	}
	out.Longitude = lon
	return out, nil
}

func (h Handler) submit(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	raw, err := httpapi.ReadBody(r, 64<<10)
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

// confirmDTO is the strict wire parse for support votes.
type confirmDTO struct {
	ClientSubmissionID string `json:"client_submission_id"`
}

// disputeDTO is the strict wire parse for structured reports.
type disputeDTO struct {
	ClientSubmissionID       string  `json:"client_submission_id"`
	Reason                   string  `json:"reason"`
	Detail                   string  `json:"detail"`
	ReplacementObservationID *string `json:"replacement_observation_id"`
}

func parseConfirmBody(raw []byte) (application.ConfirmDTO, error) {
	var in confirmDTO
	if err := httpapi.DecodeJSON(raw, &in); err != nil {
		return application.ConfirmDTO{}, err
	}
	if strings.TrimSpace(in.ClientSubmissionID) == "" {
		return application.ConfirmDTO{}, errors.New("client_submission_id required")
	}
	return application.ConfirmDTO{ClientSubmissionID: in.ClientSubmissionID}, nil
}

func parseDisputeBody(raw []byte) (application.DisputeDTO, error) {
	var in disputeDTO
	if err := httpapi.DecodeJSON(raw, &in); err != nil {
		return application.DisputeDTO{}, err
	}
	if strings.TrimSpace(in.ClientSubmissionID) == "" || strings.TrimSpace(in.Reason) == "" {
		return application.DisputeDTO{}, errors.New("client_submission_id and reason required")
	}
	out := application.DisputeDTO{
		ClientSubmissionID: in.ClientSubmissionID,
		Reason:             in.Reason,
		Detail:             in.Detail,
	}
	if in.ReplacementObservationID != nil {
		out.ReplacementID = *in.ReplacementObservationID
	}
	return out, nil
}

func readVoteBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	raw, err := httpapi.ReadBody(r, 64<<10)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-body", "unreadable body",
			[]httpapi.Detail{{Field: "body", Code: "unreadable"}})
		return nil, false
	}
	return raw, true
}

func (h Handler) confirm(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	raw, ok := readVoteBody(w, r)
	if !ok {
		return
	}
	dto, err := parseConfirmBody(raw)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-body", "malformed confirmation",
			[]httpapi.Detail{{Field: "body", Code: "malformed"}})
		return
	}
	result, err := h.Confirm(r.Context(), caller, chi.URLParam(r, "observation_id"), dto, raw)
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"confirmation_id": result.ConfirmationID,
		"observation_id":  chi.URLParam(r, "observation_id"),
		"received_at":     result.ReceivedAt.Format(time.RFC3339),
	})
	httpapi.WriteJSON(w, r, http.StatusCreated, "no-store", body)
}

func (h Handler) dispute(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	raw, ok := readVoteBody(w, r)
	if !ok {
		return
	}
	dto, err := parseDisputeBody(raw)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-body", "malformed dispute",
			[]httpapi.Detail{{Field: "body", Code: "malformed"}})
		return
	}
	result, err := h.Dispute(r.Context(), caller, chi.URLParam(r, "observation_id"), dto, raw)
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	body, _ := json.Marshal(map[string]any{
		"dispute_id":            result.DisputeID,
		"target_observation_id": chi.URLParam(r, "observation_id"),
		"state":                 result.State,
		"received_at":           result.ReceivedAt.Format(time.RFC3339),
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
