package adapters

import (
	"context"
	"errors"
	"time"

	community "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/community"
	application "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/community/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func photoCaptureModel(row community.CommunityPhotoCapture) application.PhotoCapture {
	return application.PhotoCapture{
		ID: uuidString(row.ID), ContributorRef: row.ContributorRef, KeyID: row.KeyID,
		ClientCaptureID: row.ClientCaptureID, StationID: uuidString(row.StationID),
		IssuedAt: row.IssuedAt.Time, CameraExpiresAt: row.CameraExpiresAt.Time,
		ExpiresAt: row.ExpiresAt.Time, PolicyVersion: row.PolicyVersion,
		EvidenceSessionID: uuidString(row.EvidenceSessionID), CapturedAt: row.CapturedAt.Time,
	}
}

func (s *Store) InsertPhotoCapture(ctx context.Context, receipt application.PhotoCapture) (application.PhotoCapture, error) {
	id, err := mustUUID(receipt.ID)
	if err != nil {
		return application.PhotoCapture{}, err
	}
	station, err := mustUUID(receipt.StationID)
	if err != nil {
		return application.PhotoCapture{}, err
	}
	row, err := community.New(s.pool).InsertPhotoCapture(ctx, community.InsertPhotoCaptureParams{
		ID: id, ContributorRef: receipt.ContributorRef, KeyID: receipt.KeyID,
		ClientCaptureID: receipt.ClientCaptureID, StationID: station,
		IssuedAt: pgTime(receipt.IssuedAt), CameraExpiresAt: pgTime(receipt.CameraExpiresAt),
		ExpiresAt: pgTime(receipt.ExpiresAt), PolicyVersion: receipt.PolicyVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.PhotoCapture{}, application.ErrConflict
	}
	if err != nil {
		return application.PhotoCapture{}, err
	}
	return photoCaptureModel(row), nil
}

func (s *Store) PhotoCapture(ctx context.Context, id, owner string) (application.PhotoCapture, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return application.PhotoCapture{}, application.ErrPhotoCaptureIneligible
	}
	row, err := community.New(s.pool).OwnedPhotoCapture(ctx, community.OwnedPhotoCaptureParams{ID: uid, ContributorRef: owner})
	if errors.Is(err, pgx.ErrNoRows) {
		return application.PhotoCapture{}, application.ErrPhotoCaptureIneligible
	}
	if err != nil {
		return application.PhotoCapture{}, err
	}
	return photoCaptureModel(row), nil
}

// BindPhotoCapture atomically restricts one receipt to one evidence session.
// An identical retry converges; a second session, owner or key cannot consume it.
func (s *Store) BindPhotoCapture(ctx context.Context, id, owner, key, session string, capturedAt, now time.Time) error {
	uid, err := mustUUID(id)
	if err != nil {
		return application.ErrPhotoCaptureIneligible
	}
	sid, err := mustUUID(session)
	if err != nil {
		return application.ErrPhotoCaptureIneligible
	}
	affected, err := community.New(s.pool).BindPhotoCapture(ctx, community.BindPhotoCaptureParams{
		ID: uid, ContributorRef: owner, KeyID: key, SessionID: sid,
		CapturedAt: pgTime(capturedAt), NowAt: pgTime(now),
	})
	if err != nil {
		var conflict *pgconn.PgError
		if errors.As(err, &conflict) && conflict.Code == "23505" {
			return application.ErrPhotoCaptureIneligible
		}
		return err
	}
	if affected != 1 {
		return application.ErrPhotoCaptureIneligible
	}
	return nil
}

// PurgeExpiredPhotoCaptures bounds metadata cleanup independently from byte retention.
func (s *Store) PurgeExpiredPhotoCaptures(ctx context.Context, now time.Time, batch int) (int64, error) {
	if batch < 1 || batch > 10000 {
		return 0, errors.New("community: invalid retention batch")
	}
	return community.New(s.pool).PurgePhotoCaptures(ctx, community.PurgePhotoCapturesParams{NowAt: pgTime(now), Batch: int32(batch)})
}
