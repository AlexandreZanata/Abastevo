package application_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgsync/protocol"
	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	app "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/application"
	track "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/tracking/domain"
	tripapp "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/application"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

func id(n int) string { return fmt.Sprintf("7b0e2a54-0000-4000-8000-%012d", n) }

const origin = "https://api.example.invalid"

var base = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func at(seconds int) time.Time { return base.Add(time.Duration(seconds) * time.Second) }

func scope(t *testing.T) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(id(65))
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func operation(t *testing.T, kind string, declared time.Time, body map[string]any, change func(envelope, payload map[string]any)) protocol.Operation {
	t.Helper()
	envelope := map[string]any{"schema_version": 1, "operation_id": id(20), "owner_account_id": id(1), "api_origin": origin, "org_id": id(65), "membership_id": id(3),
		"enrollment_id": id(4), "type": kind, "operation_schema": 1, "entity": map[string]any{"type": app.EntityType, "id": id(11), "expected_revision": nil},
		"occurred_at": orgtime.FormatInstant(declared), "entry_zone": "America/Cuiaba", "device_offset_seconds": -14400, "depends_on": []any{},
		"soft_references": []any{}, "attachments": []any{}, "payload": body}
	if change != nil {
		change(envelope, body)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := protocol.NewBinding(id(1), origin, id(65))
	if err != nil {
		t.Fatal(err)
	}
	op, err := protocol.Seal(raw, binding)
	if err != nil {
		t.Fatal("invalid unit operation fixture", err)
	}
	return op
}

func driverBody() map[string]any {
	return map[string]any{"trip_id": id(21), "origin": "TRIP_SCREEN", "permission": "PRECISE", "notification_visible": true}
}

func eventBody(cause string) map[string]any { return map[string]any{"trip_id": id(21), "cause": cause} }

func fixes(n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = []any{i * 10000, 0, -12670, 8, 1389, 270, 0}
	}
	return out
}

func batchBody(list []any) map[string]any {
	return map[string]any{"trip_id": id(21), "seq": 42, "source": "PHONE", "boot_id": "boot-a1", "base_elapsed_millis": 9063000,
		"base": map[string]any{"lat_e7": -100000000, "lon_e7": -500000000}, "fixes": list,
		"dropped": map[string]any{"accuracy": 1, "stale": 0, "jump": 0, "mock": 0}}
}

// B-BR-EMP-429: the envelope owns identity, author and declaration; the
// payload carries the trip and the command's own facts.
func TestParseSessionKeepsEveryOperation(t *testing.T) {
	for _, kind := range app.SessionOperationTypes() {
		t.Run(kind, func(t *testing.T) {
			body := eventBody("PROCESS_STOPPED")
			if kind == app.StartOperationType || kind == app.ResumeOperationType {
				body = driverBody()
			}
			op := operation(t, kind, at(60), body, nil)
			command, err := app.ParseSession(op)
			if err != nil {
				t.Fatal(err)
			}
			v, err := command.Snapshot()
			if err != nil || v.Type != kind || v.OperationID != op.ID() || v.PayloadDigest != op.PayloadDigest() || v.SessionID != id(11) || v.TripID != id(21) ||
				v.AuthorMembershipID != id(3) || !v.Declared.Equal(at(60)) {
				t.Fatal("parsed session operation lost its identity", err)
			}
			if kind == app.StartOperationType || kind == app.ResumeOperationType {
				if v.Origin != track.OriginTripScreen || v.Permission != track.PermissionPrecise || !v.NotificationVisible || v.Cause != "" {
					t.Fatal("driver facts", v.Origin, v.Permission)
				}
			} else if v.Cause != track.CauseProcessStopped || v.Origin != "" || v.Permission != "" {
				t.Fatal("event facts", v.Cause)
			}
		})
	}
	if _, err := (app.SessionCommand{}).Snapshot(); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("the zero command is not usable")
	}
}

