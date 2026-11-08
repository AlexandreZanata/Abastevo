//go:build integration

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
)

func bulletinTestPost(id, key, group string, supersedes *string, now time.Time) *domain.Post {
	provenance := &domain.BulletinProvenance{
		MethodologyVersion: "forum-official-mean-v1",
		TemplateVersion:    "forum-bulletin-v1",
		Fuel:               "GASOLINE_REGULAR",
		Unit:               "L",
		Current: domain.BulletinRevisionProvenance{
			RevisionID: "11111111-1111-4111-8111-111111111111", SurveyStart: "2026-10-26",
			SurveyEnd: "2026-11-01", PublishedAt: "2026-11-02T12:00:00Z",
			SourceURL: "https://www.gov.br/anp/a.xlsx", SourceChecksum: "sha256:a",
			ParserVersion: "v1", SumMilliBrl: 601800, ObservationCount: 100,
			SampledStations: 100, SourceObservations: 105, SourceStations: 120,
			MeanNumerator: 6018, MeanDenominator: 1, Mean: "6018",
		},
		Previous: domain.BulletinRevisionProvenance{
			RevisionID: "22222222-2222-4222-8222-222222222222", SurveyStart: "2026-10-19",
			SurveyEnd: "2026-10-25", PublishedAt: "2026-10-27T12:00:00Z",
			SourceURL: "https://www.gov.br/anp/b.xlsx", SourceChecksum: "sha256:b",
			ParserVersion: "v1", SumMilliBrl: 590000, ObservationCount: 100,
			SampledStations: 100, SourceObservations: 100, SourceStations: 118,
			MeanNumerator: 5900, MeanDenominator: 1, Mean: "5900",
		},
	}
	return &domain.Post{
		ID: id, Kind: domain.KindBulletin, Category: domain.CategoryAbastevoData,
		Title: "Gasolina comum em SP: R$ 6,018/L", Excerpt: "Média amostral de 100 postos.",
		BodyText:         "Proveniência determinística.",
		SourceID:         "00000000-0000-0000-0000-000000000001",
		PublicationState: domain.StateDraft, Visibility: domain.VisibilityVisible, Revision: 1,
		Region: &domain.Region{UF: "SP"}, BulletinKey: &key, BulletinGroupKey: &group,
		BulletinProvenance: provenance, SupersedesPostID: supersedes,
		LocalCreatedAt: now, LocalUpdatedAt: now,
	}
}

func TestHandler_Integration_BulletinProvenanceAndCorrectionVisibility(t *testing.T) {
	store, _, router := freshForumTestEnv(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	originalID := "00000000-0000-4000-8000-000000000101"
	correctedID := "00000000-0000-4000-8000-000000000102"
	group := "forum-bulletin-v1:group-a"
	originalKey := "forum-bulletin-v1:original"
	correctedKey := "forum-bulletin-v1:corrected"

	original := bulletinTestPost(originalID, originalKey, group, nil, now)
	if err := store.CreateEditorialPost(ctx, original); err != nil {
		t.Fatalf("create original: %v", err)
	}
	if _, err := store.PublishEditorialPost(ctx, originalID, now); err != nil {
		t.Fatalf("publish original: %v", err)
	}

	getDetail := func(id string) (*httptest.ResponseRecorder, PostDetailResponseDTO) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts/"+id, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		var resp PostDetailResponseDTO
		if rec.Code == http.StatusOK {
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode detail: %v", err)
			}
		}
		return rec, resp
	}
	getSnapshot := func() string {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v1/forum/posts", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("feed: %d", rec.Code)
		}
		var feed FeedResponseDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
			t.Fatal(err)
		}
		return feed.Snapshot
	}

	rec, detail := getDetail(originalID)
	if rec.Code != http.StatusOK || detail.Post.Bulletin == nil ||
		detail.Post.Bulletin.MethodologyVersion != "forum-official-mean-v1" ||
		detail.Post.Bulletin.Current.SumMilliBrl != 601800 ||
		detail.Post.Bulletin.Previous.Mean != "5900" ||
		detail.Post.SupersedesPostID != nil {
		t.Fatalf("original detail %d %+v", rec.Code, detail.Post)
	}
	snapshotBefore := getSnapshot()

	corrected := bulletinTestPost(correctedID, correctedKey, group, &originalID, now.Add(time.Hour))
	if err := store.CreateEditorialPost(ctx, corrected); err != nil {
		t.Fatalf("create correction: %v", err)
	}
	if _, err := store.PublishEditorialPost(ctx, correctedID, now.Add(time.Hour)); err != nil {
		t.Fatalf("publish correction: %v", err)
	}
	if err := store.SetPostVisibility(ctx, originalID, domain.VisibilityHidden); err != nil {
		t.Fatalf("hide original: %v", err)
	}

	rec, _ = getDetail(originalID)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("superseded bulletin must be hidden, got %d", rec.Code)
	}
	rec, correctedDetail := getDetail(correctedID)
	if rec.Code != http.StatusOK || correctedDetail.Post.SupersedesPostID == nil ||
		*correctedDetail.Post.SupersedesPostID != originalID || correctedDetail.Post.Bulletin == nil {
		t.Fatalf("corrected detail %d %+v", rec.Code, correctedDetail.Post)
	}
	if snapshotAfter := getSnapshot(); snapshotAfter == snapshotBefore {
		t.Fatalf("feed snapshot did not change after correction: %s", snapshotAfter)
	}
}
