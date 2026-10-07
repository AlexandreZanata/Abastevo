//go:build integration

package adapters

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/domain"
	"github.com/jackc/pgx/v5"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
)

func TestPhotoCapturePostGISOwnershipReplayAndConcurrency(t *testing.T) {
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	seedPreciseStation(t, ctx, pool, e2eStationA, e2eLonA, e2eLatA)
	now := time.Now().UTC().Truncate(time.Second)
	p := e2eSubmitPorts(s, pool, now)
	caller := e2eCaller("capture-owner")
	caller.KeyID = "key-one"
	in := app.PhotoCaptureIntent{ClientCaptureID: "capture-one", StationID: e2eStationA, Location: e2eLocationDTO("", e2eStationA, now, e2eLatA, e2eLonA).Location}
	receipt, _, err := app.AuthorizePhotoCapture(ctx, p, s, caller, in.ClientCaptureID, []byte("synthetic capture"), in)
	if err != nil {
		t.Fatal(err)
	}
	p.Clock = func() time.Time { return now.Add(time.Minute) }
	retry, _, err := app.AuthorizePhotoCapture(ctx, p, s, caller, in.ClientCaptureID, []byte("synthetic capture"), in)
	if err != nil || retry.ID != receipt.ID || !retry.ExpiresAt.Equal(receipt.ExpiresAt) {
		t.Fatalf("retry changed receipt: %v", err)
	}
	if _, err = s.PhotoCapture(ctx, receipt.ID, "another-owner"); !errors.Is(err, app.ErrPhotoCaptureIneligible) {
		t.Fatalf("owner disclosure: %v", err)
	}
	if err = s.BindPhotoCapture(ctx, receipt.ID, caller.Token, "other-key", e2eStationB, now.Add(time.Second), now); !errors.Is(err, app.ErrPhotoCaptureIneligible) {
		t.Fatal("wrong key accepted")
	}
	if err = s.BindPhotoCapture(ctx, receipt.ID, caller.Token, caller.KeyID, e2eStationB, now.Add(2*time.Minute), now); !errors.Is(err, app.ErrPhotoCaptureIneligible) {
		t.Fatal("camera deadline accepted")
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for _, session := range []string{e2eStationA, e2eStationB} {
		wg.Add(1)
		go func(session string) {
			defer wg.Done()
			if err := s.BindPhotoCapture(ctx, receipt.ID, caller.Token, caller.KeyID, session, now.Add(time.Second), now); err == nil {
				winners.Add(1)
			} else if !errors.Is(err, app.ErrPhotoCaptureIneligible) {
				t.Error(err)
			}
		}(session)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("concurrent winners=%d", winners.Load())
	}
	bound, err := s.PhotoCapture(ctx, receipt.ID, caller.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindPhotoCapture(ctx, receipt.ID, caller.Token, caller.KeyID, bound.EvidenceSessionID, now.Add(time.Second), now); err != nil {
		t.Fatalf("same binding retry: %v", err)
	}
	if err = s.BindPhotoCapture(ctx, receipt.ID, caller.Token, caller.KeyID, bound.EvidenceSessionID, now.Add(time.Second), receipt.ExpiresAt); !errors.Is(err, app.ErrPhotoCaptureIneligible) {
		t.Fatal("expired receipt accepted")
	}
	p.Clock = func() time.Time { return receipt.CameraExpiresAt }
	if _, _, err = app.AuthorizePhotoCapture(ctx, p, s, caller, in.ClientCaptureID, []byte("synthetic capture"), in); !errors.Is(err, app.ErrPhotoCaptureIneligible) {
		t.Fatal("retry renewed expired permission")
	}
	in.ClientCaptureID = "outside"
	in.Location = e2eLocationDTO("", e2eStationA, now, e2eLatB, e2eLonB).Location
	p.Clock = func() time.Time { return now }
	if _, _, err = app.AuthorizePhotoCapture(ctx, p, s, caller, in.ClientCaptureID, []byte("synthetic outside"), in); !errors.Is(err, app.ErrPhotoCaptureIneligible) {
		t.Fatal("real PostGIS outside accepted")
	}
	// The append-only migration is replay safe through the migration ledger.
	if _, err = migrate.Apply(ctx, pool.Config().ConnConfig.ConnString(), migrations.Files); err != nil {
		t.Fatalf("migration recovery: %v", err)
	}
}

func TestPhotoCaptureSessionCannotBindTwoReceipts(t *testing.T) {
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	seedPreciseStation(t, ctx, pool, e2eStationA, e2eLonA, e2eLatA)
	now := time.Now().UTC().Truncate(time.Second)
	ids := []string{e2eStationA, e2eStationB}
	for i, id := range ids {
		if _, err := s.InsertPhotoCapture(ctx, app.PhotoCapture{ID: id, ContributorRef: "owner", KeyID: "key", ClientCaptureID: []string{"one", "two"}[i], StationID: e2eStationA, IssuedAt: now, CameraExpiresAt: now.Add(2 * time.Minute), ExpiresAt: now.Add(24 * time.Hour), PolicyVersion: app.PhotoCapturePolicy}); err != nil {
			t.Fatal(err)
		}
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			err := s.BindPhotoCapture(ctx, id, "owner", "key", "bbbbbbbb-1111-4111-8111-000000000001", now.Add(time.Second), now.Add(time.Second))
			if err == nil {
				winners.Add(1)
			} else if !errors.Is(err, app.ErrPhotoCaptureIneligible) {
				t.Error(err)
			}
		}(id)
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("one session bound %d receipts", winners.Load())
	}
}

func TestPhotoSubsetStoresDistinctProductsAndRejectsDuplicateFacts(t *testing.T) {
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	station := "d6c74c23-63db-4c24-a2e5-408cb23bad26"
	// freshStore seeds the canonical test station used by existing store fixtures.
	capture := "c0000000-0000-4000-8000-000000000051"
	evidence := "e0000000-0000-4000-8000-000000000051"
	captured := time.Now().UTC().Truncate(time.Millisecond)
	obs := testObs(station, "stable-row-one")
	obs.PhotoCaptureID = capture
	obs.EvidenceID = evidence
	obs.ClaimedCapturedAt = captured
	var jobs atomic.Int32
	enqueue := func(context.Context, pgx.Tx, string, []byte, string) error { jobs.Add(1); return nil }
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, _, err := s.Submit(ctx, obs, enqueue); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if jobs.Load() != 1 {
		t.Fatalf("duplicate jobs=%d", jobs.Load())
	}
	changed := obs
	changed.ClientSubmissionID = "new-random-id"
	changed.ID = "d0000000-0000-4000-8000-000000000052"
	if _, _, err := s.Submit(ctx, changed, enqueue); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("new id duplicated capture product: %v", err)
	}
	changed = obs
	changed.ClaimedCapturedAt = captured.Add(time.Second)
	if _, _, err := s.Submit(ctx, changed, enqueue); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed capture time replay: %v", err)
	}
	second := obs
	second.ID = "d0000000-0000-4000-8000-000000000053"
	second.ClientSubmissionID = "stable-row-two"
	second.Product = "ETHANOL"
	second.AmountMilli = 3990
	if _, _, err := s.Submit(ctx, second, enqueue); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Observation(ctx, second.ID)
	if err != nil || loaded.PhotoCaptureID != capture || !loaded.ClaimedCapturedAt.Equal(captured) {
		t.Fatalf("loaded=%+v %v", loaded, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM community_observations WHERE photo_capture_id=$1", capture).Scan(&count); err != nil || count != 2 {
		t.Fatalf("subset count=%d err=%v", count, err)
	}
}

