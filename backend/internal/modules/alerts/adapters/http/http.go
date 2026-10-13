package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/alerts/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

type Service interface {
	Apply(ctx context.Context, accountID string, batch application.Batch) (application.BatchResult, error)
	ListRules(ctx context.Context, accountID string) ([]application.RuleRecord, error)
	ListInbox(ctx context.Context, accountID string, limit int) ([]application.InboxItem, error)
	EraseAll(ctx context.Context, accountID string) error
	EnrollToken(ctx context.Context, accountID, token string) error
	RevokeToken(ctx context.Context, accountID, token string) error
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
	status, code := 503, "unavailable"
	switch {
	case isInvalid(err):
		status, code = 400, "invalid"
	case isUnauthorized(err):
		status, code = 401, "auth-required"
	case isDenied(err):
		status, code = 403, "capability-required"
	case isNotFound(err):
		status, code = 404, "not-found"
	case isConflict(err):
		status, code = 409, "conflict"
	case isQuota(err):
		status, code = 429, "rate-limited"
	}
	httpapi.WriteError(w, r, status, "alerts."+code, "alert request unavailable or refused", nil)
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

func isInvalid(err error) bool      { return err != nil && contains(err, "invalid") }
func isUnauthorized(err error) bool { return err != nil && contains(err, "authentication") }
func isDenied(err error) bool       { return err != nil && contains(err, "capability") }
func isNotFound(err error) bool {
	return err != nil && contains(err, "unavailable") && contains(err, "record")
}
func isConflict(err error) bool { return err != nil && contains(err, "conflict") }
func isQuota(err error) bool    { return err != nil && contains(err, "quota") }

func write(w http.ResponseWriter, r *http.Request, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Del("ETag")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(raw)
}

func read(r *http.Request, v any) error {
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		return errors.New("alerts: invalid input")
	}
	raw, err := httpapi.ReadBody(r, 65536)
	if err != nil || httpapi.DecodeJSON(raw, v) != nil {
		return errors.New("alerts: invalid input")
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	return nil
}

func (h Handler) principal(r *http.Request, s session, signed bool) (string, error) {
	if len(s.FamilyID) != 36 || len(s.AccessToken) < 1 || len(s.AccessToken) > 512 {
		return "", errors.New("alerts: invalid input")
	}
	owner, err := h.Sessions(r.Context(), s.FamilyID, s.AccessToken)
	if err != nil || len(owner) != 36 {
		return "", errors.New("alerts: authentication required")
	}
	if signed {
		if h.SignedOwner == nil {
			return "", errors.New("alerts: authentication required")
		}
		if _, _, err = h.SignedOwner(r, owner); err != nil {
			return "", errors.New("alerts: authentication required")
		}
	}
	return owner, nil
}

func (h Handler) RegisterRoutes(r chi.Router) {
	r.Post("/v1/alerts/commands", h.commands)
	r.Post("/v1/alerts/rules", h.rules)
	r.Post("/v1/alerts/inbox", h.inbox)
	r.Post("/v1/alerts/erase", h.erase)
	r.Post("/v1/alerts/push/enroll", h.pushEnroll)
	r.Post("/v1/alerts/push/revoke", h.pushRevoke)
}

func (h Handler) commands(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		application.Batch
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	owner, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	got, err := h.Service.Apply(r.Context(), owner, dto.Batch)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, got)
}

func (h Handler) rules(w http.ResponseWriter, r *http.Request) {
	var dto session
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	owner, err := h.principal(r, dto, false)
	if err != nil {
		fail(w, r, err)
		return
	}
	rows, err := h.Service.ListRules(r.Context(), owner)
	if err != nil {
		fail(w, r, err)
		return
	}
	if rows == nil {
		rows = []application.RuleRecord{}
	}
	write(w, r, struct {
		Rules []application.RuleRecord `json:"rules"`
	}{rows})
}

func (h Handler) inbox(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Limit *int `json:"limit,omitempty"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	limit := 20
	if dto.Limit != nil {
		limit = *dto.Limit
	}
	if limit < 1 || limit > 100 {
		fail(w, r, errors.New("alerts: invalid input"))
		return
	}
	owner, err := h.principal(r, dto.session, false)
	if err != nil {
		fail(w, r, err)
		return
	}
	rows, err := h.Service.ListInbox(r.Context(), owner, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	if rows == nil {
		rows = []application.InboxItem{}
	}
	write(w, r, struct {
		Items []application.InboxItem `json:"items"`
	}{rows})
}

func (h Handler) erase(w http.ResponseWriter, r *http.Request) {
	var dto session
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	owner, err := h.principal(r, dto, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err = h.Service.EraseAll(r.Context(), owner); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, struct {
		Status string `json:"status"`
	}{"erased"})
}

func (h Handler) pushEnroll(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Token string `json:"token"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if len(dto.Token) < 1 || len(dto.Token) > 256 {
		fail(w, r, errors.New("alerts: invalid input"))
		return
	}
	owner, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err = h.Service.EnrollToken(r.Context(), owner, dto.Token); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, struct {
		Status string `json:"status"`
	}{"enrolled"})
}

func (h Handler) pushRevoke(w http.ResponseWriter, r *http.Request) {
	var dto struct {
		session
		Token string `json:"token"`
	}
	if err := read(r, &dto); err != nil {
		fail(w, r, err)
		return
	}
	if len(dto.Token) < 1 || len(dto.Token) > 256 {
		fail(w, r, errors.New("alerts: invalid input"))
		return
	}
	owner, err := h.principal(r, dto.session, true)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err = h.Service.RevokeToken(r.Context(), owner, dto.Token); err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, struct {
		Status string `json:"status"`
	}{"revoked"})
}