func TestParseSessionRefusesEveryMalformedForm(t *testing.T) {
	cases := map[string]struct {
		kind   string
		change func(e, p map[string]any)
	}{
		"other entity type": {app.StartOperationType, func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": "trip", "id": id(11), "expected_revision": nil}
		}},
		"expected revision": {app.StartOperationType, func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": app.EntityType, "id": id(11), "expected_revision": 1}
		}},
		"unknown key":              {app.StartOperationType, func(_, p map[string]any) { p["latitude"] = 1 }},
		"missing key":              {app.StartOperationType, func(_, p map[string]any) { delete(p, "notification_visible") }},
		"absent permission":        {app.StartOperationType, func(_, p map[string]any) { p["permission"] = nil }},
		"unknown permission":       {app.ResumeOperationType, func(_, p map[string]any) { p["permission"] = "FINE" }},
		"unknown origin":           {app.StartOperationType, func(_, p map[string]any) { p["origin"] = "SCREEN" }},
		"textual notification":     {app.StartOperationType, func(_, p map[string]any) { p["notification_visible"] = "true" }},
		"non canonical trip":       {app.StartOperationType, func(_, p map[string]any) { p["trip_id"] = strings.ToUpper(id(21)) }},
		"cause on a driver action": {app.ResumeOperationType, func(_, p map[string]any) { p["cause"] = "PROCESS_STOPPED" }},
		"unknown cause":            {app.InterruptOperationType, func(_, p map[string]any) { p["cause"] = "SIGNAL_LOST" }},
		"absent cause":             {app.EndOperationType, func(_, p map[string]any) { p["cause"] = nil }},
		"permission on an event":   {app.SuspendOperationType, func(_, p map[string]any) { p["permission"] = "PRECISE" }},
		"nanosecond instant":       {app.ReduceOperationType, func(e, _ map[string]any) { e["occurred_at"] = "2026-11-15T12:01:00.000000123Z" }},
		"attachment": {app.EndOperationType, func(e, _ map[string]any) {
			e["attachments"] = []any{map[string]any{"id": id(90), "sha256": strings.Repeat("a", 64), "bytes": 1, "mime": "image/jpeg"}}
		}},
		"soft reference": {app.EndOperationType, func(e, _ map[string]any) { e["soft_references"] = []any{map[string]any{"type": "trip", "id": id(21)}} }},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			body := eventBody("TRIP_COMPLETED")
			if c.kind == app.StartOperationType || c.kind == app.ResumeOperationType {
				body = driverBody()
			}
			if _, err := app.ParseSession(operation(t, c.kind, at(60), body, c.change)); !errors.Is(err, app.ErrMalformed) {
				t.Fatal("malformed session operation admitted", err)
			}
		})
	}
	if _, err := app.ParseSession(operation(t, app.BatchOperationType, at(60), driverBody(), nil)); !errors.Is(err, app.ErrMalformed) {
		t.Fatal("a batch parsed as a session operation", err)
	}
}

// B-BR-EMP-431: one bounded batch with absolute instants and positions.
func TestParseBatchKeepsTheExactFixes(t *testing.T) {
	list := []any{[]any{0, 0, 0, 8, 1389, 270, 0}, []any{10000, 0, -12670, 6, nil, nil, 1}, []any{25000, 15, 30, 0, 0, 0, 2}}
	op := operation(t, app.BatchOperationType, at(120), batchBody(list), nil)
	command, err := app.ParseBatch(op)
	if err != nil {
		t.Fatal(err)
	}
	v, err := command.Snapshot()
	if err != nil || v.OperationID != op.ID() || v.PayloadDigest != op.PayloadDigest() || v.SessionID != id(11) || v.TripID != id(21) || v.Seq != 42 || v.Source != "PHONE" ||
		v.BootID == nil || *v.BootID != "boot-a1" || v.BaseElapsed == nil || *v.BaseElapsed != 9063000 || !v.Base.Equal(at(120)) || v.BaseLat != -100000000 || v.BaseLon != -500000000 ||
		v.Dropped != (app.Dropped{Accuracy: 1}) || len(v.Fixes) != 3 {
		t.Fatal("parsed batch lost its envelope", err)
	}
	want := []struct {
		at        time.Time
		lat, lon  int32
		accuracy  int32
		flags     int16
		hasSpeed  bool
		hasCourse bool
	}{
		{at(120), -100000000, -500000000, 8, 0, true, true},
		{at(130), -100000000, -500012670, 6, 1, false, false},
		{at(145), -99999985, -500012640, 0, 2, true, true},
	}
	for i, w := range want {
		f := v.Fixes[i]
		if !f.At.Equal(w.at) || f.Lat != w.lat || f.Lon != w.lon || f.Accuracy != w.accuracy || f.Flags != w.flags || (f.Speed != nil) != w.hasSpeed || (f.Bearing != nil) != w.hasCourse {
			t.Fatalf("fix %d: %v %d %d", i, f.At, f.Lat, f.Lon)
		}
	}
	if *v.Fixes[0].Speed != 1389 || *v.Fixes[0].Bearing != 270 || *v.Fixes[2].Speed != 0 || *v.Fixes[2].Bearing != 0 {
		t.Fatal("optional speed and bearing")
	}
	*v.Fixes[0].Speed = 1
	v.Fixes[1].Lat = 0
	if again, _ := command.Snapshot(); *again.Fixes[0].Speed != 1389 || again.Fixes[1].Lat != -100000000 {
		t.Fatal("a snapshot shares its fixes")
	}
	op = operation(t, app.BatchOperationType, at(120), batchBody(fixes(app.MaxFixes)), func(_, p map[string]any) { p["boot_id"], p["base_elapsed_millis"] = nil, nil })
	if command, err := app.ParseBatch(op); err != nil {
		t.Fatal("the largest batch was refused", err)
	} else if v, _ := command.Snapshot(); len(v.Fixes) != app.MaxFixes || v.BootID != nil || v.BaseElapsed != nil {
		t.Fatal("largest batch")
	}
	op = operation(t, app.BatchOperationType, at(120), batchBody(fixes(1)), func(e, _ map[string]any) { e["depends_on"] = []any{id(30)} })
	if _, err := app.ParseBatch(op); err != nil {
		t.Fatal("a batch naming its session operation was refused", err)
	}
}

