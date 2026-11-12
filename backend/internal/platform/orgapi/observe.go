package orgapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry/metrics"
	"github.com/go-chi/chi/v5"
)

type observationKey struct{}
type observation struct{ code Code }

func recordCode(r *http.Request, code Code) {
	if state, _ := r.Context().Value(observationKey{}).(*observation); state != nil {
		state.code = code
	}
}

// Observe reuses the existing registry/listener. No request payload or raw
// error is read; WriteError supplies only its already allowlisted final code.
// Install once on the composition router, inside its panic recovery middleware.
func Observe(reg *metrics.Registry) (func(http.Handler) http.Handler, error) {
	if reg == nil {
		return nil, errors.New("orgapi: telemetry registry required")
	}
	requests, err := reg.Counter("org_requests_total", "Organization requests by source route, result and code.", "route", "result", "code")
	if err != nil {
		return nil, err
	}
	seconds, err := reg.Counter("org_request_seconds_total", "Organization request seconds by source route, result and code.", "route", "result", "code")
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !IsOrganizationPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			state := &observation{}
			r = r.WithContext(context.WithValue(r.Context(), observationKey{}, state))
			rec := &observationWriter{ResponseWriter: w, status: 200}
			start := time.Now()
			defer func() {
				panicked := recover()
				if panicked != nil {
					state.code = Unavailable
					if !rec.written {
						rec.status = 503
					}
				}
				route := "unmatched"
				if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
					route = rc.RoutePattern()
				}
				result, code := outcome(rec.status, state.code)
				labels := map[string]string{"route": route, "result": result, "code": code}
				_ = requests.Inc(reg, labels)
				_ = seconds.Add(reg, time.Since(start).Seconds(), labels)
				if panicked != nil {
					panic(panicked)
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}, nil
}

func outcome(status int, code Code) (string, string) {
	if _, known := statuses[code]; !known {
		switch {
		case status >= 200 && status < 300:
			return "success", "OK"
		case status >= 400 && status < 500:
			return "refused", "REFUSED"
		default:
			return "unavailable", string(Unavailable)
		}
	}
	if code == Unavailable {
		return "unavailable", string(code)
	}
	return "refused", string(code)
}

type observationWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *observationWriter) WriteHeader(status int) {
	if status >= 100 && status < 200 && status != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.written {
		return
	}
	w.status, w.written = status, true
	w.ResponseWriter.WriteHeader(status)
}
func (w *observationWriter) Write(body []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
func (w *observationWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *observationWriter) FlushError() error {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}
