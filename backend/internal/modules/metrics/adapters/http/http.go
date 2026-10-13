package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/metrics/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

type Service interface {
	Compare(ctx context.Context, accountID string, baseline, current application.Side) (application.Comparison, error)
}

type Handler struct {
	Service     Service
	Sessions    func(context.Context, string, string) (string, error)
	SignedOwner func(*http.Request, string) (string, string, error)
}

type session struct {
	FamilyID    string `json:"family_id"`
	AccessToken string `json:"access_token"`
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	status := 503
	if contains(err, "invalid") || contains(err, "Invalid") {
		status = 400
	}
	httpapi.WriteError(w, r, status, "metrics.invalid", "comparison unavailable or refused", nil)
}

func contains(err error, sub string) bool {
	for err != nil {
		msg := err.Error()
		for i := 0; i+len(sub) <= len(msg); i++ {
			if msg[i:i+len(sub)] == sub {
				return true
			}
		}
		err = errors.Unwrap(err)
	}
	return false
}

func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/metrics/compare", h.compare)
}

func (h Handler) compare(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Baseline application.Side `json:"baseline"`
		Current  application.Side `json:"current"`
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		fail(w, r, errors.New("metrics: invalid input"))
		return
	}
	raw, err := httpapi.ReadBody(r, 65536)
	if err != nil || httpapi.DecodeJSON(raw, &dto) != nil {
		fail(w, r, errors.New("metrics: invalid input"))
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if len(dto.FamilyID) != 36 || len(dto.AccessToken) < 1 {
		fail(w, r, errors.New("metrics: invalid input"))
		return
	}
	owner, err := h.Sessions(r.Context(), dto.FamilyID, dto.AccessToken)
	if err != nil || len(owner) != 36 {
		fail(w, r, errors.New("metrics: invalid input"))
		return
	}
	if h.SignedOwner == nil {
		fail(w, r, errors.New("metrics: invalid input"))
		return
	}
	if _, _, err = h.SignedOwner(r, owner); err != nil {
		fail(w, r, errors.New("metrics: invalid input"))
		return
	}
	got, err := h.Service.Compare(r.Context(), owner, dto.Baseline, dto.Current)
	if err != nil {
		fail(w, r, err)
		return
	}
	body, _ := json.Marshal(got)
	w.Header().Del("ETag")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(body)
}
