package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const StationFactReprojectionKind = "station-facts-reproject"
const zeroStationFactCursor = "00000000-0000-0000-0000-000000000000"

type StationFactReprojection struct {
	Pool      *pgxpool.Pool
	BatchSize int
}
type stationFactReprojectionPayload struct {
	Version    int    `json:"version"`
	CampaignID string `json:"campaign_id"`
	AfterID    string `json:"after_id,omitempty"`
	StationID  string `json:"station_id,omitempty"`
}

func (StationFactReprojection) Kind() string { return StationFactReprojectionKind }
func (StationFactReprojection) Version() int { return 1 }

func factCampaignPrefix(campaign, mode string) string {
	return StationFactReprojectionKind + ":" + mode + ":" + campaign + ":"
}
func enqueueStationFactReprojection(ctx context.Context, tx pgx.Tx, payload stationFactReprojectionPayload) (platformjobs.EnqueueResult, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return platformjobs.EnqueueResult{}, err
	}
	mode, key := "scan", payload.AfterID
	if payload.StationID != "" {
		mode, key = "station", payload.StationID
	}
	return platformjobs.Enqueue(ctx, tx, StationFactReprojectionKind, raw, factCampaignPrefix(payload.CampaignID, mode)+key, 5, time.Now())
}

// StartStationFactReprojection audits and enqueues an explicit request only.
// It does not perform any source fetch or synchronous station backfill.
func StartStationFactReprojection(ctx context.Context, pool *pgxpool.Pool, campaign, operator, reason string) (platformjobs.EnqueueResult, error) {
	uid, err := mustUUID(campaign)
	if err != nil {
		return platformjobs.EnqueueResult{}, err
	}
	campaign = uuidString(uid)
	if strings.TrimSpace(operator) == "" || len(operator) > 200 || strings.TrimSpace(reason) == "" || len(reason) > 500 {
		return platformjobs.EnqueueResult{}, errors.New("directory: operator and bounded reason required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return platformjobs.EnqueueResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := directory.New(tx)
	_, err = q.CreateStationFactReprojectionCampaign(ctx, directory.CreateStationFactReprojectionCampaignParams{CampaignID: uid, OperatorRef: operator, Reason: reason})
	if errors.Is(err, pgx.ErrNoRows) {
		prior, getErr := q.GetStationFactReprojectionCampaign(ctx, uid)
		if getErr != nil {
			return platformjobs.EnqueueResult{}, getErr
		}
		if prior.OperatorRef != operator || prior.Reason != reason {
			return platformjobs.EnqueueResult{}, errors.New("directory: campaign request is immutable")
		}
	} else if err != nil {
		return platformjobs.EnqueueResult{}, err
	}
	result, err := enqueueStationFactReprojection(ctx, tx, stationFactReprojectionPayload{Version: 1, CampaignID: campaign, AfterID: zeroStationFactCursor})
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func (h StationFactReprojection) Handle(ctx context.Context, job platformjobs.Job) error {
	if len(job.Payload) > 2048 {
		return errors.New("directory: oversized re-projection envelope")
	}
	var payload stationFactReprojectionPayload
	decoder := json.NewDecoder(bytes.NewReader(job.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("directory: trailing re-projection payload")
	}
	campaign, err := mustUUID(payload.CampaignID)
	if err != nil || payload.Version != 1 {
		return errors.New("directory: invalid re-projection campaign/version")
	}
	payload.CampaignID = uuidString(campaign)
	if payload.StationID != "" {
		station, err := mustUUID(payload.StationID)
		if err != nil || payload.AfterID != "" {
			return errors.New("directory: invalid station re-projection envelope")
		}
		payload.StationID = uuidString(station)
	} else {
		if payload.AfterID == "" {
			payload.AfterID = zeroStationFactCursor
		}
		cursor, err := mustUUID(payload.AfterID)
		if err != nil {
			return err
		}
		payload.AfterID = uuidString(cursor)
	}
	batch := h.BatchSize
	if batch == 0 {
		batch = 100
	}
	if batch < 1 || batch > 100 {
		return errors.New("directory: re-projection batch must be 1..100")
	}
	if h.Pool == nil {
		return errors.New("directory: re-projection pool required")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := directory.New(h.Pool).GetStationFactReprojectionCampaign(ctx, campaign); err != nil {
		return err
	}
	if payload.StationID != "" {
		return (RegistryCanonicalizer{Repo: NewRepository(h.Pool)}).ProjectLatestSourceFacts(ctx, payload.StationID)
	}
	tx, err := h.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := directory.New(tx)
	cursor, _ := mustUUID(payload.AfterID)
	page, err := q.ListStationFactReprojectionPage(ctx, directory.ListStationFactReprojectionPageParams{AfterID: cursor, PageLimit: int32(batch)})
	if err != nil {
		return err
	}
	for _, station := range page {
		if _, err := enqueueStationFactReprojection(ctx, tx, stationFactReprojectionPayload{Version: 1, CampaignID: payload.CampaignID, StationID: uuidString(station)}); err != nil {
			return err
		}
	}
	if len(page) == batch {
		if _, err := enqueueStationFactReprojection(ctx, tx, stationFactReprojectionPayload{Version: 1, CampaignID: payload.CampaignID, AfterID: uuidString(page[len(page)-1])}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type StationFactReprojectionStatus struct {
	CampaignID string           `json:"campaign_id"`
	Scan       map[string]int64 `json:"scan_jobs"`
	Stations   map[string]int64 `json:"station_jobs"`
	Complete   bool             `json:"complete"`
}

func ReadStationFactReprojectionStatus(ctx context.Context, pool *pgxpool.Pool, campaign string) (StationFactReprojectionStatus, error) {
	uid, err := mustUUID(campaign)
	if err != nil {
		return StationFactReprojectionStatus{}, err
	}
	campaign = uuidString(uid)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return StationFactReprojectionStatus{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := directory.New(tx).GetStationFactReprojectionCampaign(ctx, uid); err != nil {
		return StationFactReprojectionStatus{}, err
	}
	scan, err := platformjobs.DedupePrefixStats(ctx, tx, factCampaignPrefix(campaign, "scan"))
	if err != nil {
		return StationFactReprojectionStatus{}, err
	}
	stations, err := platformjobs.DedupePrefixStats(ctx, tx, factCampaignPrefix(campaign, "station"))
	if err != nil {
		return StationFactReprojectionStatus{}, err
	}
	complete := scan["done"] > 0 && scan["queued"] == 0 && scan["leased"] == 0 && scan["dead"] == 0 && stations["queued"] == 0 && stations["leased"] == 0 && stations["dead"] == 0
	status := StationFactReprojectionStatus{CampaignID: campaign, Scan: scan, Stations: stations, Complete: complete}
	return status, tx.Commit(ctx)
}

var _ platformjobs.Handler = StationFactReprojection{}
