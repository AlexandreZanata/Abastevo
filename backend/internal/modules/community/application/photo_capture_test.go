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

type savedCaptureStore struct {
	noCaptureStore
	receipt PhotoCapture
	err     error
}

func (s savedCaptureStore) PhotoCapture(context.Context, string, string) (PhotoCapture, error) {
	return s.receipt, s.err
}

func TestPhotoCaptureUseRejectsChangedScopeTimeSessionAndExpiry(t *testing.T) {
	for _, policy := range []string{PhotoCapturePolicy, CommunityPhotoCapturePolicy} {
		t.Run(policy, func(t *testing.T) {
			now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
			receipt := PhotoCapture{ID: "capture", ContributorRef: "owner", KeyID: "key", StationID: "station", PolicyVersion: policy, IssuedAt: now.Add(-time.Minute), CameraExpiresAt: now.Add(time.Minute), ExpiresAt: now.Add(23 * time.Hour)}
			caller := Caller{ContributorID: "contributor", Token: "owner", KeyID: "key"}
			in := PhotoCaptureUse{CaptureID: "capture", StationID: "station", CapturedAt: now.Add(-time.Second)}
			if got, err := ValidatePhotoCaptureUse(context.Background(), savedCaptureStore{receipt: receipt}, caller, in, now); err != nil || got.ID != receipt.ID {
				t.Fatalf("valid use=%+v %v", got, err)
			}
			for _, mutate := range []func(*PhotoCapture){
				func(r *PhotoCapture) { r.KeyID = "other" }, func(r *PhotoCapture) { r.ContributorRef = "other" },
				func(r *PhotoCapture) { r.StationID = "other" }, func(r *PhotoCapture) { r.PolicyVersion = "other" },
				func(r *PhotoCapture) { r.ExpiresAt = now }, func(r *PhotoCapture) { r.IssuedAt = now },
				func(r *PhotoCapture) { r.CameraExpiresAt = in.CapturedAt },
				func(r *PhotoCapture) { r.EvidenceSessionID = "session"; r.CapturedAt = now.Add(-2 * time.Second) },
			} {
				changed := receipt
				mutate(&changed)
				if _, err := ValidatePhotoCaptureUse(context.Background(), savedCaptureStore{receipt: changed}, caller, in, now); !errors.Is(err, ErrPhotoCaptureIneligible) {
					t.Fatalf("changed receipt accepted: %v", err)
				}
			}
			bound := receipt
			bound.EvidenceSessionID = "session"
			bound.CapturedAt = in.CapturedAt
			in.EvidenceSessionID = "other"
			if _, err := ValidatePhotoCaptureUse(context.Background(), savedCaptureStore{receipt: bound}, caller, in, now); !errors.Is(err, ErrPhotoCaptureIneligible) {
				t.Fatal("other session accepted")
			}
			in.EvidenceSessionID = "session"
			if _, err := ValidatePhotoCaptureUse(context.Background(), savedCaptureStore{receipt: bound}, caller, in, now); err != nil {
				t.Fatal(err)
			}
			in.CapturedAt = now.Add(time.Second)
			if _, err := ValidatePhotoCaptureUse(context.Background(), savedCaptureStore{receipt: receipt}, caller, in, now); !errors.Is(err, ErrPhotoCaptureIneligible) {
				t.Fatal("future capture accepted")
			}
		})
	}
}

type previewCaptureStore struct {
	noCaptureStore
	receipt PhotoCapture
}

func (s *previewCaptureStore) InsertPhotoCapture(_ context.Context, in PhotoCapture) (PhotoCapture, error) {
	s.receipt = in
	return in, nil
}

