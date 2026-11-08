package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type mockIngestionStore struct {
	sources     map[string]*domain.Source
	checkpoints map[string]application.SourceCheckpoint
	runs        []application.SourceRun
	posts       []*domain.Post
	extIDs      map[string]bool
	urlHashes   map[string]bool
}

func newMockIngestionStore() *mockIngestionStore {
	return &mockIngestionStore{
		sources:     make(map[string]*domain.Source),
		checkpoints: make(map[string]application.SourceCheckpoint),
		runs:        nil,
		posts:       nil,
		extIDs:      make(map[string]bool),
		urlHashes:   make(map[string]bool),
	}
}

func (m *mockIngestionStore) GetSourceByID(ctx context.Context, id string) (*domain.Source, error) {
	s, ok := m.sources[id]
	if !ok {
		return nil, domain.ErrMissingSource
	}
	return s, nil
}

func (m *mockIngestionStore) ListEnabledSources(ctx context.Context) ([]domain.Source, error) {
	var list []domain.Source
	for _, s := range m.sources {
		if s.CollectionEnabled {
			list = append(list, *s)
		}
	}
	return list, nil
}

func (m *mockIngestionStore) GetSourceCheckpoint(ctx context.Context, sourceID string) (application.SourceCheckpoint, error) {
	cp, ok := m.checkpoints[sourceID]
	if !ok {
		return application.SourceCheckpoint{SourceID: sourceID}, nil
	}
	return cp, nil
}

func (m *mockIngestionStore) UpsertSourceCheckpoint(ctx context.Context, cp application.SourceCheckpoint) error {
	m.checkpoints[cp.SourceID] = cp
	return nil
}

func (m *mockIngestionStore) RecordSourceRun(ctx context.Context, run application.SourceRun) error {
	m.runs = append(m.runs, run)
	return nil
}

func (m *mockIngestionStore) HasPostByExternalID(ctx context.Context, sourceID, externalID string) (bool, error) {
	return m.extIDs[sourceID+":"+externalID], nil
}

func (m *mockIngestionStore) HasPostByUrlHash(ctx context.Context, sourceID, urlHash string) (bool, error) {
	return m.urlHashes[sourceID+":"+urlHash], nil
}

func (m *mockIngestionStore) InsertNewsCandidatePost(ctx context.Context, post *domain.Post) error {
	if post.PublicationState == domain.StatePublished {
		return errors.New("candidate news cannot be directly published")
	}
	if post.FirstPublicationSeq != 0 || post.FirstPublishedAt != nil {
		return errors.New("candidate news cannot have publication sequence or timestamp")
	}
	m.posts = append(m.posts, post)
	if post.ExternalID != "" {
		m.extIDs[post.SourceID+":"+post.ExternalID] = true
	}
	if post.CanonicalURL != "" {
		m.urlHashes[post.SourceID+":"+sha256Hex(post.CanonicalURL)] = true
	}
	return nil
}

type mockFetcher struct {
	fetchFunc func(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error)
	called    bool
}

func (m *mockFetcher) Fetch(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error) {
	m.called = true
	if m.fetchFunc != nil {
		return m.fetchFunc(ctx, req)
	}
	return application.FetchResponse{}, nil
}

type mockParser struct {
	parseFunc func(raw []byte, now time.Time) ([]application.FeedItem, error)
}

func (m *mockParser) Parse(raw []byte, now time.Time) ([]application.FeedItem, error) {
	if m.parseFunc != nil {
		return m.parseFunc(raw, now)
	}
	return nil, nil
}

