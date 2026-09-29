// Command api is the public HTTP process role.
//
// Routes land in later tasks (health in P01-T06, domain reads in P02+);
// this task wires the bounded server lifecycle with signal-aware drain.
// No business code lives here.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	communityhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters/http"
	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	directoryhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/http"
	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	evidenceadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters"
	evidencehttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/http"
	evidencestorage "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityauth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	officialhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/http"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/health"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpserver"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
)

// newUUID mints v4 identifiers for server-owned facts.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32], nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "anpfuel api:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger, err := telemetry.New(os.Stdout, cfg.LogLevel, telemetry.RoleAPI)
	if err != nil {
		return err
	}
	router := httpserver.NewRouter(logger)
	// Lazy pool: the process boots (live, not ready) while PostgreSQL is
	// down. Closed after the server drains, at shutdown.
	pool, err := database.Open(context.Background(), cfg.DatabaseURL, database.DefaultOptions())
	if err != nil {
		return err
	}
	defer pool.Close()
	healthHandler, err := health.New(logger, 0, health.Check{Name: "db", Fn: pool.Ping})
	if err != nil {
		return err
	}
	router.Get("/health/live", healthHandler.Live)
	router.Get("/health/ready", healthHandler.Ready)
	// Anonymous catalog reads (P02-T08). Only this composition root wires
	// modules together: the official handler gets station existence as a
	// closure so modules never cross-read.
	stations := directoryread.NewReader(pool.Underlying())
	prices := officialread.NewReader(pool.Underlying())
	directoryhttp.Handler{Stations: stations, Secrets: cfg.CursorSecret}.RegisterRoutes(router)
	officialhttp.Handler{
		Prices:  prices,
		Secrets: cfg.CursorSecret,
		StationsExist: func(ctx context.Context, id string) (bool, error) {
			_, err := stations.Detail(ctx, id)
			if err == nil {
				return true, nil
			}
			if errors.Is(err, directoryapp.ErrUnknownStation) {
				return false, nil
			}
			return false, err
		},
	}.RegisterRoutes(router)
	// Community writes and owner reads (P04-T04). Same closure rule: every
	// cross-module collaborator arrives as a narrow function built here.
	communityStore := communityadapters.NewStore(pool.Underlying())
	identityRegistrar := identityadapters.NewRegistrar(pool.Underlying())
	identityRunner := identityadapters.NewRunner(pool.Underlying())
	identityLimiter := identityadapters.NewLimiter(pool.Underlying(), identitydomain.DefaultQuotaPolicy())
	authVerifier := &identityauth.Verifier{Pool: pool.Underlying(), Authority: cfg.CanonicalHost}
	communityPorts := communityapp.Ports{
		Clock: time.Now,
		NewID: newUUID,
		Attribution: func(ctx context.Context, contributorID string) (string, error) {
			return identityRegistrar.AttributionToken(ctx, contributorID)
		},
		CheckQuota: func(ctx context.Context, subject, operation string) (time.Duration, error) {
			retryAfter, err := identityLimiter.Check(ctx, subject, operation)
			if err != nil {
				var denied *identityadapters.QuotaError
				if errors.As(err, &denied) {
					return denied.RetryAfter, &communityapp.QuotaDeniedError{RetryAfter: denied.RetryAfter}
				}
				return 0, err
			}
			return retryAfter, nil
		},
		Idempotent: func(ctx context.Context, key communityapp.IdempotencyKey, body []byte, run func(ctx context.Context) (communityapp.Outcome, error)) (communityapp.Outcome, error) {
			out, err := identityRunner.Do(ctx, identitydomain.IdempotencyKey{
				ContributorID: key.ContributorID, Method: key.Method, Route: key.Route, Key: key.Key,
			}, body, func(ctx context.Context, tx pgx.Tx) (int, []byte, error) {
				o, err := run(ctx)
				return o.StatusCode, o.Body, err
			})
			if err != nil {
				if errors.Is(err, identitydomain.ErrIdempotentConflict) {
					return communityapp.Outcome{}, communityapp.ErrConflict
				}
				return communityapp.Outcome{}, err
			}
			return communityapp.Outcome{StatusCode: out.StatusCode, Body: out.Response, Replayed: out.Replayed}, nil
		},
		EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			_, err := platformjobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
			return err
		},
		Store: communityStore,
	}
	communityhttp.Handler{
		Authenticate: func(r *http.Request) (communityapp.Caller, error) {
			id, err := authVerifier.Verify(r.Context(), r)
			if err != nil {
				return communityapp.Caller{}, err
			}
			token, err := identityRegistrar.AttributionToken(r.Context(), id.ContributorID)
			if err != nil {
				return communityapp.Caller{}, err
			}
			return communityapp.Caller{
				ContributorID: id.ContributorID, Fingerprint: id.Fingerprint,
				KeyID: id.KeyID, Token: token,
			}, nil
		},
		Submit: func(ctx context.Context, caller communityapp.Caller, key string, dto communityapp.SubmitDTO, body []byte) (communityapp.SubmitResult, bool, error) {
			// Kernel validation first so only exact values reach the domain.
			product, err := kernel.ParseProduct(dto.Product)
			if err != nil {
				return communityapp.SubmitResult{}, false, err
			}
			unit, err := product.Unit()
			if err != nil {
				return communityapp.SubmitResult{}, false, err
			}
			price, err := kernel.ParsePrice(product, unit, dto.RawText)
			if err != nil {
				// Fall back to the DTO amount only when no raw text exists.
				if dto.RawText != "" {
					return communityapp.SubmitResult{}, false, err
				}
				price = kernel.Price{Product: product, Unit: unit, Milli: dto.AmountMilli, Raw: ""}
			}
			_ = price
			dto.Product, dto.Unit, dto.AmountMilli = string(product), string(unit), price.Milli
			if dto.ConditionKind != "" {
				var qualifier *string
				if dto.Qualifier != "" {
					qualifier = &dto.Qualifier
				}
				if _, err := kernel.ParseCondition(dto.ConditionKind, qualifier); err != nil {
					return communityapp.SubmitResult{}, false, err
				}
			}
			res, err := communityapp.Submit(ctx, communityPorts, caller, http.MethodPost, "/v1/observations", key, body, dto)
			if err != nil {
				return communityapp.SubmitResult{}, false, err
			}
			return res, res.Replayed, nil
		},
		Status: func(ctx context.Context, caller communityapp.Caller, id string) (communityapp.StatusResult, error) {
			return communityapp.Status(ctx, communityPorts, caller, id)
		},
		History: func(ctx context.Context, caller communityapp.Caller, limit int, after time.Time, afterID string, hasCursor bool) ([]communityapp.HistoryItem, string, error) {
			return communityapp.History(ctx, communityPorts, caller, limit, after, afterID, hasCursor)
		},
		Secrets: cfg.CursorSecret,
	}.RegisterRoutes(router)
	// Private upload reservation and owner status (P05-T04). The
	// presigned issuance arrives as a narrow port built here from the
	// configured storage identity; unset storage refuses explicitly
	// with 503 instead of misbehaving.
	evidenceStore := evidenceadapters.NewStore(pool.Underlying())
	evidencePorts := evidenceapp.Ports{
		Clock: time.Now,
		NewID: newUUID,
		NewKey: func() (string, error) {
			id, err := newUUID()
			if err != nil {
				return "", err
			}
			return "q/" + strings.ReplaceAll(id, "-", ""), nil
		},
		CheckQuota: func(ctx context.Context, subject, operation string) (time.Duration, error) {
			retryAfter, err := identityLimiter.Check(ctx, subject, operation)
			if err != nil {
				var denied *identityadapters.QuotaError
				if errors.As(err, &denied) {
					return denied.RetryAfter, &evidenceapp.QuotaDeniedError{RetryAfter: denied.RetryAfter}
				}
				return 0, err
			}
			return retryAfter, nil
		},
		Presign: func(ctx context.Context, key, mime string, maxBytes int64) (string, map[string]string, time.Time, error) {
			if cfg.R2 == nil {
				return "", nil, time.Time{}, evidenceapp.ErrStorageUnavailable
			}
			pre, err := evidencestorage.PresignPUT(evidencestorage.PresignInput{
				Endpoint: cfg.R2.Endpoint, Bucket: cfg.R2.Bucket,
				Key: key, Namespace: "q/",
				ContentType: mime, MaxBytes: maxBytes,
				TTL: 5 * time.Minute, Region: cfg.R2.Region, Now: time.Now(),
			}, evidencestorage.Credentials{AccessKeyID: cfg.R2.AccessKeyID, SecretAccessKey: cfg.R2.SecretAccessKey})
			if err != nil {
				return "", nil, time.Time{}, err
			}
			return pre.URL, pre.RequiredHeaders, pre.ExpiresAt, nil
		},
		Store: evidenceStore,
	}
	evidenceComplete := evidenceapp.CompletePorts{
		Store: evidenceStore,
		EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			_, err := platformjobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
			return err
		},
	}
	evidencehttp.Handler{
		Authenticate: func(r *http.Request) (evidenceapp.Caller, error) {
			id, err := authVerifier.Verify(r.Context(), r)
			if err != nil {
				return evidenceapp.Caller{}, err
			}
			token, err := identityRegistrar.AttributionToken(r.Context(), id.ContributorID)
			if err != nil {
				return evidenceapp.Caller{}, err
			}
			return evidenceapp.Caller{ContributorID: id.ContributorID, Token: token}, nil
		},
		Reserve: func(ctx context.Context, caller evidenceapp.Caller, in evidenceapp.Intent, _ []byte) (evidenceapp.Result, error) {
			return evidenceapp.Reserve(ctx, evidencePorts, caller, in)
		},
		Complete: func(ctx context.Context, caller evidenceapp.Caller, id string) (evidenceapp.StatusView, error) {
			return evidenceapp.Complete(ctx, evidenceComplete, caller, id)
		},
		Status: func(ctx context.Context, caller evidenceapp.Caller, id string) (evidenceapp.StatusView, error) {
			return evidenceapp.Status(ctx, evidenceStore, caller, id)
		},
	}.RegisterRoutes(router)
	server, err := httpserver.New(httpserver.Options{
		Addr:              cfg.HTTPAddr,
		ReadTimeout:       httpserver.DefaultReadTimeout,
		ReadHeaderTimeout: httpserver.DefaultReadHeaderTimeout,
		WriteTimeout:      httpserver.DefaultWriteTimeout,
		IdleTimeout:       httpserver.DefaultIdleTimeout,
		ShutdownTimeout:   cfg.ShutdownTimeout,
		MaxBodyBytes:      cfg.MaxBodyBytes,
		Logger:            logger,
	}, router)
	if err != nil {
		return err
	}
	logger.Info(context.Background(), "api.startup", "api serving")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Run(ctx)
}
