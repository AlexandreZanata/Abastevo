package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	adminhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/adapters/http"
	adminapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/application"
	admindomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/admin/domain"
	forumadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
)

// AdminHandler exposes administrative editorial endpoints for forum publications.
type AdminHandler struct {
	Service *application.AdminService
	Admin   *adminhttp.Handler
}

// NewAdminHandler constructs an AdminHandler.
func NewAdminHandler(service *application.AdminService, admin *adminhttp.Handler) *AdminHandler {
	return &AdminHandler{
		Service: service,
		Admin:   admin,
	}
}

// RegisterRoutes registers editorial routes onto an administrative chi router.
func (h AdminHandler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/admin/forum/sources", h.listSources)
	r.Get("/v1/admin/forum/posts", h.listPosts)
	r.Get("/v1/admin/forum/posts/{id}", h.getPost)
	r.Post("/v1/admin/forum/posts", h.createDraft)
	r.Post("/v1/admin/forum/posts/{id}/edit", h.editPost)
	r.Post("/v1/admin/forum/posts/{id}/publish", h.publishPost)
	r.Post("/v1/admin/forum/posts/{id}/reject", h.rejectPost)
	r.Post("/v1/admin/forum/posts/{id}/hide", h.hidePost)
	r.Post("/v1/admin/forum/posts/{id}/unhide", h.unhidePost)
}

func (h AdminHandler) listSources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.Service.ListSources(r.Context())
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": sources,
	})
}

func (h AdminHandler) listPosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var pubState *string
	if s := q.Get("publication_state"); s != "" {
		pubState = &s
	}
	var category *string
	if c := q.Get("category"); c != "" {
		category = &c
	}
	var kind *string
	if k := q.Get("kind"); k != "" {
		kind = &k
	}

	limit := 20
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	input := application.AdminListInput{
		PublicationState: pubState,
		Category:         category,
		Kind:             kind,
		Limit:            limit,
	}

	if cursorToken := q.Get("cursor"); cursorToken != "" {
		createdAt, id, err := forumadapters.DecodeAdminCursor(cursorToken)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "forum.invalid")
			return
		}
		input.CursorCreatedAt = &createdAt
		input.CursorID = &id
	}

	posts, nextCursor, err := h.Service.ListPosts(r.Context(), input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":       posts,
		"next_cursor": nextCursor,
	})
}

func (h AdminHandler) getPost(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	post, err := h.Service.GetPost(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, post)
}

type createDraftDTO struct {
	Kind                   string  `json:"kind"`
	Category               string  `json:"category"`
	Title                  string  `json:"title"`
	Excerpt                string  `json:"excerpt"`
	Body                   string  `json:"body_text"`
	SourceID               string  `json:"source_id"`
	CanonicalURL           string  `json:"canonical_url"`
	FuelProduct            *string `json:"fuel_product"`
	RegionUF               *string `json:"region_uf"`
	RegionMunicipalityIBGE *int    `json:"region_municipality_ibge"`
	StationID              *string `json:"station_id"`
	Reason                 string  `json:"reason"`
}

func (h AdminHandler) createDraft(w http.ResponseWriter, r *http.Request) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}

	var dto createDraftDTO
	m, ok := h.Admin.PrepareMutation(w, r, session, "admin.forum.post.create", "forum_post", "", &dto)
	if !ok {
		return
	}

	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason

	input := application.CreateDraftInput{
		Kind:                   dto.Kind,
		Category:               dto.Category,
		Title:                  dto.Title,
		Excerpt:                dto.Excerpt,
		Body:                   dto.Body,
		SourceID:               dto.SourceID,
		CanonicalURL:           dto.CanonicalURL,
		FuelProduct:            dto.FuelProduct,
		RegionUF:               dto.RegionUF,
		RegionMunicipalityIBGE: dto.RegionMunicipalityIBGE,
		StationID:              dto.StationID,
	}

	post, err := h.Service.CreateDraft(r.Context(), m, input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	body, err := json.Marshal(post)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "forum.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusCreated, Body: body}, nil, false)
}

type editPostDTO struct {
	Title                  string  `json:"title"`
	Excerpt                string  `json:"excerpt"`
	Body                   string  `json:"body_text"`
	Category               string  `json:"category"`
	ExpectedRevision       int     `json:"expected_revision"`
	CommentsEnabled        bool    `json:"comments_enabled"`
	Correction             *string `json:"correction"`
	FuelProduct            *string `json:"fuel_product"`
	RegionUF               *string `json:"region_uf"`
	RegionMunicipalityIBGE *int    `json:"region_municipality_ibge"`
	StationID              *string `json:"station_id"`
	Reason                 string  `json:"reason"`
}

func (h AdminHandler) editPost(w http.ResponseWriter, r *http.Request) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}

	id := chi.URLParam(r, "id")
	var dto editPostDTO
	m, ok := h.Admin.PrepareMutation(w, r, session, "admin.forum.post.edit", "forum_post", id, &dto)
	if !ok {
		return
	}

	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason

	input := application.EditPostInput{
		ID:                     id,
		Title:                  dto.Title,
		Excerpt:                dto.Excerpt,
		Body:                   dto.Body,
		Category:               dto.Category,
		ExpectedRevision:       dto.ExpectedRevision,
		CommentsEnabled:        dto.CommentsEnabled,
		Correction:             dto.Correction,
		FuelProduct:            dto.FuelProduct,
		RegionUF:               dto.RegionUF,
		RegionMunicipalityIBGE: dto.RegionMunicipalityIBGE,
		StationID:              dto.StationID,
	}

	post, err := h.Service.EditPost(r.Context(), m, input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	body, err := json.Marshal(post)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "forum.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusOK, Body: body}, nil, false)
}

