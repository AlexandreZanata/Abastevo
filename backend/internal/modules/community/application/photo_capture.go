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
	ClientCaptureID string
	StationID       string
	Location        *LocationEvidence
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
		if err := CheckPhotoCaptureLocation(ctx, p, in.StationID, in.Location, now); err != nil {
			return Outcome{}, err
		}
		id, err := p.NewID()
		if err != nil {
			return Outcome{}, err
		}
		receipt, err := store.InsertPhotoCapture(ctx, PhotoCapture{
			ID: id, ContributorRef: caller.Token, KeyID: caller.KeyID, ClientCaptureID: in.ClientCaptureID,
			StationID: in.StationID, IssuedAt: now, CameraExpiresAt: now.Add(2 * time.Minute),
			ExpiresAt: now.Add(24 * time.Hour), PolicyVersion: PhotoCapturePolicy,
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
	if !p.Clock().Before(receipt.CameraExpiresAt) {
		return PhotoCapture{}, false, ErrPhotoCaptureIneligible
	}
	return receipt, out.Replayed, nil
}
