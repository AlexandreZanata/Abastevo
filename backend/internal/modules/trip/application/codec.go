package application

import (
	"encoding/json"
	"strconv"
	"strings"

	fleet "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain"
	meter "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

// Exact schema 1 payloads (EDR-202). Identity, author, declaration and
// receipt belong to the envelope or the server.
var (
	startKeys   = []string{"asset_id", "driver_employee_id", "reading", "missing_reading_reason", "run_clock"}
	finishKeys  = []string{"reading", "missing_reading_reason", "run_clock"}
	readingKeys = []string{"meter_id", "segment_id", "reading_id", "kind", "value"}
	clockKeys   = []string{"boot_id", "elapsed_millis"}
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
	raw, ok := fields[key]
	if !ok || isNull(raw) || json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	return value, true
}

func optional(fields map[string]json.RawMessage, key string) (*string, bool) {
	raw := fields[key]
	if isNull(raw) {
		return nil, true
	}
	value, ok := text(fields, key)
	return &value, ok
}

// canonicalCount accepts the canonical non-negative int64 text only.
func canonicalCount(raw string) (int64, bool) {
	n, err := strconv.ParseInt(raw, 10, 64)
	return n, err == nil && n >= 0 && strconv.FormatInt(n, 10) == raw
}

// scopeOf binds an envelope to its organization, refusing anything else.
func scopeOf(v protocol.Envelope) (kernel.OrgScope, bool) {
	org, err := kernel.ParseOrgID(v.OrgID)
	if err != nil {
		return kernel.OrgScope{}, false
	}
	scope, err := kernel.NewOrgScope(org)
	return scope, err == nil
}

// event decodes the shared reading, reason, run clock and declaration and
// proves them through the EMP-TRIP-001/017 domain types.
func event(op protocol.Operation, fields map[string]json.RawMessage) (Event, bool) {
	v := op.Snapshot()
	e := Event{OperationID: op.ID(), PayloadDigest: op.PayloadDigest(), TripID: v.Entity.ID, AuthorMembershipID: v.MembershipID}
	at, err := orgtime.ParseInstant(v.OccurredAt)
	if err != nil {
		return Event{}, false
	}
	if e.Declared, err = orgtime.ValidateDeclaration(orgtime.Declaration{OccurredAt: at, EntryZone: v.EntryZone, DeviceOffsetSeconds: v.DeviceOffsetSeconds}); err != nil {
		return Event{}, false
	}
	reason, ok := optional(fields, "missing_reading_reason")
	if !ok {
		return Event{}, false
	}
	if reason != nil {
		if _, err := meter.NewMissing(*reason); err != nil {
			return Event{}, false
		}
		e.MissingReason = reason
	}
	if raw := fields["reading"]; !isNull(raw) {
		r, ok := object(raw, readingKeys)
		if !ok {
			return Event{}, false
		}
		values := map[string]string{}
		for _, key := range readingKeys {
			if values[key], ok = text(r, key); !ok {
				return Event{}, false
			}
		}
		count, err := meter.ParseBase(meter.Kind(values["kind"]), values["value"], nil)
		if err != nil || !canonicalID(values["meter_id"]) || !canonicalID(values["segment_id"]) || !canonicalID(values["reading_id"]) {
			return Event{}, false
		}
		e.Reading = &Reading{MeterID: values["meter_id"], SegmentID: values["segment_id"], ReadingID: values["reading_id"], Value: count}
	}
	if (e.Reading == nil) == (e.MissingReason == nil) {
		return Event{}, false
	}
	if raw := fields["run_clock"]; !isNull(raw) {
		c, ok := object(raw, clockKeys)
		if !ok {
			return Event{}, false
		}
		boot, ok := text(c, "boot_id")
		elapsed, okElapsed := text(c, "elapsed_millis")
		count, okCount := canonicalCount(elapsed)
		if !ok || !okElapsed || !okCount {
			return Event{}, false
		}
		clock, err := trip.NewRunClock(boot, count)
		if err != nil {
			return Event{}, false
		}
		e.Clock = &clock
	}
	return e, true
}

// observation is the domain evidence of an event on the given asset.
func observation(scope kernel.OrgScope, asset fleet.AssetRef, e Event) (trip.MeterObservation, error) {
	if e.Reading == nil {
		missing, err := meter.NewMissing(*e.MissingReason)
		if err != nil {
			return trip.MeterObservation{}, err
		}
		return trip.AbsentReading(missing)
	}
	m, err := meter.NewMeterRef(scope, asset, e.Reading.MeterID, e.Reading.Value.Kind(), nil)
	if err != nil {
		return trip.MeterObservation{}, err
	}
	segment, err := meter.NewSegmentRef(scope, m, e.Reading.SegmentID)
	if err != nil {
		return trip.MeterObservation{}, err
	}
	ref, err := meter.NewReadingRef(scope, segment, e.Reading.ReadingID)
	if err != nil {
		return trip.MeterObservation{}, err
	}
	return trip.ObservedReading(ref, e.Reading.Value)
}

// envelope checks the shared operation shape of both trip operations.
func envelope(op protocol.Operation, kind string) (protocol.Envelope, bool) {
	v := op.Snapshot()
	return v, v.Type == kind && op.SupportsSchema() && v.Entity.Type == EntityType && v.Entity.ExpectedRevision == nil && canonicalID(v.Entity.ID)
}

// ParseStart decodes one trip.start schema 1 operation (B-BR-EMP-312).
// ErrMalformed is final; ErrUnsupported keeps a recognised form unavailable.
func ParseStart(op protocol.Operation) (StartCommand, error) {
	v, ok := envelope(op, StartOperationType)
	if !ok {
		return StartCommand{}, ErrMalformed
	}
	fields, ok := object(op.PayloadBytes(), startKeys)
	if !ok {
		return StartCommand{}, ErrMalformed
	}
	asset, okAsset := text(fields, "asset_id")
	driver, okDriver := text(fields, "driver_employee_id")
	e, okEvent := event(op, fields)
	if !okAsset || !okDriver || !okEvent || !canonicalID(asset) || !canonicalID(driver) {
		return StartCommand{}, ErrMalformed
	}
	// The pure EMP-TRIP-001 start re-proves the eligible context; zones are
	// server facts, so a placeholder zone proves only the event shape here.
	if _, err := StartDomain(v, StartInput{Event: e, AssetID: asset, DriverID: driver}, orgtime.ReportingZones{Organization: "UTC"}); err != nil {
		return StartCommand{}, ErrMalformed
	}
	if len(v.Attachments) != 0 || len(v.DependsOn) != 0 || len(v.SoftReferences) != 0 {
		return StartCommand{}, ErrUnsupported
	}
	return StartCommand{valid: true, value: StartInput{Event: e, AssetID: asset, DriverID: driver}}, nil
}

// StartDomain builds the EMP-TRIP-001 trip of a start with the resolved zones.
func StartDomain(v protocol.Envelope, in StartInput, zones orgtime.ReportingZones) (trip.Trip, error) {
	scope, ok := scopeOf(v)
	if !ok {
		return trip.Trip{}, ErrMalformed
	}
	assetID, err := fleet.ParseAssetID(in.AssetID)
	if err != nil {
		return trip.Trip{}, err
	}
	asset, err := fleet.NewAssetRef(scope, assetID)
	if err != nil {
		return trip.Trip{}, err
	}
	s := trip.StartInput{Asset: asset, Declared: &in.Declared, Zones: zones, Clock: in.Clock}
	if s.Reference, err = trip.NewTripRef(scope, in.TripID); err != nil {
		return trip.Trip{}, err
	}
	if s.Author, err = trip.NewMembershipRef(scope, in.AuthorMembershipID); err != nil {
		return trip.Trip{}, err
	}
	if s.Driver, err = trip.NewEmployeeRef(scope, in.DriverID); err != nil {
		return trip.Trip{}, err
	}
	if s.Departure, err = observation(scope, asset, in.Event); err != nil {
		return trip.Trip{}, err
	}
	return trip.Start(scope, s)
}

// ParseFinish decodes one trip.finish schema 1 operation (B-BR-EMP-315).
func ParseFinish(op protocol.Operation) (FinishCommand, error) {
	v, ok := envelope(op, FinishOperationType)
	if !ok || len(v.DependsOn) > 1 {
		return FinishCommand{}, ErrMalformed
	}
	fields, ok := object(op.PayloadBytes(), finishKeys)
	if !ok {
		return FinishCommand{}, ErrMalformed
	}
	e, ok := event(op, fields)
	if !ok {
		return FinishCommand{}, ErrMalformed
	}
	if len(v.Attachments) != 0 || len(v.SoftReferences) != 0 {
		return FinishCommand{}, ErrUnsupported
	}
	return FinishCommand{valid: true, value: FinishInput{Event: e, DependsOn: v.DependsOn}}, nil
}

// FinishDomain rebuilds the stored trip through the pure domain and finishes
// it with this event (EMP-TRIP-001 rules at receipt).
func FinishDomain(scope kernel.OrgScope, record Record, in FinishInput) (trip.Trip, error) {
	start := StartInput{Event: Event{OperationID: record.OperationID, TripID: record.ID, AuthorMembershipID: record.AuthorMembershipID, Declared: record.Declared,
		MissingReason: copyPointer(record.MissingReason), Clock: copyPointer(record.Clock)}, AssetID: record.AssetID, DriverID: record.DriverID}
	if record.ReadingID != "" {
		if record.ReadingValue == nil {
			return trip.Trip{}, ErrUnavailable
		}
		kind, err := meterKindOf(record)
		if err != nil {
			return trip.Trip{}, err
		}
		value, err := meter.NewValue(kind, *record.ReadingValue, nil)
		if err != nil {
			return trip.Trip{}, ErrUnavailable
		}
		start.Reading = &Reading{MeterID: record.MeterID, SegmentID: record.SegmentID, ReadingID: record.ReadingID, Value: value}
	}
	v := protocol.Envelope{OrgID: scope.OrgID().String()}
	started, err := StartDomain(v, start, orgtime.ReportingZones{Organization: record.Reporting.Zone})
	if err != nil || started.Reporting().Day != record.Reporting.Day {
		return trip.Trip{}, ErrUnavailable
	}
	finish := trip.FinishInput{Declared: &in.Declared, Clock: in.Clock}
	if finish.Author, err = trip.NewMembershipRef(scope, in.AuthorMembershipID); err != nil {
		return trip.Trip{}, err
	}
	if finish.Final, err = observation(scope, started.Asset(), in.Event); err != nil {
		return trip.Trip{}, err
	}
	return started.Finish(scope, finish)
}

// meterKindOf is the stored departure kind, carried by the record's value kind.
func meterKindOf(record Record) (meter.Kind, error) {
	if record.ReadingKind == "" {
		return "", ErrUnavailable
	}
	return meter.Kind(record.ReadingKind), nil
}
