package meter_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/fleet/domain/meter"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

func TestReadingValidationAndCopiedEvidence(t *testing.T) {
	s := scope(t, 1)
	seg := segment(t, s, meter.Odometer)
	base := reading(t, s, seg, 20, 30, 10, "2026-11-02T00:00:00Z", meter.Valid)
	foreign := scope(t, 2)
	for name, alter := range map[string]func(*meter.ReadingInput){
		"absent-declaration": func(i *meter.ReadingInput) { i.Declared = nil },
		"invalid-zone":       func(i *meter.ReadingInput) { i.Declared.EntryZone = "Invalid/Zone" },
		"invalid-offset":     func(i *meter.ReadingInput) { i.Declared.DeviceOffsetSeconds = 64801 },
		"unset-receipt":      func(i *meter.ReadingInput) { i.ReceivedAt = time.Time{} },
		"invalid-year":       func(i *meter.ReadingInput) { i.Declared.OccurredAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		"missing-author":     func(i *meter.ReadingInput) { i.Author = meter.MembershipRef{} },
		"missing-device":     func(i *meter.ReadingInput) { i.Device = meter.DeviceRef{} },
		"missing-operation":  func(i *meter.ReadingInput) { i.Operation = meter.OperationRef{} },
		"foreign-author":     func(i *meter.ReadingInput) { i.Author, _ = meter.NewMembershipRef(foreign, uuid(13)) },
		"foreign-device":     func(i *meter.ReadingInput) { i.Device, _ = meter.NewDeviceRef(foreign, uuid(14)) },
		"foreign-operation":  func(i *meter.ReadingInput) { i.Operation, _ = meter.NewOperationRef(foreign, uuid(30)) },
		"foreign-photo":      func(i *meter.ReadingInput) { p, _ := meter.NewPhotoRef(foreign, uuid(15)); i.Photo = &p },
		"unknown-origin":     func(i *meter.ReadingInput) { i.Origin = "UNKNOWN" },
		"unknown-method":     func(i *meter.ReadingInput) { i.Method = "UNKNOWN" },
		"initial-voided":     func(i *meter.ReadingInput) { i.State = meter.Voided },
		"wrong-value-kind":   func(i *meter.ReadingInput) { i.Value, _ = meter.NewValue(meter.HourMeter, 10, nil) },
		"zero-value-type":    func(i *meter.ReadingInput) { i.Value = meter.Value{} },
	} {
		t.Run(name, func(t *testing.T) {
			in := base.Snapshot()
			alter(&in)
			if _, err := meter.NewReading(s, in); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
	input := base.Snapshot()
	photo, _ := meter.NewPhotoRef(s, uuid(15))
	input.Photo = &photo
	input.Declared.DeviceOffsetSeconds = 64800
	copy, err := meter.NewReading(s, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Declared.EntryZone = "Invalid/Zone"
	*input.Photo = meter.PhotoRef{}
	snapshot := copy.Snapshot()
	if snapshot.Declared.EntryZone != "America/Cuiaba" || snapshot.Photo.ID() != uuid(15) || snapshot.Declared.DeviceOffsetSeconds != 64800 {
		t.Fatal("input alias or device metadata rewrite")
	}
	snapshot.Declared.EntryZone = "Invalid/Zone"
	*snapshot.Photo = meter.PhotoRef{}
	if copy.Snapshot().Declared.EntryZone != "America/Cuiaba" || copy.Snapshot().Photo.ID() != uuid(15) {
		t.Fatal("snapshot pointer escaped")
	}
	explicitMinimum := base.Snapshot()
	explicitMinimum.Declared.OccurredAt = time.Time{}
	if _, err = meter.NewReading(s, explicitMinimum); err != nil {
		t.Fatal("explicit minimum declaration must follow time kernel", err)
	}
}

func TestSeriesForksFactsAndAnchorsStayImmutable(t *testing.T) {
	s := scope(t, 1)
	seg := segment(t, s, meter.Odometer)
	open := reading(t, s, seg, 20, 30, 0, "2026-11-01T00:00:00Z", meter.Valid)
	base, err := meter.NewSeries(s, seg, open)
	if err != nil {
		t.Fatal(err)
	}
	r1 := reading(t, s, seg, 21, 31, 10, "2026-11-02T00:00:00Z", meter.Valid)
	base, err = base.Append(s, r1)
	if err != nil {
		t.Fatal(err)
	}
	// Construct a branch point with an unused backing slot. An unsafe append
	// could replace one branch's last evidence with the other branch's value.
	base, err = base.Append(s, reading(t, s, seg, 26, 36, 5, "2026-11-01T12:00:00Z", meter.Valid))
	if err != nil {
		t.Fatal(err)
	}
	backing := reflect.ValueOf(base).FieldByName("readings")
	if backing.Cap() <= backing.Len() {
		t.Fatal("fixture did not exercise a spare-capacity branch")
	}
	forkA, err := base.Append(s, reading(t, s, seg, 22, 32, 20, "2026-11-03T00:00:00Z", meter.Valid))
	if err != nil {
		t.Fatal(err)
	}
	forkB, err := base.Append(s, reading(t, s, seg, 23, 33, 30, "2026-11-04T00:00:00Z", meter.Valid))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		s      meter.Series
		count  int
		latest string
	}{{base, 3, uuid(21)}, {forkA, 4, uuid(22)}, {forkB, 4, uuid(23)}} {
		rows, e := tc.s.Ordered(s)
		if e != nil || len(rows) != tc.count {
			t.Fatal("branch changed")
		}
		rows[0] = meter.Reading{}
		last, e := tc.s.Latest(s, instant(t, "2026-11-10T00:00:00Z"))
		if e != nil || last.Snapshot().Reference.ID() != tc.latest {
			t.Fatal("branch read escaped")
		}
	}
	replacement := reading(t, s, seg, 24, 34, 11, "2026-11-02T00:00:00Z", meter.Valid)
	corrected, err := base.Supersede(s, r1.Snapshot().Reference, replacement, fact(t, s, 40, 34))
	if err != nil {
		t.Fatal(err)
	}
	voided, err := base.Void(s, r1.Snapshot().Reference, fact(t, s, 41, 35))
	if err != nil {
		t.Fatal(err)
	}
	facts, err := corrected.Facts(s)
	if err != nil {
		t.Fatal(err)
	}
	*facts[0].Replacement() = meter.ReadingRef{}
	facts[0] = meter.Fact{}
	if state, _ := base.State(s, r1.Snapshot().Reference); state != meter.Valid {
		t.Fatal("base terminal state mutated")
	}
	if state, _ := voided.State(s, r1.Snapshot().Reference); state != meter.Voided {
		t.Fatal("void fork lost")
	}
	facts, _ = corrected.Facts(s)
	if facts[0].Replacement().ID() != uuid(24) {
		t.Fatal("replacement getter leaked")
	}
	usage, err := corrected.Usage(s, open.Snapshot().Declared.OccurredAt, replacement.Snapshot().Declared.OccurredAt)
	if err != nil {
		t.Fatal(err)
	}
	*usage.From() = meter.Reading{}
	*usage.To() = meter.Reading{}
	if usage.From().Snapshot().Reference.ID() != uuid(20) || usage.To().Snapshot().Reference.ID() != uuid(24) {
		t.Fatal("anchor getter leaked")
	}
	modulus := int64(100)
	m, err := meter.NewMeterRef(s, seg.Meter().Asset(), uuid(50), meter.Odometer, &modulus)
	if err != nil {
		t.Fatal(err)
	}
	modulus = 0
	*m.Modulus() = 0
	if *m.Modulus() != 100 {
		t.Fatal("modulus alias escaped")
	}
}

func TestSeriesOperationsAreTerminalAndScoped(t *testing.T) {
	s := scope(t, 1)
	seg := segment(t, s, meter.Odometer)
	open := reading(t, s, seg, 20, 30, 10, "2026-11-01T00:00:00Z", meter.Valid)
	base, err := meter.NewSeries(s, seg, open)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = base.Append(s, reading(t, s, seg, 21, 30, 10, "2026-11-02T00:00:00Z", meter.Valid)); !errors.Is(err, meter.ErrOperationReused) {
		t.Fatal("operation replay accepted", err)
	}
	if _, err = base.Append(s, open); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("duplicate evidence accepted", err)
	}
	for name, alter := range map[string]func(*meter.FactInput){"missing-author": func(f *meter.FactInput) { f.Author = meter.MembershipRef{} }, "missing-device": func(f *meter.FactInput) { f.Device = meter.DeviceRef{} }, "missing-receipt": func(f *meter.FactInput) { f.ReceivedAt = time.Time{} }, "control": func(f *meter.FactInput) { f.Reason = "a\u0085b" }, "blank": func(f *meter.FactInput) { f.Reason = "\u00a0" }, "too-long": func(f *meter.FactInput) { f.Reason = strings.Repeat("😀", 281) }, "bad-utf8": func(f *meter.FactInput) { f.Reason = string([]byte{0xff}) }, "reused-op": func(f *meter.FactInput) { f.Operation = open.Snapshot().Operation }} {
		t.Run(name, func(t *testing.T) {
			f := fact(t, s, 40, 31)
			alter(&f)
			if _, e := base.Void(s, open.Snapshot().Reference, f); e == nil {
				t.Fatal("invalid fact accepted")
			}
		})
	}
	voided, err := base.Void(s, open.Snapshot().Reference, fact(t, s, 40, 31))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = voided.Void(s, open.Snapshot().Reference, fact(t, s, 41, 32)); !errors.Is(err, meter.ErrAlreadySuperseded) {
		t.Fatal("terminal void", err)
	}
	replacement := reading(t, s, seg, 21, 32, 11, "2026-11-01T00:00:00Z", meter.Valid)
	if _, err = base.Supersede(s, open.Snapshot().Reference, replacement, fact(t, s, 41, 33)); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("two different correction commands accepted", err)
	}
	foreignScope := scope(t, 2)
	foreign := reading(t, foreignScope, segment(t, foreignScope, meter.Odometer), 21, 31, 10, "2026-11-02T00:00:00Z", meter.Valid)
	if _, err = base.Append(s, foreign); !errors.Is(err, meter.ErrScope) {
		t.Fatal("foreign read appended", err)
	}
	if _, err = base.Ordered(foreignScope); !errors.Is(err, meter.ErrScope) {
		t.Fatal("foreign history exposed", err)
	}
	hour := reading(t, s, segment(t, s, meter.HourMeter), 21, 31, 10, "2026-11-02T00:00:00Z", meter.Valid)
	if _, err = base.Append(s, hour); !errors.Is(err, meter.ErrScope) {
		t.Fatal("mixed kind appended", err)
	}
	if _, err = base.Latest(kernel.OrgScope{}, instant(t, "2026-11-14T00:00:00Z")); !errors.Is(err, meter.ErrScope) {
		t.Fatal("unscoped projection exposed", err)
	}
	otherMeter := segment(t, s, meter.HourMeter).Meter()
	if err = meter.ValidateOpenSet(s, []meter.MeterRef{seg.Meter(), otherMeter}); !errors.Is(err, meter.ErrInvalid) {
		t.Fatal("same meter identity cannot describe two kinds", err)
	}
}

func TestLatestPermutationNeverReturnsTerminalOrSuspect(t *testing.T) {
	s := scope(t, 1)
	seg := segment(t, s, meter.Odometer)
	open := reading(t, s, seg, 20, 30, 0, "2026-11-01T00:00:00Z", meter.Valid)
	r1 := reading(t, s, seg, 21, 31, 10, "2026-11-02T00:00:00Z", meter.Valid)
	r2 := reading(t, s, seg, 22, 32, 20, "2026-11-02T00:00:00Z", meter.Valid)
	r3 := reading(t, s, seg, 23, 33, 30, "2026-11-02T00:00:00Z", meter.Valid)
	in := r1.Snapshot()
	in.ReceivedAt = in.ReceivedAt.Add(time.Second)
	r1, _ = meter.NewReading(s, in)
	r4 := reading(t, s, seg, 24, 34, 999, "2026-11-04T00:00:00Z", meter.Suspect)
	rows := []meter.Reading{r1, r2, r3, r4}
	permutations := 0
	var visit func(int)
	visit = func(index int) {
		if index == len(rows) {
			permutations++
			series, e := meter.NewSeries(s, seg, open)
			if e != nil {
				t.Fatal(e)
			}
			for _, r := range rows {
				series, e = series.Append(s, r)
				if e != nil {
					t.Fatal(e)
				}
			}
			latest, e := series.Latest(s, instant(t, "2026-11-10T00:00:00Z"))
			if e != nil || latest.Snapshot().Reference.ID() != uuid(21) {
				t.Fatal("receipt tie order unstable")
			}
			series, e = series.Void(s, r1.Snapshot().Reference, fact(t, s, 40, 35))
			if e != nil {
				t.Fatal(e)
			}
			replacement := reading(t, s, seg, 25, 36, 31, "2026-11-03T00:00:00Z", meter.Valid)
			series, e = series.Supersede(s, r3.Snapshot().Reference, replacement, fact(t, s, 41, 36))
			if e != nil {
				t.Fatal(e)
			}
			latest, e = series.Latest(s, instant(t, "2026-11-10T00:00:00Z"))
			if e != nil || latest.Snapshot().Reference.ID() != uuid(25) {
				t.Fatal("terminal/suspect selected")
			}
			ordered, _ := series.Ordered(s)
			for _, r := range ordered {
				if r.Snapshot().Reference.ID() == uuid(23) && r.Snapshot().Value.Count() != 30 {
					t.Fatal("original observation overwritten")
				}
			}
			return
		}
		for i := index; i < len(rows); i++ {
			rows[index], rows[i] = rows[i], rows[index]
			visit(index + 1)
			rows[index], rows[i] = rows[i], rows[index]
		}
	}
	visit(0)
	if permutations != 24 {
		t.Fatal("incomplete property permutations")
	}
}

func TestConcurrentPureForksShareNoMutableEvidence(t *testing.T) {
	s := scope(t, 1)
	seg := segment(t, s, meter.Odometer)
	opening := reading(t, s, seg, 20, 30, 0, "2026-11-01T00:00:00Z", meter.Valid)
	base, e := meter.NewSeries(s, seg, opening)
	if e != nil {
		t.Fatal(e)
	}
	var group sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		r := reading(t, s, seg, 40+i, 60+i, int64(i), "2026-11-02T00:00:00Z", meter.Valid)
		group.Go(func() {
			branch, err := base.Append(s, r)
			if err != nil {
				errs <- err
				return
			}
			result, err := branch.Latest(s, r.Snapshot().Declared.OccurredAt)
			if err != nil || result.Snapshot().Reference.ID() != r.Snapshot().Reference.ID() {
				errs <- errors.New("fork lost")
			}
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	rows, _ := base.Ordered(s)
	if len(rows) != 1 {
		t.Fatal("base changed across threads")
	}
}

func TestPrivateValuesRedactEveryDiagnosticSurface(t *testing.T) {
	s := scope(t, 1)
	seg := segment(t, s, meter.Odometer)
	r := reading(t, s, seg, 20, 30, 123, "2026-11-02T00:00:00Z", meter.Valid)
	series, _ := meter.NewSeries(s, seg, r)
	f := fact(t, s, 40, 31)
	series, _ = series.Void(s, r.Snapshot().Reference, f)
	facts, _ := series.Facts(s)
	missing, _ := meter.NewMissing("private synthetic note")
	usage, _ := series.Usage(s, instant(t, "2026-11-01T00:00:00Z"), instant(t, "2026-11-10T00:00:00Z"))
	values := []any{r, r.Snapshot(), r.Snapshot().Author, r.Snapshot().Device, r.Snapshot().Operation, r.Snapshot().Value, seg.Meter(), seg, r.Snapshot().Reference, facts[0], f, series, missing, usage}
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var log strings.Builder
		slog.New(slog.NewJSONHandler(&log, nil)).Info("value", "value", value)
		for _, out := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value), string(raw), log.String()} {
			if !strings.Contains(out, "redacted") || strings.Contains(out, uuid(20)) || strings.Contains(out, "synthetic") || strings.Contains(out, "Cuiaba") {
				t.Fatalf("private diagnostic leaked for %T", value)
			}
		}
	}
}
