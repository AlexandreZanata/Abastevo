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

	dbmigrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	dbdirectory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	accountadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters"
	accountjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/adapters/jobs"
	accountapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/application"
	accountdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/account/domain"
	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	communityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters/jobs"
	communityapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	communitydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	directoryadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters"
	directoryjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/jobs"
	directoryregistry "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/registry"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	directorydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
	driveradapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/adapters"
	driverjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/adapters/jobs"
	enterprisejobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/enterpriseexport/adapters/jobs"
	evidenceadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters"
	evidencejobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/jobs"
	evidencestorage "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	evidenceapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/application"
	evidencedomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
	feedbackadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/feedback/adapters"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	moderationadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/adapters"
	moderationdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/moderation/domain"
	officialadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/anp"
	officialjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/jobs"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/source"
	privacyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/adapters"
	privacyjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/adapters/jobs"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	privacydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
	qualityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters"
	qualityjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/jobs"
	qualityread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/quality/adapters/read"
	subscriptionadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters"
	subscriptionjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/adapters/jobs"
	subscriptionapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/application"
	trustadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/adapters"
	trustdomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/telemetry"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// inventoryFunc adapts a closure to the privacy inventory port.
type inventoryFunc func(ctx context.Context, contributorID, contributorRef string) (privacyapp.RawInventory, error)

func (f inventoryFunc) Snapshot(ctx context.Context, contributorID, contributorRef string) (privacyapp.RawInventory, error) {
	return f(ctx, contributorID, contributorRef)
}

// trainingTermsWorker serves the current training-terms pair the
// consent grants must present (CS10C; same provisional digest as the
// API until the verified terms text lands in CS11).
type trainingTermsWorker struct {
	digest string
}

func (t trainingTermsWorker) CurrentVersion() string { return privacydomain.TrainingTermsV1 }
func (t trainingTermsWorker) CurrentDigest() string  { return t.digest }

// sanitizeFunc adapts the media-mapped sanitizer to the promotion port.
type sanitizeFunc func(ctx context.Context, source []byte) ([]byte, privacydomain.SanitizeVerdict, error)

