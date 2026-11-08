package jobs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

const (
	ForumSourceFetchKind = "forum-source-fetch"
	MaxFetchAttempts     = 3
)

// FetchJobPayload represents the parameters passed in the job envelope.
type FetchJobPayload struct {
	Version  int    `json:"version"`
	SourceID string `json:"source_id,omitempty"`
	Force    bool   `json:"force,omitempty"`
}

// SourceFetchReport summarizes the outcome of one candidate news collection pass.
type SourceFetchReport struct {
	SourceID        string
	Status          string // 'SUCCESS', 'NOT_MODIFIED', 'FAILED', 'BLOCKED', 'SKIPPED'
	ItemsDiscovered int
	ItemsIngested   int
	ErrorMessage    string
}

// SourceFetchRunner coordinates fetching, parsing, classifying, and deduplicating
// news candidates from authorized sources.
type SourceFetchRunner struct {
	Store   application.IngestionStore
	Fetcher application.SourceFetcher
	Parser  application.FeedParser
	Now     func() time.Time
}

func (r SourceFetchRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

// Run executes candidate news collection for a specific source ID or all enabled sources.
func (r *SourceFetchRunner) Run(ctx context.Context, payload FetchJobPayload) ([]SourceFetchReport, error) {
	if r.Store == nil || r.Fetcher == nil || r.Parser == nil {
		return nil, fmt.Errorf("forum-source-fetch: runner dependencies not initialized")
	}

	var sources []domain.Source
	if payload.SourceID != "" {
		src, err := r.Store.GetSourceByID(ctx, payload.SourceID)
		if err != nil {
			return nil, fmt.Errorf("forum-source-fetch: get source %s: %w", payload.SourceID, err)
		}
		sources = []domain.Source{*src}
	} else {
		enabled, err := r.Store.ListEnabledSources(ctx)
		if err != nil {
			return nil, fmt.Errorf("forum-source-fetch: list enabled sources: %w", err)
		}
		sources = enabled
	}

	reports := make([]SourceFetchReport, 0, len(sources))
	var lastErr error

	for _, src := range sources {
		report, err := r.fetchOneSource(ctx, src, payload.Force)
		reports = append(reports, report)
		if err != nil {
			lastErr = err
		}
	}

	return reports, lastErr
}

func (r *SourceFetchRunner) fetchOneSource(ctx context.Context, src domain.Source, force bool) (SourceFetchReport, error) {
	now := r.now()

	// 1. Rights governance and enabled check
	if !force {
		if !src.CollectionEnabled {
			return SourceFetchReport{SourceID: src.ID, Status: "SKIPPED"}, nil
		}
		if src.RightsStatus != domain.RightsApproved {
			// Record run as BLOCKED per ADR-019 / NEWS_SOURCE_BLOCKED
			runID := newUUID()
			completed := now
			_ = r.Store.RecordSourceRun(ctx, application.SourceRun{
				ID:           runID,
				SourceID:     src.ID,
				Status:       "BLOCKED",
				ErrorMessage: "source rights not approved; automated egress prohibited",
				StartedAt:    now,
				CompletedAt:  &completed,
			})
			_ = r.Store.UpsertSourceCheckpoint(ctx, application.SourceCheckpoint{
				SourceID:   src.ID,
				LastStatus: "BLOCKED",
				UpdatedAt:  now,
			})
			return SourceFetchReport{
				SourceID:     src.ID,
				Status:       "BLOCKED",
				ErrorMessage: "source rights not approved; automated egress prohibited",
			}, nil
		}
		if src.ReviewExpiresAt != nil && src.ReviewExpiresAt.Before(now) {
			runID := newUUID()
			completed := now
			_ = r.Store.RecordSourceRun(ctx, application.SourceRun{
				ID:           runID,
				SourceID:     src.ID,
				Status:       "BLOCKED",
				ErrorMessage: "rights review period expired",
				StartedAt:    now,
				CompletedAt:  &completed,
			})
			return SourceFetchReport{
				SourceID:     src.ID,
				Status:       "BLOCKED",
				ErrorMessage: "rights review period expired",
			}, nil
		}
	}

	if src.FetchURL == "" {
		return SourceFetchReport{SourceID: src.ID, Status: "SKIPPED"}, nil
	}

	// 2. Read existing conditional GET checkpoint
	cp, err := r.Store.GetSourceCheckpoint(ctx, src.ID)
	if err != nil {
		return SourceFetchReport{SourceID: src.ID, Status: "FAILED", ErrorMessage: err.Error()}, err
	}

	runID := newUUID()
	run := application.SourceRun{
		ID:        runID,
		SourceID:  src.ID,
		Status:    "RUNNING",
		StartedAt: now,
	}

	// 3. Conditional fetch
	fetchReq := application.FetchRequest{
		SourceID:     src.ID,
		URL:          src.FetchURL,
		ETag:         cp.ETag,
		LastModified: cp.LastModified,
	}
	resp, fetchErr := r.Fetcher.Fetch(ctx, fetchReq)
	if fetchErr != nil {
		completed := r.now()
		run.Status = "FAILED"
		run.ErrorMessage = fetchErr.Error()
		run.CompletedAt = &completed
		_ = r.Store.RecordSourceRun(ctx, run)

		cp.ConsecutiveFailures++
		cp.LastStatus = "FAILED"
		cp.UpdatedAt = completed
		_ = r.Store.UpsertSourceCheckpoint(ctx, cp)

		return SourceFetchReport{
			SourceID:     src.ID,
			Status:       "FAILED",
			ErrorMessage: fetchErr.Error(),
		}, fetchErr
	}

	// 4. Handle 304 Not Modified
	if resp.NotModified {
		completed := r.now()
		run.Status = "NOT_MODIFIED"
		run.ETag = resp.ETag
		run.LastModified = resp.LastModified
		run.CompletedAt = &completed
		_ = r.Store.RecordSourceRun(ctx, run)

		cp.LastStatus = "NOT_MODIFIED"
		cp.LastRunID = &runID
		cp.LastSuccessAt = &completed
		cp.ConsecutiveFailures = 0
		cp.UpdatedAt = completed
		_ = r.Store.UpsertSourceCheckpoint(ctx, cp)

		return SourceFetchReport{
			SourceID: src.ID,
			Status:   "NOT_MODIFIED",
		}, nil
	}

	// 5. Parse feed items safely
	items, parseErr := r.Parser.Parse(resp.Body, now)
	if parseErr != nil {
		completed := r.now()
		run.Status = "FAILED"
		run.ErrorMessage = parseErr.Error()
		run.CompletedAt = &completed
		_ = r.Store.RecordSourceRun(ctx, run)

		cp.ConsecutiveFailures++
		cp.LastStatus = "FAILED"
		cp.UpdatedAt = completed
		_ = r.Store.UpsertSourceCheckpoint(ctx, cp)

		return SourceFetchReport{
			SourceID:     src.ID,
			Status:       "FAILED",
			ErrorMessage: parseErr.Error(),
		}, parseErr
	}

	// 6. Ingest and deduplicate items transactionally
	discovered := len(items)
	ingested := 0

	for _, item := range items {
		// Deduplicate by GUID / external_id
		if item.GUID != "" {
			exists, err := r.Store.HasPostByExternalID(ctx, src.ID, item.GUID)
			if err != nil {
				return SourceFetchReport{SourceID: src.ID, Status: "FAILED", ErrorMessage: err.Error()}, err
			}
			if exists {
				continue
			}
		}

		// Deduplicate by canonical URL
		if item.Link != "" {
			urlHash := sha256Hex(item.Link)
			exists, err := r.Store.HasPostByUrlHash(ctx, src.ID, urlHash)
			if err != nil {
				return SourceFetchReport{SourceID: src.ID, Status: "FAILED", ErrorMessage: err.Error()}, err
			}
			if exists {
				continue
			}
		}

		// Deterministic category classification
		category := ClassifyCategory(item.Title, item.Excerpt, item.Categories)

		// Create candidate post as DRAFT (never direct publication)
		postID := newUUID()
		postTime := item.PublishedAt
		if postTime.IsZero() {
			postTime = now
		}

		post := &domain.Post{
			ID:                  postID,
			Kind:                domain.KindNews,
			Category:            category,
			Title:               item.Title,
			Excerpt:             item.Excerpt,
			BodyText:            item.Excerpt, // candidates ingest excerpt; full body requires bilateral license
			SourceID:            src.ID,
			CanonicalURL:        item.Link,
			ExternalID:          item.GUID,
			SourcePublishedAt:   &postTime,
			LocalCreatedAt:      now,
			LocalUpdatedAt:      now,
			FirstPublicationSeq: 0,
			PublicationState:    domain.StateDraft,
			Visibility:          domain.VisibilityVisible,
			Revision:            1,
			CommentsEnabled:     true,
		}

		if err := r.Store.InsertNewsCandidatePost(ctx, post); err != nil {
			return SourceFetchReport{SourceID: src.ID, Status: "FAILED", ErrorMessage: err.Error()}, err
		}
		ingested++
	}

	// 7. Record successful run and checkpoint
	completed := r.now()
	run.Status = "SUCCESS"
	run.ETag = resp.ETag
	run.LastModified = resp.LastModified
	run.ItemsDiscovered = discovered
	run.ItemsIngested = ingested
	run.CompletedAt = &completed
	_ = r.Store.RecordSourceRun(ctx, run)

	cp.ETag = resp.ETag
	cp.LastModified = resp.LastModified
	cp.LastRunID = &runID
	cp.LastSuccessAt = &completed
	cp.LastStatus = "SUCCESS"
	cp.ConsecutiveFailures = 0
	cp.UpdatedAt = completed
	_ = r.Store.UpsertSourceCheckpoint(ctx, cp)

	return SourceFetchReport{
		SourceID:        src.ID,
		Status:          "SUCCESS",
		ItemsDiscovered: discovered,
		ItemsIngested:   ingested,
	}, nil
}

// ForumSourceFetchHandler implements jobs.Handler for scheduled news collection.
type ForumSourceFetchHandler struct {
	Runner SourceFetchRunner
}

func (ForumSourceFetchHandler) Kind() string { return ForumSourceFetchKind }
func (ForumSourceFetchHandler) Version() int { return 1 }

func (h ForumSourceFetchHandler) Handle(ctx context.Context, job platformjobs.Job) error {
	if len(job.Payload) > 4096 {
		return fmt.Errorf("forum-source-fetch: payload oversized")
	}
	var payload FetchJobPayload
	if len(job.Payload) > 0 {
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			return fmt.Errorf("forum-source-fetch: invalid payload: %w", err)
		}
	}
	if payload.Version == 0 {
		payload.Version = 1
	}
	if payload.Version != 1 {
		return fmt.Errorf("forum-source-fetch: unsupported version %d", payload.Version)
	}

	_, err := h.Runner.Run(ctx, payload)
	return err
}

// NewForumSourceFetchSchedule constructs the hourly schedule definition.
func NewForumSourceFetchSchedule() platformjobs.Schedule {
	return platformjobs.Schedule{
		Name:     "forum-source-fetch-hourly",
		Kind:     ForumSourceFetchKind,
		Version:  1,
		Interval: time.Hour,
		Enabled:  false, // Disabled by default per ADR-019 / NEWS_SOURCE_BLOCKED
		Reason:   "hourly candidate news ingestion (disabled until commercial bilateral license)",
		Build: func(period string) map[string]any {
			return map[string]any{
				"period": period,
			}
		},
	}
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