func TestParseBatchRefusesEveryMalformedOrOversizedForm(t *testing.T) {
	fix := func(values ...any) []any { return []any{values} }
	cases := map[string]func(e, p map[string]any){
		"no fix":                 func(_, p map[string]any) { p["fixes"] = []any{} },
		"oversized":              func(_, p map[string]any) { p["fixes"] = fixes(app.MaxFixes + 1) },
		"repeated offset":        func(_, p map[string]any) { p["fixes"] = []any{[]any{0, 0, 0, 8, 0, 0, 0}, []any{0, 1, 1, 8, 0, 0, 0}} },
		"decreasing offset":      func(_, p map[string]any) { p["fixes"] = []any{[]any{5, 0, 0, 8, 0, 0, 0}, []any{4, 1, 1, 8, 0, 0, 0}} },
		"negative offset":        func(_, p map[string]any) { p["fixes"] = fix(-1, 0, 0, 8, 0, 0, 0) },
		"offset beyond a day":    func(_, p map[string]any) { p["fixes"] = fix(app.MaxOffsetMillis+1, 0, 0, 8, 0, 0, 0) },
		"latitude out of range":  func(_, p map[string]any) { p["fixes"] = fix(0, -800000001, 0, 8, 0, 0, 0) },
		"longitude out of range": func(_, p map[string]any) { p["fixes"] = fix(0, 0, -1300000001, 8, 0, 0, 0) },
		"base out of range":      func(_, p map[string]any) { p["base"] = map[string]any{"lat_e7": 900000001, "lon_e7": 0} },
		"accuracy too large":     func(_, p map[string]any) { p["fixes"] = fix(0, 0, 0, app.MaxAccuracy+1, 0, 0, 0) },
		"unknown accuracy":       func(_, p map[string]any) { p["fixes"] = fix(0, 0, 0, nil, 0, 0, 0) },
		"speed too large":        func(_, p map[string]any) { p["fixes"] = fix(0, 0, 0, 8, app.MaxSpeed+1, 0, 0) },
		"negative speed":         func(_, p map[string]any) { p["fixes"] = fix(0, 0, 0, 8, -1, 0, 0) },
		"bearing 360":            func(_, p map[string]any) { p["fixes"] = fix(0, 0, 0, 8, 0, 360, 0) },
		"unknown flag":           func(_, p map[string]any) { p["fixes"] = fix(0, 0, 0, 8, 0, 0, 4) },
		"short fix":              func(_, p map[string]any) { p["fixes"] = fix(0, 0, 0, 8, 0, 0) },
		"long fix":               func(_, p map[string]any) { p["fixes"] = fix(0, 0, 0, 8, 0, 0, 0, 0) },
		"textual fix value":      func(_, p map[string]any) { p["fixes"] = fix("0", 0, 0, 8, 0, 0, 0) },
		"zero sequence":          func(_, p map[string]any) { p["seq"] = 0 },
		"sequence overflow":      func(_, p map[string]any) { p["seq"] = int64(app.MaxSequence) + 1 },
		"other source":           func(_, p map[string]any) { p["source"] = "TELEMATICS" },
		"boot without elapsed":   func(_, p map[string]any) { p["base_elapsed_millis"] = nil },
		"elapsed without boot":   func(_, p map[string]any) { p["boot_id"] = nil },
		"malformed boot":         func(_, p map[string]any) { p["boot_id"] = "boot a1" },
		"negative elapsed":       func(_, p map[string]any) { p["base_elapsed_millis"] = -1 },
		"negative dropped":       func(_, p map[string]any) { p["dropped"].(map[string]any)["stale"] = -1 },
		"dropped overflow":       func(_, p map[string]any) { p["dropped"].(map[string]any)["jump"] = app.MaxDropped + 1 },
		"missing dropped key":    func(_, p map[string]any) { delete(p["dropped"].(map[string]any), "mock") },
		"unknown key":            func(_, p map[string]any) { p["precision"] = "FINE" },
		"non canonical trip":     func(_, p map[string]any) { p["trip_id"] = "trip" },
		"nanosecond base":        func(e, _ map[string]any) { e["occurred_at"] = "2026-11-15T12:02:00.000000001Z" },
		"other entity": func(e, _ map[string]any) {
			e["entity"] = map[string]any{"type": "trip", "id": id(11), "expected_revision": nil}
		},
		"attachment": func(e, _ map[string]any) {
			e["attachments"] = []any{map[string]any{"id": id(90), "sha256": strings.Repeat("a", 64), "bytes": 1, "mime": "image/jpeg"}}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := app.ParseBatch(operation(t, app.BatchOperationType, at(120), batchBody(fixes(3)), change)); !errors.Is(err, app.ErrMalformed) {
				t.Fatal("malformed batch admitted", err)
			}
		})
	}
	// A batch whose operation exceeds the protocol bound never reaches an owner.
	big := batchBody(fixes(app.MaxFixes))
	for i := range big["fixes"].([]any) {
		big["fixes"].([]any)[i] = []any{i * 10000, -100000, -100000, 10000, 100000, 359, 0}
	}
	raw, _ := json.Marshal(map[string]any{"schema_version": 1, "operation_id": id(20), "owner_account_id": id(1), "api_origin": origin, "org_id": id(65), "membership_id": id(3),
		"enrollment_id": id(4), "type": app.BatchOperationType, "operation_schema": 1, "entity": map[string]any{"type": app.EntityType, "id": id(11), "expected_revision": nil},
		"occurred_at": orgtime.FormatInstant(at(120)), "entry_zone": "UTC", "device_offset_seconds": 0, "depends_on": []any{}, "soft_references": []any{}, "attachments": []any{}, "payload": big})
	binding, _ := protocol.NewBinding(id(1), origin, id(65))
	if _, err := protocol.Seal(raw, binding); !errors.Is(err, protocol.ErrLimit) {
		t.Fatal("an operation above 8 KiB was sealed", len(raw), err)
	}
}

