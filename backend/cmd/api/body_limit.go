package main

import (
	"net/http"
	"strings"

	evidencedomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

func requestBodyLimit(cfg config.Config) func(*http.Request) int64 {
	return func(r *http.Request) int64 {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method == http.MethodPut && len(parts) == 4 && parts[0] == "v1" && parts[1] == "uploads" && parts[3] == "bytes" {
			return evidencedomain.MaxUploadBytes + 1
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