func TestPhotoCaptureMetadataExpiryIsBoundedAndErasureRemovesOwnedReceipts(t *testing.T) {
	s, pool, _ := freshStore(t)
	ctx := context.Background()
	seedPreciseStation(t, ctx, pool, e2eStationA, e2eLonA, e2eLatA)
	now := time.Now().UTC().Truncate(time.Second)
	for i, id := range []string{e2eStationA, e2eStationB} {
		if _, err := s.InsertPhotoCapture(ctx, app.PhotoCapture{ID: id, ContributorRef: "owner", KeyID: "key", ClientCaptureID: []string{"one", "two"}[i], StationID: e2eStationA, IssuedAt: now, CameraExpiresAt: now.Add(2 * time.Minute), ExpiresAt: now.Add(24 * time.Hour), PolicyVersion: app.PhotoCapturePolicy}); err != nil {
			t.Fatal(err)
		}
	}
	if count, err := s.PurgeExpiredPhotoCaptures(ctx, now.Add(24*time.Hour), 1); err != nil || count != 1 {
		t.Fatalf("bounded expiry=%d %v", count, err)
	}
	if _, _, _, err := s.EraseContributor(ctx, "owner", "anonymous", nil); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM community_photo_captures WHERE contributor_ref='owner'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("owned metadata=%d %v", count, err)
	}
}
