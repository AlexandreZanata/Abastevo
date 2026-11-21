package adapters

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgconnector"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/application"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgconnector/domain"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	tracking "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	quality "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type TelematicsCredentials interface {
	SecretFor(context.Context, kernel.OrgScope, domain.CredentialRef) ([]byte, error)
}

type trackingEvidence interface {
	LockTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) error
	PolicyTx(context.Context, syncapp.DBTX, kernel.OrgScope) (tracking.PolicyVersion, error)
	SessionOfTripTx(context.Context, syncapp.DBTX, kernel.OrgScope, string) (tracking.SessionRecord, error)
}

type Telematics struct {
	Access      Access
	Credentials TelematicsCredentials
	Tracking    trackingEvidence
	Trips       tracking.Trips
}

type observation struct {
	At      time.Time       `json:"observed_at"`
	Quality quality.Quality `json:"quality"`
}

type evidenceBatch struct {
	Version int           `json:"schema_version"`
	ID      string        `json:"batch_id"`
	Trip    string        `json:"trip_id"`
	Asset   string        `json:"asset_id"`
	Samples []observation `json:"observations"`
}

type TelematicsReceipt struct {
	Provider                           string
	Precise, Approximate, Insufficient int32
	ReceivedAt                         time.Time
	Replayed                           bool
}

// Accept is the vendor-neutral evidence port. A reviewed credential source
// supplies only the scoped signing key. The normal phone recording pipeline
// keeps its separate device proof and raw track ownership.
func (s Telematics) Accept(ctx context.Context, scope kernel.OrgScope, connectionID string, raw []byte, signature string) (TelematicsReceipt, error) {
	if s.Credentials == nil || s.Tracking == nil || s.Trips == nil {
		return TelematicsReceipt{}, app.ErrUnavailable
	}
	var batch evidenceBatch
	if len(raw) > orgapi.MaxBodyBytes || httpapi.DecodeJSON(raw, &batch) != nil || batch.Version != 1 ||
		!audit.ValidID(batch.ID) || !audit.ValidID(batch.Trip) || !audit.ValidID(batch.Asset) ||
		len(batch.Samples) == 0 || len(batch.Samples) > tracking.MaxFixes || len(signature) != 64 {
		return TelematicsReceipt{}, domain.ErrInvalid
	}
	tag, err := hex.DecodeString(signature)
	if err != nil {
		return TelematicsReceipt{}, domain.ErrDenied
	}
	digest := sha256.Sum256(raw)
	var receipt TelematicsReceipt
	err = s.Access.WithActive(ctx, scope, domain.Telematics, connectionID, batch.Asset, func(ctx context.Context, tx pgx.Tx, approved domain.Approval) error {
		if int64(len(raw)) > approved.MaxBytes || len(batch.Samples) > int(approved.MaxItems) {
			return domain.ErrInvalid
		}
		secret, err := s.Credentials.SecretFor(ctx, scope, approved.Credential)
		if err != nil {
			return app.ErrUnavailable
		}
		if len(secret) < 32 {
			return domain.ErrDenied
		}
		mac := hmac.New(sha256.New, secret)
		mac.Write([]byte("abastevo.telematics.v1\n" + scope.OrgID().String() + "\n" + connectionID + "\n"))
		mac.Write(raw)
		if !hmac.Equal(tag, mac.Sum(nil)) {
			return domain.ErrDenied
		}
		q := db.New(tx)
		at, err := q.ReadConnectionClock(ctx)
		if err != nil || !at.Valid {
			return app.ErrUnavailable
		}
		session, evidence, err := s.tripEvidence(ctx, tx, scope, batch, at.Time)
		if err != nil {
			return err
		}
		receipt, err = persistEvidence(ctx, q, scope.OrgID().String(), connectionID, approved.Provider,
			batch, session, hex.EncodeToString(digest[:]), evidence, at)
		return err
	})
	if err != nil {
		return TelematicsReceipt{}, err
	}
	return receipt, nil
}

