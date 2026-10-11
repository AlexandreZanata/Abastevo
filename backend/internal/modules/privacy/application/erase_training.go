package application

import (
	"context"
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

// TrainingInventory reads every owned derivative for erasure,
// active and tombstoned alike.
type TrainingInventory interface {
	PromotionInventory
	ListByOwner(ctx context.Context, contributorID string) ([]domain.Derivative, error)
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
	token, err := p.Attribution(ctx, contributorID)
	if err != nil {
		return 0, 0, err
	}
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	stored, err := p.Consent.Store.Get(ctx, contributorID)
	var revision uint64 = 1
	if err == nil {
		revision = stored.Revision + 1
	} else if !isNotFound(err) {
		return 0, 0, err
	}
	next, _, err := domain.RevokeConsent(ptrConsent(stored, err == nil), contributorID, token, revision, now)
	if err != nil {
		return 0, 0, err
	}
	if _, _, err := p.Consent.Store.Put(ctx, next); err != nil {
		return 0, 0, err
	}
	flipped, err := RevokeOwner(ctx, PromotePorts{
		ConsentPorts: p.Consent, Inventory: p.Inventory,
	}, Caller{ContributorID: contributorID})
	if err != nil {
		return 0, 0, err
	}
	rows, err := p.Inventory.ListByOwner(ctx, contributorID)
	if err != nil {
		return flipped, 0, err
	}
	purged := 0
	for _, d := range rows {
		key := TrainingObjectKey(d.ImageID, d.ConsentRevision, d.PromotedAt)
		if key == "" {
			return flipped, purged, domain.ErrInvalidRequest
		}
		exists, err := p.Objects.Exists(ctx, key)
		if err != nil {
			return flipped, purged, err
		}
		if !exists {
			continue
		}
		if err := p.Objects.Delete(ctx, key); err != nil {
			return flipped, purged, err
		}
		purged++
	}
	return flipped, purged, nil
}

// ptrConsent returns nil for absent consent (revocation baseline) or
// the stored row otherwise.
func ptrConsent(stored domain.Consent, ok bool) *domain.Consent {
	if !ok {
		return nil
	}
	return &stored
}
