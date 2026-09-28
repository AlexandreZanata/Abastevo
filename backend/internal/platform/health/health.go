// Package health separates process liveness from dependency readiness.
//
//   - Live answers whether the process event loop runs; it never touches
//     dependencies, so a DB outage cannot cause restart loops.
//   - Ready runs injected checks (DB/schema compatibility, admission) with a
//     per-check timeout and fails closed.
//
// Both endpoints return a minimal {"status":...} envelope with no-store
// semantics. Check internals (errors, DSNs, versions) are never serialized;
// failures are logged with the check name and a stable code only.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

// DefaultTimeout bounds each readiness check when the caller passes none.
const DefaultTimeout = 2 * time.Second

// Check is a named readiness probe injected by the composition root.
// The database/schema probe lands with the pool in P01-T09.
type Check struct {
	Name string
	Fn   func(ctx context.Context) error
}

// Handler serves liveness and readiness.
type Handler struct {
	logger  *telemetry.Logger
	timeout time.Duration
	checks  []Check
}

// New builds a Handler. A non-positive timeout selects DefaultTimeout.
func New(logger *telemetry.Logger, timeout time.Duration, checks ...Check) (*Handler, error) {
	if logger == nil {
		return nil, fmt.Errorf("health: logger is required")
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &Handler{logger: logger, timeout: timeout, checks: checks}, nil
}

// Live reports process liveness without touching dependencies.
func (h *Handler) Live(w http.ResponseWriter, r *http.Request) {
	writeStatus(w, http.StatusOK, "ok")
}

// Ready reports dependency readiness; any failing or slow check yields 503.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	for _, check := range h.checks {
		ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
		err := check.Fn(ctx)
		cancel()
		if err != nil {
			h.logger.Warn(r.Context(), "health.not_ready", "readiness check failed",
				// Name only: error text may carry connection internals.
				slog.String("check", check.Name),
			)
			writeStatus(w, http.StatusServiceUnavailable, "not_ready")
			return
		}
	}
	writeStatus(w, http.StatusOK, "ready")
}

func writeStatus(w http.ResponseWriter, status int, value string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": value})
}