func (f sanitizeFunc) Sanitize(ctx context.Context, source []byte) ([]byte, privacydomain.SanitizeVerdict, error) {
	return f(ctx, source)
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
	startup, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	err = migrate.Check(startup, raw, dbmigrations.Files)
	cancelStartup()
	if err != nil {
		return errors.New("worker: database schema is not compatible")
	}

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
	enqueueEnterpriseExport := func(ctx context.Context, payload enterprisejobs.BuildPayload, dedupe string) (jobs.EnqueueResult, error) {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return jobs.EnqueueResult{}, err
		}
		return jobs.Enqueue(ctx, raw, enterprisejobs.BuildKind, encoded, dedupe, 3, time.Time{})
	}
	enqueuePortalRefresh := func(ctx context.Context, payload enterprisejobs.RefreshPayload, dedupe string) (jobs.EnqueueResult, error) {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return jobs.EnqueueResult{}, err
		}
		return jobs.Enqueue(ctx, raw, enterprisejobs.RefreshKind, encoded, dedupe, 3, time.Time{})
	}
	communityStore := communityadapters.NewStore(raw)
	privacyStore := privacyadapters.NewStore(raw)
	modStore := moderationadapters.NewStore(raw)
	identityRunner := identityadapters.NewRunner(raw)
	identityRegistrar := identityadapters.NewRegistrar(raw)
	// exportInventory assembles one owner's export sections from the
	// real ledgers (P07-T03): identity profile, owned observations with
	// derived validation states, and the current trust tier. Every row
	// is read through the owner's attribution token; evidence object
	// metadata joins with the erasure inventory in P07-T04, which needs
	// the same owner listing. Reads page bounded (100/page, 5000 cap)
	// so one export stays a bounded archive.
	exportInventory := func(ctx context.Context, contributorID, contributorRef string) (privacyapp.RawInventory, error) {
		var out privacyapp.RawInventory
		profile, err := identityRegistrar.Profile(ctx, contributorID)
		if err != nil {
			return privacyapp.RawInventory{}, err
		}
		out.Contributor = privacyapp.ContributorView{
			ContributorID: profile.ContributorID, Status: profile.Status,
			CreatedAt: profile.CreatedAt.UTC().Format(time.RFC3339),
		}
		var after time.Time
		var afterID string
		hasCursor := false
		for total := 0; total < 5000; {
			page, err := communityStore.ListByContributor(ctx, contributorRef, 100, after, afterID, hasCursor)
			if err != nil {
				return privacyapp.RawInventory{}, err
			}
			if len(page) == 0 {
				break
			}
			for _, obs := range page {
				decisions, err := communityStore.Decisions(ctx, obs.ID)
				if err != nil {
					return privacyapp.RawInventory{}, err
				}
				state := communitydomain.StateReceived
				for _, d := range decisions {
					state = d.ToState
				}
				out.Observations = append(out.Observations, privacyapp.RawObservation{
					OwnerRef: contributorRef,
					View: privacyapp.ObservationView{
						ObservationID: obs.ID, StationID: obs.StationID,
						Product: obs.Product, Unit: obs.Unit,
						AmountMilli: obs.AmountMilli, Condition: obs.ConditionKind,
						State:      state,
						ReceivedAt: obs.ReceivedAt.UTC().Format(time.RFC3339),
					},
				})
				total++
			}
			last := page[len(page)-1]
			after, afterID, hasCursor = last.ReceivedAt, last.ID, true
			if len(page) < 100 {
				break
			}
		}
		tier, err := trustStore.Tier(ctx, contributorRef)
		if err != nil {
			return privacyapp.RawInventory{}, err
		}
		out.Trust = privacyapp.RawTrust{OwnerRef: contributorRef, Tier: tier}
		return out, nil
	}
	privacyPorts := privacyapp.Ports{
		Clock: time.Now,
		NewID: jobs.NewUUIDv4,
		Attribution: func(ctx context.Context, contributorID string) (string, error) {
			return identityRegistrar.AttributionToken(ctx, contributorID)
		},
		Store:     privacyStore,
		Inventory: inventoryFunc(exportInventory),
		EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			_, err := jobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
			return err
		},
	}
	// privacyErasePorts drives owner erasure over the same real
	// ledgers (P07-T04): identity revocation, community unlinking with
	// recompute, evidence purge with server-side storage deletes, trust
	// removal and export-archive purge, all converging on re-run.
	// Training-derivative promotion and purge ports (CS10C/CS10D):
	// guarded consent reads plus the media-mapped sanitizer for
	// offers, and the namespace object store for the daily
	// revoked-copy sweep and the training erasure scope. Storage
	// stays nil-guarded at call time (same convention as erasure
	// deletes); the quarantine source read lands with the
	// client-review producer in CS11.
	trainingTerms := trainingTermsWorker{digest: cfg.TrainingTermsDigest}
	consentStore := privacyadapters.NewConsentStore(pool.Underlying())
	promotionStore := privacyadapters.NewPromotionStore(pool.Underlying())
	promotePorts := privacyapp.PromotePorts{
		ConsentPorts: privacyapp.ConsentPorts{
			Clock: time.Now,
			Attribution: func(ctx context.Context, contributorID string) (string, error) {
				return identityRegistrar.AttributionToken(ctx, contributorID)
			},
			Store: consentStore,
			Terms: trainingTerms,
		},
		Inventory: promotionStore,
		Sanitizer: sanitizeFunc(privacyadapters.SanitizeImage),
	}
	var trainingObjects *privacyadapters.TrainingObjects
	if cfg.R2 != nil {
		trainingObjects = privacyadapters.NewTrainingObjects(privacyadapters.TrainingObjectConfig{
			Endpoint: cfg.R2.Endpoint, Bucket: cfg.R2.Bucket, Region: cfg.R2.Region,
			AccessKeyID: cfg.R2.AccessKeyID, SecretAccessKey: cfg.R2.SecretAccessKey,
		})
	}
	purgePorts := privacyapp.PurgePorts{
		Clock:     time.Now,
		Inventory: promotionStore,
		Objects:   privacyadapters.TrainingObjectsOrRefusal(trainingObjects),
	}
	erasureDelete := func(ctx context.Context, key, namespace string) error {
		if cfg.R2 == nil {
			return errors.New("evidence: storage not configured")
		}
		copyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return evidencestorage.DeleteConfirmed(copyCtx, httpClient, evidencestorage.PresignInput{
			Endpoint: cfg.R2.Endpoint, Bucket: cfg.R2.Bucket, Key: key, Namespace: namespace,
			TTL: 5 * time.Minute, Region: cfg.R2.Region, Now: time.Now(),
		}, storageCreds())
	}
	privacyErasePorts := privacyapp.ErasePorts{
		Clock: time.Now,
		NewID: jobs.NewUUIDv4,
		Attribution: func(ctx context.Context, contributorID string) (string, error) {
			return identityRegistrar.AttributionToken(ctx, contributorID)
		},
		Store:     privacyStore,
		Identity:  identityRegistrar,
		Community: communityStore,
		Evidence:  evidenceStore,
		Training: privacyadapters.TrainingErasure{
			Clock: time.Now,
			Attribution: func(ctx context.Context, contributorID string) (string, error) {
				return identityRegistrar.AttributionToken(ctx, contributorID)
			},
			Consents:  consentStore,
			Terms:     trainingTerms,
			Inventory: promotionStore,
			Objects:   privacyadapters.TrainingObjectsOrRefusal(trainingObjects),
		},
		Trust:        trustStore,
		DeleteObject: erasureDelete,
		EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			_, err := jobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
			return err
		},
	}
	feedbackStore := feedbackadapters.NewPGStore(pool.Underlying())
	qualityReportStore := &qualityadapters.ReportStore{Pool: raw, Now: time.Now}
	accountRecovery := accountapp.ErasureRecovery{
		Store: accountadapters.NewPGStore(pool.Underlying()), NewNonce: jobs.NewUUIDv4, Now: time.Now,
		EraseFeedback: func(ctx context.Context, id string) error {
			qualityDone, qualityErr := qualityReportStore.EraseAccountBatch(ctx, id)
			if qualityErr != nil {
				return qualityErr
			}
			if !qualityDone {
				return accountapp.ErrErasurePending
			}

			done, err := feedbackStore.EraseAccountBatch(ctx, id, time.Now().Unix())
			if err == nil && !done {
				return accountapp.ErrErasurePending
			}
			return err
		},
		EraseContributor: func(ctx context.Context, accountID, contributorID string) error {
			_, err := privacyapp.Erase(ctx, privacyErasePorts, contributorID, privacyapp.EraseDTO{ClientSubmissionID: "account-erasure:" + accountID + ":" + contributorID, Reason: "account-deletion"})
			return err
		},
	}
	// retentionPurges runs every inventory category bounded
	// oldest-first (P07-T05): expired challenges, idempotency windows
	// and export bytes purge on expiry; long-closed moderation cases
	// and aged-out ledger rows purge on their horizons; the
	// observation horizon reports metrics only until an FK-consistent
	// cascade design lands. Rate windows self-clean on check and
	// evidence media purges on its hourly sweeper.
	retentionPurges := []privacyapp.NamedPurge{
		{Name: "community-photo-captures", Purge: func(ctx context.Context) (privacyapp.CategoryReport, error) {
			count, err := communityStore.PurgeExpiredPhotoCaptures(ctx, time.Now(), privacyapp.RetentionBatch)
			return privacyapp.CategoryReport{Purged: count}, err
		}},
		{Name: "identity-challenges", Purge: func(ctx context.Context) (privacyapp.CategoryReport, error) {
			purged, oldest, err := identityRegistrar.PurgeExpiredChallenges(ctx, time.Now(), privacyapp.RetentionBatch)
			return privacyapp.CategoryReport{Purged: purged, OldestOverdue: oldest}, err
		}},
		{Name: "identity-idempotency", Purge: func(ctx context.Context) (privacyapp.CategoryReport, error) {
			purged, oldest, err := identityRunner.PurgeExpiredAttempts(ctx, time.Now(), privacyapp.RetentionBatch)
			return privacyapp.CategoryReport{Purged: purged, OldestOverdue: oldest}, err
		}},
		{Name: "moderation-closed", Purge: func(ctx context.Context) (privacyapp.CategoryReport, error) {
			cases, actions, oldest, err := modStore.PurgeClosedCases(ctx, time.Now().Add(-privacyapp.ModerationClosedTTL), privacyapp.RetentionBatch)
			return privacyapp.CategoryReport{Purged: cases + actions, OldestOverdue: oldest}, err
		}},
		{Name: "privacy-archives", Purge: func(ctx context.Context) (privacyapp.CategoryReport, error) {
			purged, oldest, err := privacyStore.PurgeExpiredArchives(ctx, time.Now(), privacyapp.RetentionBatch)
			return privacyapp.CategoryReport{Purged: purged, OldestOverdue: oldest}, err
		}},
		{Name: "privacy-ledger", Purge: func(ctx context.Context) (privacyapp.CategoryReport, error) {
			purged, oldest, err := privacyStore.PurgeOldLedger(ctx, time.Now().Add(-privacyapp.LedgerHorizon), privacyapp.RetentionBatch)
			return privacyapp.CategoryReport{Purged: purged, OldestOverdue: oldest}, err
		}},
		{Name: "community-observations", Purge: func(ctx context.Context) (privacyapp.CategoryReport, error) {
			oldest, err := communityStore.OldestObservation(ctx)
			return privacyapp.CategoryReport{Purged: 0, OldestOverdue: oldest}, err
		}},
	}
	// resolvePhoto verifies one evidence object for both consumers:
	// the signals loader counts independent duplicates, the recompute
	// loader binds validated bytes with proximity. Absent or
	// unavailable objects report empty without error.
	type photoResolved struct {
		present   bool
		validated bool
		dhash     uint64
	}
	resolvePhoto := func(ctx context.Context, evidenceID string) (photoResolved, error) {
		view, err := evidenceStore.ForCommunity(ctx, evidenceID)
		if err != nil {
			return photoResolved{}, err
		}
		if !view.Found || !view.Ready {
			return photoResolved{}, nil
		}
		dhash, _, err := evidenceStore.ObjectSignals(ctx, evidenceID)
		if err != nil {
			return photoResolved{}, err
		}
		return photoResolved{present: true, validated: true, dhash: dhash}, nil
	}
	duplicateCount := func(ctx context.Context, dhash uint64) (int, error) {
		matches, err := evidenceStore.FindByDHash(ctx, dhash, time.Now().Add(-90*24*time.Hour))
		if err != nil {
			return 0, err
		}
		if dups := len(matches) - 1; dups > 0 {
			return dups, nil
		}
		return 0, nil
	}
	recomputePorts := communityapp.RecomputePorts{
		Clock: time.Now,
		Store: communityStore,
		Anchors: func(ctx context.Context, key communitydomain.PriceKey, cutoff time.Time) ([]communitydomain.Observation, error) {
			return communityStore.EligibleAnchors(ctx, key, cutoff)
		},
		Confirmations: func(ctx context.Context, anchorIDs []string) ([]communityapp.ConfirmationVote, error) {
			return communityStore.ConfirmationsForAnchors(ctx, anchorIDs)
		},
		Tiers: func(ctx context.Context, refs []string) (map[string]string, error) {
			out := make(map[string]string, len(refs))
			for _, ref := range refs {
				tier, err := trustStore.Tier(ctx, ref)
				if err != nil {
					return nil, err
				}
				out[ref] = tier
			}
			return out, nil
		},
		Photo: func(ctx context.Context, anchor communitydomain.Observation) (communityapp.PhotoInfo, error) {
			if anchor.EvidenceID == "" {
				return communityapp.PhotoInfo{}, nil
			}
			resolved, err := resolvePhoto(ctx, anchor.EvidenceID)
			if err != nil || !resolved.present {
				return communityapp.PhotoInfo{}, err
			}
			// Independently verified bytes count under v2; location is not a trust input.

			return communityapp.PhotoInfo{
				Found: true, Validated: true,
				ProximityOK: false, MediaKey: fmt.Sprintf("%016x", resolved.dhash),
			}, nil
		},
		// Regional deviation already routes through the
		// derived signals (P06-T01); consensus weighs votes
		// only, never benchmarks.
		Moderation: func(context.Context, communitydomain.PriceKey) (bool, error) {
			return false, nil
		},
	}
	// PAY-19: verified payment runtime on its own serial slot. Provider
	// clients exist only for maintenance-ready channels; pending events
	// are preserved, never dropped, while unconfigured.
	premiumStore := subscriptionadapters.NewStore(raw)
	premiumService := &subscriptionapp.Service{Store: premiumStore, Clock: time.Now, Environment: "production", PackageName: "com.anpfuel"}
	officialImport := officialjobs.Import{
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
	}
	if cfg.EnterpriseExportEnabled {
		officialImport.AfterPublish = func(ctx context.Context, revisionID string) error {
			_, err := enqueueEnterpriseExport(ctx, enterprisejobs.BuildPayload{
				Version: 1, Trigger: "after-publication", RevisionID: revisionID,
				ObservedAt: time.Now().UTC().Format(time.RFC3339),
			}, "enterprise-export:official:"+revisionID)
			return err
		}
	}
	handlers := map[string]jobs.Handler{
		"subscription-refresh":     subscriptionjobs.Handler{Name: "subscription-refresh", RunWithID: premiumService.Refresh},
		"subscription-acknowledge": subscriptionjobs.Handler{Name: "subscription-acknowledge", Run: premiumService.Acknowledge},
		"payment-inbox":            composePaymentInbox(cfg.Payments, raw, time.Now),
		"anp-discovery": officialjobs.Discovery{
			Fetch:       &fetcher,
			ListingBase: "https://www.gov.br/anp/pt-br/assuntos/precos-e-defesa-da-concorrencia/precos/arquivos-lpc/",
			Enqueue:     enqueueImport,
		},
		"anp-import": officialImport,
		"geocode-station": directoryjobs.Geocode{
			Pool:    raw,
			Service: nil,
			Batch:   25,
		},
		"registry-reconcile": directoryjobs.Reconcile{
			Store: &directoryregistry.PGStore{Q: dbdirectory.New(pool.Underlying())},
			Canon: directoryadapters.RegistryCanonicalizer{Repo: directoryRepo},
		},
		"registry-discover": directoryjobs.Discover{
			Store:  &directoryregistry.PGStore{Q: dbdirectory.New(pool.Underlying())},
			Config: directoryregistry.ProductionAPIConfig(),
		},
		"suggestion-verify": directoryjobs.VerifySweep{
			Store: &directoryadapters.IntakeStore{Q: dbdirectory.New(pool.Underlying())},
			Resolve: func(ctx context.Context, cnpj, display string, address map[string]string) (string, string, string, error) {
				// Exact official match only: the CNPJ must already sit
				// in staged official assertions from a complete run.
				// Unknown CNPJs defer to human review instead of
				// minting stations from user input alone.
				official, err := dbdirectory.New(pool.Underlying()).FindOfficialAssertion(ctx, cnpj)
				if err != nil {
					return "", "", "", directoryapp.ErrStationUnknown
				}
				station, err := directoryRepo.ResolveCNPJ(ctx, cnpj, display, address)
				if err != nil {
					return "", "", "", err
				}
				return station.ID, official.MunicipalityCode.String, official.State.String, nil
			},
			RecordPin: func(ctx context.Context, stationID string, lat, lon float64, ref string) error {
				_, err := directoryRepo.RecordLocation(ctx, directorydomain.LocationRevision{
					StationID:       stationID,
					PointWKT:        fmt.Sprintf("POINT(%f %f)", lon, lat),
					Quality:         "unknown",
					Provider:        "suggestion",
					SourceReference: ref,
				})
				return err
			},
			NewID: jobs.NewUUIDv4,
			Batch: 25,
			AccountLive: func(ctx context.Context, accountID string) (bool, error) {
				account, found, err := accountadapters.NewPGStore(pool.Underlying()).GetAccount(ctx, accountID)
				if err != nil || !found {
					return false, err
				}
				return account.Status == accountdomain.StatusActive, nil
			},
		},
		"validate-observation": communityjobs.Validate{
			// ADR-021: new signal processing does not read station geometry or
			// require contributor location. The upsert converges, so derivation
			// failures retry without forking history; legacy replay stays separate.
			Derive: func(ctx context.Context, observationID string) error {
				_, err := communityapp.DeriveCommunitySignals(ctx, communityapp.SignalPorts{
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
						resolved, err := resolvePhoto(ctx, evidenceID)
						if err != nil || !resolved.present {
							return false, 0, err
						}
						dups, err := duplicateCount(ctx, resolved.dhash)
						if err != nil {
							return false, 0, err
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
						return communityapp.EvidenceState{Found: view.Found, Ready: view.Ready, OwnerRef: view.OwnerRef, ExpiresAt: view.ExpiresAt}, nil
					},
					// Exactly-once photo binding (P05-T04): set-if-unbound-
					// or-same converges replays, reuse across observations
					// maps onto the stable reused refusal.
					ClaimPhotoEvidence: func(ctx context.Context, obs communitydomain.Observation) error {
						view, err := evidenceStore.ForCommunity(ctx, obs.EvidenceID)
						if err != nil {
							return err
						}
						receipt, err := store.PhotoCapture(ctx, obs.PhotoCaptureID, obs.ContributorRef)
						if errors.Is(err, communityapp.ErrPhotoCaptureIneligible) {
							return communityapp.ErrEvidenceInUse
						}
						if err != nil {
							return err
						}
						if !view.Ready || view.OwnerRef != obs.ContributorRef || view.PhotoCaptureID != obs.PhotoCaptureID {
							return communityapp.ErrEvidenceInUse
						}
						_, err = communityapp.ValidatePhotoCaptureUse(ctx, store,
							communityapp.Caller{ContributorID: "validation-worker", Token: obs.ContributorRef, KeyID: receipt.KeyID},
							communityapp.PhotoCaptureUse{CaptureID: obs.PhotoCaptureID, StationID: obs.StationID, CapturedAt: obs.ClaimedCapturedAt, EvidenceSessionID: view.SessionID}, time.Now(), cfg.DevelopmentPhotoPreviewUntil)
						if errors.Is(err, communityapp.ErrPhotoCaptureIneligible) {
							return communityapp.ErrEvidenceInUse
						}
						if err != nil {
							return err
						}
						err = evidenceStore.TryBindPhotoObject(ctx, obs.EvidenceID, obs.PhotoCaptureID, obs.ContributorRef, time.Now())
						if errors.Is(err, evidenceadapters.ErrAlreadyBound) {
							return communityapp.ErrEvidenceInUse
						}
						return err
					},
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
					BeforeUpload: func(ctx context.Context, key, sanitizedHash string) (evidenceapp.FinalWrite, error) {
						nonce, err := jobs.NewUUIDv4()
						if err != nil {
							return evidenceapp.FinalWrite{}, err
						}
						return evidenceStore.BeginVerificationWrite(ctx, sess.ID, key, sanitizedHash, nonce)
					},
					Clock:    time.Now,
					Download: downloadSnapshot,
					Upload:   uploadFinal,
				}, sess)
			},
		},
		// Price recomputation (P06-T05): trigger observations resolve
		// their key, eligible votes assemble through ports, Compute
		// decides, and the versioned projection persists under the
		// price-key lock. This handler also drains the consensus jobs
		// parked since P04-T05: their observation_id payloads replay
		// through the same path.
		"community-consensus": communityjobs.Consensus{
			ByObservation: func(ctx context.Context, observationID string) (communitydomain.Result, error) {
				return communityapp.Recompute(ctx, recomputePorts, observationID)
			},
			ByKey: func(ctx context.Context, key communitydomain.PriceKey) (communitydomain.Result, error) {
				return communityapp.RecomputeKey(ctx, recomputePorts, key)
			},
		},
		"trust-affected-recompute": communityjobs.TrustAffected{
			Batch:    100,
			Affected: communityStore.AffectedContributorKeys,
			Recompute: func(ctx context.Context, key communitydomain.PriceKey) (communitydomain.Result, error) {
				return communityapp.RecomputeKey(ctx, recomputePorts, key)
			},
			Enqueue: func(ctx context.Context, kind string, payload []byte, dedupe string) error {
				_, err := jobs.Enqueue(ctx, raw, kind, payload, dedupe, 5, time.Time{})
				return err
			},
		},
		"consensus-boundary": communityjobs.Boundary{
			Clock: time.Now,
			Batch: 100,
			Due: func(ctx context.Context, now time.Time, batch int) ([]communitydomain.PriceKey, error) {
				return communityStore.DueProjections(ctx, now, batch)
			},
			Enqueue: func(ctx context.Context, kind string, payload []byte, dedupe string) error {
				_, err := jobs.Enqueue(ctx, raw, kind, payload, dedupe, 5, time.Time{})
				return err
			},
		},
		// Owner export assembly (P07-T03): durable requests build one
		// bounded owner-only archive over the real ledgers; terminal
		// replays converge and inventory failures mark FAILED.
		"privacy-export-build": privacyjobs.ExportBuild{
			Build: func(ctx context.Context, requestID string) (string, bool, error) {
				return privacyapp.Build(ctx, privacyPorts, requestID)
			},
		},
		// Owner erasure (P07-T04): durable deletion requests revoke and
		// purge scope by scope with ledger rows; terminal replays
		// converge and scope failures park for audited retry.
		"quality-report-reviewed": qualityjobs.ReportReviewed{Pool: raw},
		"quality-report-review": qualityjobs.ReportReview{Pool: raw, Open: func(ctx context.Context, tx pgx.Tx, id, kind string) error {
			caseID, err := jobs.NewUUIDv4()
			if err != nil {
				return err
			}
			c, _, err := moderationdomain.NewCase(moderationdomain.CaseParams{ID: caseID, TargetType: moderationdomain.TargetQualityReport, TargetID: id, Priority: moderationdomain.PriorityP2, Reason: kind, Detail: "private document-free quality report", OpenedAt: time.Now()})
			if err != nil {
				return err
			}
			_, _, err = modStore.OpenCaseTx(ctx, tx, c)
			return err
		}},
		"account-erasure-reconcile": accountjobs.Erasure{Reconcile: accountRecovery.Reconcile},
		"privacy-erasure": privacyjobs.Erasure{
			Load: func(ctx context.Context, requestID string) (privacydomain.Request, error) {
				return privacyStore.Get(ctx, requestID)
			},
			Erase: func(ctx context.Context, contributorID string, dto privacyapp.EraseDTO) (privacyapp.ErasureReport, error) {
				return privacyapp.Erase(ctx, privacyErasePorts, contributorID, dto)
			},
		},
		// Training-derivative promotion (CS10C): one offered image per
		// job through the fail-closed pipeline with hash-bound source
		// bytes. The quarantine source read arrives with the
		// client-review producer (CS11); until then the handler
		// refuses explicitly instead of misbehaving. Namespace writes
		// require configured storage, same as erasure deletes.
		"privacy-training-promote": privacyjobs.Promote{
			LoadSource: privacyadapters.TrainingSourceUnavailable,
			Propose: func(ctx context.Context, caller privacyapp.Caller, dto privacyapp.PromotionDTO) (privacyapp.PromotionResult, []byte, error) {
				return privacyapp.Propose(ctx, promotePorts, caller, dto)
			},
			Publish: func(ctx context.Context, result privacyapp.PromotionResult, minimized []byte) error {
				if trainingObjects == nil {
					return errors.New("training promotion: storage not configured")
				}
				return privacyapp.Publish(ctx, privacyapp.PublishPorts{Store: promotionStore, Objects: trainingObjects, Terms: trainingTerms, NewNonce: jobs.NewUUIDv4, Clock: time.Now}, result, minimized)
			},
		},
		// Training-copy purge: bounded every-minute disposal starts at
		// revocation; expired output leases are reconciled too. Receipts
		// stay for the restore filter, independently of collection being OFF.
		"privacy-training-reconcile": privacyjobs.TrainingReconcile{Run: promotionStore.ReconcileRevocations},
		"privacy-training-sweep": privacyjobs.Sweep{
			Run: func(ctx context.Context) (privacyapp.SweepReport, error) {
				return privacyapp.SweepRevoked(ctx, purgePorts)
			},
		},
		// Retention sweep (P07-T05): every inventory category purges
		// bounded oldest-first with per-category metrics; evidence
		// media keeps its own every-minute sweeper, and the 24-month
		// observation horizon reports metrics only.
		"privacy-retention": privacyjobs.Retention{
			Run: func(ctx context.Context) (privacyapp.RetentionReport, error) {
				_ = ctx
				return privacyapp.Retain(ctx, privacyapp.RetentionPorts{
					Clock:  time.Now,
					Purges: retentionPurges,
				})
			},
			Log: func(msg string, args ...any) {
				logger.Info(context.Background(), "retention.sweep", msg, args...)
			},
		},
		"evidence-sweep": evidencejobs.Sweep{
			BeginObservation:  evidenceStore.BeginSweepObservation,
			FinishObservation: evidenceStore.FinishSweepObservation,
			Run: func(ctx context.Context) (evidenceapp.SweepReport, error) {
				return evidenceapp.Sweep(ctx, evidenceapp.SweepDeps{
					Clock:   time.Now,
					Batch:   evidenceapp.DefaultSweepBatch,
					Store:   evidenceStore,
					Storage: sweepStoragePorts{deleteKey: erasureDelete},
				})
			},
		},
	}
	if cfg.EnterpriseExportEnabled {
		var afterBuild func(context.Context, enterprisejobs.State) error
		if cfg.EnterprisePortalRefreshEnabled {
			afterBuild = func(ctx context.Context, state enterprisejobs.State) error {
				_, err := enqueuePortalRefresh(ctx, enterprisejobs.RefreshPayload{
					Version: 1, Trigger: "after-build",
					ObservedAt: state.ObservedAt, ExportContentSHA256: state.ContentSHA256,
				}, portalRefreshDedupe(state))
				return err
			}
		}
		handlers[enterprisejobs.BuildKind] = enterprisejobs.Build{
			Pool: raw, Root: cfg.EnterpriseExportRoot,
			WindowWeeks: cfg.EnterpriseExportWindowWeeks,
			AfterBuild:  afterBuild,
			OfficialRecords: func(tx pgx.Tx) enterprisejobs.OfficialProvider {
				return enterprisejobs.NewQualityOfficialProvider(qualityread.NewExportReader(tx))
			},
		}
		handlers[enterprisejobs.ReconcileKind] = enterprisejobs.Reconcile{
			Pool: raw, Root: cfg.EnterpriseExportRoot,
			WindowWeeks: cfg.EnterpriseExportWindowWeeks,
			Enqueue:     enqueueEnterpriseExport,
			OfficialRecords: func(pool *pgxpool.Pool) enterprisejobs.OfficialProvider {
				return enterprisejobs.NewQualityOfficialProvider(qualityread.NewExportReader(pool))
			},
		}
		if cfg.EnterprisePortalRefreshEnabled {
			handlers[enterprisejobs.RefreshKind] = enterprisejobs.Refresh{
				ExportRoot: cfg.EnterpriseExportRoot, PortalRoot: cfg.EnterprisePortalRoot,
				SiteURL: cfg.EnterprisePortalSiteURL, Indexable: cfg.EnterprisePortalIndexable,
				OfficialOnly:     cfg.EnterprisePortalOfficialOnly,
				SearchScriptPath: cfg.EnterprisePortalSearchScript,
				Status:           qualitySourceStatus(raw),
			}
		}
	}
	nativeKinds := []string{}
	if cfg.EnterpriseExportEnabled {
		nativeKinds = append(nativeKinds, enterprisejobs.BuildKind)
	}
	if cfg.EnterprisePortalRefreshEnabled {
		nativeKinds = append(nativeKinds, enterprisejobs.RefreshKind)
	}
	if cfg.QualityImportsEnabled {

		if cfg.QualityEnforcementImportsEnabled {
			handlers[qualityjobs.EnforcementImportKind] = nativeEnforcementImport(raw, bundledEnforcementPreparer, "")
			handlers[qualityjobs.EnforcementDiscoveryKind] = nativeEnforcementLiveDiscovery(raw)
			nativeKinds = append(nativeKinds, qualityjobs.EnforcementImportKind, qualityjobs.EnforcementDiscoveryKind)
		}
		fineImport := nativeFineImport(raw, bundledFinePreparer, bundledFineHistoryPreparer, "")
		if cfg.EnterprisePortalRefreshEnabled {
			fineImport.AfterPublish = func(ctx context.Context, revisionID string) error {
				_, err := enqueueEnterpriseExport(ctx, enterprisejobs.BuildPayload{
					Version: 1, Trigger: "fine-publication", RevisionID: revisionID,
					ObservedAt: time.Now().UTC().Format(time.RFC3339),
				}, "enterprise-export:fine:"+revisionID)
				return err
			}
		}
		handlers[qualityjobs.NativeFineImportKind] = fineImport
		handlers[qualityjobs.NativeFineDiscoveryKind] = nativeFineLiveDiscovery(raw)
		handlers["quality-import"] = nativeQualityImport(raw, bundledQualityPreparer, "")
		handlers[qualityjobs.RevalidationKind] = nativeQualityRevalidate(raw)
		handlers["quality-discovery"] = nativeQualityLiveDiscovery(raw)
		handlers[qualityjobs.RecomputeKind] = qualityjobs.Recompute{Store: qualityadapters.NewProjectionStore(raw)}
		nativeKinds = append(nativeKinds, qualityjobs.NativeFineImportKind, qualityjobs.NativeFineDiscoveryKind, "quality-import", qualityjobs.RevalidationKind, "quality-discovery", qualityjobs.RecomputeKind)
	}
	if cfg.DriverEnabled {
		handlers[driverjobs.ReportCleanupKind] = driverjobs.ReportCleanup{
			Store: driveradapters.NewReportStore(raw), Retention: 400 * 24 * time.Hour, Limit: 1000,
		}
	}
	dispatchers, err := workerDispatchers(queue, handlers, nativeKinds...)
	if err != nil {
		return err
	}
	scheduler := jobs.NewScheduler(raw, []jobs.Schedule{
		{
			Name: "anp-discovery-daily", Kind: "anp-discovery", Version: 1,
			Interval: 24 * time.Hour, Enabled: cfg.ANPDiscoveryEnabled, Reason: "disabled by configuration",
			Build: func(period string) map[string]any { return map[string]any{"period": period} },
		},
		{
			Name: "enterprise-export-reconcile", Kind: enterprisejobs.ReconcileKind, Version: 1,
			Interval: 6 * time.Hour, Enabled: cfg.EnterpriseExportEnabled, Reason: "disabled by configuration",
			Build: func(period string) map[string]any {
				return map[string]any{"version": 1, "period": period}
			},
		},
		{
			Name: "enterprise-portal-refresh", Kind: enterprisejobs.RefreshKind, Version: 1,
			Interval: 15 * time.Minute, Enabled: cfg.EnterprisePortalRefreshEnabled, Reason: "disabled by configuration",
			Build: func(string) map[string]any {
				return map[string]any{"version": 1, "trigger": "reconcile"}
			},
		},
		{
			Name: "geocode-hourly", Kind: "geocode-station", Version: 1,
			Interval: time.Hour, Enabled: false, Reason: "D05 pending: no live provider",
			Build: func(period string) map[string]any { return map[string]any{"period": period} },
		},
		{
			Name: "registry-reconcile-daily", Kind: "registry-reconcile", Version: 1,
			Interval: 24 * time.Hour, Enabled: false, Reason: "P25 pending: no live source access yet; staging stays operator-triggered",
			Build: func(period string) map[string]any {
				return map[string]any{"version": 1, "source": "registry-csv", "snapshot": period}
			},
		},
		{
			Name: "registry-discover-daily", Kind: "registry-discover", Version: 1,
			Interval: 24 * time.Hour, Enabled: false, Reason: "P25 pending: no live source access yet; discovery stays operator-triggered",
			Build: func(period string) map[string]any {
				return map[string]any{"version": 1, "snapshot": "api:daily:" + period}
			},
		},
		{
			Name: "suggestion-verify-hourly", Kind: "suggestion-verify", Version: 1,
			Interval: time.Hour, Enabled: true, Reason: "",
			Build: func(string) map[string]any {
				return map[string]any{"version": 1, "batch": 25}
			},
		},
		{
			// Bounded PMQC discovery and Directory revalidation schedules
			// (FQ-05W8). Independent explicit switches; neither activates
			// unrelated ANP/registry imports. Handlers exist only with the
			// native imports switch, so queued jobs wait undrained without
			// charging attempts.
			Name: "quality-discovery-daily", Kind: "quality-discovery", Version: 1,
			Interval: 24 * time.Hour, Enabled: cfg.QualityDiscoveryEnabled, Reason: "disabled by configuration",
			Build: func(period string) map[string]any { return map[string]any{"period": period} },
		},
		{
			Name: "quality-revalidate-daily", Kind: qualityjobs.RevalidationKind, Version: 1,
			Interval: 24 * time.Hour, Enabled: cfg.QualityRevalidationEnabled, Reason: "disabled by configuration",
			Build: func(period string) map[string]any { return map[string]any{"period": period} },
		},
		{
			Name: "quality-sanctions-discovery-daily", Kind: qualityjobs.NativeFineDiscoveryKind, Version: 1,
			Interval: 24 * time.Hour, Enabled: cfg.QualitySanctionsDiscoveryEnabled, Reason: "disabled by configuration",
			Build: func(period string) map[string]any { return map[string]any{"period": period} },
		},

		{Name: "quality-enforcement-discovery-daily", Kind: qualityjobs.EnforcementDiscoveryKind, Version: 1, Interval: 24 * time.Hour, Enabled: cfg.QualityEnforcementDiscoveryEnabled, Reason: "disabled by configuration", Build: func(period string) map[string]any { return map[string]any{"period": period} }},
		evidenceSweepSchedule(),
		{Name: "account-erasure-every-minute", Kind: "account-erasure-reconcile", Version: 1, Interval: time.Minute, Enabled: true, Build: func(string) map[string]any { return map[string]any{"version": 1} }},
		{
			Name: "privacy-retention-daily", Kind: "privacy-retention", Version: 1,
			Interval: 24 * time.Hour, Enabled: true,
			Build: func(string) map[string]any { return map[string]any{"version": 1} },
		},
		{Name: "privacy-training-reconcile-every-minute", Kind: "privacy-training-reconcile", Version: 1, Interval: time.Minute, Enabled: true, Build: func(string) map[string]any { return map[string]any{"version": 1} }},
		{
			Name: "driver-report-snapshot-cleanup-daily", Kind: driverjobs.ReportCleanupKind, Version: 1,
			Interval: 24 * time.Hour, Enabled: cfg.DriverEnabled, Reason: "disabled by configuration",
			Build: func(string) map[string]any { return map[string]any{"version": 1} },
		},
		{
			Name: "privacy-training-sweep-every-minute", Kind: "privacy-training-sweep", Version: 1,
			Interval: time.Minute, Enabled: true,
			Build: func(string) map[string]any { return map[string]any{"version": 1} },
		},
	})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dispatchErr := make(chan error, 1)
	go func() {
		if err := runDispatchers(ctx, dispatchers); err != nil && !errors.Is(err, context.Canceled) {
			dispatchErr <- err
			return
		}
		dispatchErr <- nil
	}()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if premiumService.Configured() {
			if _, err := premiumService.Reconcile(ctx); err != nil {
				logger.Warn(ctx, "subscription.reconcile-unavailable", "bounded reconciliation unavailable")
			}
		}
		if err := premiumStore.CleanupReceipts(ctx); err != nil {
			logger.Warn(ctx, "subscription.cleanup-unavailable", "bounded cleanup unavailable")
		}
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
