package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	directoryread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/adapters/read"
	directoryapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/application"
	officialread "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/adapters/read"
	officialapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

// Reconcile is the lost-notification recovery path. It compares the current
// official window and Directory fingerprint with the durable state and enqueues
// one bounded build only when the state is stale or incomplete.
type Reconcile struct {
	Pool            *pgxpool.Pool
	Root            string
	WindowWeeks     int
	PageSize        int
	MaxRevisions    int
	Enqueue         func(ctx context.Context, payload BuildPayload, dedupe string) (platformjobs.EnqueueResult, error)
	OfficialRecords func(pool *pgxpool.Pool) OfficialProvider
}

func (Reconcile) Kind() string { return ReconcileKind }
func (Reconcile) Version() int { return 1 }

type reconcilePayload struct {
	Version int    `json:"version"`
	Period  string `json:"period"`
}

func (r Reconcile) Handle(ctx context.Context, job platformjobs.Job) error {
	if r.Pool == nil || r.Root == "" || r.Enqueue == nil {
		return fmt.Errorf("enterprise export: pool, root and enqueue are required")
	}
	period := time.Now().UTC().Format("20060102T15")
	if len(job.Payload) > 0 {
		var payload reconcilePayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("enterprise export: bad reconcile payload: %w", err)
		}
		if payload.Version != 1 {
			return fmt.Errorf("enterprise export: unsupported reconcile payload version")
		}
		if payload.Period != "" {
			period = payload.Period
		}
	}
	officialReader := officialread.NewExportReader(r.Pool)
	latest, err := officialReader.LatestRevision(ctx)
	if errors.Is(err, officialapp.ErrNoPublishedRevision) {
		return nil
	}
	if err != nil {
		return err
	}
	weeks := normalizeWindowWeeks(r.WindowWeeks)
	fromSurvey, toSurvey, err := windowFor(latest, weeks)
	if err != nil {
		return err
	}
	pageSize := r.PageSize
	if pageSize <= 0 || pageSize > defaultPageSize {
		pageSize = defaultPageSize
	}
	maxRevisions := r.MaxRevisions
	if maxRevisions <= 0 {
		maxRevisions = defaultMaxRevisions
	}
	pins, err := collectPins(ctx, officialReader, fromSurvey, toSurvey, pageSize, maxRevisions)
	if err != nil {
		return err
	}
	fingerprint, err := directoryread.NewExportReader(r.Pool).Fingerprint(ctx)
	if err != nil {
		return err
	}
	var currentOfficial *OfficialPin
	if r.OfficialRecords != nil {
		currentOfficial, err = r.OfficialRecords(r.Pool).Pin(ctx)
		if err != nil {
			return err
		}
	}
	state, hasState := LoadState(r.Root)
	if hasState && stateMatches(state, fingerprint, pins, fromSurvey, toSurvey, currentOfficial) && state.Valid() {
		return nil
	}
	_, err = r.Enqueue(ctx, BuildPayload{
		Version: 1, Trigger: "reconcile", RevisionID: latest.ID,
	}, "enterprise-export:reconcile:"+period+":"+exportStateKey(fingerprint, pins, currentOfficial))
	return err
}

func exportStateKey(fingerprint directoryapp.ExportFingerprint, pins []RevisionState, official *OfficialPin) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(fmt.Sprintf("%s|%d|", fingerprint.Hash, fingerprint.Stations)))
	for _, pin := range pins {
		_, _ = hash.Write([]byte(pin.RevisionID + "|" + pin.SourceChecksum + "|"))
	}
	if official != nil {
		_, _ = hash.Write([]byte(official.RevisionID + "|" + official.LeaseID + "|" + official.Edition + "|"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
