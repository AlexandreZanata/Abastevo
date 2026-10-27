package domain

import (
	"errors"
	"testing"
)

func maintenanceSchedule() MaintenanceSchedule {
	interval := int64(10_000_000)
	zero := int64(0)
	return MaintenanceSchedule{
		ID: "11111111-1111-4111-8111-111111111111", Vehicle: "22222222-2222-4222-8222-222222222222", Version: 1,
		Timezone: "America/Cuiaba", Kind: "OIL", Basis: "ODOMETER", EffectiveFrom: "2026-01-01",
		IntervalMetres: &interval, OdometerSegment: 1, BaselineMetres: &zero,
	}
}

func TestMaintenanceDistinguishesZeroAndUnknown(t *testing.T) {
	s := maintenanceSchedule()
	normalized, e := s.Normalize()
	if e != nil || normalized.BaselineMetres == nil || *normalized.BaselineMetres != 0 || normalized.NextDueMetres != nil {
		t.Fatalf("zero/unknown %+v %v", normalized, e)
	}
	zero := int64(0)
	s.NextDueMetres = &zero
	if normalized, e = s.Normalize(); e != nil || normalized.NextDueMetres == nil || *normalized.NextDueMetres != 0 {
		t.Fatal("known zero lost", e)
	}
	s.NextDueMetres = nil
	s.OdometerSegment = 2
	if normalized, e = s.Normalize(); e != nil || normalized.OdometerSegment != 2 {
		t.Fatal("reset segment lost", e)
	}
}

func TestMaintenanceRejectsMissingBasisAndInvalidPeriod(t *testing.T) {
	s := maintenanceSchedule()
	s.IntervalMetres = nil
	if _, e := s.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("odometer basis without interval", e)
	}
	s = maintenanceSchedule()
	s.Basis = "DATE"
	if _, e := s.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("date basis without interval", e)
	}
	days := int64(0)
	s.IntervalDays = &days
	if _, e := s.Normalize(); e != nil {
		t.Fatal("known zero interval day", e)
	}
	s = maintenanceSchedule()
	s.EffectiveUntil = s.EffectiveFrom
	if _, e := s.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("empty period", e)
	}
	s = maintenanceSchedule()
	s.Kind = "INVENTED"
	if _, e := s.Normalize(); !errors.Is(e, ErrInvalid) {
		t.Fatal("unknown kind", e)
	}
}
