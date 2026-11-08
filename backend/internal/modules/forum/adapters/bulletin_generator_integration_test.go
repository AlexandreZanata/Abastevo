//go:build integration

package adapters

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/forum/domain"
	officialapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/official/application"
	platformjobs "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

type bulletinIntegrationReader struct {
	set officialapp.BulletinSet
}

func (r *bulletinIntegrationReader) ReadWeeklyBulletins(ctx context.Context, q officialapp.BulletinQuery) (officialapp.BulletinSet, error) {
	if q.Fuel == r.set.Query.Fuel && q.Unit == r.set.Query.Unit {
		return r.set, nil
	}
	return officialapp.BulletinSet{}, officialapp.ErrBulletinUnavailable
}

type bulletinIntegrationClock struct{ now time.Time }

func (c bulletinIntegrationClock) Now() time.Time { return c.now }

type bulletinIntegrationIDs struct{}

func (bulletinIntegrationIDs) NewID() string {
	id, _ := platformjobs.NewUUIDv4()
	return id
}

func bulletinIntegrationSet(currentRevision, currentPublished string, currentSum int64) officialapp.BulletinSet {
	revision := func(id, start, end, published string) officialapp.BulletinRevision {
		return officialapp.BulletinRevision{
			RevisionID: id, SurveyStart: start, SurveyEnd: end, PublishedAt: published,
			SourceURL: "https://www.gov.br/anp/synthetic.xlsx", SourceChecksum: "sha256:" + id,
			ParserVersion: "synthetic-v1",
		}
	}
	sample := func(sum int64) officialapp.BulletinSample {
		return officialapp.BulletinSample{
			UF: "SP", SumMilliBrl: sum, ObservationCount: 100,
			MeanNumerator: sum / 100, MeanDenominator: 1, Mean: fmt.Sprintf("%d", sum/100),
			Coverage: officialapp.BulletinCoverage{
				SourceObservations: 105, SampledStations: 100, SourceStations: 120,
			},
		}
	}
	return officialapp.BulletinSet{
		MethodologyVersion: officialapp.BulletinMethodologyVersion,
		Query: officialapp.BulletinQuery{
			MethodologyVersion: officialapp.BulletinMethodologyVersion,
			Fuel:               "GASOLINE_REGULAR", Unit: "L",
		},
		CurrentRevision:  revision(currentRevision, "2026-10-26", "2026-11-01", currentPublished),
		PreviousRevision: revision("22222222-2222-4222-8222-222222222222", "2026-10-19", "2026-10-25", "2026-10-27T12:00:00Z"),
		Pairs: []officialapp.BulletinPair{{
			UF: "SP", Fuel: "GASOLINE_REGULAR", Unit: "L",
			Current: sample(currentSum), Previous: sample(590000),
		}},
	}
}

func newBulletinIntegrationGenerator(store *Store, reader officialapp.BulletinSet, now time.Time) application.BulletinGenerator {
	return application.BulletinGenerator{
		Official: &bulletinIntegrationReader{set: reader},
		Store:    store,
		Clock:    bulletinIntegrationClock{now: now},
		IDs:      &bulletinIntegrationIDs{},
	}
}

func TestBulletinGenerator_ConcurrentRestartAndCorrection(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()
	set := bulletinIntegrationSet("11111111-1111-4111-8111-111111111111", "2026-11-02T12:00:00Z", 601800)

	const workers = 4
	sharedGenerator := newBulletinIntegrationGenerator(store, set, time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC))
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := sharedGenerator.Generate(ctx)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent generate: %v", err)
		}
	}
	var bulletins int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM forum_posts WHERE kind = 'BULLETIN'`).Scan(&bulletins); err != nil {
		t.Fatal(err)
	}
	if bulletins != 1 {
		t.Fatalf("concurrent generation created %d bulletins", bulletins)
	}

	// Restart/retry converges on the same key.
	generator := newBulletinIntegrationGenerator(store, set, time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC))
	report, err := generator.Generate(ctx)
	if err != nil || report.Created != 0 || report.Existing != 1 {
		t.Fatalf("restart report %+v, %v", report, err)
	}

	var originalID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM forum_posts WHERE kind = 'BULLETIN'`).Scan(&originalID); err != nil {
		t.Fatal(err)
	}
	post, err := store.GetPostByID(ctx, originalID)
	if err != nil {
		t.Fatal(err)
	}
	if post.PublicationState != domain.StatePendingReview || post.BulletinProvenance == nil ||
		post.BulletinKey == nil || post.BulletinGroupKey == nil {
		t.Fatalf("persisted bulletin %+v", post)
	}
	if post.BulletinProvenance.Current.SumMilliBrl != 601800 ||
		post.BulletinProvenance.Previous.SumMilliBrl != 590000 ||
		post.BulletinProvenance.Fuel != "GASOLINE_REGULAR" {
		t.Fatalf("persisted provenance %+v", post.BulletinProvenance)
	}

	// A newer accepted revision pair with the same windows is a correction.
	correction := bulletinIntegrationSet("33333333-3333-4333-8333-333333333333", "2026-11-04T12:00:00Z", 602000)
	correctedGenerator := newBulletinIntegrationGenerator(store, correction, time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC))
	report, err = correctedGenerator.Generate(ctx)
	if err != nil || report.Created != 1 {
		t.Fatalf("correction report %+v, %v", report, err)
	}
	var correctedID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM forum_posts WHERE kind = 'BULLETIN' AND id::text <> $1`, originalID).Scan(&correctedID); err != nil {
		t.Fatal(err)
	}
	corrected, err := store.GetPostByID(ctx, correctedID)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.SupersedesPostID == nil || *corrected.SupersedesPostID != originalID {
		t.Fatalf("correction link %+v", corrected.SupersedesPostID)
	}
}

func TestBulletinGenerator_NoInputAndMissingSource(t *testing.T) {
	store, pool, _ := freshForumStore(t)
	ctx := context.Background()
	generator := application.BulletinGenerator{
		Official: &bulletinIntegrationReader{},
		Store:    store,
		Clock:    bulletinIntegrationClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)},
		IDs:      &bulletinIntegrationIDs{},
	}
	report, err := generator.Generate(ctx)
	if err != nil || report.Created != 0 {
		t.Fatalf("no input: %+v %v", report, err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM forum_posts WHERE kind = 'BULLETIN'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("no-input post count %d, %v", count, err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM forum_sources WHERE code = 'abastevo-editorial'`); err != nil {
		t.Fatal(err)
	}
	_, err = newBulletinIntegrationGenerator(store, bulletinIntegrationSet(
		"11111111-1111-4111-8111-111111111111", "2026-11-02T12:00:00Z", 601800,
	), time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)).Generate(ctx)
	if !errors.Is(err, domain.ErrMissingSource) {
		t.Fatalf("expected missing source, got %v", err)
	}
}