func TestForumSourceFetch_HandlerIdentityAndPayload(t *testing.T) {
	h := ForumSourceFetchHandler{}
	if h.Kind() != ForumSourceFetchKind || h.Version() != 1 {
		t.Fatalf("unexpected kind/version: %s v%d", h.Kind(), h.Version())
	}

	ctx := context.Background()

	// 1. Oversized payload
	oversized := make([]byte, 5000)
	err := h.Handle(ctx, platformjobs.Job{Kind: h.Kind(), Payload: oversized})
	if err == nil || !strings.Contains(err.Error(), "oversized") {
		t.Fatalf("expected oversized error, got %v", err)
	}

	// 2. Unsupported payload version
	err = h.Handle(ctx, platformjobs.Job{Kind: h.Kind(), Payload: []byte(`{"version": 99}`)})
	if err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("expected unsupported version error, got %v", err)
	}

	// 3. Malformed payload
	err = h.Handle(ctx, platformjobs.Job{Kind: h.Kind(), Payload: []byte(`{bad json`)})
	if err == nil || !strings.Contains(err.Error(), "invalid payload") {
		t.Fatalf("expected invalid payload error, got %v", err)
	}
}

func TestForumSourceFetch_ScheduleDefaults(t *testing.T) {
	sch := NewForumSourceFetchSchedule()
	if sch.Kind != ForumSourceFetchKind || sch.Version != 1 {
		t.Fatalf("unexpected schedule identity: %s v%d", sch.Kind, sch.Version)
	}
	if sch.Interval != time.Hour {
		t.Fatalf("expected 1h interval, got %v", sch.Interval)
	}
	if sch.Enabled {
		t.Fatalf("schedule must be disabled by default per ADR-019 / NEWS_SOURCE_BLOCKED")
	}
	body := sch.Build("2026-11-05T12")
	if body["period"] != "2026-11-05T12" {
		t.Fatalf("unexpected build payload: %+v", body)
	}
}

func TestForumSourceFetch_BlockedSource_DoesNotAttemptEgress(t *testing.T) {
	store := newMockIngestionStore()
	const srcID = "src-denied-001"
	store.sources[srcID] = &domain.Source{
		ID:                srcID,
		Code:              "ebc-agenciabrasil",
		Name:              "Agência Brasil",
		FetchURL:          "https://agenciabrasil.ebc.com.br/rss",
		CollectionEnabled: true,
		RightsStatus:      domain.RightsDenied, // BLOCKED per ADR-019
	}

	fetcher := &mockFetcher{}
	runner := &SourceFetchRunner{
		Store:   store,
		Fetcher: fetcher,
		Parser:  &mockParser{},
		Now:     func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) },
	}

	reports, err := runner.Run(context.Background(), FetchJobPayload{Version: 1, SourceID: srcID})
	if err != nil {
		t.Fatalf("unexpected error for blocked source: %v", err)
	}
	if fetcher.called {
		t.Fatalf("network egress must NOT be called for blocked source")
	}
	if len(reports) != 1 || reports[0].Status != "BLOCKED" {
		t.Fatalf("expected BLOCKED status report, got %+v", reports)
	}
	if len(store.runs) != 1 || store.runs[0].Status != "BLOCKED" {
		t.Fatalf("expected BLOCKED run audit record, got %+v", store.runs)
	}
}

func TestForumSourceFetch_ConditionalGET_304NotModified(t *testing.T) {
	store := newMockIngestionStore()
	const srcID = "src-app-001"
	store.sources[srcID] = &domain.Source{
		ID:                srcID,
		Code:              "approved-source",
		FetchURL:          "https://example.com/feed.xml",
		CollectionEnabled: true,
		RightsStatus:      domain.RightsApproved,
	}
	store.checkpoints[srcID] = application.SourceCheckpoint{
		SourceID:     srcID,
		ETag:         `"prev-etag"`,
		LastModified: "Thu, 05 Nov 2026 10:00:00 GMT",
	}

	fetcher := &mockFetcher{
		fetchFunc: func(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error) {
			if req.ETag != `"prev-etag"` {
				t.Errorf("expected ETag in fetch request, got %s", req.ETag)
			}
			return application.FetchResponse{
				NotModified:  true,
				StatusCode:   304,
				ETag:         `"prev-etag"`,
				LastModified: req.LastModified,
			}, nil
		},
	}

	runner := &SourceFetchRunner{
		Store:   store,
		Fetcher: fetcher,
		Parser:  &mockParser{},
		Now:     func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) },
	}

	reports, err := runner.Run(context.Background(), FetchJobPayload{Version: 1, SourceID: srcID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reports) != 1 || reports[0].Status != "NOT_MODIFIED" {
		t.Fatalf("expected NOT_MODIFIED report, got %+v", reports)
	}
	if len(store.posts) != 0 {
		t.Fatalf("no posts should be ingested on 304, got %d", len(store.posts))
	}

	cp := store.checkpoints[srcID]
	if cp.LastStatus != "NOT_MODIFIED" || cp.ConsecutiveFailures != 0 {
		t.Fatalf("unexpected checkpoint after 304: %+v", cp)
	}
}

