package http

import (
	"encoding/json"
	"net/http"
	"strings"

	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

func (h Handler) photoCapture(w http.ResponseWriter, r *http.Request) {
	caller, ok := h.caller(w, r)
	if !ok {
		return
	}
	if h.PhotoCapture == nil {
		httpapi.WriteError(w, r, 503, "community.capture-unavailable", "photo capture unavailable", nil)
		return
	}
	raw, err := httpapi.ReadBody(r, 16<<10)
	var in struct {
		ClientCaptureID string       `json:"client_capture_id"`
		StationID       string       `json:"station_id"`
		Location        *locationDTO `json:"location"`
	}
	if err == nil {
		err = httpapi.DecodeJSON(raw, &in)
	}
	if err != nil || in.Location == nil || strings.TrimSpace(in.StationID) == "" {
		httpapi.WriteError(w, r, 400, "community.capture-invalid", "invalid photo capture request", nil)
		return
	}
	loc, err := parseLocationEvidence(in.Location)
	if err != nil {
		httpapi.WriteError(w, r, 400, "community.capture-invalid", "invalid photo capture request", nil)
		return
	}
	result, _, err := h.PhotoCapture(r.Context(), caller, strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		application.PhotoCaptureIntent{ClientCaptureID: in.ClientCaptureID, StationID: in.StationID, Location: loc}, raw)
	if err != nil {
		writeAPIError(w, r, err)
		return
	}
	body, _ := json.Marshal(result)
	httpapi.WriteJSON(w, r, http.StatusCreated, "no-store", body)
}
