package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// Revoked-copy purge (CS10C, AT05): derivatives whose consent was
// withdrawn must vanish from every byte copy within 24 hours of the
// revocation receipt. The sweeper lists revoked rows past the
// deadline, deletes their namespace objects, and confirms each copy
// is gone. Copies already absent are reported missing (observability,
// not failure — the SLO is trivially met for them); copies that
// resist deletion are failures for retry. Inventory rows stay
// REVOKED in all cases: receipts feed the restore filter, bytes are
// what purge owns. Deadline math uses the injected clock for tests;
// the wall-clock proof is the controlled soak, never a virtual-clock
// assertion.
const (
	// RevokedCopyTTL bounds revoked bytes: purge is due 24 hours
	// after the revocation receipt.
	RevokedCopyTTL = 24 * time.Hour
	// TrainingNamespace is the single private prefix derivative
	// objects land under. Keys never escape it.
	TrainingNamespace = "training/v1/"
)

// ObjectStore is the owned byte-copy port: existence checks bound
// their reads, deletes are idempotent.
type ObjectStore interface {
	Exists(ctx context.Context, key string) (bool, error)
	Delete(ctx context.Context, key string) error
}

// PurgePorts wires one sweep: the injected clock, the revoked-row
// inventory read and the byte-copy store.
type PurgePorts struct {
	Clock     func() time.Time
	Inventory RevokedInventory
	Objects   ObjectStore
}

// RevokedInventory reads revoked rows due for purge.
type RevokedInventory interface {
	ListRevokedBefore(ctx context.Context, before time.Time) ([]domain.Derivative, error)
}

// SweepReport is the auditable outcome: how many rows were due, how
// many copies demonstrably vanished, which keys were already gone,
// and which resisted deletion.
type SweepReport struct {
	Checked  int
	Purged   int
	Missing  []string
	Failures []string
}

// TrainingObjectKey derives the opaque object locator from image,
// revision and promotion date: date- and image-scoped, never
// owner-keyed. Empty output refuses traversal and zero revisions so
// no caller can smuggle keys outside the namespace.
func TrainingObjectKey(imageID string, revision uint64, at time.Time) string {
	imageID = strings.TrimSpace(imageID)
	if imageID == "" || revision == 0 || at.IsZero() ||
		strings.Contains(imageID, "..") || strings.Contains(imageID, "/") || strings.Contains(imageID, "\\") {
		return ""
	}
	return fmt.Sprintf("%s%s/%s-%d.jpg", TrainingNamespace, at.UTC().Format("2006-01-02"), imageID, revision)
}

// inventoriedObjectKey refuses corrupt/missing locators rather than deriving
// a replacement that could silently leave the committed output behind.
func inventoriedObjectKey(d domain.Derivative) string {
	expected := TrainingObjectKey(d.ImageID, d.ConsentRevision, d.PromotedAt)
	if expected == "" || d.ObjectKey != expected {
		return ""
	}
	return d.ObjectKey
}

// SweepRevoked purges due revoked copies: rows revoked longer ago
// than RevokedCopyTTL. Each due key is existence-checked, deleted,
// and confirmed gone; the report distinguishes purged, already
// missing and failed copies. A failed delete errors the sweep only
// through the report — rows stay REVOKED for the next pass, never
// resurrected, never dropped.
func SweepRevoked(ctx context.Context, p PurgePorts) (SweepReport, error) {
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	due, err := p.Inventory.ListRevokedBefore(ctx, now.Add(-RevokedCopyTTL))
	if err != nil {
		return SweepReport{}, err
	}
	report := SweepReport{}
	for _, d := range due {
		key := inventoriedObjectKey(d)
		if key == "" {
			report.Failures = append(report.Failures, d.ImageID)
			continue
		}
		report.Checked++
		exists, err := p.Objects.Exists(ctx, key)
		if err != nil {
			report.Failures = append(report.Failures, key)
			continue
		}
		if !exists {
			report.Missing = append(report.Missing, key)
			continue
		}
		if err := p.Objects.Delete(ctx, key); err != nil {
			report.Failures = append(report.Failures, key)
			continue
		}
		gone, err := p.Objects.Exists(ctx, key)
		if err != nil || gone {
			report.Failures = append(report.Failures, key)
			continue
		}
		report.Purged++
	}
	return report, nil
}
