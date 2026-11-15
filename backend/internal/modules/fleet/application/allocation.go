package application

import (
	"context"
	"errors"
	"time"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	syncapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/application"
	"github.com/jackc/pgx/v5"
)

// ErrUnallocated reports an instant that no unit period of the asset covers
// (a DRAFT asset, or an instant before activation). It is never a guessed unit.
var ErrUnallocated = errors.New("fleet: no unit allocation covers the instant")

// AllocationHistory selects one of the two EMP-FLEET-005 period histories.
type AllocationHistory string

const (
	UnitAllocationHistory       AllocationHistory = "UNIT"
	CostCentreAllocationHistory AllocationHistory = "COST_CENTRE"
)

func (h AllocationHistory) valid() bool {
	return h == UnitAllocationHistory || h == CostCentreAllocationHistory
}

// AllocationEntry is one stored period: the unit or cost centre, its exact
// half-open bounds and the database instant at which it was recorded.
type AllocationEntry struct {
	private
	TargetID   string
	Period     fleet.AllocationPeriod
	RecordedAt time.Time
}

func (e AllocationEntry) clone() AllocationEntry {
	if e.Period.To != nil {
		end := *e.Period.To
		e.Period.To = &end
	}
	return e
}

// AllocationFilter is a start keyset: After is the start of the last period
// already returned, or nil for the first page.
type AllocationFilter struct {
	private
	After *time.Time
	Limit int
}

func (f AllocationFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 {
		return fleet.ErrInvalid
	}
	if f.After != nil {
		if _, err := fleet.NewAllocationPeriod(*f.After, nil); err != nil {
			return fleet.ErrInvalid
		}
	}
	return nil
}

type AllocationPage struct {
	private
	Entries []AllocationEntry
	Next    *time.Time
}

type AllocationHistoryRepository interface {
	ListAssetAllocations(context.Context, kernel.OrgScope, FleetActor, string, string, AllocationHistory, AllocationFilter) (AllocationPage, error)
}

type AllocationHistoryService struct {
	Enabled    bool
	Repository AllocationHistoryRepository
}

// validPage mirrors the database invariants on one returned page: canonical
// targets, recorded instants, start order after the keyset, no overlap, an
// open period only last, no gap in a unit history and an exact next keyset.
func validPage(v AllocationPage, history AllocationHistory, filter AllocationFilter) bool {
	if len(v.Entries) > filter.Limit {
		return false
	}
	periods := make([]fleet.AllocationPeriod, 0, len(v.Entries))
	for _, entry := range v.Entries {
		if !canonicalID(entry.TargetID) || !validInstant(entry.RecordedAt) {
			return false
		}
		periods = append(periods, entry.Period)
	}
	if fleet.ValidAllocationPage(periods, history == UnitAllocationHistory) != nil {
		return false
	}
	if len(periods) > 0 && filter.After != nil && !periods[0].From.After(*filter.After) {
		return false
	}
	if v.Next != nil {
		last := len(periods) - 1
		return len(periods) == filter.Limit && periods[last].To != nil && v.Next.Equal(periods[last].From)
	}
	return true
}

// History reads one allocation history of one asset under current
// fleet.asset.read for that asset (BUC-EMP-114).
func (s AllocationHistoryService) History(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, assetID string, history AllocationHistory, filter AllocationFilter) (AllocationPage, error) {
	if !s.Enabled {
		return AllocationPage{}, ErrSurfaceDisabled
	}
	if !canonicalID(assetID) || !history.valid() {
		return AllocationPage{}, fleet.ErrInvalid
	}
	if err := filter.Validate(); err != nil {
		return AllocationPage{}, err
	}
	if err := current(ctx, scope, actor, decision, assetID, false); err != nil {
		return AllocationPage{}, err
	}
	if unavailable(s.Repository) {
		return AllocationPage{}, ErrUnavailable
	}
	v, err := s.Repository.ListAssetAllocations(ctx, scope, actor, decision.Subject().MembershipID(), assetID, history, filter)
	if guard := current(ctx, scope, actor, decision, assetID, false); guard != nil {
		return AllocationPage{}, guard
	}
	if err != nil {
		return AllocationPage{}, err
	}
	if !validPage(v, history, filter) {
		return AllocationPage{}, ErrUnavailable
	}
	out := AllocationPage{Entries: make([]AllocationEntry, 0, len(v.Entries))}
	for _, entry := range v.Entries {
		out.Entries = append(out.Entries, entry.clone())
	}
	if v.Next != nil {
		next := *v.Next
		out.Next = &next
	}
	return out, nil
}

// AllocationSnapshot is the answer of the snapshot port (B-BR-EMP-226): the
// unit and, when one covers the instant, the cost centre. CostCentreID is
// empty when none does; it is never derived from the unit.
type AllocationSnapshot struct {
	private
	AssetID      string
	At           time.Time
	UnitID       string
	CostCentreID string
}

// NewAllocationSnapshot validates canonical identifiers and floors the
// instant to the microsecond precision of every stored boundary.
func NewAllocationSnapshot(asset string, at time.Time, unit, costCentre string) (AllocationSnapshot, error) {
	instant, err := fleet.AllocationInstant(at)
	if err != nil || !canonicalID(asset) || !canonicalID(unit) || costCentre != "" && !canonicalID(costCentre) {
		return AllocationSnapshot{}, fleet.ErrInvalid
	}
	return AllocationSnapshot{AssetID: asset, At: instant, UnitID: unit, CostCentreID: costCentre}, nil
}

// AllocationSnapshotPort answers "unit and cost centre of this asset at this
// instant" for modules creating a record (BUC-EMP-115). The Tx form reads in
// the caller's organization transaction without taking locks; a caller that
// must serialize against a reallocation holds the organization root first.
type AllocationSnapshotPort interface {
	UnitAndCostCentreAt(context.Context, kernel.OrgScope, string, time.Time) (AllocationSnapshot, error)
	UnitAndCostCentreAtTx(context.Context, pgx.Tx, kernel.OrgScope, string, time.Time) (AllocationSnapshot, error)
	// UnitAndCostCentreAtDBTX is the same answer on a borrowed organization
	// port, for signed intake owners that never hold a pgx.Tx (EMP-FUEL-002).
	UnitAndCostCentreAtDBTX(context.Context, syncapp.DBTX, kernel.OrgScope, string, time.Time) (AllocationSnapshot, error)
}
