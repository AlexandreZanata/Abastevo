//go:build integration

package adapters

import (
	"context"
	"errors"
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
