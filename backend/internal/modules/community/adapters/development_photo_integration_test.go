//go:build integration

package adapters

import (
	"context"
	"errors"
	migrations "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/migrations"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/migrate"
	"testing"
	"time"
)

func TestDevelopmentPhotoPreviewPostGISRecoveryAndPolicyIsolation(t *testing.T) {
	store, pool, _ := freshStore(t)
	ctx := context.Background()
	seedPreciseStation(t, ctx, pool, e2eStationA, e2eLonA, e2eLatA)
	now := time.Now().UTC().Truncate(time.Second)
	p := e2eSubmitPorts(store, pool, now)
	p.DevelopmentPhotoPreviewUntil = now.Add(time.Hour)
	p.CheckDevelopmentStation = func(context.Context, string) error { return nil }
	caller := e2eCaller("development-photo-owner")
	caller.KeyID = "test-key"
	in := app.PhotoCaptureIntent{ClientCaptureID: "development-photo", StationID: e2eStationA, DevelopmentPreview: true}
	got, _, err := app.AuthorizePhotoCapture(ctx, p, store, caller, in.ClientCaptureID, []byte("development-photo"), in)
	if err != nil {
		t.Fatal(err)
	}
	if got.PolicyVersion != app.DevelopmentPhotoCapturePolicy {
		t.Fatal("incorrect stored provenance")
	}
	if _, err = migrate.Apply(ctx, pool.Config().ConnConfig.ConnString(), migrations.Files); err != nil {
		t.Fatal("recovery", err)
	}
	again, _, err := app.AuthorizePhotoCapture(ctx, p, store, caller, in.ClientCaptureID, []byte("development-photo"), in)
	if err != nil || again.ID != got.ID {
		t.Fatal("replay", err)
	}
	use := app.PhotoCaptureUse{CaptureID: got.ID, StationID: e2eStationA, CapturedAt: now.Add(time.Second)}
	if _, err = app.ValidatePhotoCaptureUse(ctx, store, caller, use, now.Add(time.Second)); !errors.Is(err, app.ErrPhotoCaptureIneligible) {
		t.Fatal("production policy accepted preview")
	}
	if _, err = app.ValidatePhotoCaptureUse(ctx, store, caller, use, now.Add(time.Second), p.DevelopmentPhotoPreviewUntil); err != nil {
		t.Fatal(err)
	}
	if err = store.BindPhotoCapture(ctx, got.ID, "other-owner", caller.KeyID, e2eStationB, use.CapturedAt, now); !errors.Is(err, app.ErrPhotoCaptureIneligible) {
		t.Fatal("wrong owner")
	}
}
