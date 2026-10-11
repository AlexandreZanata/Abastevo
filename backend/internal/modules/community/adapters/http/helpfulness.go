package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// helpfulCastDTO is the strict wire parse for helpfulness votes:
// frozen choice, immutable revision and client operation identity.
// Unknown fields rejected; numbers in canonical form.
type helpfulCastDTO struct {
	Choice             string      `json:"choice"`
	DecisionSequence   json.Number `json:"decision_sequence"`
	ClientSubmissionID string      `json:"client_submission_id"`
}

// helpfulRemoveDTO is the strict wire parse for helpfulness removals.
type helpfulRemoveDTO struct {
	DecisionSequence json.Number `json:"decision_sequence"`
}

func parseHelpfulSequence(raw json.Number) (int64, error) {
	n, err := raw.Int64()
	if err != nil || n < 1 {
		return 0, errors.New("decision_sequence must be a positive integer")
	}
	return n, nil
}

func parseCastHelpfulBody(raw []byte) (application.CastDTO, error) {
	if !utf8.Valid(raw) {
		return application.CastDTO{}, errors.New("body must be valid UTF-8")
	}
	var in helpfulCastDTO
	if err := httpapi.DecodeJSON(raw, &in); err != nil {
		return application.CastDTO{}, err
	}
	seq, err := parseHelpfulSequence(in.DecisionSequence)
	if err != nil {
		return application.CastDTO{}, err
	}
	if !domain.ValidChoice(domain.Choice(in.Choice)) {
		return application.CastDTO{}, errors.New("choice must be HELPFUL or NOT_HELPFUL")
	}
	if strings.TrimSpace(in.ClientSubmissionID) == "" {
		return application.CastDTO{}, errors.New("client_submission_id required")
	}
	return application.CastDTO{
		DecisionSequence: seq, Choice: domain.Choice(in.Choice),
		ClientSubmissionID: in.ClientSubmissionID,
	}, nil
}

func parseRemoveHelpfulBody(raw []byte) (int64, error) {
	if !utf8.Valid(raw) {
		return 0, errors.New("body must be valid UTF-8")
	}
	var in helpfulRemoveDTO
	if err := httpapi.DecodeJSON(raw, &in); err != nil {
		return 0, err
	}
	return parseHelpfulSequence(in.DecisionSequence)
}

// ReviewNotice is the frozen public wording for tallies under open
// contribution review. The literal is pinned by tests; translate it
// only through a frozen-string change with contract review, never
// with per-case text (case internals stay operator-side).
const ReviewNotice = "contribution under review"

// helpfulTallyBody renders counts plus the private owner overlay. Voter,
// account, key, device and cursor internals never appear here.
func helpfulTallyBody(observationID string, sequence int64, tally domain.Tally, share domain.Share, myChoice *domain.Choice, underReview bool) map[string]any {
	out := map[string]any{
		"observation_id":    observationID,
		"decision_sequence": sequence,
		"helpful":           tally.Helpful,
		"not_helpful":       tally.NotHelpful,
		"total":             tally.Helpful + tally.NotHelpful,
	}
	if share.HasVotes {
		out["helpful_basis_points"] = share.BasisPoints
	}
	if myChoice != nil {
		out["my_choice"] = string(*myChoice)
	}
	if underReview {
		out["under_review"] = true
		out["review_notice"] = ReviewNotice
	}
	return out
}

func (h Handler) castHelpfulness(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	raw, ok := readVoteBody(w, r)
	if !ok {
		return
	}
	dto, err := parseCastHelpfulBody(raw)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-body", "malformed helpfulness vote",
			[]httpapi.Detail{{Field: "body", Code: "malformed"}})
		return
	}
	dto.ObservationID = chi.URLParam(r, "observation_id")
	result, err := h.CastHelpfulness(r.Context(), caller, dto.ObservationID, dto)
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	bodyMap := helpfulTallyBody(dto.ObservationID, result.Tally.DecisionSequence, result.Tally, result.Share, &result.MyChoice, false)
	bodyMap["vote_id"] = result.VoteID
	bodyMap["received_at"] = result.Vote.CreatedAt.Format(time.RFC3339)
	body, _ := json.Marshal(bodyMap)
	httpapi.WriteJSON(w, r, http.StatusCreated, "no-store", body)
}

func (h Handler) removeHelpfulness(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	raw, ok := readVoteBody(w, r)
	if !ok {
		return
	}
	sequence, err := parseRemoveHelpfulBody(raw)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-body", "malformed helpfulness removal",
			[]httpapi.Detail{{Field: "body", Code: "malformed"}})
		return
	}
	observationID := chi.URLParam(r, "observation_id")
	res, err := h.RemoveHelpfulness(r.Context(), caller, observationID, sequence)
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	body, _ := json.Marshal(helpfulTallyBody(observationID, res.Tally.DecisionSequence, res.Tally, res.Share, res.MyChoice, res.UnderReview))
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

func (h Handler) helpfulnessTally(w http.ResponseWriter, r *http.Request) {
	// Anonymous users can browse counts; the owner overlay only travels
	// the authenticated read path, never public caches.
	var caller application.Caller
	if c, err := h.Authenticate(r); err == nil {
		caller = c
	}
	sequence, err := parseHelpfulQuerySequence(r.URL.Query().Get("decision_sequence"))
	if err != nil {
		httpapi.WriteError(w, r, http.StatusBadRequest, "community.bad-tally-filter", "decision_sequence invalid", nil)
		return
	}
	observationID := chi.URLParam(r, "observation_id")
	res, err := h.HelpfulnessTally(r.Context(), caller, observationID, sequence)
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	body, _ := json.Marshal(helpfulTallyBody(observationID, res.Tally.DecisionSequence, res.Tally, res.Share, res.MyChoice, res.UnderReview))
	httpapi.WriteJSON(w, r, http.StatusOK, "no-store", body)
}

func parseHelpfulQuerySequence(raw string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n < 1 {
		return 0, errors.New("decision_sequence must be a positive integer")
	}
	return n, nil
}
