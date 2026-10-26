package domain

import (
	"math/big"
	"regexp"
	"time"
)

const (
	ShiftReady            = "READY"
	ShiftRunning          = "RUNNING"
	ShiftPaused           = "PAUSED"
	ShiftClosed           = "CLOSED"
	ShiftRecoveryRequired = "RECOVERY_REQUIRED"
)

const (
	ActionStart      = "START"
	ActionPause      = "PAUSE"
	ActionResume     = "RESUME"
	ActionEnd        = "END"
	ActionCheckpoint = "CHECKPOINT"
	ActionRecover    = "RECOVER"
	ActionReviewGap  = "REVIEW_GAP"
)

const (
	GapBootChanged        = "boot-changed"
	GapClockDiscontinuity = "clock-discontinuity"
	GapCheckpoint         = "checkpoint-gap"
	GapProcessRecovery    = "process-recovery"
)

const MaxShiftReceipts = 50

var bootIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
var segmentPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,64}$`)
var qualityNotePattern = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

type Clock struct {
	BootID        string `json:"boot_id"`
	ElapsedMillis int64  `json:"elapsed_millis"`
	UTCMillis     int64  `json:"utc_millis"`
}

func (c Clock) Validate() error {
	if !bootIDPattern.MatchString(c.BootID) || c.ElapsedMillis < 0 {
		return ErrInvalid
	}
	return nil
}

type ClockPolicy struct {
	MaximumCheckpointGapMillis int64 `json:"maximum_checkpoint_gap_millis"`
	WallToleranceMillis        int64 `json:"wall_tolerance_millis"`
}

func (p ClockPolicy) Validate() error {
	if p.MaximumCheckpointGapMillis <= 0 || p.WallToleranceMillis < 0 {
		return ErrInvalid
	}
	return nil
}

type ShiftCommand struct {
	ID                 string `json:"id"`
	Action             string `json:"action"`
	ExpectedVersion    int64  `json:"expected_version"`
	Clock              Clock  `json:"clock"`
	GapID              string `json:"gap_id,omitempty"`
	ReviewedWorkMillis *int64 `json:"reviewed_work_millis,omitempty"`
}

func (c ShiftCommand) Validate() error {
	if !ValidID(c.ID) || c.ExpectedVersion < 0 || c.Clock.Validate() != nil || !validAction(c.Action) {
		return ErrInvalid
	}
	if c.Action == ActionReviewGap {
		if !ValidID(c.GapID) || (c.ReviewedWorkMillis != nil && (*c.ReviewedWorkMillis < 0 || *c.ReviewedWorkMillis > MaxAmount)) {
			return ErrInvalid
		}
	} else if c.GapID != "" || c.ReviewedWorkMillis != nil {
		return ErrInvalid
	}
	return nil
}

func (c ShiftCommand) Fingerprint() (string, error) {
	if e := c.Validate(); e != nil {
		return "", e
	}
	return Digest(struct {
		ID                 string `json:"id"`
		Action             string `json:"action"`
		ExpectedVersion    int64  `json:"expected_version"`
		Clock              Clock  `json:"clock"`
		GapID              string `json:"gap_id,omitempty"`
		ReviewedWorkMillis *int64 `json:"reviewed_work_millis,omitempty"`
	}{c.ID, c.Action, c.ExpectedVersion, c.Clock, c.GapID, c.ReviewedWorkMillis})
}

func validAction(action string) bool {
	switch action {
	case ActionStart, ActionPause, ActionResume, ActionEnd, ActionCheckpoint, ActionRecover, ActionReviewGap:
		return true
	default:
		return false
	}
}

type ShiftGap struct {
	ID                 string `json:"id"`
	Reason             string `json:"reason"`
	Previous           Clock  `json:"previous"`
	Next               Clock  `json:"next"`
	Reviewed           bool   `json:"reviewed"`
	ReviewedWorkMillis *int64 `json:"reviewed_work_millis,omitempty"`
}

type ShiftReceipt struct {
	ID              string `json:"id"`
	Fingerprint     string `json:"fingerprint"`
	AcceptedVersion int64  `json:"accepted_version"`
}

type Shift struct {
	ID                     string         `json:"id"`
	Owner                  string         `json:"owner"`
	Origin                 string         `json:"origin"`
	Vehicle                string         `json:"vehicle"`
	Timezone               string         `json:"timezone"`
	State                  string         `json:"state"`
	Version                int64          `json:"version"`
	Last                   *Clock         `json:"last,omitempty"`
	ActiveMillis           int64          `json:"active_millis"`
	ManuallyReviewedMillis int64          `json:"manually_reviewed_millis"`
	Gaps                   []ShiftGap     `json:"gaps"`
	Receipts               []ShiftReceipt `json:"receipts"`
}

func NewShift(owner, id, origin, vehicle, timezone string) (Shift, error) {
	s := Shift{ID: id, Owner: owner, Origin: origin, Vehicle: vehicle, Timezone: timezone, State: ShiftReady, Gaps: []ShiftGap{}, Receipts: []ShiftReceipt{}}
	return s, s.Validate()
}

func (s Shift) Validate() error {
	if !ValidID(s.ID) || !ValidID(s.Owner) || !ValidID(s.Vehicle) || s.Origin == "" || len(s.Origin) > 128 || s.Timezone == "" || s.Timezone == "Local" {
		return ErrInvalid
	}
	if _, e := time.LoadLocation(s.Timezone); e != nil {
		return ErrInvalid
	}
	if !validState(s.State) || s.Version < 0 || s.ActiveMillis < 0 || s.ActiveMillis > MaxAmount || s.ManuallyReviewedMillis < 0 || s.ManuallyReviewedMillis > s.ActiveMillis {
		return ErrInvalid
	}
	if (s.State == ShiftReady) != (s.Last == nil) {
		return ErrInvalid
	}
	if s.Last != nil && s.Last.Validate() != nil {
		return ErrInvalid
	}
	if len(s.Gaps) > 50 || len(s.Receipts) > MaxShiftReceipts {
		return ErrInvalid
	}
	gaps := map[string]bool{}
	for _, g := range s.Gaps {
		if !ValidID(g.ID) || gaps[g.ID] || !validGapReason(g.Reason) || g.Previous.Validate() != nil || g.Next.Validate() != nil || (!g.Reviewed && g.ReviewedWorkMillis != nil) || (g.ReviewedWorkMillis != nil && (*g.ReviewedWorkMillis < 0 || *g.ReviewedWorkMillis > MaxAmount)) {
			return ErrInvalid
		}
		gaps[g.ID] = true
	}
	receipts := map[string]bool{}
	for _, r := range s.Receipts {
		if !ValidID(r.ID) || receipts[r.ID] || !ValidSource(r.Fingerprint) || r.AcceptedVersion < 1 {
			return ErrInvalid
		}
		receipts[r.ID] = true
	}
	return nil
}

func validState(state string) bool {
	switch state {
	case ShiftReady, ShiftRunning, ShiftPaused, ShiftClosed, ShiftRecoveryRequired:
		return true
	default:
		return false
	}
}

func validGapReason(reason string) bool {
	switch reason {
	case GapBootChanged, GapClockDiscontinuity, GapCheckpoint, GapProcessRecovery:
		return true
	default:
		return false
	}
}

func (s Shift) KnownActiveSeconds() int64 { return s.ActiveMillis / 1000 }

func (s Shift) CompleteWorkSeconds() *int64 {
	for _, g := range s.Gaps {
		if g.ReviewedWorkMillis == nil {
			return nil
		}
	}
	n := s.KnownActiveSeconds()
	return &n
}

type ShiftTransition struct {
	Shift    Shift
	Receipt  ShiftReceipt
	Replayed bool
}

func (s Shift) Apply(cmd ShiftCommand, policy ClockPolicy) (ShiftTransition, error) {
	if e := s.Validate(); e != nil {
		return ShiftTransition{}, e
	}
	if e := policy.Validate(); e != nil {
		return ShiftTransition{}, e
	}
	fingerprint, e := cmd.Fingerprint()
	if e != nil {
		return ShiftTransition{}, e
	}
	for _, receipt := range s.Receipts {
		if receipt.ID == cmd.ID {
			if receipt.Fingerprint != fingerprint {
				return ShiftTransition{}, ErrConflict
			}
			return ShiftTransition{Shift: s, Receipt: receipt, Replayed: true}, nil
		}
	}
	if cmd.ExpectedVersion != s.Version {
		return ShiftTransition{}, ErrConflict
	}
	if !transitionAllowed(s.State, cmd.Action) {
		return ShiftTransition{}, ErrConflict
	}
	nextVersion := s.Version + 1
	if nextVersion < 1 {
		return ShiftTransition{}, ErrInvalid
	}
	receipt := ShiftReceipt{ID: cmd.ID, Fingerprint: fingerprint, AcceptedVersion: nextVersion}
	accepted := func(candidate Shift) (ShiftTransition, error) {
		candidate.Version = nextVersion
		candidate.Receipts = appendReceipt(s.Receipts, receipt)
		candidate.Gaps = append([]ShiftGap(nil), candidate.Gaps...)
		if e := candidate.Validate(); e != nil {
			return ShiftTransition{}, e
		}
		return ShiftTransition{Shift: candidate, Receipt: receipt}, nil
	}
	gap := func(reason string) (ShiftTransition, error) {
		if s.Last == nil || len(s.Gaps) >= 50 {
			return ShiftTransition{}, ErrConflict
		}
		next := s
		next.State = ShiftRecoveryRequired
		next.Last = &Clock{BootID: cmd.Clock.BootID, ElapsedMillis: cmd.Clock.ElapsedMillis, UTCMillis: cmd.Clock.UTCMillis}
		next.Gaps = append(append([]ShiftGap(nil), s.Gaps...), ShiftGap{ID: cmd.ID, Reason: reason, Previous: *s.Last, Next: cmd.Clock})
		return accepted(next)
	}
	if cmd.Action == ActionReviewGap {
		target := -1
		for i := range s.Gaps {
			if s.Gaps[i].ID == cmd.GapID {
				target = i
				break
			}
		}
		if target < 0 {
			return ShiftTransition{}, ErrInvalid
		}
		if s.Gaps[target].Reviewed {
			return ShiftTransition{}, ErrConflict
		}
		extra := int64(0)
		if cmd.ReviewedWorkMillis != nil {
			extra = *cmd.ReviewedWorkMillis
		}
		active, e := boundedAdd(s.ActiveMillis, extra)
		if e != nil {
			return ShiftTransition{}, e
		}
		reviewed, e := boundedAdd(s.ManuallyReviewedMillis, extra)
		if e != nil {
			return ShiftTransition{}, e
		}
		next := s
		next.State = ShiftPaused
		next.Last = &cmd.Clock
		next.ActiveMillis = active
		next.ManuallyReviewedMillis = reviewed
		next.Gaps = append([]ShiftGap(nil), s.Gaps...)
		next.Gaps[target].Reviewed = true
		next.Gaps[target].ReviewedWorkMillis = cmd.ReviewedWorkMillis
		return accepted(next)
	}
	if cmd.Action == ActionRecover && s.State == ShiftRunning {
		return gap(GapProcessRecovery)
	}
	var elapsed int64
	if s.Last != nil {
		previous := *s.Last
		if previous.BootID != cmd.Clock.BootID {
			return gap(GapBootChanged)
		}
		mono := new(big.Int).Sub(big.NewInt(cmd.Clock.ElapsedMillis), big.NewInt(previous.ElapsedMillis))
		wall := new(big.Int).Sub(big.NewInt(cmd.Clock.UTCMillis), big.NewInt(previous.UTCMillis))
		if mono.Sign() < 0 || wall.Sign() < 0 || new(big.Int).Abs(new(big.Int).Sub(wall, mono)).Cmp(big.NewInt(policy.WallToleranceMillis)) > 0 {
			return gap(GapClockDiscontinuity)
		}
		if s.State == ShiftRunning && mono.Cmp(big.NewInt(policy.MaximumCheckpointGapMillis)) > 0 {
			return gap(GapCheckpoint)
		}
		if !mono.IsInt64() {
			return ShiftTransition{}, ErrInvalid
		}
		elapsed = mono.Int64()
	}
	worked := s.ActiveMillis
	if s.State == ShiftRunning {
		worked, e = boundedAdd(s.ActiveMillis, elapsed)
		if e != nil {
			return ShiftTransition{}, e
		}
	}
	nextState := s.State
	switch cmd.Action {
	case ActionStart, ActionResume, ActionCheckpoint:
		nextState = ShiftRunning
	case ActionPause, ActionRecover:
		nextState = ShiftPaused
	case ActionEnd:
		nextState = ShiftClosed
	}
	next := s
	next.State = nextState
	next.Last = &cmd.Clock
	next.ActiveMillis = worked
	next.Gaps = append([]ShiftGap(nil), s.Gaps...)
	return accepted(next)
}

func transitionAllowed(state, action string) bool {
	switch action {
	case ActionStart:
		return state == ShiftReady
	case ActionResume:
		return state == ShiftPaused
	case ActionPause, ActionCheckpoint:
		return state == ShiftRunning
	case ActionEnd:
		return state == ShiftRunning || state == ShiftPaused
	case ActionRecover:
		return state == ShiftRunning || state == ShiftPaused
	case ActionReviewGap:
		return state == ShiftRecoveryRequired || state == ShiftPaused
	default:
		return false
	}
}

func appendReceipt(receipts []ShiftReceipt, receipt ShiftReceipt) []ShiftReceipt {
	out := append([]ShiftReceipt(nil), receipts...)
	out = append(out, receipt)
	if len(out) > MaxShiftReceipts {
		out = out[len(out)-MaxShiftReceipts:]
	}
	return out
}

func boundedAdd(a, b int64) (int64, error) {
	if b < 0 || a > MaxAmount-b {
		return 0, ErrInvalid
	}
	return a + b, nil
}

type DistanceRecord struct {
	ID                  string    `json:"id"`
	Origin              string    `json:"origin"`
	VehicleID           string    `json:"vehicle_id"`
	Segment             string    `json:"segment"`
	Basis               string    `json:"basis"`
	From                time.Time `json:"from"`
	To                  time.Time `json:"to"`
	Timezone            string    `json:"timezone"`
	Metres              *int64    `json:"metres,omitempty"`
	Complete            bool      `json:"complete"`
	HasGap              bool      `json:"has_gap"`
	Quality             string    `json:"quality"`
	QualityVersion      *int32    `json:"quality_version,omitempty"`
	QualityNote         string    `json:"quality_note,omitempty"`
	StartOdometerMetres *int64    `json:"start_odometer_metres,omitempty"`
	EndOdometerMetres   *int64    `json:"end_odometer_metres,omitempty"`
}

func (r DistanceRecord) Normalize() (DistanceRecord, error) {
	own := func(v *int64) *int64 {
		if v == nil {
			return nil
		}
		n := *v
		return &n
	}
	r.Metres = own(r.Metres)
	r.StartOdometerMetres = own(r.StartOdometerMetres)
	r.EndOdometerMetres = own(r.EndOdometerMetres)
	if r.QualityVersion != nil {
		v := *r.QualityVersion
		r.QualityVersion = &v
	}
	if e := r.validate(); e != nil {
		return r, e
	}
	r.From = r.From.UTC()
	r.To = r.To.UTC()
	if r.From.Year() < 1 || r.From.Year() > 9999 || r.To.Year() < 1 || r.To.Year() > 9999 || !r.To.After(r.From) || r.From.Nanosecond()%1000 != 0 || r.To.Nanosecond()%1000 != 0 {
		return r, ErrInvalid
	}
	return r, nil
}

func (r DistanceRecord) validate() error {
	if !ValidID(r.ID) || !ValidID(r.VehicleID) || r.Origin == "" || len(r.Origin) > 128 || !segmentPattern.MatchString(r.Segment) || r.Timezone == "" || r.Timezone == "Local" {
		return ErrInvalid
	}
	if _, e := time.LoadLocation(r.Timezone); e != nil {
		return ErrInvalid
	}
	if r.Basis != "ODOMETER" && r.Basis != "GPS_ESTIMATE" && r.Basis != "PAID_REPORTED" && r.Basis != "MANUAL_TRIP" && r.Basis != "ROUTE_PREDICTION" {
		return ErrInvalid
	}
	if r.Metres != nil && !validAmount(*r.Metres) {
		return ErrInvalid
	}
	if r.Complete && (r.HasGap || r.Metres == nil) {
		return ErrInvalid
	}
	if r.HasGap && r.Complete {
		return ErrInvalid
	}
	if r.QualityVersion != nil && *r.QualityVersion <= 0 {
		return ErrInvalid
	}
	if r.QualityNote != "" && !qualityNotePattern.MatchString(r.QualityNote) {
		return ErrInvalid
	}
	switch r.Basis {
	case "ODOMETER":
		if r.Quality != "MEASURED" || r.StartOdometerMetres == nil || r.EndOdometerMetres == nil || !validAmount(*r.StartOdometerMetres) || !validAmount(*r.EndOdometerMetres) || *r.EndOdometerMetres < *r.StartOdometerMetres {
			return ErrInvalid
		}
		if r.Metres != nil && *r.Metres != *r.EndOdometerMetres-*r.StartOdometerMetres {
			return ErrInvalid
		}
	case "MANUAL_TRIP":
		if r.Quality != "MEASURED" || r.StartOdometerMetres != nil || r.EndOdometerMetres != nil {
			return ErrInvalid
		}
	case "GPS_ESTIMATE":
		if r.Quality != "ESTIMATED" || r.QualityVersion == nil || r.StartOdometerMetres != nil || r.EndOdometerMetres != nil {
			return ErrInvalid
		}
	case "ROUTE_PREDICTION":
		if r.Quality != "ESTIMATED" || r.StartOdometerMetres != nil || r.EndOdometerMetres != nil {
			return ErrInvalid
		}
	case "PAID_REPORTED":
		if r.Quality != "REPORTED" || r.StartOdometerMetres != nil || r.EndOdometerMetres != nil {
			return ErrInvalid
		}
	}
	return nil
}

func (r DistanceRecord) ContentHash() (string, error) {
	normalized, e := r.Normalize()
	if e != nil {
		return "", e
	}
	return Digest(normalized)
}
