package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/privacy/domain"
)

// InventoryReader assembles one owner's inventory for export. Rows
// carry their owner reference so Build can redact defensively:
// unrelated contributors are dropped, never exported (B-BR-011).
type InventoryReader interface {
	Snapshot(ctx context.Context, contributorID, contributorRef string) (RawInventory, error)
}

// RawInventory is the unredacted owner inventory: every row names its
// owner for the Build filter. Reader bugs that return foreign rows
// become dropped rows, never leaks.
type RawInventory struct {
	Contributor  ContributorView  `json:"contributor"`
	Observations []RawObservation `json:"observations"`
	Trust        RawTrust         `json:"trust"`
}

// RawObservation is one fact candidate with its owner reference.
type RawObservation struct {
	OwnerRef string          `json:"-"`
	View     ObservationView `json:"view"`
}

// RawTrust is the trust candidate with its owner reference.
type RawTrust struct {
	OwnerRef string `json:"-"`
	Tier     string `json:"tier"`
}

// Inventory is the redacted export payload: owner rows only, with
// owner references stripped. Evidence object metadata joins with the
// erasure inventory in P07-T04, which needs the same owner listing;
// only identity, community and trust sections ship in T03.
type Inventory struct {
	Contributor  ContributorView   `json:"contributor"`
	Observations []ObservationView `json:"observations"`
	Trust        TrustView         `json:"trust"`
}

// ContributorView carries the owner's identity profile: status and
// dates only, never keys or tokens.
type ContributorView struct {
	ContributorID string `json:"contributor_id"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

// ObservationView carries one owned fact with its derived validation
// state: prices and provenance, never other contributors' votes.
type ObservationView struct {
	ObservationID string `json:"observation_id"`
	StationID     string `json:"station_id"`
	Product       string `json:"product"`
	Unit          string `json:"unit"`
	AmountMilli   int64  `json:"amount_milli_brl"`
	Condition     string `json:"condition"`
	State         string `json:"validation_state"`
	ReceivedAt    string `json:"received_at"`
}

// TrustView carries the owner's current tier: the verdict, never other
// contributors' history.
type TrustView struct {
	Tier string `json:"tier"`
}

// Archive is the canonical export envelope: format version, owner,
// generation time and the redacted inventory. Struct field order keeps
// the bytes deterministic for the recorded hash.
type Archive struct {
	Format        string    `json:"format"`
	ContributorID string    `json:"contributor_id"`
	GeneratedAt   string    `json:"generated_at"`
	Inventory     Inventory `json:"inventory"`
}

// Build assembles and persists one READY archive for a REQUESTED
// request. It snapshots the inventory, redacts foreign rows, and
// completes through the guarded transition. Terminal (non-REQUESTED)
// requests replay as no-ops so duplicate deliveries converge;
// inventory failures mark FAILED with the bounded reason.
func Build(ctx context.Context, p Ports, requestID string) (string, bool, error) {
	now := time.Now()
	if p.Clock != nil {
		now = p.Clock()
	}
	req, err := p.Store.Get(ctx, requestID)
	if err != nil {
		return "", false, err
	}
	if req.Status != domain.StatusRequested {
		return req.ID, true, nil
	}
	inv, err := p.Inventory.Snapshot(ctx, req.ContributorID, req.ContributorRef)
	if err != nil {
		if ferr := p.Store.FailExport(ctx, req.ID, now); ferr != nil {
			return "", false, ferr
		}
		return req.ID, false, err
	}
	archive, err := json.Marshal(Archive{
		Format: domain.FormatV1, ContributorID: req.ContributorID,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Inventory:   redact(inv, req),
	})
	if err != nil {
		return "", false, err
	}
	if len(archive) > domain.MaxArchiveBytes {
		if ferr := p.Store.FailExport(ctx, req.ID, now); ferr != nil {
			return "", false, ferr
		}
		return req.ID, false, domain.ErrTooLarge
	}
	sum := sha256.Sum256(archive)
	completed, _, err := domain.Complete(req, archive, hex.EncodeToString(sum[:]), now)
	if err != nil {
		return "", false, err
	}
	if err := p.Store.CompleteExport(ctx, completed.ID, archive, completed.ArchiveSHA256, now); err != nil {
		return "", false, err
	}
	return completed.ID, false, nil
}

// redact keeps only rows owned by the requesting contributor and
// strips owner references from the survivors. A reader that returns
// foreign rows produces dropped rows, never exported data.
func redact(raw RawInventory, req domain.Request) Inventory {
	out := Inventory{Trust: TrustView{Tier: raw.Trust.Tier}}
	if raw.Trust.OwnerRef != "" && raw.Trust.OwnerRef != req.ContributorRef {
		out.Trust = TrustView{}
	}
	if raw.Contributor.ContributorID == req.ContributorID {
		out.Contributor = raw.Contributor
	} else {
		out.Contributor = ContributorView{ContributorID: req.ContributorID}
	}
	for _, o := range raw.Observations {
		if o.OwnerRef != req.ContributorRef {
			continue
		}
		out.Observations = append(out.Observations, o.View)
	}
	if out.Observations == nil {
		out.Observations = []ObservationView{}
	}
	return out
}
