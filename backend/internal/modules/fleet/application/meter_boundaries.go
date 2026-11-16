package application

import (
	"context"
	"slices"
	"time"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// MeterChange is a boundary command on the meter in service (EDR-313).
type MeterChange string

const (
	MeterReplace MeterChange = "REPLACE"
	MeterReset   MeterChange = "RESET"
	// MeterRollover names the boundary a CONFIRM_ROLLOVER decision records.
	MeterRollover MeterChange = "ROLLOVER"
)

// MeterChangeInput replaces or resets one meter (B-BR-EMP-484/485).
// ExpectedSegments is the meter's segment count (its revision for these
// commands). Declared is the boundary instant with the zone and offset of
// the readings it records.
type MeterChangeInput struct {
	private
	OperationID, AssetID, MeterID string
	ExpectedSegments              int64
	Change                        MeterChange
	Declared                      orgtime.Declaration
	// REPLACE: the final value (nil when unknown) and the new meter.
	Final   *meter.Value
	Label   string
	Modulus *int64
	Opening meter.Value
	// RESET: the values observed before and after.
	Before, After meter.Value
}
type MeterChangeCommand struct {
	private
	value MeterChangeInput
}

// MeterChangeIDs are the server identifiers of the rows a command may write.
type MeterChangeIDs struct {
	Segment, Meter, OpeningReading, RemovalReading string
}

func (ids MeterChangeIDs) valid() bool {
	all := []string{ids.Segment, ids.Meter, ids.OpeningReading, ids.RemovalReading}
	for i, id := range all {
		if !canonicalID(id) || slices.Contains(all[:i], id) {
			return false
		}
	}
	return true
}

func NewMeterChangeCommand(v MeterChangeInput) (MeterChangeCommand, error) {
	for _, id := range []string{v.OperationID, v.AssetID, v.MeterID} {
		if !canonicalID(id) {
			return MeterChangeCommand{}, meter.ErrInvalid
		}
	}
	if v.ExpectedSegments < 1 || v.ExpectedSegments >= 1_000_000 {
		return MeterChangeCommand{}, meter.ErrInvalid
	}
	declared, e := orgtime.ValidateDeclaration(v.Declared)
	if e != nil {
		return MeterChangeCommand{}, meter.ErrInvalid
	}
	v.Declared = declared
	switch v.Change {
	case MeterReplace:
		if !validMeterLabel(v.Label) || v.Before != (meter.Value{}) || v.After != (meter.Value{}) || v.Modulus != nil && *v.Modulus <= 0 {
			return MeterChangeCommand{}, meter.ErrInvalid
		}
		if _, e := meter.NewValue(v.Opening.Kind(), v.Opening.Count(), nil); e != nil {
			return MeterChangeCommand{}, meter.ErrInvalid
		}
		if v.Final != nil {
			if _, e := meter.NewValue(v.Final.Kind(), v.Final.Count(), nil); e != nil || v.Final.Kind() != v.Opening.Kind() {
				return MeterChangeCommand{}, meter.ErrInvalid
			}
			f := *v.Final
			v.Final = &f
		}
		if v.Modulus != nil {
			n := *v.Modulus
			v.Modulus = &n
		}
	case MeterReset:
		if v.Final != nil || v.Label != "" || v.Modulus != nil || v.Opening != (meter.Value{}) || v.Before.Kind() != v.After.Kind() {
			return MeterChangeCommand{}, meter.ErrInvalid
		}
		for _, value := range []meter.Value{v.Before, v.After} {
			if _, e := meter.NewValue(value.Kind(), value.Count(), nil); e != nil {
				return MeterChangeCommand{}, meter.ErrInvalid
			}
		}
	default:
		return MeterChangeCommand{}, meter.ErrInvalid
	}
	return MeterChangeCommand{value: v}, nil
}

func (c MeterChangeCommand) Snapshot() (MeterChangeInput, error) {
	copy, e := NewMeterChangeCommand(c.value)
	return copy.value, e
}

// Kind is the meter kind the command's values state.
func (in MeterChangeInput) Kind() meter.Kind {
	if in.Change == MeterReset {
		return in.Before.Kind()
	}
	return in.Opening.Kind()
}

// Digest binds every field of the command.
func (c MeterChangeCommand) Digest(scope kernel.OrgScope) string {
	in, e := c.Snapshot()
	if e != nil || scope.Validate() != nil {
		return ""
	}
	fields := []any{1, "meter.boundary", scope.OrgID().String(), in.OperationID, in.AssetID, in.MeterID, in.ExpectedSegments, in.Change, in.Kind(),
		orgtime.FormatInstant(in.Declared.OccurredAt), in.Declared.EntryZone, in.Declared.DeviceOffsetSeconds}
	if in.Change == MeterReplace {
		var final any
		if in.Final != nil {
			final = in.Final.Count()
		}
		fields = append(fields, final, in.Label, in.Modulus, in.Opening.Count())
	} else {
		fields = append(fields, in.Before.Count(), in.After.Count())
	}
	return meterIntentDigest(fields)
}

// MeterBoundaryResult is rebuilt from stored rows only, so a replay returns
// it. Close and Implied are nil when the final value is unknown; NewMeterID
// is set for a replacement; Segments is the closed meter's segment count.
type MeterBoundaryResult struct {
	private
	Change            MeterChange
	MeterID           string
	ClosedSegmentID   string
	At                time.Time
	Close             *int64
	ClosingReadingID  string
	PreviousReadingID string
	Implied           *int64
	NewMeterID        string
	OpenedSegmentID   string
	OpeningReadingID  string
	Open              int64
	Segments          int64
	Reevaluated       []MeterReadingState
}

func (r MeterBoundaryResult) Clone() MeterBoundaryResult {
	for _, p := range []**int64{&r.Close, &r.Implied} {
		if *p != nil {
			v := **p
			*p = &v
		}
	}
	out := make([]MeterReadingState, len(r.Reevaluated))
	for i, v := range r.Reevaluated {
		out[i] = v.Clone()
	}
	r.Reevaluated = out
	return r
}

func validBoundary(r MeterBoundaryResult, meterID string, change MeterChange) bool {
	if r.Change != change || r.MeterID != meterID || !canonicalID(r.ClosedSegmentID) || !canonicalID(r.PreviousReadingID) || !canonicalID(r.OpenedSegmentID) || !canonicalID(r.OpeningReadingID) ||
		r.At.IsZero() || (r.Close == nil) != (r.Implied == nil) || r.Implied != nil && *r.Implied < 0 || r.Open < 0 || r.Segments < 1 {
		return false
	}
	if (change == MeterReplace) != canonicalID(r.NewMeterID) || change != MeterReplace && r.Close == nil || r.ClosingReadingID != "" && (change != MeterReplace || !canonicalID(r.ClosingReadingID)) {
		return false
	}
	for _, v := range r.Reevaluated {
		if !validDecisionState(v) || v.State == meter.Voided || v.Revision < 1 {
			return false
		}
	}
	return true
}

// Change replaces or resets a meter under meter.reading.correct (EDR-313).
func (s MeterCorrectionService) Change(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command MeterChangeCommand) (MeterBoundaryResult, bool, error) {
	if !s.Enabled {
		return MeterBoundaryResult{}, false, ErrSurfaceDisabled
	}
	in, e := command.Snapshot()
	if e != nil {
		return MeterBoundaryResult{}, false, e
	}
	policy := MeterReviewPolicy(true)
	if e := authorized(ctx, scope, actor, decision, in.AssetID, policy); e != nil {
		return MeterBoundaryResult{}, false, e
	}
	if unavailable(s.Repository) || s.NewID == nil {
		return MeterBoundaryResult{}, false, ErrUnavailable
	}
	var ids MeterChangeIDs
	for _, p := range []*string{&ids.Segment, &ids.Meter, &ids.OpeningReading, &ids.RemovalReading} {
		if *p, e = s.NewID(); e != nil {
			return MeterBoundaryResult{}, false, ErrUnavailable
		}
	}
	if !ids.valid() {
		return MeterBoundaryResult{}, false, ErrUnavailable
	}
	result, replay, e := s.Repository.ChangeMeter(ctx, scope, actor, decision.Subject().MembershipID(), ids, command)
	if guard := authorized(ctx, scope, actor, decision, in.AssetID, policy); guard != nil {
		return MeterBoundaryResult{}, false, guard
	}
	if e != nil {
		return MeterBoundaryResult{}, false, e
	}
	if !validBoundary(result, in.MeterID, in.Change) {
		return MeterBoundaryResult{}, false, ErrUnavailable
	}
	return result.Clone(), replay, nil
}
