package domain

import (
	"math/big"
	"regexp"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// bootIDPattern is the driver-shift boot identity profile (B-BR-DRV-07).
var bootIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// RunClock is a device monotonic witness of one event (BR-TIME-005): a boot
// identity and the monotonic count in milliseconds since that boot. It
// confirms or questions elapsed time; it never becomes the duration.
type RunClock struct {
	redacted
	bootID        string
	elapsedMillis int64
}

// NewRunClock validates the boot identity profile and a non-negative count.
func NewRunClock(bootID string, elapsedMillis int64) (RunClock, error) {
	c := RunClock{bootID: bootID, elapsedMillis: elapsedMillis}
	if !c.valid() {
		return RunClock{}, ErrInvalid
	}
	return c, nil
}
func (c RunClock) valid() bool          { return bootIDPattern.MatchString(c.bootID) && c.elapsedMillis >= 0 }
func (c RunClock) BootID() string       { return c.bootID }
func (c RunClock) ElapsedMillis() int64 { return c.elapsedMillis }

// optionalClock re-validates a supplied witness and copies it.
func optionalClock(c *RunClock) (*RunClock, error) {
	if c == nil {
		return nil, nil
	}
	if !c.valid() {
		return nil, ErrInvalid
	}
	return copyPointer(c), nil
}

// localDay is the BR-TIME-004 local day of an instant in a stored reporting zone.
func localDay(event orgtime.Declaration, zone string) (string, error) {
	a, e := orgtime.AttributeReportingDay(event.OccurredAt, orgtime.ReportingZones{Organization: zone})
	if e != nil {
		return "", ErrInvalid
	}
	return a.Day, nil
}

// Crossing is the informational time-crossing view of a trip. The trip's
// reporting day is always its start day; nothing here splits the trip.
type Crossing struct {
	finished, midnight, entryZone, offset bool
	day, endDay                           string
}

func (c Crossing) Finished() bool         { return c.finished }
func (c Crossing) Day() string            { return c.day }
func (c Crossing) EndDay() string         { return c.endDay }
func (c Crossing) CrossesMidnight() bool  { return c.midnight }
func (c Crossing) EntryZoneChanged() bool { return c.entryZone }
func (c Crossing) OffsetChanged() bool    { return c.offset }

// Crossing reports the end day in the stored reporting zone and whether the
// device entry zone or offset changed between start and finish (B-BR-EMP-276/277).
func (t Trip) Crossing() Crossing {
	out := Crossing{day: t.reporting.Day}
	if t.finish == nil {
		return out
	}
	from, to := t.start.declared, t.finish.declared
	out.finished, out.endDay, out.midnight = true, t.endDay, t.endDay != t.reporting.Day
	out.entryZone, out.offset = from.EntryZone != to.EntryZone, from.DeviceOffsetSeconds != to.DeviceOffsetSeconds
	return out
}

// ElapsedStatus says how far elapsed time is known.
type ElapsedStatus string

const (
	ElapsedNotFinished ElapsedStatus = "NOT_FINISHED"
	ElapsedDeclared    ElapsedStatus = "DECLARED"
	ElapsedConfirmed   ElapsedStatus = "CONFIRMED"
	ElapsedGap         ElapsedStatus = "GAP"
)

// GapReason explains a GAP.
type GapReason string

const (
	GapBootChanged        GapReason = "BOOT_CHANGED"
	GapClockDiscontinuity GapReason = "CLOCK_DISCONTINUITY"
)

// Elapsed is the trip's elapsed time from its instants, or why it has none.
type Elapsed struct {
	redacted
	status  ElapsedStatus
	seconds int64
	nanos   int32
	gap     GapReason
}

func (e Elapsed) Status() ElapsedStatus { return e.status }
func (e Elapsed) Gap() GapReason        { return e.gap }

// Duration returns seconds and non-negative nanoseconds when DECLARED or CONFIRMED.
func (e Elapsed) Duration() (int64, int32, bool) {
	return e.seconds, e.nanos, e.status == ElapsedDeclared || e.status == ElapsedConfirmed
}

var (
	nanosPerSecond = big.NewInt(1_000_000_000)
	nanosPerMilli  = big.NewInt(1_000_000)
)

// Elapsed derives elapsed time only from the start and finish instants and
// checks it against the run clocks with an explicit tolerance in milliseconds
// (B-BR-EMP-278/279). A gap carries no value; nothing is invented.
func (t Trip) Elapsed(toleranceMillis int64) (Elapsed, error) {
	if toleranceMillis < 0 {
		return Elapsed{}, ErrInvalid
	}
	if t.finish == nil {
		return Elapsed{status: ElapsedNotFinished}, nil
	}
	from, to := t.start.declared.OccurredAt, t.finish.declared.OccurredAt
	// Exact seconds and nanoseconds: time.Duration saturates after ~292 years.
	seconds, nanos := to.Unix()-from.Unix(), int64(to.Nanosecond()-from.Nanosecond())
	if nanos < 0 {
		seconds, nanos = seconds-1, nanos+1_000_000_000
	}
	measured := Elapsed{status: ElapsedDeclared, seconds: seconds, nanos: int32(nanos)}
	a, b := t.start.clock, t.finish.clock
	if a == nil || b == nil {
		return measured, nil
	}
	if a.bootID != b.bootID {
		return Elapsed{status: ElapsedGap, gap: GapBootChanged}, nil
	}
	// Both counts are non-negative, so the difference cannot overflow.
	mono := b.elapsedMillis - a.elapsedMillis
	instant := new(big.Int).Add(new(big.Int).Mul(big.NewInt(seconds), nanosPerSecond), big.NewInt(nanos))
	drift := new(big.Int).Abs(instant.Sub(instant, new(big.Int).Mul(big.NewInt(mono), nanosPerMilli)))
	if mono < 0 || drift.Cmp(new(big.Int).Mul(big.NewInt(toleranceMillis), nanosPerMilli)) > 0 {
		return Elapsed{status: ElapsedGap, gap: GapClockDiscontinuity}, nil
	}
	measured.status = ElapsedConfirmed
	return measured, nil
}
