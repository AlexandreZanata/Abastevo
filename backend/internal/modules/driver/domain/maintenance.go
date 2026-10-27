package domain

import "time"

type MaintenanceSchedule struct {
	ID              string `json:"id"`
	Vehicle         string `json:"vehicle_id"`
	Version         int64  `json:"version"`
	Timezone        string `json:"timezone"`
	Kind            string `json:"kind"`
	Basis           string `json:"basis"`
	EffectiveFrom   string `json:"effective_from"`
	EffectiveUntil  string `json:"effective_until,omitempty"`
	IntervalMetres  *int64 `json:"interval_metres,omitempty"`
	IntervalDays    *int64 `json:"interval_days,omitempty"`
	OdometerSegment int64  `json:"odometer_segment"`
	BaselineMetres  *int64 `json:"baseline_metres,omitempty"`
	BaselineDay     string `json:"baseline_day,omitempty"`
	NextDueMetres   *int64 `json:"next_due_metres,omitempty"`
	NextDueDay      string `json:"next_due_day,omitempty"`
}

func (s MaintenanceSchedule) Normalize() (MaintenanceSchedule, error) {
	own := func(v *int64) *int64 {
		if v == nil {
			return nil
		}
		n := *v
		return &n
	}
	s.IntervalMetres = own(s.IntervalMetres)
	s.IntervalDays = own(s.IntervalDays)
	s.BaselineMetres = own(s.BaselineMetres)
	s.NextDueMetres = own(s.NextDueMetres)
	if e := s.validate(); e != nil {
		return s, e
	}
	return s, nil
}

func (s MaintenanceSchedule) validate() error {
	if !ValidID(s.ID) || !ValidID(s.Vehicle) || s.Version < 1 || s.Timezone == "" || s.Timezone == "Local" || s.OdometerSegment < 1 {
		return ErrInvalid
	}
	if _, e := time.LoadLocation(s.Timezone); e != nil {
		return ErrInvalid
	}
	switch s.Kind {
	case "OIL", "TIRES", "BRAKES", "FILTER", "INSPECTION", "OTHER":
	default:
		return ErrInvalid
	}
	if !validPeriod(s.EffectiveFrom, s.EffectiveUntil) || !validOptionalDay(s.BaselineDay) || !validOptionalDay(s.NextDueDay) {
		return ErrInvalid
	}
	for _, v := range []*int64{s.IntervalMetres, s.BaselineMetres, s.NextDueMetres} {
		if v != nil && !validAmount(*v) {
			return ErrInvalid
		}
	}
	if s.IntervalDays != nil && (*s.IntervalDays < 0 || *s.IntervalDays > 36500) {
		return ErrInvalid
	}
	switch s.Basis {
	case "ODOMETER":
		if s.IntervalMetres == nil {
			return ErrInvalid
		}
	case "DATE":
		if s.IntervalDays == nil {
			return ErrInvalid
		}
	case "ODOMETER_OR_DATE":
		if s.IntervalMetres == nil && s.IntervalDays == nil {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func validOptionalDay(day string) bool {
	if day == "" {
		return true
	}
	_, e := civil(day)
	return e == nil
}

func (s MaintenanceSchedule) ContentHash() (string, error) {
	normalized, e := s.Normalize()
	if e != nil {
		return "", e
	}
	return Digest(normalized)
}
