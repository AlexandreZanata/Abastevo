package application

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	access "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaccess/application"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// EMP-METER-004 refusals beyond the shared fleet errors (EDR-187, EDR-189).
var (
	ErrReadingAlreadySuperseded = errors.New("fleet: reading already corrected or voided")
	ErrAmendmentRequired        = errors.New("fleet: approved parent record requires an amendment")
)

// MeterDecision is one person decision on one reading (B-BR-EMP-286..288).
type MeterDecision string

const (
	MeterSupersede MeterDecision = "SUPERSEDE"
	MeterVoid      MeterDecision = "VOID"
	MeterConfirm   MeterDecision = "CONFIRM"
)

type MeterDecisionInput struct {
	private
	OperationID, AssetID, MeterID, ReadingID string
	ExpectedRevision                         int64
	Decision                                 MeterDecision
	Reason                                   string
	// SUPERSEDE only: the corrected value and an optional corrected
	// declaration (nil keeps the original's).
	Value    meter.Value
	Declared *orgtime.Declaration
}
type MeterDecisionCommand struct {
	private
	value MeterDecisionInput
}

func validMeterReason(raw string) bool {
	if !utf8.ValidString(raw) || strings.TrimSpace(raw) == "" || utf8.RuneCountInString(raw) > 280 {
		return false
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func NewMeterDecisionCommand(v MeterDecisionInput) (MeterDecisionCommand, error) {
	for _, id := range []string{v.OperationID, v.AssetID, v.MeterID, v.ReadingID} {
		if !canonicalID(id) {
			return MeterDecisionCommand{}, meter.ErrInvalid
		}
	}
	if v.ExpectedRevision < 0 || v.ExpectedRevision >= 1_000_000 || !validMeterReason(v.Reason) {
		return MeterDecisionCommand{}, meter.ErrInvalid
	}
	switch v.Decision {
	case MeterSupersede:
		if _, e := meter.NewValue(v.Value.Kind(), v.Value.Count(), nil); e != nil {
			return MeterDecisionCommand{}, meter.ErrInvalid
		}
		if v.Declared != nil {
			declared, e := orgtime.ValidateDeclaration(*v.Declared)
			if e != nil {
				return MeterDecisionCommand{}, meter.ErrInvalid
			}
			v.Declared = &declared
		}
	case MeterVoid, MeterConfirm:
		if v.Value != (meter.Value{}) || v.Declared != nil {
			return MeterDecisionCommand{}, meter.ErrInvalid
		}
	default:
		return MeterDecisionCommand{}, meter.ErrInvalid
	}
	return MeterDecisionCommand{value: v}, nil
}
func (c MeterDecisionCommand) Snapshot() (MeterDecisionInput, error) {
	in := c.value
	if in.Declared != nil {
		d := *in.Declared
		in.Declared = &d
	}
	copy, e := NewMeterDecisionCommand(in)
	return copy.value, e
}

// Digest binds every field of the decision; the reason travels only inside it.
func (c MeterDecisionCommand) Digest(scope kernel.OrgScope) string {
	in, e := c.Snapshot()
	if e != nil || scope.Validate() != nil {
		return ""
	}
	fields := []any{1, "meter.reading.decide", scope.OrgID().String(), in.OperationID, in.AssetID, in.MeterID, in.ReadingID, in.ExpectedRevision, in.Decision, in.Reason}
	if in.Decision == MeterSupersede {
		fields = append(fields, in.Value.Kind(), in.Value.Count())
		if in.Declared != nil {
			fields = append(fields, orgtime.FormatInstant(in.Declared.OccurredAt), in.Declared.EntryZone, in.Declared.DeviceOffsetSeconds)
		}
	}
	return meterIntentDigest(fields)
}

// MeterReadingState is one stored current state after a decision.
type MeterReadingState struct {
	private
	ReadingID string
	State     meter.State
	Flags     []meter.Flag
	Revision  int64
}

func (s MeterReadingState) Clone() MeterReadingState {
	s.Flags = slices.Clone(s.Flags)
	return s
}

// MeterDecisionResult is rebuilt from stored rows only, so a replay returns it.
type MeterDecisionResult struct {
	private
	Target      MeterReadingState
	Replacement *MeterReadingState
	Reevaluated []MeterReadingState
}

func (r MeterDecisionResult) Clone() MeterDecisionResult {
	r.Target = r.Target.Clone()
	if r.Replacement != nil {
		v := r.Replacement.Clone()
		r.Replacement = &v
	}
	out := make([]MeterReadingState, len(r.Reevaluated))
	for i, v := range r.Reevaluated {
		out[i] = v.Clone()
	}
	r.Reevaluated = out
	return r
}

// MeterHistoryItem is one reading of the open segment with its history
// (B-BR-EMP-291). Previous and Next are filled for the suspect list only.
type MeterHistoryItem struct {
	private
	Reading      meter.Reading
	State        meter.State
	Flags        []meter.Flag
	Retroactive  bool
	Revision     int64
	SupersededBy string
	Supersedes   string
	Reason       string
	Previous     string
	Next         string
}

func (v MeterHistoryItem) Clone() MeterHistoryItem {
	v.Flags = slices.Clone(v.Flags)
	return v
}

type MeterHistoryFilter struct {
	private
	AfterID string
	Limit   int
	State   meter.State
}

func (f MeterHistoryFilter) Validate() error {
	if f.Limit < 1 || f.Limit > 100 || f.AfterID != "" && !canonicalID(f.AfterID) || f.State != "" && f.State != meter.Valid && f.State != meter.Suspect && f.State != meter.Voided {
		return meter.ErrInvalid
	}
	return nil
}

type MeterHistoryPage struct {
	private
	Items  []MeterHistoryItem
	NextID string
	At     time.Time
}

// MeterCorrectionRepository is the EMP-METER-004 store port. The bool of the
// history read selects the suspect list (current SUSPECT with neighbours).
type MeterCorrectionRepository interface {
	DecideMeterReading(context.Context, kernel.OrgScope, FleetActor, string, string, MeterDecisionCommand) (MeterDecisionResult, bool, error)
	MeterReadingHistory(context.Context, kernel.OrgScope, FleetActor, string, string, string, MeterHistoryFilter, bool) (MeterHistoryPage, error)
}

type MeterCorrectionService struct {
	Enabled    bool
	Repository MeterCorrectionRepository
	NewID      func() (string, error)
}

func validDecisionState(v MeterReadingState) bool {
	if !canonicalID(v.ReadingID) || v.Revision < 0 || (v.State != meter.Valid && v.State != meter.Suspect && v.State != meter.Voided) || (v.State == meter.Suspect) != (len(v.Flags) > 0) {
		return false
	}
	return true
}

func (s MeterCorrectionService) Decide(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, command MeterDecisionCommand) (MeterDecisionResult, bool, error) {
	if !s.Enabled {
		return MeterDecisionResult{}, false, ErrSurfaceDisabled
	}
	in, e := command.Snapshot()
	if e != nil {
		return MeterDecisionResult{}, false, e
	}
	policy := MeterReviewPolicy(true)
	if e := authorized(ctx, scope, actor, decision, in.AssetID, policy); e != nil {
		return MeterDecisionResult{}, false, e
	}
	if unavailable(s.Repository) || s.NewID == nil {
		return MeterDecisionResult{}, false, ErrUnavailable
	}
	id, e := s.NewID()
	if e != nil || !canonicalID(id) {
		return MeterDecisionResult{}, false, ErrUnavailable
	}
	result, replay, e := s.Repository.DecideMeterReading(ctx, scope, actor, decision.Subject().MembershipID(), id, command)
	if guard := authorized(ctx, scope, actor, decision, in.AssetID, policy); guard != nil {
		return MeterDecisionResult{}, false, guard
	}
	if e != nil {
		return MeterDecisionResult{}, false, e
	}
	want := map[MeterDecision]meter.State{MeterSupersede: meter.Voided, MeterVoid: meter.Voided, MeterConfirm: meter.Valid}[in.Decision]
	if result.Target.ReadingID != in.ReadingID || result.Target.State != want || result.Target.Revision != in.ExpectedRevision+1 || !validDecisionState(result.Target) || (result.Replacement != nil) != (in.Decision == MeterSupersede) {
		return MeterDecisionResult{}, false, ErrUnavailable
	}
	if result.Replacement != nil && (!validDecisionState(*result.Replacement) || result.Replacement.State == meter.Voided || result.Replacement.ReadingID == in.ReadingID) {
		return MeterDecisionResult{}, false, ErrUnavailable
	}
	for _, r := range result.Reevaluated {
		if !validDecisionState(r) || r.State == meter.Voided || r.Revision < 1 {
			return MeterDecisionResult{}, false, ErrUnavailable
		}
	}
	return result.Clone(), replay, nil
}

func (s MeterCorrectionService) read(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, assetID, meterID string, filter MeterHistoryFilter, suspects bool) (MeterHistoryPage, error) {
	if !s.Enabled {
		return MeterHistoryPage{}, ErrSurfaceDisabled
	}
	if !canonicalID(assetID) || !canonicalID(meterID) {
		return MeterHistoryPage{}, meter.ErrInvalid
	}
	if e := filter.Validate(); e != nil {
		return MeterHistoryPage{}, e
	}
	policy := Policy(false)
	if suspects {
		policy = MeterReviewPolicy(false)
		if filter.State != meter.Suspect {
			return MeterHistoryPage{}, meter.ErrInvalid
		}
	}
	if e := authorized(ctx, scope, actor, decision, assetID, policy); e != nil {
		return MeterHistoryPage{}, e
	}
	if unavailable(s.Repository) {
		return MeterHistoryPage{}, ErrUnavailable
	}
	page, e := s.Repository.MeterReadingHistory(ctx, scope, actor, decision.Subject().MembershipID(), assetID, meterID, filter, suspects)
	if guard := authorized(ctx, scope, actor, decision, assetID, policy); guard != nil {
		return MeterHistoryPage{}, guard
	}
	if e != nil {
		return MeterHistoryPage{}, e
	}
	if len(page.Items) > filter.Limit || page.NextID != "" && (len(page.Items) != filter.Limit || !canonicalID(page.NextID) || page.Items[len(page.Items)-1].Reading.Snapshot().Reference.ID() != page.NextID) || page.At.IsZero() {
		return MeterHistoryPage{}, ErrUnavailable
	}
	items := make([]MeterHistoryItem, 0, len(page.Items))
	for _, v := range page.Items {
		in := v.Reading.Snapshot()
		if _, e := meter.NewReading(scope, in); e != nil || in.Reference.Segment().Meter().ID() != meterID || in.Reference.Segment().Meter().Asset().ID().Value() != assetID || filter.State != "" && v.State != filter.State || (v.State == meter.Suspect) != (len(v.Flags) > 0) || suspects && v.State != meter.Suspect {
			return MeterHistoryPage{}, ErrUnavailable
		}
		items = append(items, v.Clone())
	}
	return MeterHistoryPage{Items: items, NextID: page.NextID, At: page.At}, nil
}

func (s MeterCorrectionService) History(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, assetID, meterID string, filter MeterHistoryFilter) (MeterHistoryPage, error) {
	return s.read(ctx, scope, actor, decision, assetID, meterID, filter, false)
}
func (s MeterCorrectionService) Suspects(ctx context.Context, scope kernel.OrgScope, actor FleetActor, decision access.Decision, assetID, meterID string, filter MeterHistoryFilter) (MeterHistoryPage, error) {
	return s.read(ctx, scope, actor, decision, assetID, meterID, filter, true)
}

// AmendmentSupersession is one corrected reading of an approved parent
// record's amendment (EMP-TRIP-015, B-BR-EMP-366): the reading superseded,
// the pre-allocated superseding reading, the corrected count in the meter's
// own kind, and the parent decision's reason, operation and digest.
type AmendmentSupersession struct {
	private
	AssetID, MeterID, ReadingID, NewReadingID string
	Value                                     int64
	Reason, OperationID, RequestHash          string
}