type tripRecord struct {
	start  time.Time
	finish *time.Time
}

func (r tripRecord) record() tripapp.Record {
	rec := tripapp.Record{ID: id(21), AssetID: id(40), DriverID: id(30), AuthorMembershipID: id(3), State: trip.InProgress}
	rec.Declared.OccurredAt = r.start
	if r.finish != nil {
		rec.State = trip.Completed
		rec.Finish = &tripapp.FinishRecord{AuthorMembershipID: id(3)}
		rec.Finish.Declared.OccurredAt = *r.finish
	}
	return rec
}

// EDR-278: the trip facts of a command are those at its declared instant.
func TestTripFactsFollowTheStoredStartAndFinish(t *testing.T) {
	s := scope(t)
	finish := at(600).Add(7 * time.Nanosecond)
	rec := tripRecord{start: at(0).Add(123 * time.Nanosecond), finish: &finish}.record()
	if _, err := app.TripFactsAt(s, rec, at(0)); !errors.Is(err, app.ErrMalformed) {
		t.Fatal("an instant before the trip started", err)
	}
	for _, c := range []struct {
		at   time.Time
		want trip.State
	}{{at(1), trip.InProgress}, {at(600), trip.InProgress}, {at(600).Add(7 * time.Nanosecond), trip.Completed}, {at(601), trip.Completed}} {
		facts, err := app.TripFactsAt(s, rec, c.at)
		if err != nil || facts.State != c.want || facts.Ref.ID() != id(21) || facts.Driver.ID() != id(3) || facts.Ref.OrganizationID() != s.OrgID() {
			t.Fatal("trip facts at", c.at, facts.State, err)
		}
	}
	open := tripRecord{start: at(0)}.record()
	if facts, err := app.TripFactsAt(s, open, at(86400)); err != nil || facts.State != trip.InProgress {
		t.Fatal("an unfinished trip", err)
	}
	b := app.Bounds(rec)
	if !b.Start.Equal(rec.Declared.OccurredAt) || b.Finish == nil || !b.Finish.Equal(finish) || app.Bounds(open).Finish != nil {
		t.Fatal("trip bounds")
	}
	odd := open
	odd.State = trip.Accepted
	if _, err := app.TripFactsAt(s, odd, at(1)); err == nil {
		t.Fatal("a trip state the intake does not know was used")
	}
}

type step struct {
	kind       string
	at         int
	origin     track.Origin
	permission track.Permission
	visible    bool
	cause      track.Cause
	author     int
}

func input(kind string, n int, st step) app.SessionInput {
	in := app.SessionInput{Type: kind, OperationID: id(100 + n), PayloadDigest: "sha256:" + strings.Repeat("b", 64), SessionID: id(11), TripID: id(21), AuthorMembershipID: id(3), Declared: at(st.at)}
	if st.author != 0 {
		in.AuthorMembershipID = id(st.author)
	}
	switch kind {
	case app.StartOperationType, app.ResumeOperationType:
		in.Origin, in.Permission, in.NotificationVisible = st.origin, st.permission, st.visible
		if in.Origin == "" {
			in.Origin = track.OriginTripScreen
		}
	default:
		in.Cause = st.cause
	}
	return in
}

// run applies steps through Decide and Change and returns the stored record.
func run(t *testing.T, policy app.PolicyVersion, finish *time.Time, steps ...step) (*app.SessionRecord, error) {
	t.Helper()
	s := scope(t)
	rec := tripRecord{start: at(0), finish: finish}.record()
	var stored *app.SessionRecord
	for n, st := range steps {
		in := input(st.kind, n, st)
		facts, err := app.TripFactsAt(s, rec, in.Declared)
		if err != nil {
			return stored, err
		}
		var existing *track.Session
		if stored != nil {
			rebuilt, err := app.Rebuild(s, *stored)
			if err != nil {
				t.Fatal("a stored session does not rebuild", err)
			}
			existing = &rebuilt
		}
		next, err := app.Decide(s, in, facts, existing, policy)
		if err != nil {
			return stored, err
		}
		change, err := app.Change(stored, next, in, policy)
		if err != nil {
			t.Fatal(err)
		}
		if (stored == nil) != change.Open || len(change.Transitions) == 0 {
			t.Fatal("change shape", change.Open, len(change.Transitions))
		}
		record := change.Session.Copy()
		if stored != nil {
			record = stored.Copy()
		}
		record.Transitions = append(record.Transitions, change.Transitions...)
		if len(record.Transitions) != len(next.History()) {
			t.Fatal("stored rows do not follow the history")
		}
		stored = &record
	}
	return stored, nil
}

var expected = app.PolicyVersion{Policy: track.PolicyExpected, Revision: 3}

