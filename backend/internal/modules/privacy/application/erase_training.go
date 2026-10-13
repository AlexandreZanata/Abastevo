package application

import (
	"context"
	"errors"
	"strings"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// Training erasure (CS10D, AT02/AT05): account erasure covers training
// assets. Consent ends REVOKED (baseline established when absent),
// every owned derivative flips to its tombstone, and every owned
// copy deletes immediately — erasure never waits for the 24-hour
// sweeper. Re-runs converge to zero; byte failures error for the
// FAILED receipt and audited retry. Receipts stay for the restore
// filter in all cases.

// TrainingErasePorts wires training erasure through the shared
// consent ports, the derivative inventory and the byte-copy store.
type TrainingErasePorts struct {
	Clock       func() time.Time
	Attribution func(ctx context.Context, contributorID string) (string, error)
	Consent     ConsentPorts
	Inventory   TrainingInventory
	Objects     ObjectStore
}

// TrainingInventory advances a durable owner keyset only after confirmed
// absence. A new replay rechecks prior PURGED receipts for restored bytes.
type TrainingInventory interface {
	ClaimOwnerPurge(context.Context, string, time.Time) (PurgeClaim, bool, error)
	CompletePurge(context.Context, PurgeClaim, time.Time) error
	HasRemainingErasure(context.Context, string) (bool, error)
}

// EraseTrainingContributor revokes one owner's training consent and
// purges its derivatives and bytes, reporting flipped rows and
// deleted copies. Consent revocation moves past the stored revision
// (or establishes the baseline when absent); inventory marks
// converge; byte deletes are idempotent, so crashes resume safely.
func EraseTrainingContributor(ctx context.Context, p TrainingErasePorts, contributorID string) (revoked, objectsPurged int, err error) {
	contributorID = strings.TrimSpace(contributorID)
	if contributorID == "" {
		return 0, 0, ErrUnauthorized
	}
	if p.Attribution == nil || p.Consent.Store == nil || p.Inventory == nil || p.Objects == nil {
		return 0, 0, errors.New("training erasure: unconfigured")
	}
	token, err := p.Attribution(ctx, contributorID)
	if err != nil {
		return 0, 0, err
	}
	clock := p.Clock
	if clock == nil {
		clock = time.Now
	}
	now := clock()
	stored, err := p.Consent.Store.Get(ctx, contributorID)
	found := err == nil
	if err != nil && !isNotFound(err) {
		return 0, 0, err
	}
	next := stored
	if !found || stored.State != domain.StateRevoked {
		revision := uint64(1)
		if found {
			revision = stored.Revision + 1
		}
		next, _, err = domain.RevokeConsent(ptrConsent(stored, found), contributorID, token, revision, now)
		if err != nil {
			return 0, 0, err
		}
	}
	// Equal OFF replay retains the original revision/anchor and materializes
	// another bounded tombstone batch. It does not mint a new revocation.
	if _, _, err := p.Consent.Store.Put(ctx, next); err != nil {
		return 0, 0, err
	}
	run, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for i := 0; i < MaxTrainingPurgeBatch; i++ {
		claim, ok, err := p.Inventory.ClaimOwnerPurge(run, contributorID, clock())
		if err != nil {
			return revoked, objectsPurged, err
		}
		if !ok {
			break
		}
		existed, err := disposeTrainingClaim(run, p.Inventory, p.Objects, claim, clock)
		if err != nil {
			return revoked, objectsPurged, err
		}
		if !claim.WasPurged || existed {
			revoked++
		}
		if existed {
			objectsPurged++
		}
	}
	pending, err := p.Inventory.HasRemainingErasure(run, contributorID)
	if err != nil {
		return revoked, objectsPurged, err
	}
	if pending {
		return revoked, objectsPurged, domain.ErrBadState
	}
	return revoked, objectsPurged, nil
}

// ptrConsent returns nil for absent consent (revocation baseline).
func ptrConsent(stored domain.Consent, ok bool) *domain.Consent {
	if !ok {
		return nil
	}
	return &stored
}
