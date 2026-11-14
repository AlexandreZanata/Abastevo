package main

import (
	"net/http"
	"strings"

	documentapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/document/application"
	evidencedomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	media "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgmedia/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

func requestBodyLimit(cfg config.Config) func(*http.Request) int64 {
	return func(r *http.Request) int64 {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method == http.MethodPut && len(parts) == 4 && parts[0] == "v1" && parts[1] == "uploads" && parts[3] == "bytes" {
			return evidencedomain.MaxUploadBytes + 1
		}
		// The private document ingress (EMP-DOC-002) carries one bounded
		// original; the extra byte lets orgmedia prove an oversized body.
		if r.Method == http.MethodPost && len(parts) == 7 && parts[0] == "v1" && parts[1] == "orgs" && parts[3] == "documents" && parts[5] == "content" && parts[6] == "upload" {
			if limit, err := media.ByteLimit(documentapp.MediaTypeJPEG); err == nil {
				return limit + 1
			}
		}
		// Only the explicitly configured staging preview relays private S3
		// uploads. Keep JSON and production limits unchanged.
		preview := cfg.Env != config.EnvProduction && !cfg.DevelopmentPhotoPreviewUntil.IsZero() && cfg.R2 != nil && cfg.R2.Bucket == "abastevo-dev-photos"
		if preview && r.Method == http.MethodPut && (strings.HasPrefix(r.URL.Path, "/abastevo-dev-photos/q/") || strings.HasPrefix(r.URL.Path, "/abastevo-dev-photos/f/")) {
			return evidencedomain.MaxUploadBytes
		}
		return cfg.MaxBodyBytes
	}
}
