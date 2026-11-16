package application

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
)

// Exact schema 1 payloads (EDR-277). Identity, author and declaration belong
// to the envelope; the server decides everything else.
var (
	driverKeys  = []string{"trip_id", "origin", "permission", "notification_visible"}
	eventKeys   = []string{"trip_id", "cause"}
	batchKeys   = []string{"trip_id", "seq", "source", "boot_id", "base_elapsed_millis", "base", "fixes", "dropped"}
	baseKeys    = []string{"lat_e7", "lon_e7"}
	droppedKeys = []string{"accuracy", "stale", "jump", "mock"}
	bootPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

const (
	maxLat = 900000000
	maxLon = 1800000000
)

func object(raw json.RawMessage, keys []string) (map[string]json.RawMessage, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || len(fields) != len(keys) {
		return nil, false
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return nil, false
		}
	}
	return fields, true
}

func isNull(raw json.RawMessage) bool { return strings.TrimSpace(string(raw)) == "null" }

func text(fields map[string]json.RawMessage, key string) (string, bool) {
	var value string
	raw := fields[key]
	if isNull(raw) || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

// integer decodes a JSON integer (the protocol admits no fraction or
// exponent) within [low, high].
func integer(raw json.RawMessage, low, high int64) (int64, bool) {
	var value int64
	if len(raw) == 0 || isNull(raw) || json.Unmarshal(raw, &value) != nil || value < low || value > high {
		return 0, false
	}
	return value, true
}

func optionalInteger(raw json.RawMessage, low, high int64) (*int64, bool) {
	if isNull(raw) {
		return nil, true
	}
	v, ok := integer(raw, low, high)
	return &v, ok
}

func canonicalID(raw string) bool {
	v, e := kernel.ParseOrgID(raw)
	return e == nil && v.String() == raw
}

// envelope checks the shared operation shape and returns the declared
// instant: the tracking entity, no expected revision, no attachment or soft
// reference, and whole microseconds (EDR-279).
func envelope(op protocol.Operation, kind string) (protocol.Envelope, time.Time, bool) {
	v := op.Snapshot()
	if v.Type != kind || !op.SupportsSchema() || v.Entity.Type != EntityType || v.Entity.ExpectedRevision != nil || !canonicalID(v.Entity.ID) ||
		len(v.Attachments) != 0 || len(v.SoftReferences) != 0 {
		return protocol.Envelope{}, time.Time{}, false
	}
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if err != nil || at.IsZero() || at.Nanosecond()%1000 != 0 {
		return protocol.Envelope{}, time.Time{}, false
	}
	return v, at.UTC(), true
}

func sessionKind(kind string) bool {
	for _, k := range SessionOperationTypes() {
		if k == kind {
			return true
		}
	}
	return false
}

// ParseSession decodes one gps.session.* schema 1 operation
// (B-BR-EMP-429). ErrMalformed is final.
func ParseSession(op protocol.Operation) (SessionCommand, error) {
	kind := op.Snapshot().Type
	if !sessionKind(kind) {
		return SessionCommand{}, ErrMalformed
	}
	v, at, ok := envelope(op, kind)
	if !ok {
		return SessionCommand{}, ErrMalformed
	}
	in := SessionInput{Type: kind, OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), SessionID: v.Entity.ID, AuthorMembershipID: v.MembershipID, Declared: at}
	keys := eventKeys
	if kind == StartOperationType || kind == ResumeOperationType {
		keys = driverKeys
	}
	fields, ok := object(op.PayloadBytes(), keys)
	if !ok {
		return SessionCommand{}, ErrMalformed
	}
	if in.TripID, ok = text(fields, "trip_id"); !ok || !canonicalID(in.TripID) {
		return SessionCommand{}, ErrMalformed
	}
	if kind == StartOperationType || kind == ResumeOperationType {
		origin, okOrigin := text(fields, "origin")
		permission, okPermission := text(fields, "permission")
		var visible *bool
		if !okOrigin || !okPermission || json.Unmarshal(fields["notification_visible"], &visible) != nil || visible == nil {
			return SessionCommand{}, ErrMalformed
		}
		var errOrigin, errPermission error
		in.Origin, errOrigin = domain.ParseOrigin(origin)
		in.Permission, errPermission = domain.ParsePermission(permission)
		if errOrigin != nil || errPermission != nil {
			return SessionCommand{}, ErrMalformed
		}
		in.NotificationVisible = *visible
		return SessionCommand{valid: true, value: in}, nil
	}
	cause, ok := text(fields, "cause")
	if !ok {
		return SessionCommand{}, ErrMalformed
	}
	var err error
	if in.Cause, err = domain.ParseCause(cause); err != nil || in.Cause == domain.CauseNotStarted {
		return SessionCommand{}, ErrMalformed
	}
	return SessionCommand{valid: true, value: in}, nil
}

