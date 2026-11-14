package meter

import (
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"sort"
	"time"
)

type Series struct {
	redacted
	segment  SegmentRef
	opening  ReadingRef
	readings []Reading
	facts    []Fact
}

func NewSeries(scope kernel.OrgScope, segment SegmentRef, opening Reading) (Series, error) {
	if err := segment.validate(scope); err != nil {
		return Series{}, err
	}
	if _, err := NewReading(scope, opening.Snapshot()); err != nil {
		return Series{}, err
	}
	if opening.input.Reference.segment != segment {
		return Series{}, ErrScope
	}
	return Series{segment: segment, opening: opening.input.Reference, readings: []Reading{opening}}, nil
}
func (s Series) validate(scope kernel.OrgScope) error {
	if err := s.segment.validate(scope); err != nil {
		return err
	}
	if len(s.readings) == 0 {
		return ErrInvalid
	}
	return nil
}
func (s Series) operationUsed(id string) bool {
	for _, r := range s.readings {
		if r.input.Operation.id == id {
			return true
		}
	}
	for _, f := range s.facts {
		if f.input.Operation.id == id {
			return true
		}
	}
	return false
}
func (s Series) idUsed(id string) bool {
	for _, r := range s.readings {
		if r.input.Reference.id == id {
			return true
		}
	}
	for _, f := range s.facts {
		if f.input.ID == id {
			return true
		}
	}
	return false
}
func (s Series) checkReading(scope kernel.OrgScope, r Reading) error {
	if _, err := NewReading(scope, r.Snapshot()); err != nil {
		return err
	}
	if r.input.Reference.segment != s.segment {
		return ErrScope
	}
	if s.idUsed(r.input.Reference.id) {
		return ErrInvalid
	}
	if s.operationUsed(r.input.Operation.id) {
		return ErrOperationReused
	}
	return nil
}
func (s Series) Append(scope kernel.OrgScope, r Reading) (Series, error) {
	if err := s.validate(scope); err != nil {
		return Series{}, err
	}
	if err := s.checkReading(scope, r); err != nil {
		return Series{}, err
	}
	s.readings = append(append([]Reading(nil), s.readings...), r)
	return s, nil
}
func (s Series) State(scope kernel.OrgScope, target ReadingRef) (State, error) {
	if err := s.validate(scope); err != nil {
		return "", err
	}
	if err := target.validate(scope); err != nil {
		return "", err
	}
	if target.segment != s.segment {
		return "", ErrScope
	}
	for _, r := range s.readings {
		if r.input.Reference == target {
			for _, f := range s.facts {
				if f.target == target {
					return Voided, nil
				}
			}
			return r.input.State, nil
		}
	}
	return "", ErrInvalid
}
func (s Series) checkFact(scope kernel.OrgScope, target ReadingRef, fact FactInput) error {
	if err := s.validate(scope); err != nil {
		return err
	}
	state, err := s.State(scope, target)
	if err != nil {
		return err
	}
	if state == Voided {
		return ErrAlreadySuperseded
	}
	if err = fact.validate(scope); err != nil {
		return err
	}
	if s.idUsed(fact.ID) {
		return ErrInvalid
	}
	if s.operationUsed(fact.Operation.id) {
		return ErrOperationReused
	}
	return nil
}
func (s Series) Void(scope kernel.OrgScope, target ReadingRef, fact FactInput) (Series, error) {
	if err := s.checkFact(scope, target, fact); err != nil {
		return Series{}, err
	}
	fact.ReceivedAt = fact.ReceivedAt.UTC()
	s.facts = append(append([]Fact(nil), s.facts...), Fact{input: fact, target: target})
	return s, nil
}
func (s Series) Supersede(scope kernel.OrgScope, target ReadingRef, replacement Reading, fact FactInput) (Series, error) {
	if err := s.checkFact(scope, target, fact); err != nil {
		return Series{}, err
	}
	if err := s.checkReading(scope, replacement); err != nil {
		return Series{}, err
	}
	if replacement.input.Operation != fact.Operation || replacement.input.Reference.id == fact.ID {
		return Series{}, ErrInvalid
	}
	r := replacement.input.Reference
	fact.ReceivedAt = fact.ReceivedAt.UTC()
	s.readings = append(append([]Reading(nil), s.readings...), replacement)
	s.facts = append(append([]Fact(nil), s.facts...), Fact{input: fact, target: target, replacement: &r})
	return s, nil
}
func less(a, b Reading) bool {
	ad, bd := a.input.Declared.OccurredAt, b.input.Declared.OccurredAt
	if !ad.Equal(bd) {
		return ad.Before(bd)
	}
	ar, br := a.input.ReceivedAt, b.input.ReceivedAt
	if !ar.Equal(br) {
		return ar.Before(br)
	}
	return a.input.Reference.id < b.input.Reference.id
}
func (s Series) Ordered(scope kernel.OrgScope) ([]Reading, error) {
	if err := s.validate(scope); err != nil {
		return nil, err
	}
	out := append([]Reading(nil), s.readings...)
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out, nil
}
func (s Series) Facts(scope kernel.OrgScope) ([]Fact, error) {
	if err := s.validate(scope); err != nil {
		return nil, err
	}
	return append([]Fact(nil), s.facts...), nil
}
func validInstant(at time.Time) bool { return at.UTC().Year() >= 1 && at.UTC().Year() <= 9999 }
func (s Series) Latest(scope kernel.OrgScope, at time.Time) (*Reading, error) {
	if err := s.validate(scope); err != nil {
		return nil, err
	}
	if !validInstant(at) {
		return nil, ErrInvalid
	}
	var best *Reading
	for _, r := range s.readings {
		if r.input.Declared.OccurredAt.After(at) {
			continue
		}
		state, err := s.State(scope, r.input.Reference)
		if err != nil {
			return nil, err
		}
		if state == Valid && (best == nil || less(*best, r)) {
			copy := r
			best = &copy
		}
	}
	return best, nil
}

type Usage struct {
	redacted
	kind     Kind
	from, to *Reading
	delta    int64
	reason   string
}

func (s Series) Usage(scope kernel.OrgScope, from, to time.Time) (Usage, error) {
	if !validInstant(from) || !validInstant(to) || from.After(to) {
		return Usage{}, ErrInvalid
	}
	a, err := s.Latest(scope, from)
	if err != nil {
		return Usage{}, err
	}
	b, err := s.Latest(scope, to)
	if err != nil {
		return Usage{}, err
	}
	u := Usage{kind: s.segment.meter.kind, from: a, to: b}
	if a == nil || b == nil {
		u.reason = "NO_READING_BEFORE"
		return u, nil
	}
	u.delta = b.input.Value.count - a.input.Value.count
	if u.delta < 0 {
		return Usage{}, ErrInvalid
	}
	return u, nil
}
func (u Usage) Kind() Kind { return u.kind }
func (u Usage) From() *Reading {
	if u.from == nil {
		return nil
	}
	r := *u.from
	return &r
}
func (u Usage) To() *Reading {
	if u.to == nil {
		return nil
	}
	r := *u.to
	return &r
}
func (u Usage) Delta() (int64, bool) { return u.delta, u.reason == "" && bound(u.kind) >= 0 }
func (u Usage) Reason() string       { return u.reason }
