// Command worker is the background-job process role.
//
// It composes the dispatcher, the recurring schedules and the durable
// handlers, then ticks schedules and drains the queue until signaled. Only
// composition lives here: schedules are versioned payloads, handlers own
// their logic, and the geocode schedule stays disabled until D05 selects a
// live provider.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	dbplatform "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/platform"
	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	communityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters/jobs"
	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	directoryadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/jobs"
	directorydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
	evidenceadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters"
	evidencejobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/jobs"
	evidencestorage "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	evidencedomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	officialadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/anp"
	officialjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/jobs"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	trustadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/adapters"
	trustdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// sweepJobPorts adapts the platform job lookup to the evidence sweep
// port: only queued or leased verify jobs count as live leases.
type sweepJobPorts struct {
	hasLive func(ctx context.Context, sessionID string) (bool, error)
}

func (s sweepJobPorts) HasLiveJob(ctx context.Context, sessionID string) (bool, error) {
	return s.hasLive(ctx, sessionID)
}

// sweepStoragePorts adapts presigned deletes to the sweep storage port,
// one namespace per call site.
type sweepStoragePorts struct {
	deleteKey func(ctx context.Context, key, namespace string) error
}

func (s sweepStoragePorts) DeleteQuarantine(ctx context.Context, key string) error {
	return s.deleteKey(ctx, key, "q/")
}

