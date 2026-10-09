package application

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

const PhotoCaptureRadiusM = 150.0
const PhotoCapturePolicy = "photo-capture-v1"
const DevelopmentPhotoCapturePolicy = "photo-capture-ui-test-v1"

var ErrPhotoCaptureIneligible = errors.New("community: photo capture not eligible")

// PhotoCapture contains authorization facts only; exact fixes never persist.
type PhotoCapture struct {
	ID                string    `json:"capture_id"`
	ContributorRef    string    `json:"-"`
	KeyID             string    `json:"-"`
	ClientCaptureID   string    `json:"-"`
	StationID         string    `json:"station_id"`
	IssuedAt          time.Time `json:"issued_at"`
	CameraExpiresAt   time.Time `json:"camera_expires_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	PolicyVersion     string    `json:"policy_version"`
	EvidenceSessionID string    `json:"-"`
	CapturedAt        time.Time `json:"-"`
}

type PhotoCaptureStore interface {
	InsertPhotoCapture(context.Context, PhotoCapture) (PhotoCapture, error)
	PhotoCapture(context.Context, string, string) (PhotoCapture, error)
	BindPhotoCapture(context.Context, string, string, string, string, time.Time, time.Time) error
}

type PhotoCaptureIntent struct {
	ClientCaptureID    string
	StationID          string
	Location           *LocationEvidence
	DevelopmentPreview bool
}

// CheckPhotoCaptureLocation reuses the frozen location classification and
// reviewed-station distance port. It never trusts client proximity or keeps GPS.
func CheckPhotoCaptureLocation(ctx context.Context, p Ports, stationID string, loc *LocationEvidence, now time.Time) error {
	if strings.TrimSpace(stationID) == "" || loc == nil || p.Locate == nil || now.IsZero() ||
		loc.Latitude == nil || loc.Longitude == nil || loc.AccuracyMeters == nil ||
		!finite(*loc.Latitude) || math.Abs(*loc.Latitude) > 90 ||
		!finite(*loc.Longitude) || math.Abs(*loc.Longitude) > 180 ||
		!finite(*loc.AccuracyMeters) || *loc.AccuracyMeters < 0 {
		return ErrPhotoCaptureIneligible
	}
	fix, err := VerifyDeviceFix(DeviceFix{
		ClaimedVerdict: loc.ClaimedVerdict, PermissionGranted: loc.PermissionGranted,
		HasFix: loc.HasFix, SourceInfoPresent: loc.SourceInfoPresent, Simulated: loc.Simulated,
		AccuracyMeters: loc.AccuracyMeters, ClockSkewSeconds: loc.ClockSkewSeconds,
		Manual: loc.Manual, CapturedAt: loc.CapturedAt, Latitude: loc.Latitude, Longitude: loc.Longitude,
	}, now)
	if err != nil || !fix.AllowsClaim {
		return ErrPhotoCaptureIneligible
	}
	// Subsecond future fixes must not truncate to a zero age and pass.
	if loc.CapturedAt == nil || loc.CapturedAt.After(now) {
		return ErrPhotoCaptureIneligible
	}
	distance, site, err := p.Locate(ctx, stationID, *loc.Latitude, *loc.Longitude)
	if err != nil {
		return err
	}
	if site != SitePrecise || !finite(distance) || distance < 0 || distance > PhotoCaptureRadiusM {
		return ErrPhotoCaptureIneligible
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// AuthorizePhotoCapture relies on the existing signed request nonce and
// idempotency boundary. Replays preserve the original deadlines.
func AuthorizePhotoCapture(ctx context.Context, p Ports, store PhotoCaptureStore, caller Caller, key string, body []byte, in PhotoCaptureIntent) (PhotoCapture, bool, error) {
	if caller.ContributorID == "" || caller.Token == "" || caller.KeyID == "" {
		return PhotoCapture{}, false, ErrUnauthorized
	}
	if store == nil || p.Clock == nil || p.NewID == nil || p.Idempotent == nil || p.CheckQuota == nil {
		return PhotoCapture{}, false, errors.New("community: photo capture not configured")
	}
	if in.ClientCaptureID == "" || len(in.ClientCaptureID) > 128 || key != in.ClientCaptureID {
		return PhotoCapture{}, false, ErrPhotoCaptureIneligible
	}
	if p.CheckAccount != nil {
		if err := p.CheckAccount(ctx, caller.ContributorID); err != nil {
			return PhotoCapture{}, false, err
		}
	}
	if _, err := p.CheckQuota(ctx, caller.Fingerprint, "write"); err != nil {
		return PhotoCapture{}, false, err
	}
	out, err := p.Idempotent(ctx, IdempotencyKey{ContributorID: caller.ContributorID, Method: "POST", Route: "/v1/photo-captures", Key: key}, body, func(ctx context.Context) (Outcome, error) {
		now := p.Clock()
		policy := PhotoCapturePolicy
		if in.DevelopmentPreview {
			if p.DevelopmentPhotoPreviewUntil.IsZero() || !now.Before(p.DevelopmentPhotoPreviewUntil) || p.CheckDevelopmentStation == nil {
				return Outcome{}, ErrPhotoCaptureIneligible
			}
			if err := p.CheckDevelopmentStation(ctx, in.StationID); err != nil {
				return Outcome{}, err
			}
			policy = DevelopmentPhotoCapturePolicy
		}
		if !in.DevelopmentPreview {
			if err := CheckPhotoCaptureLocation(ctx, p, in.StationID, in.Location, now); err != nil {
				return Outcome{}, err
			}
		}
		id, err := p.NewID()
		if err != nil {
			return Outcome{}, err
		}
		receipt, err := store.InsertPhotoCapture(ctx, PhotoCapture{
			ID: id, ContributorRef: caller.Token, KeyID: caller.KeyID, ClientCaptureID: in.ClientCaptureID,
			StationID: in.StationID, IssuedAt: now, CameraExpiresAt: now.Add(2 * time.Minute),
			ExpiresAt: now.Add(24 * time.Hour), PolicyVersion: policy,
		})
		if err != nil {
			return Outcome{}, err
		}
		raw, err := json.Marshal(receipt)
		return Outcome{StatusCode: 201, Body: raw}, err
	})
	if err != nil {
		return PhotoCapture{}, false, err
	}
	var receipt PhotoCapture
	if err := json.Unmarshal(out.Body, &receipt); err != nil {
		return PhotoCapture{}, false, err
	}
	if receipt.PolicyVersion == DevelopmentPhotoCapturePolicy &&
		(p.DevelopmentPhotoPreviewUntil.IsZero() || !p.Clock().Before(p.DevelopmentPhotoPreviewUntil)) {
		return PhotoCapture{}, false, ErrPhotoCaptureIneligible
	}
	if !p.Clock().Before(receipt.CameraExpiresAt) {
		return PhotoCapture{}, false, ErrPhotoCaptureIneligible
	}
	return receipt, out.Replayed, nil
}

// PhotoCaptureUse is the immutable capture envelope used by upload and intake.
// EvidenceSessionID is required for observations, but absent before reservation.
type PhotoCaptureUse struct {
	CaptureID         string
	StationID         string
	CapturedAt        time.Time
	EvidenceSessionID string
}

func ValidatePhotoCaptureUse(ctx context.Context, store PhotoCaptureStore, caller Caller, in PhotoCaptureUse, now time.Time, developmentUntil ...time.Time) (PhotoCapture, error) {
	if store == nil || caller.ContributorID == "" || caller.Token == "" || caller.KeyID == "" ||
		in.CaptureID == "" || in.StationID == "" || in.CapturedAt.IsZero() || now.IsZero() || in.CapturedAt.After(now) {
		return PhotoCapture{}, ErrPhotoCaptureIneligible
	}
	receipt, err := store.PhotoCapture(ctx, in.CaptureID, caller.Token)
	if err != nil {
		return PhotoCapture{}, err
	}
	allowedPolicy := receipt.PolicyVersion == PhotoCapturePolicy
	if receipt.PolicyVersion == DevelopmentPhotoCapturePolicy && len(developmentUntil) == 1 {
		allowedPolicy = !developmentUntil[0].IsZero() && now.Before(developmentUntil[0])
	}
	if receipt.ID != in.CaptureID || receipt.ContributorRef != caller.Token || receipt.KeyID != caller.KeyID ||
		receipt.StationID != in.StationID || !allowedPolicy || !now.Before(receipt.ExpiresAt) ||
		in.CapturedAt.Before(receipt.IssuedAt) || !in.CapturedAt.Before(receipt.CameraExpiresAt) ||
		(receipt.EvidenceSessionID != "" && !receipt.CapturedAt.Equal(in.CapturedAt)) ||
		(in.EvidenceSessionID != "" && receipt.EvidenceSessionID != in.EvidenceSessionID) {
		return PhotoCapture{}, ErrPhotoCaptureIneligible
	}
	return receipt, nil
}
