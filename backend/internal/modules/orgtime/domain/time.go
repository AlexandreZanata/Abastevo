// Package domain preserves enterprise operation times without resolving access authority.
package domain

import (
	"errors"
	"regexp"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var ErrInvalid = errors.New("orgtime: invalid-time")

const DefaultSkewLimitSeconds int64 = 300

var instantPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]{1,9})?(Z|[+-][0-9]{2}:[0-9]{2})$`)

// ParseInstant accepts the shared bounded RFC3339 profile and normalizes to UTC.
func ParseInstant(raw string) (time.Time, error) {
	if len(raw) < 20 || len(raw) > 35 {
		return time.Time{}, ErrInvalid
	}
	parts := instantPattern.FindStringSubmatch(raw)
	if parts == nil || raw[:4] == "0000" {
		return time.Time{}, ErrInvalid
	}
	offset := parts[2]
	if offset != "Z" && (offset == "-00:00" || offset[1:3] > "18" || offset[4:6] > "59" ||
		(offset[1:3] == "18" && offset[4:6] != "00")) {
		return time.Time{}, ErrInvalid
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || !validInstant(at) {
		return time.Time{}, ErrInvalid
	}
	return at.UTC(), nil
}

// FormatInstant formats a previously validated instant in canonical UTC.
func FormatInstant(at time.Time) string { return at.UTC().Format(time.RFC3339Nano) }

func validInstant(at time.Time) bool {
	year := at.UTC().Year()
	return year >= 1 && year <= 9999
}

// validReceipt also refuses the zero instant: an unset server receipt must never
// become a silently valid authority time that lies outside every period.
func validReceipt(at time.Time) bool { return validInstant(at) && !at.IsZero() }

// location applies the zone profile shared with organizations (kernel.LoadTimeZone).
func location(id string) (*time.Location, error) {
	loc, ok := kernel.LoadTimeZone(id)
	if !ok {
		return nil, ErrInvalid
	}
	return loc, nil
}

// Declaration retains device metadata even if its offset disagrees with its zone.
type Declaration struct {
	OccurredAt          time.Time
	EntryZone           string
	DeviceOffsetSeconds int64
}

// ReportingZones must come from the resolved operating context, not a device claim.
// Nil Unit means no operating unit; an invalid supplied unit never falls back.
type ReportingZones struct {
	Organization string
	Unit         *string
}

// Record is a historical value snapshot. Persist all fields together.
// A value alone is not evidence of authority; only the owning server sets ReceivedAt.
type Record struct {
	OccurredAt          time.Time
	ReceivedAt          time.Time
	EntryZone           string
	DeviceOffsetSeconds int64
	ReportingDay        string
	ReportingZone       string
	ReportingSource     string
}

// AuthorityAt supplies the server receipt reference, never the declared device time.
func (r Record) AuthorityAt() time.Time { return r.ReceivedAt }

// Capture uses occurred_at for reporting. A trip's caller supplies its start instant.
func Capture(d Declaration, receivedAt time.Time, zones ReportingZones) (Record, error) {
	if !validInstant(d.OccurredAt) || !validReceipt(receivedAt) ||
		d.DeviceOffsetSeconds < -64800 || d.DeviceOffsetSeconds > 64800 {
		return Record{}, ErrInvalid
	}
	if _, err := location(d.EntryZone); err != nil {
		return Record{}, err
	}
	loc, err := location(zones.Organization)
	if err != nil {
		return Record{}, err
	}
	zone, source := zones.Organization, "organization"
	if zones.Unit != nil {
		zone, source = *zones.Unit, "unit"
		loc, err = location(zone)
		if err != nil {
			return Record{}, err
		}
	}
	local := d.OccurredAt.In(loc)
	if local.Year() < 1 || local.Year() > 9999 {
		return Record{}, ErrInvalid
	}
	return Record{
		OccurredAt: d.OccurredAt.UTC(), ReceivedAt: receivedAt.UTC(), EntryZone: d.EntryZone,
		DeviceOffsetSeconds: d.DeviceOffsetSeconds, ReportingDay: local.Format("2006-01-02"),
		ReportingZone: zone, ReportingSource: source,
	}, nil
}

// InPeriod checks a nonempty half-open instant interval. Authorization callers use AuthorityAt.
func InPeriod(at, start, end time.Time) (bool, error) {
	// Compare wall-clock instants, without process-local monotonic readings.
	at, start, end = at.UTC(), start.UTC(), end.UTC()
	if !validInstant(at) || !validInstant(start) || !validInstant(end) || !start.Before(end) {
		return false, ErrInvalid
	}
	return !at.Before(start) && at.Before(end), nil
}

// ClockSkew expresses device minus server as seconds + nonnegative nanoseconds.
// Sampling at sync is separate from an operation's historical occurred_at.
type ClockSkew struct {
	Seconds      int64
	Nanos        int32
	LimitSeconds int64
	Flag         string
}

// AssessClockSkew is a one-way receipt estimate; transit delay can affect it.
// It flags only a strictly exceeded limit and never rewrites either input.
func AssessClockSkew(deviceSample, serverReceipt time.Time, limitSeconds *int64) (ClockSkew, error) {
	limit := DefaultSkewLimitSeconds
	if limitSeconds != nil {
		limit = *limitSeconds
	}
	if !validInstant(deviceSample) || !validReceipt(serverReceipt) || limit < 0 {
		return ClockSkew{}, ErrInvalid
	}
	// Do not use time.Sub: its Duration saturates for samples over ~292 years apart.
	seconds := deviceSample.Unix() - serverReceipt.Unix()
	nanos := deviceSample.Nanosecond() - serverReceipt.Nanosecond()
	if nanos < 0 {
		seconds--
		nanos += 1_000_000_000
	}
	out := ClockSkew{Seconds: seconds, Nanos: int32(nanos), LimitSeconds: limit}
	if seconds < -limit || seconds > limit || (seconds == limit && nanos > 0) {
		out.Flag = "CLOCK_SKEW"
	}
	return out, nil
}
