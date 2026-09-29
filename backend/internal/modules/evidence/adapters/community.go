package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	evidence "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/evidence"
)

// CommunityEvidence is the read-only view consumed by community
// validation (B-BR-010): readiness plus owner, never bytes, keys or URLs.
type CommunityEvidence struct {
	Found    bool
	Ready    bool
	OwnerRef string
}

// ForCommunity resolves one evidence object for community validation.
// Missing objects report Found=false with no error: the observation waits
// for READY or the 24 h evidence deadline instead of failing. Present
// objects are READY facts by construction (rows appear only in the READY
// transaction), so Ready mirrors Found.
func (s *Store) ForCommunity(ctx context.Context, objectID string) (CommunityEvidence, error) {
	uid, err := mustUUID(objectID)
	if err != nil {
		return CommunityEvidence{}, err
	}
	row, err := evidence.New(s.pool).GetObject(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CommunityEvidence{}, nil
		}
		return CommunityEvidence{}, err
	}
	// Present objects are READY facts by construction; once the
	// sanitized bytes leave storage the fact reads unavailable while
	// its duplicate-signal hashes survive to their bound.
	return CommunityEvidence{
		Found: true, Ready: !row.FinalDeletedAt.Valid,
		OwnerRef: row.ContributorRef,
	}, nil
}

// TryBindObject claims one object for one observation with
// set-if-unbound-or-same semantics: idempotent replay converges, binding
// to a different observation conflicts (no duplicate binding), foreign
// owners and missing objects refuse. Community validation calls this only
// after its own readiness and ownership checks pass.
func (s *Store) TryBindObject(ctx context.Context, objectID, observationID, contributorRef string) error {
	uid, err := mustUUID(objectID)
	if err != nil {
		return err
	}
	if _, err := mustUUID(observationID); err != nil {
		return err
	}
	q := evidence.New(s.pool)
	row, err := q.GetObject(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownObject
		}
		return err
	}
	if row.ContributorRef != contributorRef {
		return ErrNotOwner
	}
	if row.BoundObservationID.Valid {
		if uuidString(row.BoundObservationID) == observationID {
			return nil
		}
		return ErrAlreadyBound
	}
	obsUUID, err := mustUUID(observationID)
	if err != nil {
		return err
	}
	n, err := q.ClaimObjectBinding(ctx, evidence.ClaimObjectBindingParams{
		ID: uid, ObservationID: obsUUID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return s.convergeBind(ctx, uid, observationID)
	}
	return nil
}

// convergeBind replays a lost bind race with the same set-if-unbound-or-
// same rules instead of forking the claim.
func (s *Store) convergeBind(ctx context.Context, uid pgtype.UUID, observationID string) error {
	row, err := evidence.New(s.pool).GetObject(ctx, uid)
	if err != nil {
		return err
	}
	if !row.BoundObservationID.Valid {
		return errors.New("adapters: bind claim lost without a winner")
	}
	if uuidString(row.BoundObservationID) == observationID {
		return nil
	}
	return ErrAlreadyBound
}