func TestForumSourceFetch_Deduplication_GUID_and_URL(t *testing.T) {
	store := newMockIngestionStore()
	const srcID = "src-app-002"
	store.sources[srcID] = &domain.Source{
		ID:                srcID,
		Code:              "approved-source-dedup",
		FetchURL:          "https://example.com/feed.xml",
		CollectionEnabled: true,
		RightsStatus:      domain.RightsApproved,
	}

	pubTime := time.Date(2026, 11, 5, 11, 0, 0, 0, time.UTC)

	feedItems := []application.FeedItem{
		{
			GUID:        "guid-1",
			Title:       "Preço da gasolina recua nos postos",
			Link:        "https://example.com/noticia-1",
			Excerpt:     "Pesquisa da ANP registra queda.",
			PublishedAt: pubTime,
		},
		{
			GUID:        "guid-2",
			Title:       "Novos eletropostos para veículos elétricos",
			Link:        "https://example.com/noticia-2",
			Excerpt:     "Corredor de recarga ampliado.",
			PublishedAt: pubTime,
		},
	}

	fetcher := &mockFetcher{
		fetchFunc: func(ctx context.Context, req application.FetchRequest) (application.FetchResponse, error) {
			return application.FetchResponse{
				NotModified: false,
				StatusCode:  200,
				ETag:        `"new-etag-1"`,
				Body:        []byte("<rss/>"),
			}, nil
		},
	}

	parser := &mockParser{
		parseFunc: func(raw []byte, now time.Time) ([]application.FeedItem, error) {
			return feedItems, nil
		},
	}

	runner := &SourceFetchRunner{
		Store:   store,
		Fetcher: fetcher,
		Parser:  parser,
		Now:     func() time.Time { return time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC) },
	}

	// First run: both items ingested as DRAFT
	ctx := context.Background()
	reports, err := runner.Run(ctx, FetchJobPayload{Version: 1, SourceID: srcID})
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if reports[0].ItemsDiscovered != 2 || reports[0].ItemsIngested != 2 {
		t.Fatalf("expected 2 discovered, 2 ingested, got %+v", reports[0])
	}
	if len(store.posts) != 2 {
		t.Fatalf("expected 2 stored posts, got %d", len(store.posts))
	}

	for _, p := range store.posts {
		if p.PublicationState != domain.StateDraft {
			t.Errorf("post %s must be DRAFT, got %s", p.ID, p.PublicationState)
		}
		if p.FirstPublicationSeq != 0 || p.FirstPublishedAt != nil {
			t.Errorf("post %s must not have publication sequence/timestamp", p.ID)
		}
	}

	// Second run: both items already present -> 0 newly ingested
	reports2, err := runner.Run(ctx, FetchJobPayload{Version: 1, SourceID: srcID})
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if reports2[0].ItemsDiscovered != 2 || reports2[0].ItemsIngested != 0 {
		t.Fatalf("expected 2 discovered, 0 ingested on duplicate pass, got %+v", reports2[0])
	}
	if len(store.posts) != 2 {
		t.Fatalf("posts count must remain 2 after duplicate run, got %d", len(store.posts))
	}
}