// B-BR-EMP-429/430: decided by the EMP-GPS-001 domain, stored as rows that
// replay to the same session.
func TestSessionLifecycleStoresRowsThatReplay(t *testing.T) {
	finish := at(3600)
	stored, err := run(t, expected, &finish,
		step{kind: app.StartOperationType, at: 10, permission: track.PermissionNone, visible: true},
		step{kind: app.StartOperationType, at: 20, permission: track.PermissionPrecise, visible: true},
		step{kind: app.SuspendOperationType, at: 300, cause: track.CauseLongStationary},
		step{kind: app.ResumeOperationType, at: 400, permission: track.PermissionPrecise, visible: false},
		step{kind: app.ResumeOperationType, at: 410, permission: track.PermissionPrecise, visible: true},
		step{kind: app.ReduceOperationType, at: 500, cause: track.CausePrecisionReduced},
		step{kind: app.InterruptOperationType, at: 600, cause: track.CauseStoppedByDriver},
		step{kind: app.ResumeOperationType, at: 700, permission: track.PermissionApproximate, visible: true},
		step{kind: app.InterruptOperationType, at: 800, cause: track.CauseNotificationBlocked},
		step{kind: app.ResumeOperationType, at: 900, permission: track.PermissionPrecise, visible: true},
		step{kind: app.EndOperationType, at: 3600, cause: track.CauseTripCompleted},
	)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != id(11) || stored.TripID != id(21) || stored.OwnerMembershipID != id(3) || stored.OperationID != id(100) || stored.Policy != track.PolicyExpected ||
		stored.PolicyRevision != 3 || !stored.OpenedAt.Equal(at(10)) {
		t.Fatal("session row", stored.OperationID)
	}
	want := []struct {
		seq   int
		state track.State
		cause track.Cause
		actor track.Actor
		perm  track.Permission
		op    int
	}{
		{1, track.NotStarted, track.CauseNotStarted, track.ActorDriver, "", 0},
		{2, track.NotStarted, track.CausePermissionDenied, track.ActorDriver, track.PermissionNone, 0},
		{3, track.Recording, "", track.ActorDriver, track.PermissionPrecise, 1},
		{4, track.Suspended, track.CauseLongStationary, track.ActorDevice, "", 2},
		{5, track.Suspended, track.CauseNotificationBlocked, track.ActorDriver, track.PermissionPrecise, 3},
		{6, track.Recording, "", track.ActorDriver, track.PermissionPrecise, 4},
		{7, track.Recording, track.CausePrecisionReduced, track.ActorDevice, track.PermissionApproximate, 5},
		{8, track.Interrupted, track.CauseStoppedByDriver, track.ActorDriver, "", 6},
		{9, track.Recording, "", track.ActorDriver, track.PermissionApproximate, 7},
		{10, track.Interrupted, track.CauseNotificationBlocked, track.ActorDevice, "", 8},
		{11, track.Recording, "", track.ActorDriver, track.PermissionPrecise, 9},
		{12, track.Ended, track.CauseTripCompleted, track.ActorTrip, "", 10},
	}
	if len(stored.Transitions) != len(want) {
		t.Fatal("rows", len(stored.Transitions))
	}
	for i, w := range want {
		r := stored.Transitions[i]
		if r.Seq != w.seq || r.State != w.state || r.Cause != w.cause || r.Actor != w.actor || r.Permission != w.perm || r.OperationID != id(100+w.op) {
			t.Fatalf("row %d: %+v", i+1, []any{r.Seq, r.State, r.Cause, r.Actor, r.Permission})
		}
	}
	rebuilt, err := app.Rebuild(scope(t), *stored)
	if err != nil {
		t.Fatal(err)
	}
	periods := rebuilt.RecordingPeriods()
	if len(periods) != 5 || !periods[0].Start().Equal(at(20)) || periods[2].Precision() != track.PermissionApproximate || rebuilt.State() != track.Ended {
		t.Fatal("replayed periods", len(periods))
	}
	// Corrupted stored histories never rebuild.
	corrupt := map[string]func(r *app.SessionRecord){
		"opening cause":                func(r *app.SessionRecord) { r.Transitions[0].Cause = track.CausePermissionDenied },
		"actor":                        func(r *app.SessionRecord) { r.Transitions[3].Actor = track.ActorDriver },
		"instant backwards":            func(r *app.SessionRecord) { r.Transitions[4].At = at(1) },
		"recording without permission": func(r *app.SessionRecord) { r.Transitions[2].Permission = track.PermissionNone },
		"state":                        func(r *app.SessionRecord) { r.Transitions[3].State = track.Interrupted },
		"sequence":                     func(r *app.SessionRecord) { r.Transitions[6].Seq = 9 },
		"row after the end":            func(r *app.SessionRecord) { r.Transitions = append(r.Transitions, r.Transitions[11]) },
		"no opening":                   func(r *app.SessionRecord) { r.Transitions = r.Transitions[1:] },
		"empty":                        func(r *app.SessionRecord) { r.Transitions = nil },
		"opening instant":              func(r *app.SessionRecord) { r.OpenedAt = at(11) },
		"policy":                       func(r *app.SessionRecord) { r.Policy = track.PolicyOff },
		"session identity":             func(r *app.SessionRecord) { r.ID = "session" },
		"unknown cause":                func(r *app.SessionRecord) { r.Transitions[7].Cause = "SIGNAL_LOST" },
		"attempt permission":           func(r *app.SessionRecord) { r.Transitions[1].Permission = track.PermissionPrecise },
	}
	for name, change := range corrupt {
		t.Run(name, func(t *testing.T) {
			bad := stored.Copy()
			change(&bad)
			if _, err := app.Rebuild(scope(t), bad); !errors.Is(err, app.ErrUnavailable) {
				t.Fatal("a corrupted history rebuilt", err)
			}
		})
	}
}

