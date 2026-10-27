package application

import (
	"context"
	"errors"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/driver/domain"
)

var ErrSignedProof = errors.New("driver: signed owner proof required")

type Proof struct {
	Signature   []byte
	RequestHash string
}

type Principal struct {
	Owner, Origin, Contributor, KeyID string
}

type Authority interface {
	Verify(context.Context, Proof) (Principal, error)
}

type DriverStore interface {
	EarningsStore
	ShiftStore
	DistanceStore
	CostProfileStore
	MaintenanceStore
	ReconciliationStore
}

type CommandService struct {
	Store     DriverStore
	Authority Authority
	Origin    string
}

func (s *CommandService) authorize(ctx context.Context, proof Proof, owner string) (Principal, error) {
	if s.Authority == nil || owner == "" || s.Origin == "" {
		return Principal{}, ErrSignedProof
	}
	principal, err := s.Authority.Verify(ctx, proof)
	if err != nil {
		return Principal{}, ErrSignedProof
	}
	if principal.Owner != owner || principal.Origin != s.Origin || principal.Contributor == "" || principal.KeyID == "" {
		return Principal{}, ErrSignedProof
	}
	return principal, nil
}

func (s *CommandService) SaveEarning(ctx context.Context, proof Proof, owner string, in SaveEarning) (Receipt, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return Receipt{}, err
	}
	return s.Store.Save(ctx, owner, in)
}

func (s *CommandService) AdjustEarning(ctx context.Context, proof Proof, owner string, in SaveAdjustment) (Receipt, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return Receipt{}, err
	}
	return s.Store.SaveAdjustment(ctx, owner, in)
}

func (s *CommandService) ApplyShift(ctx context.Context, proof Proof, owner string, in ApplyShift) (domain.ShiftTransition, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return domain.ShiftTransition{}, err
	}
	return s.Store.ApplyShift(ctx, owner, in)
}

func (s *CommandService) SaveDistance(ctx context.Context, proof Proof, owner string, in SaveDistance) (DistanceReceipt, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return DistanceReceipt{}, err
	}
	return s.Store.SaveDistance(ctx, owner, in)
}

func (s *CommandService) SaveCostProfile(ctx context.Context, proof Proof, owner string, in SaveCostProfile) (CostProfileReceipt, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return CostProfileReceipt{}, err
	}
	return s.Store.SaveCostProfile(ctx, owner, in)
}

func (s *CommandService) SaveMaintenance(ctx context.Context, proof Proof, owner string, in SaveMaintenance) (MaintenanceReceipt, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return MaintenanceReceipt{}, err
	}
	return s.Store.SaveMaintenance(ctx, owner, in)
}

func (s *CommandService) SaveReconciliation(ctx context.Context, proof Proof, owner string, in SaveReconciliation) (ReconciliationReceipt, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return ReconciliationReceipt{}, err
	}
	return s.Store.SaveReconciliation(ctx, owner, in)
}

func (s *CommandService) EraseEarning(ctx context.Context, proof Proof, owner, id string, revision, epoch int64) error {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return err
	}
	return s.Store.EraseRecord(ctx, owner, id, revision, epoch)
}

func (s *CommandService) EraseEarningOwner(ctx context.Context, proof Proof, owner string, epoch int64) error {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return err
	}
	return s.Store.EraseOwner(ctx, owner, epoch)
}

func (s *CommandService) EraseCostProfile(ctx context.Context, proof Proof, owner, vehicle string, version, epoch int64) error {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return err
	}
	return s.Store.EraseCostProfile(ctx, owner, vehicle, version, epoch)
}

func (s *CommandService) EraseCostOwner(ctx context.Context, proof Proof, owner string, epoch int64) error {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return err
	}
	return s.Store.EraseCostOwner(ctx, owner, epoch)
}

func (s *CommandService) EraseMaintenance(ctx context.Context, proof Proof, owner, id string, version, epoch int64) error {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return err
	}
	return s.Store.EraseMaintenance(ctx, owner, id, version, epoch)
}

func (s *CommandService) EraseMaintenanceOwner(ctx context.Context, proof Proof, owner string, epoch int64) error {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return err
	}
	return s.Store.EraseMaintenanceOwner(ctx, owner, epoch)
}

func (s *CommandService) GetEarning(ctx context.Context, proof Proof, owner, id string) (StoredEarning, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return StoredEarning{}, err
	}
	return s.Store.Get(ctx, owner, id)
}

func (s *CommandService) GetCostProfile(ctx context.Context, proof Proof, owner, vehicle string, version int64) (domain.VehicleCostProfile, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return domain.VehicleCostProfile{}, err
	}
	return s.Store.GetCostProfile(ctx, owner, vehicle, version)
}

func (s *CommandService) GetMaintenance(ctx context.Context, proof Proof, owner, id string, version int64) (domain.MaintenanceSchedule, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return domain.MaintenanceSchedule{}, err
	}
	return s.Store.GetMaintenance(ctx, owner, id, version)
}

func (s *CommandService) GetReconciliation(ctx context.Context, proof Proof, owner, id string, version int64) (domain.ReconciliationDecision, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return domain.ReconciliationDecision{}, err
	}
	return s.Store.GetReconciliation(ctx, owner, id, version)
}

func (s *CommandService) ListReconciliations(ctx context.Context, proof Proof, owner, after string, limit int) ([]domain.ReconciliationDecision, error) {
	if _, err := s.authorize(ctx, proof, owner); err != nil {
		return nil, err
	}
	return s.Store.ListReconciliations(ctx, owner, after, limit)
}