func TestDevelopmentPhotoPreviewRequiresOptInDeadlineCanonicalStationAndProof(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	p := testPorts()
	p.Clock = func() time.Time { return now }
	p.Idempotent = func(ctx context.Context, _ IdempotencyKey, _ []byte, run func(context.Context) (Outcome, error)) (Outcome, error) {
		return run(ctx)
	}
	in := PhotoCaptureIntent{ClientCaptureID: "preview", StationID: testDTO().StationID, DevelopmentPreview: true}
	store := &previewCaptureStore{}
	if _, _, err := AuthorizePhotoCapture(context.Background(), p, store, testCaller(), "preview", nil, in); !errors.Is(err, ErrPhotoCaptureIneligible) {
		t.Fatal("default bypass", err)
	}
	p.DevelopmentPhotoPreviewUntil = now.Add(time.Hour)
	p.CheckDevelopmentStation = func(context.Context, string) error { return nil }
	got, _, err := AuthorizePhotoCapture(context.Background(), p, store, testCaller(), "preview", nil, in)
	if err != nil || got.PolicyVersion != DevelopmentPhotoCapturePolicy {
		t.Fatal("preview refusal", err)
	}
	if _, _, err = AuthorizePhotoCapture(context.Background(), p, store, Caller{}, "preview", nil, in); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("unsigned allowed")
	}
	p.CheckDevelopmentStation = func(context.Context, string) error { return ErrPhotoCaptureIneligible }
	if _, _, err = AuthorizePhotoCapture(context.Background(), p, store, testCaller(), "preview", nil, in); !errors.Is(err, ErrPhotoCaptureIneligible) {
		t.Fatal("unknown station allowed")
	}
	p.CheckDevelopmentStation = func(context.Context, string) error { return nil }
	p.Clock = func() time.Time { return p.DevelopmentPhotoPreviewUntil }
	if _, _, err = AuthorizePhotoCapture(context.Background(), p, store, testCaller(), "preview", nil, in); !errors.Is(err, ErrPhotoCaptureIneligible) {
		t.Fatal("expired opt-in allowed")
	}
	p.Clock = func() time.Time { return now }
	previewReceipt := store.receipt
	in.DevelopmentPreview = false
	if normal, _, err := AuthorizePhotoCapture(context.Background(), p, store, testCaller(), "preview", nil, in); err != nil || normal.PolicyVersion != CommunityPhotoCapturePolicy {
		t.Fatal("normal flow must use distinct community policy", err)
	}
	receipt := previewReceipt
	receipt.ContributorRef = testCaller().Token
	receipt.KeyID = testCaller().KeyID
	use := PhotoCaptureUse{CaptureID: receipt.ID, StationID: receipt.StationID, CapturedAt: now.Add(time.Second)}
	if _, err = ValidatePhotoCaptureUse(context.Background(), savedCaptureStore{receipt: receipt}, testCaller(), use, now.Add(time.Second)); !errors.Is(err, ErrPhotoCaptureIneligible) {
		t.Fatal("preview receipt admitted without explicit policy")
	}
	if _, err = ValidatePhotoCaptureUse(context.Background(), savedCaptureStore{receipt: receipt}, testCaller(), use, now.Add(time.Second), p.DevelopmentPhotoPreviewUntil); err != nil {
		t.Fatal(err)
	}
}

func TestCommunityFirstCaptureNeedsStationProofNotLocation(t *testing.T) {
	for _, distance := range []float64{0, 149.9, 150, 150.1, 100000} {
		p := testPorts()
		p.CheckCaptureStation = func(context.Context, string) error { return nil }
		p.Locate = func(context.Context, string, float64, float64) (float64, StationSite, error) {
			t.Fatal("normal capture queried location")
			return distance, SiteUnknown, nil
		}
		store := &previewCaptureStore{}
		in := PhotoCaptureIntent{ClientCaptureID: "community", StationID: testDTO().StationID}
		if distance > 0 {
			now := p.Clock()
			in.Location = locationDTO(FixVerified, acc(10), &now, acc(-23.5+distance/111000), acc(-46.6))
		}
		got, _, err := AuthorizePhotoCapture(context.Background(), p, store, testCaller(), "community", nil, in)
		if err != nil || got.PolicyVersion != "photo-capture-community-v2" {
			t.Fatalf("no-GPS capture %v %+v", err, got)
		}
		if got.CameraExpiresAt.Sub(got.IssuedAt) != 2*time.Minute || got.ExpiresAt.Sub(got.IssuedAt) != 24*time.Hour {
			t.Fatal("changed deadlines")
		}
		p.CheckCaptureStation = func(context.Context, string) error { return ErrPhotoCaptureIneligible }
		if _, _, err := AuthorizePhotoCapture(context.Background(), p, noCaptureStore{}, testCaller(), "community", nil, in); !errors.Is(err, ErrPhotoCaptureIneligible) {
			t.Fatal("unknown/inactive station admitted", err)
		}
	}
}

func TestCommunityFirstCaptureRejectsForgedOptionalClaimAndMissingStationPort(t *testing.T) {
	p := testPorts()
	now := p.Clock()
	in := PhotoCaptureIntent{ClientCaptureID: "forged", StationID: testDTO().StationID, Location: locationDTO(FixVerified, acc(10), &now, acc(-23.5), acc(-46.6))}
	in.Location.Simulated = true
	if _, _, err := AuthorizePhotoCapture(context.Background(), p, noCaptureStore{}, testCaller(), in.ClientCaptureID, nil, in); !errors.Is(err, ErrLocationForged) {
		t.Fatal("forged optional claim admitted", err)
	}
	in.Location = nil
	p.CheckCaptureStation = nil
	if _, _, err := AuthorizePhotoCapture(context.Background(), p, noCaptureStore{}, testCaller(), in.ClientCaptureID, nil, in); err == nil {
		t.Fatal("missing station port admitted capture")
	}
}

func TestCommunityFirstCaptureRefusesZeroClock(t *testing.T) {
	p := testPorts()
	p.Clock = func() time.Time { return time.Time{} }
	in := PhotoCaptureIntent{ClientCaptureID: "zero-clock", StationID: testDTO().StationID}
	if _, _, err := AuthorizePhotoCapture(context.Background(), p, noCaptureStore{}, testCaller(), in.ClientCaptureID, nil, in); !errors.Is(err, ErrPhotoCaptureIneligible) {
		t.Fatal("zero clock admitted capture", err)
	}
}
