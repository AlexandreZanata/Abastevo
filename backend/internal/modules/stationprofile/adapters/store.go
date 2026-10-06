package adapters

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	stationprofile "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/stationprofile"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/stationprofile/application"
)

// Store implements application.ProfileStore over the generated
// stationprofile queries. Reads are public-shape only; writes are
// append-only with the open-revision guard in SQL.
type Store struct {
	Q *stationprofile.Queries
}

func (s Store) EnsureUnclaimed(ctx context.Context, stationID string) error {
	uid, err := mustUUID(stationID)
	if err != nil {
		return err
	}
	_, err = s.Q.EnsureUnclaimedProfile(ctx, stationprofile.EnsureUnclaimedProfileParams{
		StationID: uid, PolicyVersion: "profile-v1",
	})
	if err != nil {
		// ON CONFLICT DO NOTHING returns no rows on replay: the
		// existing profile stands, which is the converged outcome.
		if isNoRows(err) {
			return nil
		}
		return err
	}
	return nil
}

func (s Store) Profile(ctx context.Context, stationID string) (application.StoredProfile, error) {
	uid, err := mustUUID(stationID)
	if err != nil {
		return application.StoredProfile{}, err
	}
	row, err := s.Q.GetProfile(ctx, uid)
	if err != nil {
		return application.StoredProfile{}, err
	}
	business := map[string]string{}
	if len(row.Projection) > 0 {
		_ = json.Unmarshal(row.Projection, &business)
	}
	return application.StoredProfile{
		PolicyVersion: row.PolicyVersion,
		Revision:      int(row.Revision),
		Business:      business,
	}, nil
}

func (s Store) RecordOperator(ctx context.Context, id, stationID, cnpj, source, ref string) error {
	uid, err := mustUUID(id)
	if err != nil {
		return err
	}
	stationUID, err := mustUUID(stationID)
	if err != nil {
		return err
	}
	_, err = s.Q.RecordOperatorRevision(ctx, stationprofile.RecordOperatorRevisionParams{
		ID: uid, StationID: stationUID, Cnpj: cnpj, Source: source, SourceReference: ref,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return application.ErrOpenRevisionBusy
		}
		return err
	}
	return nil
}

func (s Store) CloseOperator(ctx context.Context, id string) (int64, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return 0, err
	}
	return s.Q.CloseOperatorRevision(ctx, uid)
}

func (s Store) CurrentOperator(ctx context.Context, stationID string) (application.StoredOperator, bool, error) {
	uid, err := mustUUID(stationID)
	if err != nil {
		return application.StoredOperator{}, false, err
	}
	row, err := s.Q.CurrentOperatorRevision(ctx, uid)
	if err != nil {
		if isNoRows(err) {
			return application.StoredOperator{}, false, nil
		}
		return application.StoredOperator{}, false, err
	}
	return application.StoredOperator{
		ID: uuidString(row.ID), CNPJ: row.Cnpj,
		Source: row.Source, Reference: row.SourceReference,
	}, true, nil
}

func (s Store) UpdateProjection(ctx context.Context, stationID string, expectedRevision int, fields map[string]string) (application.StoredProfile, error) {
	uid, err := mustUUID(stationID)
	if err != nil {
		return application.StoredProfile{}, err
	}
	projection, err := json.Marshal(fields)
	if err != nil {
		return application.StoredProfile{}, err
	}
	row, err := s.Q.UpdateProfileProjection(ctx, stationprofile.UpdateProfileProjectionParams{
		Projection: projection, StationID: uid, ExpectedRevision: int32(expectedRevision),
	})
	if err != nil {
		if isNoRows(err) {
			return application.StoredProfile{}, application.ErrManageVersion
		}
		return application.StoredProfile{}, err
	}
	business := map[string]string{}
	_ = json.Unmarshal(row.Projection, &business)
	return application.StoredProfile{
		PolicyVersion: row.PolicyVersion, Revision: int(row.Revision), Business: business,
	}, nil
}

func isNoRows(err error) bool {
	return err != nil && (err == pgx.ErrNoRows || (len(err.Error()) >= 7 && err.Error()[:7] == "no rows"))
}