func (s sweepStoragePorts) DeleteFinal(ctx context.Context, key string) error {
	return s.deleteKey(ctx, key, "f/")
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "anpfuel worker:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger, err := telemetry.New(os.Stdout, cfg.LogLevel, telemetry.RoleWorker)
	if err != nil {
		return err
	}
	_ = logger
	pool, err := database.Open(context.Background(), cfg.DatabaseURL, database.DefaultOptions())
	if err != nil {
		return err
	}
	defer pool.Close()
	raw := pool.Underlying()

	directoryRepo := directoryadapters.NewRepository(raw)
	evidenceStore := evidenceadapters.NewStore(raw)
	trustStore := trustadapters.NewStore(raw)
	officialReader := officialread.NewReader(raw)
	importer := officialadapters.NewImporter(raw)
	fetcher := source.NewFetcher(source.DefaultAllowlist())
	queue := jobs.NewQueue(raw)
	httpClient := &http.Client{Timeout: time.Minute}
	storageCreds := func() evidencestorage.Credentials {
		if cfg.R2 == nil {
			return evidencestorage.Credentials{}
		}
		return evidencestorage.Credentials{AccessKeyID: cfg.R2.AccessKeyID, SecretAccessKey: cfg.R2.SecretAccessKey}
	}
	// Bounded server-side download through a self-minted short GET URL:
	// the worker holds the credentials, clients never see GET URLs.
	downloadSnapshot := func(ctx context.Context, key string, maxBytes int64) ([]byte, error) {
		if cfg.R2 == nil {
			return nil, errors.New("evidence: storage not configured")
		}
		pre, err := evidencestorage.PresignGET(evidencestorage.PresignInput{
			Endpoint: cfg.R2.Endpoint, Bucket: cfg.R2.Bucket,
			Key: key, Namespace: "q/",
			TTL: 5 * time.Minute, Region: cfg.R2.Region, Now: time.Now(),
		}, storageCreds())
		if err != nil {
			return nil, err
		}
		return evidencestorage.Get(ctx, httpClient, pre.URL, maxBytes)
	}
	// Sanitized final upload through a self-minted short PUT URL under
	// the server-only namespace: never exposed, never client-writable.
	uploadFinal := func(ctx context.Context, key, contentType string, body []byte) error {
		if cfg.R2 == nil {
			return errors.New("evidence: storage not configured")
		}
		pre, err := evidencestorage.PresignPUT(evidencestorage.PresignInput{
			Endpoint: cfg.R2.Endpoint, Bucket: cfg.R2.Bucket,
			Key: key, Namespace: "f/",
			ContentType: contentType, MaxBytes: int64(len(body)),
			TTL: 5 * time.Minute, Region: cfg.R2.Region, Now: time.Now(),
		}, storageCreds())
		if err != nil {
			return err
		}
		return evidencestorage.Put(ctx, httpClient, pre.URL, contentType, body)
	}
	enqueueImport := func(ctx context.Context, payload officialjobs.ImportPayload, dedupe string) error {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = jobs.Enqueue(ctx, raw, "anp-import", encoded, dedupe, 3, time.Time{})
		return err
	}
	handlers := map[string]jobs.Handler{
		"anp-discovery": officialjobs.Discovery{
			Fetch:       &fetcher,
			ListingBase: "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/",
			Enqueue:     enqueueImport,
		},
		"anp-import": officialjobs.Import{
			Fetch:  &fetcher,
			Parser: anp.Parser{Limits: anp.DefaultLimits()},
			Import: importer,
			Resolve: func(ctx context.Context, cnpj, display string, addr map[string]string) (string, error) {
				st, err := directoryRepo.ResolveCNPJ(ctx, cnpj, display, addr)
				if err != nil {
					return "", err
				}
				return st.ID, nil
			},
		},
		"geocode-station": directoryjobs.Geocode{
			Pool:    raw,
			Service: nil,
			Batch:   25,
		},
		"validate-observation": communityjobs.Validate{
			// Fresh signal bands persist after admission (P06-T01):
			// claimant-position intake does not exist yet, so the
			// loader derives UNKNOWN proximity honestly while the
			// tested NEAR/FAR matrix activates with the intake. The
			// upsert converges, so derivation failures safely retry
			// the whole job without forking history.
			Derive: func(ctx context.Context, observationID string) error {
				_, err := communityapp.DeriveSignals(ctx, communityapp.SignalPorts{
					Clock: time.Now,
					Store: communityadapters.NewStore(raw),
					Station: func(ctx context.Context, stationID string) (communityapp.StationSite, error) {
						st, err := directoryRepo.Station(ctx, stationID)
						if err != nil {
							if errors.Is(err, directorydomain.ErrUnknownStation) {
								return communityapp.SiteUnknown, nil
							}
							return "", err
						}
						// Only reviewed precise points count; city
						// centroids never stand in for position (B-BR-014).
						if st.CurrentPointWKT == "" || st.CurrentQuality != directorydomain.QualityReviewed {
							return communityapp.SiteUnknown, nil
						}
						return communityapp.SitePrecise, nil
					},
					Photo: func(ctx context.Context, evidenceID string) (bool, int, error) {
						dhash, _, err := evidenceStore.ObjectSignals(ctx, evidenceID)
						if err != nil {
							if errors.Is(err, evidenceadapters.ErrNoObject) {
								return false, 0, nil
							}
							return false, 0, err
						}
						matches, err := evidenceStore.FindByDHash(ctx, dhash, time.Now().Add(-90*24*time.Hour))
						if err != nil {
							return false, 0, err
						}
						dups := len(matches) - 1
						if dups < 0 {
							dups = 0
						}
						return true, dups, nil
					},
					Regional: func(ctx context.Context, stationID, product, unit string) (int64, bool, error) {
						groups, err := officialReader.Groups(ctx, stationID, product)
						if err != nil {
							return 0, false, err
						}
						for _, g := range groups {
							if g.Product == product && g.Unit == unit && g.Official != nil {
								return g.Official.AmountMilli, true, nil
							}
						}
						return 0, false, nil
					},
				}, observationID)
				return err
			},
			Run: func(ctx context.Context, observationID, commandRef string) (string, error) {
				store := communityadapters.NewStore(raw)
				return communityapp.Validate(ctx, communityapp.ValidateDeps{
					Clock: time.Now,
					StationExists: func(ctx context.Context, stationID string) (bool, error) {
						if _, err := directoryRepo.Station(ctx, stationID); err != nil {
							if errors.Is(err, directorydomain.ErrUnknownStation) {
								return false, nil
							}
							return false, err
						}
						return true, nil
					},
					StationLocation: func(ctx context.Context, stationID string) (string, bool, error) {
						st, err := directoryRepo.Station(ctx, stationID)
						if err != nil {
							return "", false, err
						}
						if st.CurrentPointWKT == "" || st.CurrentQuality == "" {
							return "unknown", false, nil
						}
						return st.CurrentQuality, true, nil
					},
					// Real evidence reader (P05-T04): missing objects wait
					// for READY or the evidence deadline; present ones are
					// READY facts with owner. Metadata-only observations
					// validate without waiting.
					Evidence: func(ctx context.Context, evidenceID string) (communityapp.EvidenceState, error) {
						view, err := evidenceStore.ForCommunity(ctx, evidenceID)
						if err != nil {
							return communityapp.EvidenceState{}, err
						}
						return communityapp.EvidenceState{Found: view.Found, Ready: view.Ready, OwnerRef: view.OwnerRef}, nil
					},
					// Exactly-once photo binding (P05-T04): set-if-unbound-
					// or-same converges replays, reuse across observations
					// maps onto the stable reused refusal.
					ClaimEvidence: func(ctx context.Context, evidenceID, observationID, contributorRef string) error {
						err := evidenceStore.TryBindObject(ctx, evidenceID, observationID, contributorRef)
						if errors.Is(err, evidenceadapters.ErrAlreadyBound) {
							return communityapp.ErrEvidenceInUse
						}
						return err
					},
					// Real trust ledger (P06-T03): unknown contributors
					// read as NEW, only an audited BLOCKED verdict
					// refuses. Promotion never consults volume or payment.
					Trust: func(ctx context.Context, contributorRef string) (bool, error) {
						tier, err := trustStore.Tier(ctx, contributorRef)
						if err != nil {
							return false, err
						}
						return tier == trustdomain.TierBlocked, nil
					},
					Store: store,
					EnqueueConsensus: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
						_, err := jobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
						return err
					},
				}, observationID, commandRef)
			},
		},
		"verify-evidence": evidencejobs.Verify{
			Store: evidenceStore,
			NewID: jobs.NewUUIDv4,
			Verify: func(ctx context.Context, sess evidencedomain.Session) (evidenceapp.Outcome, error) {
				return evidenceapp.Verify(ctx, evidenceapp.VerifyPorts{
					Clock:    time.Now,
					Download: downloadSnapshot,
					Upload:   uploadFinal,
				}, sess)
			},
		},
		"evidence-sweep": evidencejobs.Sweep{
			Run: func(ctx context.Context) (evidenceapp.SweepReport, error) {
				return evidenceapp.Sweep(ctx, evidenceapp.SweepDeps{
					Clock: time.Now,
					Batch: evidenceapp.DefaultSweepBatch,
					Store: evidenceStore,
					Jobs: sweepJobPorts{hasLive: func(ctx context.Context, sessionID string) (bool, error) {
						id, err := dbplatform.New(raw).GetJobByDedupe(ctx, pgtype.Text{String: "verify:" + sessionID, Valid: true})
						if err != nil {
							if errors.Is(err, pgx.ErrNoRows) {
								return false, nil
							}
							return false, err
						}
						row, err := dbplatform.New(raw).GetJob(ctx, id)
						if err != nil {
							if errors.Is(err, pgx.ErrNoRows) {
								return false, nil
							}
							return false, err
						}
						return row.Status == "queued" || row.Status == "leased", nil
					}},
					Storage: sweepStoragePorts{deleteKey: func(ctx context.Context, key, namespace string) error {
						if cfg.R2 == nil {
							return errors.New("evidence: storage not configured")
						}
						pre, err := evidencestorage.PresignDELETE(evidencestorage.PresignInput{
							Endpoint: cfg.R2.Endpoint, Bucket: cfg.R2.Bucket,
							Key: key, Namespace: namespace,
							TTL: 5 * time.Minute, Region: cfg.R2.Region, Now: time.Now(),
						}, storageCreds())
						if err != nil {
							return err
						}
						return evidencestorage.Delete(ctx, httpClient, pre.URL)
					}},
				})
			},
		},
	}
	dispatch := &jobs.Dispatcher{
		Queue: queue, Handlers: handlers,
		WorkerID: "worker-1", LeaseTTL: 5 * time.Minute, RetryDelay: time.Minute,
	}
	scheduler := jobs.NewScheduler(raw, []jobs.Schedule{
		{
			Name: "anp-discovery-daily", Kind: "anp-discovery", Version: 1,
			Interval: 24 * time.Hour, Enabled: true,
			Build: func(period string) map[string]any { return map[string]any{"period": period} },
		},
		{
			Name: "geocode-hourly", Kind: "geocode-station", Version: 1,
			Interval: time.Hour, Enabled: false, Reason: "D05 pending: no live provider",
			Build: func(period string) map[string]any { return map[string]any{"period": period} },
		},
		{
			Name: "evidence-sweep-hourly", Kind: "evidence-sweep", Version: 1,
			Interval: time.Hour, Enabled: true,
			Build: func(string) map[string]any { return map[string]any{"version": 1} },
		},
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dispatchErr := make(chan error, 1)
	go func() {
		if err := dispatch.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			dispatchErr <- err
			return
		}
		dispatchErr <- nil
	}()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if _, err := scheduler.Tick(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return <-dispatchErr
		case err := <-dispatchErr:
			return err
		case <-ticker.C:
		}
	}
}
