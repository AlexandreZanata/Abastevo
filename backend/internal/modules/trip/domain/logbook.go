package domain

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

// Logbook failures (EMP-TRIP-003); identity, scope and author failures keep
// ErrInvalid, ErrScope and ErrForbidden.
var (
	// ErrUnsupportedEvent keeps a reserved type for its owner (B-BR-EMP-320).
	ErrUnsupportedEvent = errors.New("trip: logbook event type reserved for a later owner")
	// ErrEventBeforeStart refuses an event declared before its trip started.
	ErrEventBeforeStart = errors.New("trip: logbook event declared before the trip start")
	// ErrEventAfterFinish marks an event declared after a completed trip's finish.
	ErrEventAfterFinish = errors.New("trip: logbook event declared after the trip finish")
)

// LogbookType is one token of the closed trip-logbook-v1 vocabulary.
type LogbookType string

const (
	LogbookNote      LogbookType = "NOTE"
	LogbookStop      LogbookType = "STOP"
	LogbookArrival   LogbookType = "ARRIVAL"
	LogbookDeparture LogbookType = "DEPARTURE"
	// Reserved for their owners; recognised, never recorded here.
	LogbookFueling  LogbookType = "FUELING"
	LogbookToll     LogbookType = "TOLL"
	LogbookParking  LogbookType = "PARKING"
	LogbookIncident LogbookType = "INCIDENT"
	LogbookDamage   LogbookType = "DAMAGE"
	LogbookHandover LogbookType = "HANDOVER"
	LogbookPause    LogbookType = "PAUSE"
	LogbookResume   LogbookType = "RESUME"
	LogbookCancel   LogbookType = "CANCEL"
)

var logbookTypes = []LogbookType{LogbookNote, LogbookStop, LogbookArrival, LogbookDeparture, LogbookFueling, LogbookToll, LogbookParking,
	LogbookIncident, LogbookDamage, LogbookHandover, LogbookPause, LogbookResume, LogbookCancel}

// LogbookTypes returns the closed vocabulary in its fixed order.
func LogbookTypes() []LogbookType { return slices.Clone(logbookTypes) }

// ParseLogbookType accepts the exact tokens only; anything else is ErrInvalid.
func ParseLogbookType(raw string) (LogbookType, error) {
	if !slices.Contains(logbookTypes, LogbookType(raw)) {
		return "", ErrInvalid
	}
	return LogbookType(raw), nil
}

// Supported is true for the four types recorded in v1.
func (t LogbookType) Supported() bool {
	return t == LogbookNote || t == LogbookStop || t == LogbookArrival || t == LogbookDeparture
}

// RequiresText is true only for NOTE.
func (t LogbookType) RequiresText() bool { return t == LogbookNote }

// ValidLogbookText checks the text of a supported type: required for NOTE;
// when present nonblank, free of control characters and at most 280 Unicode
// scalars (the METER-001 reason profile, SQL org_asset_text_valid).
func ValidLogbookText(kind LogbookType, text *string) error {
	if text == nil && kind.RequiresText() {
		return ErrInvalid
	}
	if text != nil {
		if _, err := meter.NewMissing(*text); err != nil {
			return ErrInvalid
		}
	}
	return nil
}

// LogbookRef is the device identity of one logbook event; its shape proves no existence.
type LogbookRef struct{ scopedID }

func NewLogbookRef(scope kernel.OrgScope, id string) (LogbookRef, error) {
	r, e := scoped(scope, id)
	return LogbookRef{r}, e
}

// LogbookInput is one event as the author's device recorded it; Type is the raw token.
type LogbookInput struct {
	redacted
	Reference LogbookRef
	Author    MembershipRef
	Type      string
	Text      *string
	Declared  *orgtime.Declaration
}

// LogbookEntry is one logbook fact of a trip; it never changes the trip.
type LogbookEntry struct {
	redacted
	trip      TripRef
	reference LogbookRef
	author    MembershipRef
	kind      LogbookType
	text      *string
	declared  orgtime.Declaration
}

