package application

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

func TestPhotoCaptureEligibilityBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		distance float64
		allowed  bool
	}{
		{"inside", 149.9, true}, {"boundary", 150, true}, {"outside", 150.1, false},
		{"negative", -1, false}, {"nan", math.NaN(), false}, {"infinity", math.Inf(1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := locationPorts()
			p.Locate = func(context.Context, string, float64, float64) (float64, StationSite, error) {
				return tc.distance, SitePrecise, nil
			}
			loc := locationDTO(FixVerified, acc(10), &now, acc(-23.5), acc(-46.6))
			err := CheckPhotoCaptureLocation(context.Background(), p, testDTO().StationID, loc, now)
			if (err == nil) != tc.allowed {
				t.Fatalf("allowed=%v error=%v", tc.allowed, err)
			}
		})
	}
}

func TestPhotoCaptureEligibilityRefusesUntrustedFixes(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	for _, mutate := range []func(*LocationEvidence){
		func(l *LocationEvidence) { l.PermissionGranted = false },
		func(l *LocationEvidence) { l.SourceInfoPresent = false },
		func(l *LocationEvidence) { l.Simulated = true },
		func(l *LocationEvidence) { l.AccuracyMeters = acc(101) },
		func(l *LocationEvidence) { l.AccuracyMeters = acc(-1) },
		func(l *LocationEvidence) { l.AccuracyMeters = acc(math.NaN()) },
		func(l *LocationEvidence) { l.Latitude = acc(math.Inf(1)) },
		func(l *LocationEvidence) { l.Longitude = nil },
		func(l *LocationEvidence) { t := now.Add(-121 * time.Second); l.CapturedAt = &t },
		func(l *LocationEvidence) { t := now.Add(time.Second); l.CapturedAt = &t },
		func(l *LocationEvidence) { l.Manual = true },
	} {
		loc := locationDTO(FixVerified, acc(10), &now, acc(-23.5), acc(-46.6))
		mutate(loc)
		if err := CheckPhotoCaptureLocation(context.Background(), locationPorts(), testDTO().StationID, loc, now); !errors.Is(err, ErrPhotoCaptureIneligible) {
			t.Fatalf("unsafe fix accepted or wrong error: %v", err)
		}
	}
	if err := CheckPhotoCaptureLocation(context.Background(), locationPorts(), testDTO().StationID, nil, now); !errors.Is(err, ErrPhotoCaptureIneligible) {
		t.Fatal(err)
	}
	p := locationPorts()
	p.Locate = func(context.Context, string, float64, float64) (float64, StationSite, error) {
		return 0, SiteUnknown, nil
	}
	loc := locationDTO(FixVerified, acc(10), &now, acc(-23.5), acc(-46.6))
	if err := CheckPhotoCaptureLocation(context.Background(), p, testDTO().StationID, loc, now); !errors.Is(err, ErrPhotoCaptureIneligible) {
		t.Fatal(err)
	}
}

type noCaptureStore struct{}

func (noCaptureStore) InsertPhotoCapture(context.Context, PhotoCapture) (PhotoCapture, error) {
	panic("unexpected insert")
}
func (noCaptureStore) PhotoCapture(context.Context, string, string) (PhotoCapture, error) {
	panic("unexpected read")
}
func (noCaptureStore) BindPhotoCapture(context.Context, string, string, string, string, time.Time, time.Time) error {
	panic("unexpected bind")
}

func TestPhotoCaptureAuthAccountAndIdempotencyDenials(t *testing.T) {
	p := testPorts()
	in := PhotoCaptureIntent{ClientCaptureID: "capture", StationID: testDTO().StationID}
	for _, caller := range []Caller{{}, {ContributorID: "c", Token: "t"}, {ContributorID: "c", KeyID: "k"}} {
		if _, _, err := AuthorizePhotoCapture(context.Background(), p, noCaptureStore{}, caller, "capture", nil, in); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("incomplete proof accepted: %v", err)
		}
	}
	p.CheckAccount = func(context.Context, string) error { return ErrAccountBlocked }
	if _, _, err := AuthorizePhotoCapture(context.Background(), p, noCaptureStore{}, testCaller(), "capture", nil, in); !errors.Is(err, ErrAccountBlocked) {
		t.Fatal(err)
	}
	p.CheckAccount = nil
	p.Idempotent = func(context.Context, IdempotencyKey, []byte, func(context.Context) (Outcome, error)) (Outcome, error) {
		return Outcome{}, ErrConflict
	}
	if _, _, err := AuthorizePhotoCapture(context.Background(), p, noCaptureStore{}, testCaller(), "capture", nil, in); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, _, err := AuthorizePhotoCapture(context.Background(), p, noCaptureStore{}, testCaller(), "different", nil, in); !errors.Is(err, ErrPhotoCaptureIneligible) {
		t.Fatal(err)
	}
}
