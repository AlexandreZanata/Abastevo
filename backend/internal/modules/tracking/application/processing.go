package application

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// Track processing job (EMP-GPS-005, B-BR-EMP-461..466, EDR-298..301).
const (
	// ProcessKind is the queue kind of one session's processing.
	ProcessKind = "org-tracking-process"
	// ProcessAttempts bounds the retries of one queued processing job.
	ProcessAttempts int32 = 5
	// ClockTolerance is the largest disagreement between a segment's wall
	// base and its boot's monotonic base before the segment is counted
	// CLOCK_UNCERTAIN (§8.1, BR-TIME-005).
	ClockTolerance = 2 * time.Second
)

var (
	// ErrStaleLease means another claim superseded this job's lease: nothing
	// is written.
	ErrStaleLease = errors.New("tracking: processing lease superseded")
)

// ProcessOutcome is what one processing run did.
type ProcessOutcome string

const (
	// ProcessWritten stored a new summary revision.
	ProcessWritten ProcessOutcome = "WRITTEN"
	// ProcessCurrent found a summary of exactly the stored input.
	ProcessCurrent ProcessOutcome = "CURRENT"
	// ProcessNotEnded found a session that is not ended yet.
	ProcessNotEnded ProcessOutcome = "NOT_ENDED"
	// ProcessMissing found no such session in the organization.
	ProcessMissing ProcessOutcome = "MISSING"
)

// Lease is the queue lease that fences one processing run.
type Lease struct {
	JobID string
	Token int64
}

// Watermark identifies the exact stored input of a summary: the session's
// segment and transition counts (both tables are insert-only).
type Watermark struct {
	Segments, Transitions int
}

// Newer reports whether w is a strictly later input than o.
func (w Watermark) Newer(o Watermark) bool {
	return w.Segments >= o.Segments && w.Transitions >= o.Transitions && w != o
}

// ProcessingSegment is one stored segment as processed.
type ProcessingSegment struct {
	private
	Seq                                 int64
	BootID                              *string
	BaseElapsed                         *int64
	Base                                time.Time
	Fixes                               []StoredFix
	Received, Duplicates, Outside, Mock int
	Dropped                             Dropped
}

// Summary is one derived track summary revision (B-BR-EMP-465).
type Summary struct {
	private
	SessionID, TripID                                       string
	Watermark                                               Watermark
	Result                                                  domain.TrackResult
	Received, Duplicates, OutsideSession, Mock              int64
	DroppedAccuracy, DroppedStale, DroppedJump, DroppedMock int64
	ClockUncertain                                          int
}

// running derives the trip's IN_PROGRESS intervals until its end: its
// finish, cancellation or supervisor closure, else the session's end.
func running(record tripapp.Record, sessionEnd time.Time) ([]domain.Interval, error) {
	from := record.Declared.OccurredAt
	state := trip.InProgress
	var out []domain.Interval
	last := from
	for _, t := range record.Transitions {
		at := t.Declared.OccurredAt
		if t.From != state || at.Before(last) {
			return nil, ErrUnavailable
		}
		switch t.To {
		case trip.Paused, trip.Cancelled:
			if state == trip.InProgress && from.Before(at) {
				out = append(out, domain.Interval{From: from, To: at})
			}
		case trip.InProgress:
			from = at
		default:
			return nil, ErrUnavailable
		}
		state, last = t.To, at
	}
	end := sessionEnd
	switch {
	case state == trip.Cancelled:
		return out, nil
	case record.Finish != nil:
		end = record.Finish.Declared.OccurredAt
	case record.Closure != nil:
		end = record.Closure.ClosedAt
	}
	if end.Before(last) {
		return nil, ErrUnavailable
	}
	if state == trip.InProgress && from.Before(end) {
		out = append(out, domain.Interval{From: from, To: end})
	}
	return out, nil
}

// detector of a stored transition: the driver's device or the server.
func detector(a domain.Actor) domain.Detector {
	if a == domain.ActorDriver || a == domain.ActorDevice {
		return domain.DetectedByDevice
	}
	return domain.DetectedByServer
}