// B-BR-EMP-428/429, EDR-278: refusals and their final kind.
func TestSessionRefusalsKeepTheirFinalKind(t *testing.T) {
	finish := at(1000)
	start := func(st step) step { st.kind = app.StartOperationType; return st }
	cases := map[string]struct {
		policy app.PolicyVersion
		steps  []step
		want   error
	}{
		"policy off":                  {app.PolicyVersion{Policy: track.PolicyOff}, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true})}, app.ErrPolicyOff},
		"no recorded policy":          {app.PolicyVersion{Policy: track.PolicyOff}, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true})}, app.ErrPolicyOff},
		"invalid server policy":       {app.PolicyVersion{}, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true})}, app.ErrUnavailable},
		"personal origin":             {expected, []step{start(step{at: 10, origin: track.OriginPersonal, permission: track.PermissionPrecise, visible: true})}, app.ErrMalformed},
		"implicit boot start":         {expected, []step{start(step{at: 10, origin: track.OriginBoot, permission: track.PermissionPrecise, visible: true})}, app.ErrMalformed},
		"another member":              {expected, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true, author: 5})}, track.ErrForbidden},
		"after the trip ended":        {expected, []step{start(step{at: 1000, permission: track.PermissionPrecise, visible: true})}, app.ErrMalformed},
		"twice":                       {expected, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true}), start(step{at: 20, permission: track.PermissionPrecise, visible: true})}, app.ErrMalformed},
		"resume while recording":      {expected, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true}), {kind: app.ResumeOperationType, at: 20, permission: track.PermissionPrecise, visible: true}}, app.ErrMalformed},
		"cause of another command":    {expected, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true}), {kind: app.SuspendOperationType, at: 20, cause: track.CauseProcessStopped}}, app.ErrMalformed},
		"out of order":                {expected, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true}), {kind: app.InterruptOperationType, at: 5, cause: track.CauseProcessStopped}}, app.ErrMalformed},
		"end before the trip ended":   {expected, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true}), {kind: app.EndOperationType, at: 20, cause: track.CauseTripCompleted}}, app.ErrMalformed},
		"stop by another member":      {expected, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true}), {kind: app.InterruptOperationType, at: 20, cause: track.CauseStoppedByDriver, author: 5}}, track.ErrForbidden},
		"command after end":           {expected, []step{start(step{at: 10, permission: track.PermissionPrecise, visible: true}), {kind: app.EndOperationType, at: 1000, cause: track.CauseTripCompleted}, {kind: app.InterruptOperationType, at: 1001, cause: track.CauseProcessStopped}}, app.ErrMalformed},
		"resume of a missing session": {expected, []step{{kind: app.ResumeOperationType, at: 10, permission: track.PermissionPrecise, visible: true}}, app.ErrUnavailable},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := run(t, c.policy, &finish, c.steps...)
			if err == nil || !errors.Is(app.Refusal(err), c.want) {
				t.Fatalf("refusal %v mapped to %v, want %v", err, app.Refusal(err), c.want)
			}
		})
	}
	// A second session for a trip that already has one, even with policy OFF
	// after the opening, is refused; the policy is read only at opening.
	s := scope(t)
	rec := tripRecord{start: at(0)}.record()
	stored, err := run(t, expected, nil, start(step{at: 10, permission: track.PermissionPrecise, visible: true}))
	if err != nil {
		t.Fatal(err)
	}
	existing, _ := app.Rebuild(s, *stored)
	second := input(app.StartOperationType, 9, step{at: 20, permission: track.PermissionPrecise, visible: true})
	second.SessionID = id(12)
	facts, _ := app.TripFactsAt(s, rec, second.Declared)
	if _, err := app.Decide(s, second, facts, &existing, expected); !errors.Is(app.Refusal(err), app.ErrMalformed) || !errors.Is(err, track.ErrSessionExists) {
		t.Fatal("a second session opened", err)
	}
	if _, err := app.Decide(s, second, facts, &existing, app.PolicyVersion{Policy: track.PolicyOff}); !errors.Is(app.Refusal(err), app.ErrPolicyOff) {
		t.Fatal("policy is checked before the existing session", err)
	}
	suspend := input(app.SuspendOperationType, 10, step{at: 30, cause: track.CauseLongStationary})
	facts, _ = app.TripFactsAt(s, rec, suspend.Declared)
	if _, err := app.Decide(s, suspend, facts, &existing, app.PolicyVersion{Policy: track.PolicyOff}); err != nil {
		t.Fatal("a later command read the policy", err)
	}
	if !errors.Is(app.Refusal(errors.New("database")), app.ErrUnavailable) {
		t.Fatal("an unknown failure became final")
	}
}

