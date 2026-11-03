package http

import (
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

var offerRevision = regexp.MustCompile(`^[0-9a-f]{64}$`)

const offerDataFile = "derived-database.ndjson"
const offerNoticeFile = "derived-database-offer.json"

func (h Handler) catalogOffer(w http.ResponseWriter, r *http.Request) {
	if h.disabled(w, r) {
		return
	}
	revision, filename := chi.URLParam(r, "revision"), chi.URLParam(r, "file")
	var kind, contentType string
	switch filename {
	case offerDataFile:
		kind, contentType = "data", "application/x-ndjson"
	case offerNoticeFile:
		kind, contentType = "notice", "application/json"
	}
	notFound := func() {
		httpapi.WriteError(w, r, http.StatusNotFound, "charging.offer-not-found", "charging offer not found", nil)
	}
	if kind == "" || !offerRevision.MatchString(revision) {
		notFound()
		return
	}
	offer, err := h.Read.CatalogOffer(r.Context(), revision, kind)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound()
		return
	}
	if err != nil {
		sourceDown(w, r)
		return
	}
	tag := "\"" + offer.SHA256 + "\""
	w.Header().Set("ETag", tag)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if r.Header.Get("If-None-Match") == tag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(offer.Bytes, 10))
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.WriteHeader(http.StatusOK)
	if err := h.Read.WriteCatalogOffer(r.Context(), w, offer); err != nil && r.Context().Err() == nil {
		slog.Warn("charging catalog offer stream incomplete")
	}
}
