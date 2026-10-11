package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// Revoked-copy purge (CS10C, AT05): derivatives whose consent was
// withdrawn must vanish from every byte copy within 24 hours of the
// revocation receipt. The sweeper starts at revocation, deletes the
// namespace objects in bounded batches, and confirms each copy
// is gone. Copies already absent are reported missing (observability,
// not failure — the SLO is trivially met for them); copies that
// resist deletion are failures for retry. Inventory rows stay
// REVOKED in all cases: receipts feed the restore filter, bytes are
// what purge owns. Deadline math uses the injected clock for tests;
// the wall-clock proof is the controlled soak, never a virtual-clock
// assertion.
const (
	// RevokedCopyTTL is the outer physical-deletion deadline. Cleanup
	// starts immediately, with scheduler/backlog margin before this bound.
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

// PurgeClaim is one short-lived, private cleanup authority. Never log its
// token or locator; the receipt is matched atomically on completion.
type PurgeClaim struct {
	Derivative domain.Derivative
	Token      string
}

// RevokedInventory atomically claims one due/abandoned output. Purged work is
// excluded at the database boundary; an old worker cannot acknowledge a new claim.
type RevokedInventory interface {
	ClaimPurge(context.Context, time.Time) (PurgeClaim, bool, error)
	CompletePurge(context.Context, PurgeClaim, time.Time) error
}

const MaxTrainingPurgeBatch = 32

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

// SweepRevoked disposes at most 32 revoked/abandoned copies immediately.
// The 24h deadline is an upper bound, not a delay before work starts.
func SweepRevoked(ctx context.Context, p PurgePorts) (SweepReport, error) {
	if p.Inventory == nil || p.Objects == nil {
		return SweepReport{}, errors.New("training purge: unconfigured")
	}
	run, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	clock := p.Clock
	if clock == nil {
		clock = time.Now
	}
	report := SweepReport{}
	for i := 0; i < MaxTrainingPurgeBatch; i++ {
		claim, ok, err := p.Inventory.ClaimPurge(run, clock())
		if err != nil {
			return report, err
		}
		if !ok {
			break
		}
		report.Checked++
		key := inventoriedObjectKey(claim.Derivative)
		if key == "" {
			report.Failures = append(report.Failures, "invalid-locator")
			continue
		}
		item, stop := context.WithTimeout(run, 5*time.Second)
		existed, err := p.Objects.Exists(item, key)
		if err == nil && existed {
			err = p.Objects.Delete(item, key)
		}
		if err == nil {
			var remains bool
			remains, err = p.Objects.Exists(item, key)
			if err == nil && remains {
				err = errors.New("training purge: bytes remain")
			}
		}
		if err == nil {
			err = p.Inventory.CompletePurge(item, claim, clock())
		}
		stop()
		if err != nil {
			report.Failures = append(report.Failures, key)
			continue
		}
		if existed {
			report.Purged++
		} else {
			report.Missing = append(report.Missing, key)
		}
	}
	if len(report.Failures) > 0 {
		return report, errors.New("training purge: unresolved copies")
	}
	return report, nil
}
