package application

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
	"time"
)

var ErrDenied = errors.New("driver: owner or vehicle denied")
var ErrNotFound = errors.New("driver: record not found")
var ErrErased = errors.New("driver: erased or obsolete epoch")
var ErrUnavailable = errors.New("driver: storage unavailable")

type SaveEarning struct {
	OperationID                     string
	ExpectedRevision, ExpectedEpoch int64
	Record                          domain.EarningRecord
}
type SaveAdjustment struct {
	OperationID, SourceHash string
	ExpectedEpoch           int64
	Adjustment              domain.Adjustment
}
type Receipt struct {
	ID              string
	Revision, Epoch int64
	Replayed        bool
}
type StoredAdjustment struct {
	SourceHash string
	Adjustment domain.Adjustment
}
type StoredEarning struct {
	Record              domain.EarningRecord
	Revision            int64
	AdjustmentCount     int64
	NeedsReconciliation bool
}
type EarningVersion struct {
	Record   domain.EarningRecord
	Revision int64
}
type RevisionPage struct {
	Revisions    []EarningVersion
	NextRevision int64
}
type AdjustmentPage struct {
	Adjustments []StoredAdjustment
	NextID      string
}
type Page struct {
	Records []StoredEarning
	NextID  string
}

// Storage is private infrastructure. Signed authority, Premium and HTTP admission
// are composed in DRV-10–12, not simulated by this port.
type EarningsStore interface {
	Save(context.Context, string, SaveEarning) (Receipt, error)
	SaveAdjustment(context.Context, string, SaveAdjustment) (Receipt, error)
	Get(context.Context, string, string) (StoredEarning, error)
	List(context.Context, string, string, int) (Page, error)
	ListAdjustments(context.Context, string, string, string, int) (AdjustmentPage, error)
	ListRevisions(context.Context, string, string, int64, int) (RevisionPage, error)
	EraseRecord(context.Context, string, string, int64, int64) error
	EraseOwner(context.Context, string, int64) error
}

type ShiftStart struct {
	ID, Origin, Vehicle, Timezone string
}
type ApplyShift struct {
	ShiftID string
	Create  *ShiftStart
	Command domain.ShiftCommand
	Policy  domain.ClockPolicy
}
type SaveDistance struct {
	OperationID string
	Record      domain.DistanceRecord
}
type DistanceReceipt struct {
	ID, ContentHash string
	Replayed        bool
}
type ShiftStore interface {
	ApplyShift(context.Context, string, ApplyShift) (domain.ShiftTransition, error)
	GetShift(context.Context, string, string) (domain.Shift, error)
	CurrentShift(context.Context, string) (domain.Shift, error)
}
type DistanceStore interface {
	SaveDistance(context.Context, string, SaveDistance) (DistanceReceipt, error)
	GetDistance(context.Context, string, string) (domain.DistanceRecord, error)
	ListDistances(context.Context, string, string, time.Time, time.Time, int) ([]domain.DistanceRecord, error)
}

type SaveCostProfile struct {
	OperationID                    string
	ExpectedVersion, ExpectedEpoch int64
	Profile                        domain.VehicleCostProfile
}
type CostProfileReceipt struct {
	Vehicle  string
	Version  int64
	Replayed bool
}
type CostProfileStore interface {
	SaveCostProfile(context.Context, string, SaveCostProfile) (CostProfileReceipt, error)
	GetCostProfile(context.Context, string, string, int64) (domain.VehicleCostProfile, error)
	ListCostProfiles(context.Context, string, string, int64, int) ([]domain.VehicleCostProfile, error)
	EraseCostProfile(context.Context, string, string, int64, int64) error
	EraseCostOwner(context.Context, string, int64) error
}
type SaveMaintenance struct {
	OperationID                    string
	ExpectedVersion, ExpectedEpoch int64
	Schedule                       domain.MaintenanceSchedule
}
type MaintenanceReceipt struct {
	ScheduleID string
	Version    int64
	Replayed   bool
}
type MaintenanceStore interface {
	SaveMaintenance(context.Context, string, SaveMaintenance) (MaintenanceReceipt, error)
	GetMaintenance(context.Context, string, string, int64) (domain.MaintenanceSchedule, error)
	ListMaintenance(context.Context, string, string, int64, int) ([]domain.MaintenanceSchedule, error)
	EraseMaintenance(context.Context, string, string, int64, int64) error
	EraseMaintenanceOwner(context.Context, string, int64) error
}

type SaveReconciliation struct {
	OperationID                    string
	ExpectedVersion, ExpectedEpoch int64
	Decision                       domain.ReconciliationDecision
}
type ReconciliationReceipt struct {
	ID       string
	Version  int64
	Replayed bool
}
type ReconciliationStore interface {
	SaveReconciliation(context.Context, string, SaveReconciliation) (ReconciliationReceipt, error)
	GetReconciliation(context.Context, string, string, int64) (domain.ReconciliationDecision, error)
	ListReconciliations(context.Context, string, string, int) ([]domain.ReconciliationDecision, error)
}
