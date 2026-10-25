package mapdist

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

type Handler struct {
	Dist *Distribution
}

func (h Handler) RegisterRoutes(r chi.Router) {
	r.Get("/v1/map/regions/{region}/manifest", h.manifest)
	r.Get("/v1/map/packages/{package_id}", h.packageFile)
}

func (h Handler) manifest(w http.ResponseWriter, r *http.Request) {
	if h.Dist == nil || h.Dist.Manifest == nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "map.distribution-unavailable", "map distribution unavailable", nil)
		return
	}
	if chi.URLParam(r, "region") != h.Dist.Manifest.Region.ID {
		httpapi.WriteError(w, r, http.StatusNotFound, "map.region-not-found", "region not found", nil)
		return
	}
	sum := sha256.Sum256(h.Dist.EnvelopeBytes)
	etag := fmt.Sprintf(`W/"%x"`, sum)
	httpapi.WriteJSONWithETag(w, r, http.StatusOK, "public, max-age=300", h.Dist.EnvelopeBytes, etag)
}

func (h Handler) packageFile(w http.ResponseWriter, r *http.Request) {
	if h.Dist == nil {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "map.distribution-unavailable", "map distribution unavailable", nil)
		return
	}
	id := chi.URLParam(r, "package_id")
	pkg, ok := h.Dist.Packages[id]
	if !ok {
		httpapi.WriteError(w, r, http.StatusNotFound, "map.package-not-found", "package not found", nil)
		return
	}
	path, ok := h.Dist.Files[id]
	if !ok || strings.Contains(id, "/") || strings.Contains(id, "..") {
		httpapi.WriteError(w, r, http.StatusNotFound, "map.package-not-found", "package not found", nil)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "map.package-unavailable", "try again later", nil)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != pkg.Bytes {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "map.package-unavailable", "try again later", nil)
		return
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "map.package-unavailable", "try again later", nil)
		return
	}
	if hex.EncodeToString(hasher.Sum(nil)) != pkg.SHA256 {
		httpapi.WriteError(w, r, http.StatusConflict, "map.package-changed", "package no longer matches the signed manifest", nil)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		httpapi.WriteError(w, r, http.StatusInternalServerError, "map.package-unavailable", "try again later", nil)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(pkg.Path)))
	w.Header().Set("ETag", fmt.Sprintf(`"%s"`, pkg.SHA256))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, filepath.Base(pkg.Path), info.ModTime(), file)
}
