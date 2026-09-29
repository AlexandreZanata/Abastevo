package metrics

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// Request label vocabularies. Method comes from the wire (bounded HTTP
// verbs); route comes from the chi route template (never the raw path,
// so IDs cannot become label values); class buckets the status code.
var knownMethods = map[string]bool{
	"GET": true, "POST": true, "PUT": true, "PATCH": true,
	"DELETE": true, "HEAD": true, "OPTIONS": true,
}

// Observe returns chi middleware counting requests and seconds by
// method, route template and status class. Unknown methods fold into
// OTHER so a novel verb cannot mint series.
func Observe(reg *Registry, requests, seconds *Counter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			method := r.Method
			if !knownMethods[method] {
				method = "OTHER"
			}
			route := "unmatched"
			if ctx := chi.RouteContext(r.Context()); ctx != nil {
				if pattern := ctx.RoutePattern(); pattern != "" {
					route = pattern
				}
			}
			class := statusClass(rec.status)
			labels := map[string]string{"method": method, "route": route, "class": class}
			_ = requests.Inc(reg, labels)
			_ = seconds.Add(reg, time.Since(start).Seconds(), labels)
		})
	}
}

func statusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	default:
		return "other"
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
