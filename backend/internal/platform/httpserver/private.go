package httpserver

import (
	"net/http"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

// Private context encloses access logging, recovery and every module callback.
// Only the transport-owned Request method can emit structured access fields.
func privateAccessLog(logger *telemetry.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !orgapi.IsOrganizationPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			r = r.WithContext(telemetry.ContextWithPrivateRequest(r.Context()))
			rec := &statusRecorder{ResponseWriter: w, status: 200}
			start := time.Now()
			next.ServeHTTP(rec, r)
			logger.Request(r.Context(), r.Method, routeTemplate(r), rec.status, time.Since(start))
		})
	}
}
