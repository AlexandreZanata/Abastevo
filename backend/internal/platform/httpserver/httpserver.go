// Package httpserver is the bounded HTTP server and its middleware chain.
//
// Transport concerns only: request IDs, body caps, read/write/idle timeouts,
// panic recovery and graceful drain. No business rules live in middleware.
// Access records carry method, route template, status, duration and a stable
// code; raw paths, queries, bodies, IPs and headers are never logged.
package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

// Default transport policy. ShutdownTimeout comes from process config.
const (
	DefaultReadTimeout       = 15 * time.Second
	DefaultReadHeaderTimeout = 5 * time.Second
	DefaultWriteTimeout      = 15 * time.Second
	DefaultIdleTimeout       = 60 * time.Second
)

// Router is the chi router composition roots register routes on.
type Router = chi.Router

// Options configures the server. Durations map to http.Server semantics;
// ShutdownTimeout bounds the drain after cancellation.
type Options struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxBodyBytes      int64
	Logger            *telemetry.Logger
}

// Server is a gracefully draining HTTP server bound at construction.
type Server struct {
	http     *http.Server
	listener net.Listener
	shutdown time.Duration
}

// New binds Addr immediately so misconfiguration fails fast, before serving.
func New(opts Options, handler http.Handler) (*Server, error) {
	if _, _, err := net.SplitHostPort(opts.Addr); err != nil {
		return nil, fmt.Errorf("httpserver: addr must be a valid host:port")
	}
	if opts.ReadTimeout < 0 || opts.WriteTimeout < 0 || opts.IdleTimeout < 0 {
		return nil, fmt.Errorf("httpserver: timeouts must not be negative")
	}
	if opts.ReadHeaderTimeout <= 0 {
		return nil, fmt.Errorf("httpserver: header timeout must be positive")
	}
	if opts.ShutdownTimeout <= 0 {
		return nil, fmt.Errorf("httpserver: shutdown timeout must be positive")
	}
	if opts.MaxBodyBytes <= 0 {
		return nil, fmt.Errorf("httpserver: body cap must be positive")
	}
	if opts.Logger == nil {
		return nil, fmt.Errorf("httpserver: logger is required")
	}
	if handler == nil {
		return nil, fmt.Errorf("httpserver: handler is required")
	}
	listener, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return nil, fmt.Errorf("httpserver: listen: %w", err)
	}
	// Request IDs are assigned outside chi so unmatched probes carry one too;
	// access logging lives inside the router where the matched template is
	// visible (chi skips Use middlewares on its default 404 path, which is
	// why NewRouter sets explicit handlers below).
	chained := requestID(http.MaxBytesHandler(handler, opts.MaxBodyBytes))
	return &Server{
		http: &http.Server{
			Handler:           chained,
			ReadTimeout:       opts.ReadTimeout,
			ReadHeaderTimeout: opts.ReadHeaderTimeout,
			WriteTimeout:      opts.WriteTimeout,
			IdleTimeout:       opts.IdleTimeout,
		},
		listener: listener,
		shutdown: opts.ShutdownTimeout,
	}, nil
}

// Addr reports the bound address (useful with port 0 in tests).
func (s *Server) Addr() net.Addr { return s.listener.Addr() }

// Run serves until ctx is cancelled, then drains in-flight requests within
// the shutdown timeout. A clean drain returns nil.
func (s *Server) Run(ctx context.Context) error {
	serveErr := make(chan error, 1)
	go func() {
		if err := s.http.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()
	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	drain, cancel := context.WithTimeout(context.Background(), s.shutdown)
	defer cancel()
	if err := s.http.Shutdown(drain); err != nil {
		return fmt.Errorf("httpserver: shutdown: %w", err)
	}
	return <-serveErr
}

// NewRouter returns the router composition roots register routes on, with
// access logging and recovery for matched routes plus explicit handlers so
// unmatched probes and method mismatches are logged instead of silent.
func NewRouter(logger *telemetry.Logger) Router {
	r := chi.NewRouter()
	r.Use(accessLog(logger), recoverer(logger))
	r.NotFound(unmatched(logger, http.StatusNotFound))
	r.MethodNotAllowed(unmatched(logger, http.StatusMethodNotAllowed))
	return r
}

func unmatched(logger *telemetry.Logger, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Info(r.Context(), "http.request", "request served",
			slog.String("method", r.Method),
			slog.String("route", routeTemplate(r)),
			slog.Int("status", status),
			slog.Int64("duration_ms", 0),
		)
		http.Error(w, http.StatusText(status), status)
	}
}

const requestIDHeader = "X-Request-ID"

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !safeRequestID(id) {
			id = newRequestID()
		}
		r.Header.Set(requestIDHeader, id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(telemetry.ContextWithRequest(r.Context(), id)))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func routeTemplate(r *http.Request) string {
	if routeCtx := chi.RouteContext(r.Context()); routeCtx != nil {
		if pattern := routeCtx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "unmatched"
}

func accessLog(logger *telemetry.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r)
			logger.Info(r.Context(), "http.request", "request served",
				slog.String("method", r.Method),
				slog.String("route", routeTemplate(r)),
				slog.Int("status", rec.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
		})
	}
}

func recoverer(logger *telemetry.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error(r.Context(), "http.panic", "handler panicked")
					http.Error(w, http.StatusText(http.StatusInternalServerError),
						http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func safeRequestID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
