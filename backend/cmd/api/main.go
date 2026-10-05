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
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	dbdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	dbstationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accounthttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/http"
	accountkeyprover "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/keyprover"
	accountmail "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/mail"
	accountoidc "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/oidc"
	accountapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	communityhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters/http"
	communityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters/read"
	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	communitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	directoryadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/http"
	directoryintake "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/intake"
	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	directorydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
	evidenceadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters"
	evidencehttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/http"
	evidencestorage "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	feedbackadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/adapters"
	feedbackhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/adapters/http"
	feedbackapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/application"
	feedbackdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/domain"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	identityauth "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/auth"
	identityhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters/http"
	identitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/application"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	officialhttp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/http"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	profileadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/adapters"
	profileapplication "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/health"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpserver"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry/metrics"
)

// unixClock adapts wall time to the account domain Clock port.
type unixClock struct{}

func (unixClock) NowUnix() int64 { return time.Now().Unix() }

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
	// Private service metrics (P08-T05): bounded request counters by
	// method, route template and status class. The exposition stays on
	// the loopback-only metrics listener wired below, never on the
	// public router.
	metricsReg := metrics.NewRegistry()
	httpRequests, err := metricsReg.Counter("http_requests_total", "Requests by method, route template and status class.", "method", "route", "class")
	if err != nil {
		return err
	}
	httpSeconds, err := metricsReg.Counter("http_request_seconds_total", "Request seconds by method, route template and status class.", "method", "route", "class")
	if err != nil {
		return err
	}
	router.Use(metrics.Observe(metricsReg, httpRequests, httpSeconds))
	// Lazy pool: the process boots (live, not ready) while PostgreSQL is
	// down. Closed after the server drains, at shutdown.
	pool, err := database.Open(context.Background(), cfg.DatabaseURL, database.DefaultOptions())
	if err != nil {
		return err
	}
	defer pool.Close()
	healthHandler, err := health.New(logger, 0, health.Check{Name: "db", Fn: pool.Ping}, health.Check{Name: "schema", Fn: func(ctx context.Context) error { return migrate.Check(ctx, pool.Underlying(), dbmigrations.Files) }})
	if err != nil {
		return err
	}
	router.Get("/health/live", healthHandler.Live)
	router.Get("/health/ready", healthHandler.Ready)
	// Scrape-time collectors (P08-T05): cheap readers evaluated per
	// scrape; failures report NaN (unknown), never stale values.
	rawPool := pool.Underlying()
	jobQueue := platformjobs.NewQueue(rawPool)
	scrapeCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 2*time.Second)
	}
	if _, err := metricsReg.GaugeFunc("db_up", "Database reachability: 1 up, 0 down.", func() float64 {
		ctx, cancel := scrapeCtx()
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			return 0
		}
		return 1
	}); err != nil {
		return err
	}
	if _, err := metricsReg.GaugeFunc("db_pool_acquired", "Acquired pool connections.", func() float64 {
		return float64(rawPool.Stat().AcquiredConns())
	}); err != nil {
		return err
	}
	if _, err := metricsReg.GaugeFunc("db_pool_idle", "Idle pool connections.", func() float64 {
		return float64(rawPool.Stat().IdleConns())
	}); err != nil {
		return err
	}
	if _, err := metricsReg.GaugeFunc("jobs_queued", "Queued jobs awaiting claim.", func() float64 {
		ctx, cancel := scrapeCtx()
		defer cancel()
		queued, _, _, err := jobQueue.Backlog(ctx)
		if err != nil {
			return math.NaN()
		}
		return float64(queued)
	}); err != nil {
		return err
	}
	if _, err := metricsReg.GaugeFunc("jobs_dead", "Dead-lettered jobs needing audited replay.", func() float64 {
		ctx, cancel := scrapeCtx()
		defer cancel()
		_, dead, _, err := jobQueue.Backlog(ctx)
		if err != nil {
			return math.NaN()
		}
		return float64(dead)
	}); err != nil {
		return err
	}
	if _, err := metricsReg.GaugeFunc("jobs_oldest_queued_seconds", "Age of the oldest queued job.", func() float64 {
		ctx, cancel := scrapeCtx()
		defer cancel()
		_, _, oldest, err := jobQueue.Backlog(ctx)
		if err != nil || oldest.IsZero() {
			return math.NaN()
		}
		return time.Since(oldest).Seconds()
	}); err != nil {
		return err
	}
	// Anonymous catalog reads (P02-T08). Only this composition root wires
	// modules together: the official handler gets station existence as a
	// closure so modules never cross-read.
	stations := directoryread.NewReader(pool.Underlying())
	prices := officialread.NewReader(pool.Underlying())
	communityPrices := communityread.NewReader(pool.Underlying())
	directoryhttp.Handler{Stations: stations, Secrets: cfg.CursorSecret}.RegisterRoutes(router)
	// Public station profiles (P30-T02). Same composition rule: the
	// profile handler gets the canonical reader as a closure so
	// modules never cross-read. Anonymous; no badge without a grant.
	profileadapters.Handler{
		Store: profileadapters.Store{Q: dbstationprofile.New(pool.Underlying())},
		Read: func(ctx context.Context, stationID string) (string, string, *float64, *float64, error) {
			station, err := stations.Detail(ctx, stationID)
			if err != nil {
				return "", "", nil, nil, err
			}
			var lat, lon *float64
			if station.Coordinates != nil {
				lat, lon = &station.Coordinates.Lat, &station.Coordinates.Lon
			}
			return station.DisplayName, station.LocationQuality, lat, lon, nil
		},
	}.RegisterRoutes(router)
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
		// Projected community section (P06-T05): one indexed row per
		// exact key, mapped onto the public shape without contributor
		// identities. Missing keys stay null, never an ANP fill-in.
		Community: func(ctx context.Context, stationID, product, unit, conditionKind, qualifier string) (officialhttp.CommunityPrice, bool, error) {
			view, err := communityPrices.CurrentPrice(ctx, stationID, product, unit, conditionKind, qualifier)
			if err != nil {
				return officialhttp.CommunityPrice{}, false, err
			}
			if !view.Found {
				return officialhttp.CommunityPrice{}, false, nil
			}
			return officialhttp.CommunityPrice{
				Availability: view.Availability, Confidence: view.Confidence,
				Freshness: view.Freshness, AmountMilli: view.AmountMilliBrl,
				HasAmount: view.HasAmount, Supporters: view.Supporters,
				Confirmations:    view.Confirmations,
				RepresentativeID: view.RepresentativeID,
				AnchorReceivedAt: view.AnchorReceivedAt, HasAnchor: view.HasAnchor,
				ExpiresAt: view.ExpiresAt, HasExpiry: view.HasExpiry,
				AlgorithmVersion:  view.AlgorithmVersion,
				ProjectionVersion: view.ProjectionVersion,
			}, true, nil
		},
	}.RegisterRoutes(router)
	// Community writes and owner reads (P04-T04). Same closure rule: every
	// cross-module collaborator arrives as a narrow function built here.
	communityStore := communityadapters.NewStore(pool.Underlying())
	identityRegistrar := identityadapters.NewRegistrar(pool.Underlying())
	identityRunner := identityadapters.NewRunner(pool.Underlying())
	identityLimiter := identityadapters.NewLimiter(pool.Underlying(), identitydomain.DefaultQuotaPolicy())
	identityhttp.Handler{Registrar: identityRegistrar, Authority: cfg.CanonicalHost, QuotaSecret: cfg.CursorSecret, CheckQuota: identityLimiter.Check}.RegisterRoutes(router)
	// FREE email accounts (P13-T02C). Narrow closure wiring like the other
	// modules: Postgres store, memory mail sink, production generators.
	// The memory sink is an explicit preview: codes issue but deliver
	// nowhere until SMTP lands with deployment configuration.
	accountMail := accountmail.NewOutbox(unixClock{})
	// FREE provider verification (P13-T03C). Production JWKS over HTTPS
	// with the frozen 1h cache plus the DB-backed nonce ledger; stub
	// issuers stay test-only. Audience stays server-side.
	accountVerifier := &accountoidc.Verifier{
		Clock:    time.Now,
		Issuers:  map[string]string{"google": accountdomain.IssuerGoogle, "apple": accountdomain.IssuerApple},
		Audience: "anpfuel-backend",
		Keys: &accountoidc.CachedKeys{
			Source: accountoidc.HTTPKeys{URLs: accountoidc.ProductionJWKS()},
			TTL:    time.Duration(accountdomain.JWKSCacheTTLSeconds) * time.Second,
			Now:    time.Now,
		},
		Nonces:   accountadapters.NewPGNonces(pool.Underlying()),
		Skew:     time.Duration(accountdomain.OIDCClockSkewSeconds) * time.Second,
		CacheTTL: time.Duration(accountdomain.JWKSCacheTTLSeconds) * time.Second,
	}
	// Contributor device-key proofs (P13-T04C) resolve through the
	// identity-owned key source: one narrow closure, no cross-module
	// table imports.
	accountKeys := accountkeyprover.Prover{
		Keys: func(ctx context.Context, fingerprint string) (string, string, string, bool, error) {
			key, err := identityRegistrar.FindDeviceKey(ctx, fingerprint)
			if err != nil {
				return "", "", "", false, err
			}
			return key.ContributorID, key.JWKX, key.JWKY, key.Revoked, nil
		},
		Now: time.Now,
	}
	// The account store doubles as the social-write gate source
	// (P13-T04D): one shared handle feeds the service and the
	// community CheckAccount closures below.
	accountStore := accountadapters.NewPGStore(pool.Underlying())
	accountService := &accountapp.Service{
		Clock:     unixClock{},
		Hasher:    accountdomain.SHA256Hasher{},
		Mail:      accountMail,
		Store:     accountStore,
		Verifier:  accountVerifier,
		KeyProver: accountKeys,
		CodeGen:   accountdomain.GenerateCode,
		TokenGen:  accountdomain.GenerateToken,
		AliasGen:  accountdomain.GenerateAlias,
		IDGen:     newUUID,
	}
	accounthttp.Handler{Service: accountService, Audience: "anpfuel-backend"}.RegisterRoutes(router)
	// Station/fuel feedback (P14-T03). Same closure rule: session
	// ownership, account liveness and station existence arrive as
	// narrow functions; only active accounts write, reads stay
	// public. Session failures split into forbidden (dead account)
	// versus invalid (bad/expired proof) without importing the
	// account domain into the feedback handler.
	feedbackStore := feedbackadapters.NewPGStore(pool.Underlying())
	moderationStore := moderationadapters.NewStore(pool.Underlying())
	feedbackService := &feedbackapp.Service{
		Clock:    unixClock{},
		Store:    feedbackStore,
		Comments: feedbackStore,
		Votes:    feedbackStore,
		CheckAccount: func(ctx context.Context, accountID string) error {
			acc, found, err := accountStore.GetAccount(ctx, accountID)
			if err != nil {
				return err
			}
			if !found || !acc.Active() {
				return feedbackdomain.ErrAuthorForbidden
			}
			return nil
		},
		StationExists: func(ctx context.Context, stationID string) (bool, error) {
			_, err := stations.Detail(ctx, stationID)
			if err == nil {
				return true, nil
			}
			if errors.Is(err, directoryapp.ErrUnknownStation) {
				return false, nil
			}
			return false, err
		},
		ReportQuota: func(ctx context.Context, subject, operation string) (time.Duration, error) {
			retryAfter, err := identityLimiter.Check(ctx, subject, operation)
			if err != nil {
				var denied *identityadapters.QuotaError
				if errors.As(err, &denied) {
					return denied.RetryAfter, &feedbackdomain.QuotaDeniedError{RetryAfterSeconds: int64(denied.RetryAfter / time.Second)}
				}
				return 0, err
			}
			return retryAfter, nil
		},
		OpenCase: func(ctx context.Context, reporterAccountID, commentID, reason string) (string, error) {
			res, err := moderationapp.Open(ctx, moderationapp.Ports{
				Clock: time.Now,
				NewID: newUUID,
				Store: moderationStore,
			}, moderationapp.OpenDTO{
				TargetType: moderationdomain.TargetComment,
				TargetID:   commentID,
				Reason:     reason,
				Detail:     "reported via feedback API",
			})
			if err != nil {
				return "", err
			}
			return res.CaseID, nil
		},
		IDGen: newUUID,
	}
	feedbackhttp.Handler{
		Service: feedbackService,
		Sessions: func(ctx context.Context, familyID, accessToken string) (string, error) {
			id, err := accountService.ValidateAccess(ctx, familyID, accessToken)
			if err != nil {
				switch accountdomain.VerdictCode(err) {
				case "account-suspended", "account-deleted":
					return "", feedbackdomain.ErrAuthorForbidden
				default:
					return "", feedbackdomain.ErrSessionInvalid
				}
			}
			return id, nil
		},
	}.RegisterRoutes(router)
	// Private station intake (P27-T01). Only this composition root wires
	// modules together: the intake handler gets session validation as a
	// closure so modules never cross-read. Suspended/deleted accounts
	// cannot write; anything else invalid is a 401 (the handler maps
	// service verdicts itself).
	directoryintake.Handler{
		Service: directoryapp.IntakeService{
			Store: directoryadapters.IntakeStore{Q: dbdirectory.New(pool.Underlying())},
			Clock: time.Now,
			NewID: newUUID,
		},
		Sessions: func(ctx context.Context, familyID, accessToken string) (string, error) {
			id, err := accountService.ValidateAccess(ctx, familyID, accessToken)
			if err != nil {
				switch accountdomain.VerdictCode(err) {
				case "account-suspended", "account-deleted":
					return "", directoryapp.ErrAuthorForbidden
				default:
					return "", err
				}
			}
			return id, nil
		},
	}.RegisterRoutes(router)
	// Private representation claims (P30-T03). Same composition rule:
	// session validation and the current operator link arrive as
	// closures so modules never cross-read. No grant flows from
	// submission, CNPJ knowledge or client flags.
	profileStore := profileadapters.Store{Q: dbstationprofile.New(pool.Underlying())}
	profileClaimPorts := profileapplication.ClaimPorts{
		Store: profileadapters.ClaimStore{Q: dbstationprofile.New(pool.Underlying())},
		Clock: time.Now,
		NewID: newUUID,
		OperatorOf: func(ctx context.Context, stationID string) (string, string, bool, error) {
			operator, found, err := profileStore.CurrentOperator(ctx, stationID)
			if err != nil || !found {
				return "", "", false, err
			}
			return operator.CNPJ, operator.Source, true, nil
		},
	}
	profileadapters.ClaimHandler{
		Ports: profileClaimPorts,
		Sessions: func(ctx context.Context, familyID, accessToken string) (string, error) {
			return accountService.ValidateAccess(ctx, familyID, accessToken)
		},
	}.RegisterRoutes(router)
	// Private proof intake (P30-T04). Object storage is unprovisioned:
	// Bytes stays nil so intake refuses with 503 instead of any
	// approval fallback. Wire it with the storage scope when that
	// lands; nothing else changes.
	profileadapters.ProofHandler{
		Ports: profileapplication.ProofPorts{
			Proofs: profileadapters.ProofStore{Q: dbstationprofile.New(pool.Underlying())},
			Claims: profileadapters.ClaimStore{Q: dbstationprofile.New(pool.Underlying())},
			Clock:  time.Now,
			NewID:  newUUID,
			Bytes:  nil,
		},
		Sessions: func(ctx context.Context, familyID, accessToken string) (string, error) {
			return accountService.ValidateAccess(ctx, familyID, accessToken)
		},
	}.RegisterRoutes(router)
	// Scoped business management (P31-T04). Same composition rule:
	// grant/operator checks, feedback reply submission and business
	// attribution arrive as closures so modules never cross-read.
	// Every privilege enforces server-side before any app button.
	profileManageStore := profileadapters.Store{Q: dbstationprofile.New(pool.Underlying())}
	profileManagePorts := func(r *http.Request) profileapplication.ManagePorts {
		_ = r
		return profileapplication.ManagePorts{
			Grants:   profileadapters.ReviewDecisions{Pool: pool.Underlying()},
			Profiles: profileManageStore,
			OperatorOf: func(ctx context.Context, stationID string) (string, bool, error) {
				operator, found, err := profileManageStore.CurrentOperator(ctx, stationID)
				if err != nil || !found {
					return "", false, err
				}
				return operator.CNPJ, true, nil
			},
			AccountLive: func(ctx context.Context, accountID string) (bool, error) {
				acc, found, err := accountStore.GetAccount(ctx, accountID)
				if err != nil || !found {
					return false, err
				}
				return acc.Active(), nil
			},
			SubmitReply: func(ctx context.Context, accountID, stationID, product, text string) (string, error) {
				stored, err := feedbackService.SubmitComment(ctx, accountID, stationID, product, text)
				if err != nil {
					return "", err
				}
				return stored.ID, nil
			},
			Attribute: func(ctx context.Context, commentID, accountID, stationID, grantID string) error {
				return feedbackService.AttributeBusinessComment(ctx, accountID, commentID, stationID, grantID)
			},
		}
	}
	profileadapters.ManageHandler{
		EditPorts:  profileManagePorts,
		ReplyPorts: profileManagePorts,
		Sessions: func(ctx context.Context, familyID, accessToken string) (string, error) {
			return accountService.ValidateAccess(ctx, familyID, accessToken)
		},
	}.RegisterRoutes(router)
	logger.Info(context.Background(), "api.mail-sink", "sink", "memory-preview")
	authVerifier := &identityauth.Verifier{Pool: pool.Underlying(), Authority: cfg.CanonicalHost}
	// checkAccountGate refuses social writes from contributors bound
	// to a suspended or deleted FREE account (P13-T04D). Unbound
	// contributors keep the anonymous baseline; store failures
	// propagate for a 500 instead of silently allowing writes.
	checkAccountGate := func(ctx context.Context, contributorID string) error {
		blocked, err := accountapp.BindingBlocked(ctx, accountStore, contributorID)
		if err != nil {
			return err
		}
		if blocked {
			return communityapp.ErrAccountBlocked
		}
		return nil
	}
	// Verified location intake (P16-T03B): submit-time PostGIS
	// distance against the precise station point plus teleport
	// review against the contributor's last site. The transient fix
	// never persists: only bands survive the call.
	directoryRepo := directoryadapters.NewRepository(pool.Underlying())
	communityPorts := communityapp.Ports{
		Clock: time.Now,
		Locate: func(ctx context.Context, stationID string, lat, lon float64) (float64, communityapp.StationSite, error) {
			st, err := directoryRepo.Station(ctx, stationID)
			if err != nil {
				if errors.Is(err, directorydomain.ErrUnknownStation) {
					return 0, communityapp.SiteUnknown, nil
				}
				return 0, "", err
			}
			if st.CurrentPointWKT == "" || st.CurrentQuality != directorydomain.QualityReviewed {
				return 0, communityapp.SiteUnknown, nil
			}
			distanceM, err := directoryRepo.FixDistance(ctx, stationID, lon, lat)
			if err != nil {
				if errors.Is(err, directorydomain.ErrUnknownStation) {
					return 0, communityapp.SiteUnknown, nil
				}
				return 0, "", err
			}
			return distanceM, communityapp.SitePrecise, nil
		},
		LastSite: func(ctx context.Context, contributorRef string) (string, time.Time, bool, error) {
			return communityStore.LastObservationSite(ctx, contributorRef)
		},
		StationDistance: func(ctx context.Context, a, b string) (float64, error) {
			return directoryRepo.StationPairDistanceM(ctx, a, b)
		},
		NewID: newUUID,
		Attribution: func(ctx context.Context, contributorID string) (string, error) {
			return identityRegistrar.AttributionToken(ctx, contributorID)
		},
		CheckAccount: checkAccountGate,
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
	// Support votes and structured reports (P06-T02). Same closure
	// rule: quota, identity and recompute enqueue arrive as narrow
	// functions; every accepted write enqueues consensus recomputation.
	votePorts := communityapp.VotePorts{
		Clock: time.Now,
		NewID: newUUID,
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
		CheckAccount: checkAccountGate,
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
			// Wire enums are not ANP source labels: the source-label parser
			// deliberately rejects GASOLINE_REGULAR. Validate the wire value
			// through its fixed unit without changing the submitted amount.
			product := kernel.Product(dto.Product)
			unit, err := product.Unit()
			if err != nil {
				return communityapp.SubmitResult{}, false, communitydomain.ErrUnknownProduct
			}
			if dto.Unit != string(unit) {
				return communityapp.SubmitResult{}, false, communitydomain.ErrUnitMismatch
			}
			if dto.ConditionKind != "" {
				var qualifier *string
				if dto.Qualifier != "" {
					qualifier = &dto.Qualifier
				}
				if _, err := kernel.ParseCondition(dto.ConditionKind, qualifier); err != nil {
					return communityapp.SubmitResult{}, false, communitydomain.ErrUnknownCondition
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
		Confirm: func(ctx context.Context, caller communityapp.Caller, observationID string, dto communityapp.ConfirmDTO, _ []byte) (communityapp.ConfirmResult, error) {
			dto.ObservationID = observationID
			return communityapp.Confirm(ctx, votePorts, caller, dto)
		},
		Dispute: func(ctx context.Context, caller communityapp.Caller, observationID string, dto communityapp.DisputeDTO, _ []byte) (communityapp.DisputeResult, error) {
			dto.TargetObservationID = observationID
			return communityapp.Dispute(ctx, votePorts, caller, dto)
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
		Enforcement: func(ctx context.Context) error {
			// Intake circuit breaker (B-BR-M04): any copy past its
			// 24 h deadline with bytes still present stops new
			// intake until the hourly sweeper catches up.
			overdue, err := evidenceStore.OverdueFinals(ctx, time.Now().Add(-evidenceapp.CopyRetention), 1)
			if err != nil {
				return err
			}
			if len(overdue) > 0 {
				return evidenceapp.ErrEnforcementUnhealthy
			}
			return nil
		},
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
	// Private metrics listener (P08-T05): loopback-only by default,
	// disabled when ANPFUEL_METRICS_ADDR is empty. Never published by
	// the staging/production topologies.
	if cfg.MetricsAddr != "" {
		metricsServer := &http.Server{
			Addr:              cfg.MetricsAddr,
			Handler:           metricsReg.Handler(),
			ReadHeaderTimeout: httpserver.DefaultReadHeaderTimeout,
		}
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
			defer cancel()
			_ = metricsServer.Shutdown(shutdown)
		}()
		go func() {
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error(context.Background(), "metrics.startup", "metrics listener failed", slog.String("error", err.Error()))
			}
		}()
	}
	return server.Run(ctx)
}
