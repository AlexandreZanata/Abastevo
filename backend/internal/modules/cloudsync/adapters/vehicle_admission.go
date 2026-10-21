package adapters

import (
	"context"
	"errors"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/cloudsync/domain"
	"github.com/jackc/pgx/v5"
)

// This state belongs to one owner-locked transaction, never the shared Store.
type vehicleAdmission struct{ multiple bool }

func (s *Store) checkMultipleVehicles(ctx context.Context, tx pgx.Tx, owner string) error {
	if s.MultipleVehicles == nil {
		return domain.ErrDenied
	}
	err := s.MultipleVehicles(ctx, tx, owner)
	if err == nil || errors.Is(err, domain.ErrDenied) || errors.Is(err, domain.ErrUnauthorized) {
		return err
	}
	return domain.ErrUnavailable
}

func (s *Store) admitRecord(ctx context.Context, tx pgx.Tx, owner, dataset string, count int64, admitted *vehicleAdmission) error {
	if dataset != "vehicles" {
		maximum := int64(1000)
		if dataset == "preferences" {
			maximum = 1
		}
		if count >= maximum {
			return domain.ErrQuota
		}
		return nil
	}
	err := domain.RequireVehicleRegistration(count, false)
	if !errors.Is(err, domain.ErrDenied) {
		return err
	}
	if err = s.checkMultipleVehicles(ctx, tx, owner); err != nil {
		return err
	}
	admitted.multiple = true
	return domain.RequireVehicleRegistration(count, true)
}

func (s *Store) recheckVehicleAdmission(ctx context.Context, tx pgx.Tx, owner string, admitted vehicleAdmission) error {
	if !admitted.multiple {
		return nil
	}
	return s.checkMultipleVehicles(ctx, tx, owner)
}