func recordingSession(t *testing.T, steps ...step) track.Session {
	t.Helper()
	stored, err := run(t, expected, nil, steps...)
	if err != nil {
		t.Fatal(err)
	}
	s, err := app.Rebuild(scope(t), *stored)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func batchOf(n int, first int, every int, flags map[int]int16) app.BatchInput {
	b := app.BatchInput{OperationID: id(200), PayloadDigest: "sha256:" + strings.Repeat("c", 64), SessionID: id(11), TripID: id(21), Seq: 7, Source: app.SourcePhone, Base: at(first)}
	for i := range n {
		speed, bearing := int32(1389), int16(270)
		f := app.Fix{At: at(first + i*every), Lat: int32(-100000000 + i), Lon: int32(-500000000 - i), Accuracy: 8, Speed: &speed, Bearing: &bearing, Flags: flags[i]}
		b.Fixes = append(b.Fixes, f)
	}
	return b
}

// B-BR-EMP-432/433: only fixes inside a recording period and the trip are
// kept, coarse in an approximate period; the rest is counted.
func TestAdmitKeepsOnlyFixesInsideTheSession(t *testing.T) {
	session := recordingSession(t,
		step{kind: app.StartOperationType, at: 100, permission: track.PermissionPrecise, visible: true},
		step{kind: app.SuspendOperationType, at: 200, cause: track.CauseLongStationary},
		step{kind: app.ResumeOperationType, at: 300, permission: track.PermissionPrecise, visible: true},
		step{kind: app.ReduceOperationType, at: 400, cause: track.CausePrecisionReduced},
	)
	finish := at(450)
	bounds := app.Bounds(tripRecord{start: at(0), finish: &finish}.record())
	// Fixes every 10 s from 90 s to 490 s: 41 fixes.
	b := batchOf(41, 90, 10, map[int]int16{3: app.FlagMock, 30: app.FlagCoarse})
	stored := []time.Time{at(110), at(320), at(5000)}
	segment, err := app.Admit(b, session, bounds, stored)
	if err != nil {
		t.Fatal(err)
	}
	// Inside: [100,200) -> 100..190 (10 fixes: 110 stored, 120 mock -> 8);
	// [300,400) precise -> 300..390 (10: 320 stored -> 9, 390's device coarse
	// flag does not make it coarse); [400,450) coarse -> 400..440 (5).
	// Outside: 90, 200..290 and 450..490 (after the finish) = 16.
	if len(segment.Fixes) != 22 || segment.Duplicates != 2 || segment.Mock != 1 || segment.Outside != 16 {
		t.Fatal("counts", len(segment.Fixes), segment.Duplicates, segment.Mock, segment.Outside)
	}
	if len(segment.Fixes)+segment.Duplicates+segment.Mock+segment.Outside != len(b.Fixes) {
		t.Fatal("every received fix is accounted for")
	}
	for _, f := range segment.Fixes {
		inside := !f.At.Before(at(100)) && f.At.Before(at(200)) || !f.At.Before(at(300)) && f.At.Before(at(450))
		if !inside || f.At.Equal(at(110)) || f.At.Equal(at(320)) || f.At.Equal(at(120)) || f.Coarse != !f.At.Before(at(400)) {
			t.Fatal("kept fix", f.At, f.Coarse)
		}
	}
	if segment.Batch.OperationID != b.OperationID || segment.Batch.Seq != 7 || len(segment.Batch.Fixes) != 41 {
		t.Fatal("segment batch")
	}
	// The trip's start also bounds an unfinished trip; a session that never
	// recorded keeps nothing.
	none := recordingSession(t, step{kind: app.StartOperationType, at: 100, permission: track.PermissionNone, visible: true})
	segment, err = app.Admit(batchOf(5, 100, 10, nil), none, app.Bounds(tripRecord{start: at(0)}.record()), nil)
	if err != nil || len(segment.Fixes) != 0 || segment.Outside != 5 {
		t.Fatal("a session that never recorded kept a fix", err)
	}
	late := recordingSession(t, step{kind: app.StartOperationType, at: 100, permission: track.PermissionPrecise, visible: true})
	segment, err = app.Admit(batchOf(3, 100, 100, nil), late, app.Bounds(tripRecord{start: at(150)}.record()), nil)
	if err != nil || len(segment.Fixes) != 2 || segment.Outside != 1 {
		t.Fatal("a fix before the trip start kept", err)
	}
	if _, err := app.Admit(app.BatchInput{}, late, bounds, nil); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an empty batch admitted")
	}
}

func TestTrackingValuesAreRedacted(t *testing.T) {
	speed := int32(1389)
	values := []any{
		app.SessionInput{SessionID: id(11), TripID: id(21)},
		app.BatchInput{SessionID: id(11), Fixes: []app.Fix{{Lat: -100000000, Lon: -500000000, Speed: &speed}}},
		app.Fix{Lat: -100000000, Lon: -500000000},
		app.StoredFix{Lat: -100000000, Lon: -500000000},
		app.Segment{Fixes: []app.StoredFix{{Lat: -100000000, Lon: -500000000}}},
		app.SessionRecord{ID: id(11)}, app.StoredTransition{OperationID: id(12)}, app.SegmentRecord{OperationID: id(13)}, app.SessionChange{},
	}
	for _, v := range values {
		raw, _ := json.Marshal(v)
		for _, s := range []string{fmt.Sprintf("%v %+v %#v %s", v, v, v, v), string(raw), loggedValue(v)} {
			for _, secret := range []string{"100000000", "500000000", id(11), id(12), id(13), id(21), "1389"} {
				if strings.Contains(s, secret) {
					t.Fatalf("%T leaked %q", v, secret)
				}
			}
		}
	}
}