func (s Telematics) tripEvidence(ctx context.Context, tx pgx.Tx, scope kernel.OrgScope, batch evidenceBatch, received time.Time) (string, TelematicsReceipt, error) {
	if err := s.Tracking.LockTripTx(ctx, tx, scope, batch.Trip); err != nil {
		if errors.Is(err, tracking.ErrNotFound) {
			return "", TelematicsReceipt{}, domain.ErrDenied
		}
		return "", TelematicsReceipt{}, err
	}
	current, err := s.Trips.ReadTripTx(ctx, tx, scope, batch.Trip)
	if err != nil {
		return "", TelematicsReceipt{}, err
	}
	if current.State != trip.InProgress || current.AssetID != batch.Asset {
		return "", TelematicsReceipt{}, domain.ErrDenied
	}
	session, err := s.Tracking.SessionOfTripTx(ctx, tx, scope, batch.Trip)
	if errors.Is(err, tracking.ErrNotFound) {
		return "", TelematicsReceipt{}, domain.ErrDenied
	}
	if err != nil {
		return "", TelematicsReceipt{}, err
	}
	policy, err := s.Tracking.PolicyTx(ctx, tx, scope)
	if err != nil {
		return "", TelematicsReceipt{}, err
	}
	if policy.Policy == quality.PolicyOff || policy.Revision != session.PolicyRevision ||
		current.AuthorMembershipID != session.OwnerMembershipID || len(session.Transitions) == 0 {
		return "", TelematicsReceipt{}, domain.ErrDenied
	}
	latest := session.Transitions[len(session.Transitions)-1]
	if latest.State != quality.Recording {
		return "", TelematicsReceipt{}, domain.ErrDenied
	}
	var counts TelematicsReceipt
	var previous time.Time
	for _, sample := range batch.Samples {
		if sample.At.IsZero() || sample.At.Before(latest.At) || sample.At.After(received) ||
			!previous.IsZero() && !sample.At.After(previous) {
			return "", TelematicsReceipt{}, domain.ErrInvalid
		}
		previous = sample.At
		switch sample.Quality {
		case quality.QualityPrecise:
			if latest.Permission != quality.PermissionPrecise {
				return "", TelematicsReceipt{}, domain.ErrDenied
			}
			counts.Precise++
		case quality.QualityApproximate:
			counts.Approximate++
		case quality.QualityInsufficient:
			counts.Insufficient++
		default:
			return "", TelematicsReceipt{}, domain.ErrInvalid
		}
	}
	return session.ID, counts, nil
}

func persistEvidence(ctx context.Context, q *db.Queries, org, connection, provider string, batch evidenceBatch, session, digest string,
	counts TelematicsReceipt, at pgtype.Timestamptz,
) (TelematicsReceipt, error) {
	inserted, err := q.AddTelematicsEvidence(ctx, db.AddTelematicsEvidenceParams{OrgID: org, ConnectionID: connection,
		BatchID: batch.ID, TripID: batch.Trip, SessionID: session, ProviderCode: provider, PayloadHash: digest,
		Precise: counts.Precise, Approximate: counts.Approximate, Insufficient: counts.Insufficient, ReceivedAt: at})
	if err != nil {
		return TelematicsReceipt{}, err
	}
	row, err := q.ReadTelematicsEvidence(ctx, db.ReadTelematicsEvidenceParams{OrgID: org, ConnectionID: connection, BatchID: batch.ID})
	if err != nil {
		return TelematicsReceipt{}, err
	}
	if row.PayloadHash != digest {
		return TelematicsReceipt{}, domain.ErrConflict
	}
	return TelematicsReceipt{Provider: row.ProviderCode, Precise: row.Precise, Approximate: row.Approximate,
		Insufficient: row.Insufficient, ReceivedAt: row.ReceivedAt.Time.UTC(), Replayed: inserted == 0}, nil
}
