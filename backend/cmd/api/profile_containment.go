package main

import (
	"net/http"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

// Private representation routes are contained at the production composition
// boundary until the frozen contributor proof/binding and private storage
// integrations are accepted. Module handlers remain testable; bearer-only
// authentication must never expose private reads or enable business writes.
// No runtime flag can enable this incomplete integration accidentally.
func privateProfileUnavailable(_ http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpapi.WriteError(w, r, http.StatusServiceUnavailable, "profile.integration-pending", "private representation temporarily unavailable", nil)
	})
}
