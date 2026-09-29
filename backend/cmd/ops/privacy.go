package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	communityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/adapters"
	evidenceadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters"
	evidencestorage "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/adapters/storage"
	identityadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/identity/adapters"
	privacyadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/adapters"
	privacyapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/application"
	trustadapters "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trust/adapters"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// eraseArgs carries one parsed erasure intake. Parsing stays pure (no
// I/O) so arg validation is unit-testable.
type eraseArgs struct {
	operator    string
	contributor string
	clientKey   string
	reason      string
}

func parseEraseArgs(args []string) (eraseArgs, error) {
	fs := flag.NewFlagSet("privacy erase", flag.ContinueOnError)
	var e eraseArgs
	fs.StringVar(&e.operator, "operator", "", "accountable operator identity (or ANPFUEL_OPERATOR_ID)")
	fs.StringVar(&e.contributor, "contributor", "", "contributor ID to erase")
	fs.StringVar(&e.clientKey, "client-key", "", "client operation identity for safe retries")
	fs.StringVar(&e.reason, "reason", "", "mandatory reviewed reason")
	if err := fs.Parse(args); err != nil {
		return eraseArgs{}, err
	}
	if strings.TrimSpace(e.contributor) == "" || strings.TrimSpace(e.reason) == "" {
		return eraseArgs{}, errors.New("privacy erase requires --contributor and --reason")
	}
	if strings.TrimSpace(e.clientKey) == "" {
		e.clientKey = "erase-" + strings.TrimSpace(e.contributor)
	}
	return e, nil
}

type replayArgs struct {
	operator    string
	contributor string
}

func parseReplayArgs(args []string) (replayArgs, error) {
	fs := flag.NewFlagSet("privacy replay", flag.ContinueOnError)
	var r replayArgs
	fs.StringVar(&r.operator, "operator", "", "accountable operator identity (or ANPFUEL_OPERATOR_ID)")
	fs.StringVar(&r.contributor, "contributor", "", "contributor ID whose ledger replays")
	if err := fs.Parse(args); err != nil {
		return replayArgs{}, err
	}
	if strings.TrimSpace(r.contributor) == "" {
		return replayArgs{}, errors.New("privacy replay requires --contributor")
	}
	return r, nil
}

func runPrivacy(args []string, getenv func(string) string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	switch args[0] {
	case "erase":
		parsed, err := parseEraseArgs(args[1:])
		if err != nil {
			return err
		}
		return privacyErase(parsed, getenv)
	case "replay":
		parsed, err := parseReplayArgs(args[1:])
		if err != nil {
			return err
		}
		return privacyReplay(parsed, getenv)
	default:
		return fmt.Errorf("unknown privacy command %q", args[0])
	}
}

// privacyErase records one erasure intent with its durable job
// (P07-T04): the worker executes scope by scope with ledger rows.
// The operator reason travels in the job payload for the ledger.
func privacyErase(e eraseArgs, getenv func(string) string) error {
	operator, err := resolveOperatorID(e.operator, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, _, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	raw := pool.Underlying()
	registrar := identityadapters.NewRegistrar(raw)
	store := privacyadapters.NewStore(raw)
	res, err := privacyapp.RequestDeletion(ctx, privacyapp.ErasePorts{
		Clock: time.Now,
		NewID: newUUID,
		Attribution: func(ctx context.Context, contributorID string) (string, error) {
			return registrar.AttributionToken(ctx, contributorID)
		},
		Store: store,
		EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			_, err := platformjobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
			return err
		},
	}, strings.TrimSpace(e.contributor), privacyapp.EraseDTO{
		ClientSubmissionID: strings.TrimSpace(e.clientKey), Reason: e.reason,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "operator=%s erasure-request=%s replayed=%v\n", operator, res.RequestID, res.Replayed)
	return nil
}

// privacyReplay reapplies one contributor's recorded removals, for
// restore drills and post-restore traffic gates (recovery runbook).
func privacyReplay(r replayArgs, getenv func(string) string) error {
	operator, err := resolveOperatorID(r.operator, getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, cfg, err := openPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	raw := pool.Underlying()
	registrar := identityadapters.NewRegistrar(raw)
	var deleteObject func(ctx context.Context, key, namespace string) error
	if cfg.R2 != nil {
		creds := evidencestorage.Credentials{AccessKeyID: cfg.R2.AccessKeyID, SecretAccessKey: cfg.R2.SecretAccessKey}
		endpoint, bucket, region := cfg.R2.Endpoint, cfg.R2.Bucket, cfg.R2.Region
		deleteObject = func(ctx context.Context, key, namespace string) error {
			pre, err := evidencestorage.PresignDELETE(evidencestorage.PresignInput{
				Endpoint: endpoint, Bucket: bucket, Key: key, Namespace: namespace,
				TTL: 5 * time.Minute, Region: region, Now: time.Now(),
			}, creds)
			if err != nil {
				return err
			}
			return evidencestorage.Delete(ctx, httpClientForOps(), pre.URL)
		}
	} else {
		fmt.Fprintln(os.Stderr, "warning: storage not configured; replay marks database rows, orphaned bytes need a worker pass")
	}
	report, err := privacyapp.ReplayLedger(ctx, privacyapp.ErasePorts{
		Clock: time.Now,
		NewID: newUUID,
		Attribution: func(ctx context.Context, contributorID string) (string, error) {
			return registrar.AttributionToken(ctx, contributorID)
		},
		Store:        privacyadapters.NewStore(raw),
		Identity:     registrar,
		Community:    communityadapters.NewStore(raw),
		Evidence:     evidenceadapters.NewStore(raw),
		Trust:        trustadapters.NewStore(raw),
		DeleteObject: deleteObject,
		EnqueueJob: func(ctx context.Context, tx pgx.Tx, kind string, payload []byte, dedupe string) error {
			_, err := platformjobs.Enqueue(ctx, tx, kind, payload, dedupe, 5, time.Time{})
			return err
		},
	}, strings.TrimSpace(r.contributor))
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "operator=%s replay contributor=%s obs=%d votes=%d keys=%d sessions=%d objects=%d trust=%d archives=%d\n",
		operator, strings.TrimSpace(r.contributor),
		report.ObsUnlinked, report.VotesUnlinked, report.KeysRecomputed,
		report.SessionsPurged, report.ObjectsPurged, report.TrustUnlinked, report.ArchivesPurged)
	return nil
}

// httpClientForOps bounds operator-side storage calls.
func httpClientForOps() *http.Client {
	return &http.Client{Timeout: time.Minute}
}