func lifecycleRecord(state trip.State, rows ...tripapp.TransitionRecord) tripapp.Record {
	rec := tripRecord{start: at(0)}.record()
	rec.State, rec.Transitions = state, rows
	return rec
}

func lifecycleRow(command trip.Command, from, to trip.State, seconds int) tripapp.TransitionRecord {
	row := tripapp.TransitionRecord{Command: command, From: from, To: to}
	row.Declared.OccurredAt = at(seconds)
	return row
}

// EMP-TRIP-011 with GPS-001 FOLLOW: the trip's stored pauses, resumption and
// cancellation decide its facts at an instant, its running intervals and the
// session it is followed by.
func TestTrackingFollowsTheTripPausesAndCancellation(t *testing.T) {
	s := scope(t)
	pause := lifecycleRow(trip.CommandPause, trip.InProgress, trip.Paused, 100)
	resume := lifecycleRow(trip.CommandResume, trip.Paused, trip.InProgress, 200)
	cancel := lifecycleRow(trip.CommandCancel, trip.InProgress, trip.Cancelled, 300)
	rec := lifecycleRecord(trip.Cancelled, pause, resume, cancel)
	for _, c := range []struct {
		at   int
		want trip.State
	}{{50, trip.InProgress}, {100, trip.Paused}, {150, trip.Paused}, {200, trip.InProgress}, {299, trip.InProgress}, {300, trip.Cancelled}, {400, trip.Cancelled}} {
		if facts, err := app.TripFactsAt(s, rec, at(c.at)); err != nil || facts.State != c.want {
			t.Fatal("trip state at", c.at, facts.State, err)
		}
	}
	b := app.Bounds(rec)
	for second, want := range map[int]bool{0: true, 99: true, 100: false, 199: false, 200: true, 299: true, 300: false, 301: false} {
		if b.Running(at(second)) != want {
			t.Fatal("running at", second)
		}
	}
	if b.Running(at(-1)) {
		t.Fatal("a trip runs before its start")
	}
	paused := app.Bounds(lifecycleRecord(trip.Paused, pause))
	if paused.Running(at(100000)) || !paused.Running(at(99)) || len(paused.Pauses) != 1 || paused.Pauses[0].To != nil {
		t.Fatal("an open pause runs")
	}
	for name, broken := range map[string]tripapp.Record{
		"state drift":   lifecycleRecord(trip.InProgress, pause),
		"from drift":    lifecycleRecord(trip.Paused, lifecycleRow(trip.CommandPause, trip.Paused, trip.Paused, 100)),
		"instant order": lifecycleRecord(trip.InProgress, pause, lifecycleRow(trip.CommandResume, trip.Paused, trip.InProgress, 50)),
		"finish after it": func() tripapp.Record {
			r := lifecycleRecord(trip.Cancelled, cancel)
			r.Finish = &tripapp.FinishRecord{}
			return r
		}(),
	} {
		if _, err := app.TripFactsAt(s, broken, at(10)); !errors.Is(err, app.ErrUnavailable) {
			t.Fatal("an inconsistent trip was used", name, err)
		}
		if app.Bounds(broken).Running(at(10)) {
			t.Fatal("an inconsistent trip runs", name)
		}
	}
	session := recordingSession(t, step{kind: app.StartOperationType, at: 10, permission: track.PermissionPrecise, visible: true})
	same, err := app.FollowTrip(s, rec, session, at(100))
	if err != nil || same.State() != track.Recording || len(same.History()) != 2 {
		t.Fatal("an event at the operation's own instant was followed", err)
	}
	suspended, err := app.FollowTrip(s, rec, session, at(250))
	h := suspended.History()
	if err != nil || suspended.State() != track.Suspended || len(h) != 3 || h[2].Cause() != track.CauseTripPaused || h[2].Actor() != track.ActorTrip || !h[2].At().Equal(at(100)) {
		t.Fatal("a pause did not suspend the recording session", err)
	}
	ended, err := app.FollowTrip(s, rec, session, at(400))
	if err != nil || ended.State() != track.Ended || ended.Cause() != track.CauseTripCancelled || len(ended.History()) != 4 {
		t.Fatal("a cancellation did not end the session", err)
	}
	// Events before the session's last transition are its own history.
	late, _ := app.FollowTrip(s, rec, recordingSession(t, step{kind: app.StartOperationType, at: 250, permission: track.PermissionPrecise, visible: true}), at(260))
	if late.State() != track.Recording {
		t.Fatal("a pause before the session's last transition was followed")
	}
	if _, err := app.FollowTrip(s, lifecycleRecord(trip.InProgress, pause), session, at(400)); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("an inconsistent trip was followed", err)
	}
	// Fixes while paused, after the resumption without the driver's resume
	// and after the cancellation are outside the session: only 50 s is kept.
	segment, err := app.Admit(batchOf(8, 50, 50, nil), ended, b, nil)
	if err != nil || len(segment.Fixes) != 1 || segment.Outside != 7 || !segment.Fixes[0].At.Equal(at(50)) {
		t.Fatal("fixes of a paused or cancelled trip kept", len(segment.Fixes), segment.Outside, err)
	}
}