// ParseBatch decodes one gps.track.batch schema 1 operation
// (B-BR-EMP-431). ErrMalformed is final.
func ParseBatch(op protocol.Operation) (BatchCommand, error) {
	v, at, ok := envelope(op, BatchOperationType)
	if !ok {
		return BatchCommand{}, ErrMalformed
	}
	fields, ok := object(op.PayloadBytes(), batchKeys)
	if !ok {
		return BatchCommand{}, ErrMalformed
	}
	b := BatchInput{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), SessionID: v.Entity.ID, Base: at}
	var okSeq, okSource bool
	b.TripID, ok = text(fields, "trip_id")
	b.Seq, okSeq = integer(fields["seq"], 1, MaxSequence)
	b.Source, okSource = text(fields, "source")
	if !ok || !okSeq || !okSource || !canonicalID(b.TripID) || b.Source != SourcePhone {
		return BatchCommand{}, ErrMalformed
	}
	if !isNull(fields["boot_id"]) {
		boot, ok := text(fields, "boot_id")
		if !ok || !bootPattern.MatchString(boot) {
			return BatchCommand{}, ErrMalformed
		}
		b.BootID = &boot
	}
	if b.BaseElapsed, ok = optionalInteger(fields["base_elapsed_millis"], 0, 1<<53); !ok || (b.BootID == nil) != (b.BaseElapsed == nil) {
		return BatchCommand{}, ErrMalformed
	}
	base, ok := object(fields["base"], baseKeys)
	if !ok {
		return BatchCommand{}, ErrMalformed
	}
	lat, okLat := integer(base["lat_e7"], -maxLat, maxLat)
	lon, okLon := integer(base["lon_e7"], -maxLon, maxLon)
	if !okLat || !okLon {
		return BatchCommand{}, ErrMalformed
	}
	b.BaseLat, b.BaseLon = int32(lat), int32(lon)
	dropped, ok := object(fields["dropped"], droppedKeys)
	if !ok {
		return BatchCommand{}, ErrMalformed
	}
	for _, d := range []struct {
		key    string
		target *int32
	}{{"accuracy", &b.Dropped.Accuracy}, {"stale", &b.Dropped.Stale}, {"jump", &b.Dropped.Jump}, {"mock", &b.Dropped.Mock}} {
		n, ok := integer(dropped[d.key], 0, MaxDropped)
		if !ok {
			return BatchCommand{}, ErrMalformed
		}
		*d.target = int32(n)
	}
	var list []json.RawMessage
	if json.Unmarshal(fields["fixes"], &list) != nil || len(list) < 1 || len(list) > MaxFixes {
		return BatchCommand{}, ErrMalformed
	}
	previous := int64(-1)
	for _, raw := range list {
		f, offset, ok := fix(raw, &lat, &lon)
		if !ok || offset <= previous {
			return BatchCommand{}, ErrMalformed
		}
		previous = offset
		f.At = at.Add(time.Duration(offset) * time.Millisecond)
		b.Fixes = append(b.Fixes, f)
	}
	return BatchCommand{valid: true, value: b}, nil
}

// fix decodes one [offset_ms, Δlat_e7, Δlon_e7, accuracy_m, speed_cm_s,
// bearing_deg, flags] and accumulates the position.
func fix(raw json.RawMessage, lat, lon *int64) (Fix, int64, bool) {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || len(values) != 7 {
		return Fix{}, 0, false
	}
	offset, ok1 := integer(values[0], 0, MaxOffsetMillis)
	dLat, ok2 := integer(values[1], -2*maxLat, 2*maxLat)
	dLon, ok3 := integer(values[2], -2*maxLon, 2*maxLon)
	accuracy, ok4 := integer(values[3], 0, MaxAccuracy)
	speed, ok5 := optionalInteger(values[4], 0, MaxSpeed)
	bearing, ok6 := optionalInteger(values[5], 0, 359)
	flags, ok7 := integer(values[6], 0, FlagCoarse|FlagMock)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 || !ok7 {
		return Fix{}, 0, false
	}
	*lat += dLat
	*lon += dLon
	if *lat < -maxLat || *lat > maxLat || *lon < -maxLon || *lon > maxLon {
		return Fix{}, 0, false
	}
	f := Fix{Lat: int32(*lat), Lon: int32(*lon), Accuracy: int32(accuracy), Flags: int16(flags)}
	if speed != nil {
		s := int32(*speed)
		f.Speed = &s
	}
	if bearing != nil {
		c := int16(*bearing)
		f.Bearing = &c
	}
	return f, offset, true
}