func (e LogbookEntry) Trip() TripRef                 { return e.trip }
func (e LogbookEntry) Reference() LogbookRef         { return e.reference }
func (e LogbookEntry) Author() MembershipRef         { return e.author }
func (e LogbookEntry) Type() LogbookType             { return e.kind }
func (e LogbookEntry) Text() *string                 { return copyPointer(e.text) }
func (e LogbookEntry) Declared() orgtime.Declaration { return e.declared }

// Log validates one logbook event against the trip (B-BR-EMP-320/323), first
// failure first: scope and trip, event identity, author, type (unknown
// ErrInvalid, reserved ErrUnsupportedEvent), text, declaration, the starting
// member (ErrForbidden), then time (ErrEventBeforeStart; after the finish of
// a completed trip or the cancellation of a cancelled one
// ErrEventAfterFinish). The trip is never changed.
func (t Trip) Log(scope kernel.OrgScope, in LogbookInput) (LogbookEntry, error) {
	if scope.Validate() != nil {
		return LogbookEntry{}, ErrScope
	}
	if err := t.reference.validate(scope); err != nil {
		return LogbookEntry{}, err
	}
	// An absent (zero) identity or author is ErrInvalid, another organization's ErrScope.
	if err := in.Reference.validate(scope); err != nil {
		return LogbookEntry{}, err
	}
	if err := in.Author.validate(scope); err != nil {
		return LogbookEntry{}, err
	}
	kind, err := ParseLogbookType(in.Type)
	if err != nil {
		return LogbookEntry{}, err
	}
	if !kind.Supported() {
		return LogbookEntry{}, ErrUnsupportedEvent
	}
	if err := ValidLogbookText(kind, in.Text); err != nil {
		return LogbookEntry{}, err
	}
	if in.Declared == nil {
		return LogbookEntry{}, ErrInvalid
	}
	declared, err := orgtime.ValidateDeclaration(*in.Declared)
	if err != nil {
		return LogbookEntry{}, ErrInvalid
	}
	if in.Author != t.Responsible() {
		return LogbookEntry{}, ErrForbidden
	}
	if declared.OccurredAt.Before(t.start.declared.OccurredAt) {
		return LogbookEntry{}, ErrEventBeforeStart
	}
	if end, ok := t.Finished(); ok && declared.OccurredAt.After(end.Declared().OccurredAt) {
		return LogbookEntry{}, ErrEventAfterFinish
	}
	// EMP-TRIP-011: a cancellation closes the logbook like a finish.
	if end, ok := t.Cancellation(); ok && declared.OccurredAt.After(end.Declared().OccurredAt) {
		return LogbookEntry{}, ErrEventAfterFinish
	}
	// EMP-TRIP-014 (B-BR-EMP-402): a closure has no device instant, so every
	// later event is after it and is kept for review.
	if t.closure != nil {
		return LogbookEntry{}, ErrEventAfterFinish
	}
	return LogbookEntry{trip: t.reference, reference: in.Reference, author: in.Author, kind: kind, text: copyPointer(in.Text), declared: declared}, nil
}

// TimelineKind is the kind of one timeline item.
type TimelineKind string

const (
	TimelineStart  TimelineKind = "START"
	TimelineEvent  TimelineKind = "EVENT"
	TimelineFinish TimelineKind = "FINISH"
)

// TimelineItem is one item of a trip's timeline; the receipt is shown beside
// it but is never an ordering key (the device orders before any receipt).
type TimelineItem struct {
	Kind TimelineKind
	ID   string
	At   time.Time
}

func timelineRank(kind TimelineKind) int {
	switch kind {
	case TimelineStart:
		return 0
	case TimelineEvent:
		return 1
	case TimelineFinish:
		return 2
	}
	return 3
}

// CompareTimeline is the order shared with Kotlin (B-BR-EMP-324): original
// instant; at equal instants the start, then events by identity, then the
// finish.
func CompareTimeline(a, b TimelineItem) int {
	if c := a.At.Compare(b.At); c != 0 {
		return c
	}
	if c := timelineRank(a.Kind) - timelineRank(b.Kind); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

// SortTimeline orders the items in place.
func SortTimeline(items []TimelineItem) { slices.SortStableFunc(items, CompareTimeline) }