// spans derives the session's state spans from its replayed history
// (B-BR-EMP-462): the end transition must be the last.
func spans(s domain.Session) (time.Time, time.Time, []domain.Span, domain.GapReason, domain.Detector, error) {
	history := s.History()
	if len(history) < 2 || history[len(history)-1].State() != domain.Ended {
		return time.Time{}, time.Time{}, nil, "", "", ErrUnavailable
	}
	var out []domain.Span
	for i, h := range history[:len(history)-1] {
		next := history[i+1].At()
		if !h.At().Before(next) {
			continue
		}
		span := domain.Span{From: h.At(), To: next}
		if h.State() == domain.Recording {
			span.Recording, span.Precision = true, h.Permission()
		} else {
			reason, ok := domain.GapReasonOf(h.Cause())
			if !ok {
				return time.Time{}, time.Time{}, nil, "", "", ErrUnavailable
			}
			span.Reason, span.Detector = reason, detector(h.Actor())
		}
		out = append(out, span)
	}
	end := history[len(history)-1]
	reason, ok := domain.GapReasonOf(end.Cause())
	if !ok {
		return time.Time{}, time.Time{}, nil, "", "", ErrUnavailable
	}
	return history[0].At(), end.At(), out, reason, detector(end.Actor()), nil
}

// clockUncertain counts segments without a boot identity or whose wall base
// disagrees with the first segment of their boot by more than ClockTolerance.
func clockUncertain(segments []ProcessingSegment) int {
	reference := map[string]time.Time{}
	n := 0
	for _, s := range segments {
		if s.BootID == nil || s.BaseElapsed == nil {
			n++
			continue
		}
		origin := s.Base.Add(-time.Duration(*s.BaseElapsed) * time.Millisecond)
		ref, ok := reference[*s.BootID]
		if !ok {
			reference[*s.BootID] = origin
			continue
		}
		if d := origin.Sub(ref); d > ClockTolerance || d < -ClockTolerance {
			n++
		}
	}
	return n
}

// Summarize derives one summary of an ended session from its stored rows
// only: the replayed history, the trip's running intervals and every stored
// fix of the session's segments (B-BR-EMP-461..465). A segment of another
// session or trip is never used.
func Summarize(scope kernel.OrgScope, record SessionRecord, tripRecord tripapp.Record, segments []ProcessingSegment) (Summary, error) {
	if record.TripID != tripRecord.ID {
		return Summary{}, ErrUnavailable
	}
	s, err := Rebuild(scope, record)
	if err != nil {
		return Summary{}, err
	}
	opened, ended, states, reason, by, err := spans(s)
	if err != nil {
		return Summary{}, err
	}
	window, err := running(tripRecord, ended)
	if err != nil {
		return Summary{}, err
	}
	ordered := append([]ProcessingSegment(nil), segments...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Seq < ordered[j].Seq })
	out := Summary{SessionID: record.ID, TripID: record.TripID, Watermark: Watermark{Segments: len(segments), Transitions: len(record.Transitions)}}
	var fixes []domain.TrackFix
	for i, g := range ordered {
		if i > 0 && g.Seq == ordered[i-1].Seq {
			return Summary{}, ErrUnavailable
		}
		for _, f := range g.Fixes {
			fixes = append(fixes, domain.TrackFix{At: f.At, Lat: f.Lat, Lon: f.Lon, Accuracy: f.Accuracy, Coarse: f.Coarse})
		}
		out.Received += int64(g.Received)
		out.Duplicates += int64(g.Duplicates)
		out.OutsideSession += int64(g.Outside)
		out.Mock += int64(g.Mock)
		out.DroppedAccuracy += int64(g.Dropped.Accuracy)
		out.DroppedStale += int64(g.Dropped.Stale)
		out.DroppedJump += int64(g.Dropped.Jump)
		out.DroppedMock += int64(g.Dropped.Mock)
	}
	sort.Slice(fixes, func(i, j int) bool { return fixes[i].At.Before(fixes[j].At) })
	out.ClockUncertain = clockUncertain(ordered)
	result, err := domain.ProcessTrack(domain.TrackInput{Running: window, Opened: opened, Ended: ended, States: states, EndReason: reason, EndDetector: by, Fixes: fixes})
	if err != nil {
		return Summary{}, ErrUnavailable
	}
	out.Result = result
	return out, nil
}

// Processor runs one fenced processing of a session.
type Processor interface {
	Process(context.Context, kernel.OrgScope, string, Lease) (ProcessOutcome, int, error)
}
