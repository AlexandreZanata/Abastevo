package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/evidence/domain"
)

func TestExpiredReservationReplayCannotMintUploadAuthorization(t *testing.T) {
	store := newFakeStore()
	p := testPorts(store)
	if _, err := Reserve(context.Background(), p, testCaller(), testIntent()); err != nil {
		t.Fatal(err)
	}
	p.Clock = func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) }
	presigns := 0
	p.Presign = func(context.Context, string, string, int64, time.Time) (string, map[string]string, time.Time, error) {
		presigns++
		return "https://storage.example.invalid/test", nil, p.Clock().Add(time.Minute), nil
	}
	if _, err := Reserve(context.Background(), p, testCaller(), testIntent()); !errors.Is(err, domain.ErrBadTransition) {
		t.Fatalf("expired replay = %v", err)
	}
	if presigns != 0 {
		t.Fatalf("expired replay minted %d credentials", presigns)
	}
}

func TestPhotoReserveValidatesBeforeStoreBindsBeforePresignAndCapsDeadline(t *testing.T) {
	for _, refusal := range []string{"eligibility", "binding", ""} {
		t.Run(refusal, func(t *testing.T) {
			store := newFakeStore()
			p := testPorts(store)
			now := p.Clock()
			deadline := now.Add(time.Hour)
			bound := false
			p.ValidatePhotoCapture = func(_ context.Context, caller Caller, in Intent) (time.Time, error) {
				if caller.KeyID != "key-one" || in.PhotoCaptureID != "capture-one" || in.StationID != "station-one" || !in.CapturedAt.Equal(now.Add(-time.Minute)) {
					t.Fatal("lost capture envelope")
				}
				if refusal == "eligibility" {
					return time.Time{}, ErrPhotoCaptureIneligible
				}
				return deadline, nil
			}
			p.BindPhotoCapture = func(_ context.Context, _ Caller, _ Intent, sessionID string, at time.Time) error {
				if sessionID == "" || !at.Equal(now) || store.saves != 1 {
					t.Fatal("bind before durable reservation")
				}
				if refusal == "binding" {
					return ErrPhotoCaptureIneligible
				}
				bound = true
				return nil
			}
			presigns := 0
			p.Presign = func(_ context.Context, _, _ string, _ int64, until time.Time) (string, map[string]string, time.Time, error) {
				if !bound || !until.Equal(deadline) {
					t.Fatal("credential before binding or extended deadline")
				}
				presigns++
				return "https://storage.example.invalid/test", nil, now.Add(time.Minute), nil
			}
			in := testIntent()
			in.PhotoCaptureID, in.StationID, in.CapturedAt = "capture-one", "station-one", now.Add(-time.Minute)
			caller := testCaller()
			caller.KeyID = "key-one"
			result, err := Reserve(context.Background(), p, caller, in)
			if refusal != "" {
				if !errors.Is(err, ErrPhotoCaptureIneligible) || presigns != 0 {
					t.Fatalf("refusal=%v credentials=%d", err, presigns)
				}
				if refusal == "eligibility" && store.saves != 0 {
					t.Fatal("invalid capture reserved storage")
				}
			} else if err != nil || !result.ExpiresAt.Equal(deadline) || presigns != 1 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestPhotoReserveRequiresCompleteEnvelopeAndConfiguredAuthority(t *testing.T) {
	for _, in := range []Intent{
		{ClientSessionID: "x", PhotoCaptureID: "capture"},
		{ClientSessionID: "x", StationID: "station"},
		{ClientSessionID: "x", CapturedAt: time.Now()},
		{ClientSessionID: "x", PhotoCaptureID: "capture", StationID: "station", CapturedAt: time.Now()},
	} {
		store := newFakeStore()
		_, err := Reserve(context.Background(), testPorts(store), testCaller(), in)
		if !errors.Is(err, ErrPhotoCaptureIneligible) || store.saves != 0 {
			t.Fatalf("incomplete/unconfigured envelope err=%v saves=%d", err, store.saves)
		}
	}
}