type reasonDTO struct {
	ExpectedRevision int    `json:"expected_revision,omitempty"`
	Reason           string `json:"reason"`
}

func (h AdminHandler) publishPost(w http.ResponseWriter, r *http.Request) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}

	id := chi.URLParam(r, "id")
	var dto reasonDTO
	m, ok := h.Admin.PrepareMutation(w, r, session, "admin.forum.post.publish", "forum_post", id, &dto)
	if !ok {
		return
	}

	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason
	if dto.ExpectedRevision < 1 {
		writeError(w, r, http.StatusBadRequest, "forum.invalid")
		return
	}

	post, err := h.Service.PublishPost(r.Context(), m, id, dto.ExpectedRevision)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	body, err := json.Marshal(post)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "forum.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusOK, Body: body}, nil, false)
}

type rejectPostDTO struct {
	ExpectedRevision int    `json:"expected_revision"`
	Reason           string `json:"reason"`
}

func (h AdminHandler) rejectPost(w http.ResponseWriter, r *http.Request) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}

	id := chi.URLParam(r, "id")
	var dto rejectPostDTO
	m, ok := h.Admin.PrepareMutation(w, r, session, "admin.forum.post.reject", "forum_post", id, &dto)
	if !ok {
		return
	}

	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason

	post, err := h.Service.RejectPost(r.Context(), m, id, dto.ExpectedRevision)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	body, err := json.Marshal(post)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "forum.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusOK, Body: body}, nil, false)
}

func (h AdminHandler) hidePost(w http.ResponseWriter, r *http.Request) {
	h.setVisibility(w, r, domain.VisibilityHidden)
}

func (h AdminHandler) unhidePost(w http.ResponseWriter, r *http.Request) {
	h.setVisibility(w, r, domain.VisibilityVisible)
}

func (h AdminHandler) setVisibility(w http.ResponseWriter, r *http.Request, visibility domain.Visibility) {
	session, ok := adminhttp.SessionFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "admin.denied")
		return
	}

	id := chi.URLParam(r, "id")
	action := "admin.forum.post.hide"
	if visibility == domain.VisibilityVisible {
		action = "admin.forum.post.unhide"
	}

	var dto reasonDTO
	m, ok := h.Admin.PrepareMutation(w, r, session, action, "forum_post", id, &dto)
	if !ok {
		return
	}

	reason := strings.TrimSpace(dto.Reason)
	if !admindomain.ValidReason(reason) {
		writeError(w, r, http.StatusBadRequest, "admin.invalid")
		return
	}
	m.Reason = reason

	post, err := h.Service.SetVisibility(r.Context(), m, id, visibility)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	body, err := json.Marshal(post)
	if err != nil {
		writeError(w, r, http.StatusServiceUnavailable, "forum.unavailable")
		return
	}
	h.Admin.FinishMutation(w, r, adminapp.MutationResult{Status: http.StatusOK, Body: body}, nil, false)
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrPostNotFound):
		writeError(w, r, http.StatusNotFound, "forum.not-found")
	case errors.Is(err, domain.ErrRevisionMismatch):
		writeError(w, r, http.StatusConflict, "forum.revision-conflict")
	case errors.Is(err, domain.ErrMissingRights):
		writeError(w, r, http.StatusBadRequest, "forum.missing-rights")
	case errors.Is(err, domain.ErrBulletinImmutable):
		writeError(w, r, http.StatusConflict, "forum.bulletin-immutable")
	case errors.Is(err, domain.ErrForumDisabled):
		writeError(w, r, http.StatusNotFound, "forum.disabled")
	case errors.Is(err, domain.ErrInvalidCategory),
		errors.Is(err, domain.ErrInvalidPostKind),
		errors.Is(err, domain.ErrTitleEmpty),
		errors.Is(err, domain.ErrTitleTooLong),
		errors.Is(err, domain.ErrExcerptTooLong),
		errors.Is(err, domain.ErrBodyTooLong),
		errors.Is(err, domain.ErrInvalidTransition),
		errors.Is(err, domain.ErrInvalidUTF8),
		errors.Is(err, domain.ErrMissingSource),
		errors.Is(err, domain.ErrMissingSourceDate),
		errors.Is(err, domain.ErrMissingAuthor),
		errors.Is(err, domain.ErrTopicCannotHaveSource),
		errors.Is(err, domain.ErrMissingBulletinProvenance),
		errors.Is(err, domain.ErrInvalidBulletinInput),
		errors.Is(err, domain.ErrBulletinContentTooLong),
		errors.Is(err, domain.ErrBulletinArithmetic),
		errors.Is(err, domain.ErrInvalidCursor):
		writeError(w, r, http.StatusBadRequest, "forum.invalid")
	default:
		writeError(w, r, http.StatusServiceUnavailable, "forum.unavailable")
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("Referrer-Policy", "no-referrer")
	httpapi.WriteError(w, r, status, code, "forum request refused", nil)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
