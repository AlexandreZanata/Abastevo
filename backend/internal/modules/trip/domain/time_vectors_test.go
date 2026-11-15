package domain_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	trip "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/trip/domain"
)

const (
	timeCorpusPath = "../../../../../contracts/testdata/enterprise/trip/trip-time-v1.psv"
	timeCorpusHash = "c5054633152e33112d7345b5302bd264ee6a5235f399617397f3971ae45c6a22"
)

var (
	wantTimeCounts = map[string]int{"CLOCK": 12, "CROSSING": 17, "ELAPSED": 37}
	timeErrorCodes = []string{"INVALID", "FINISH_BEFORE_START"}
	timeArgs       = map[string]string{"CLOCK": "boot_hex elapsed",
		"CROSSING": "at entry offset zone unit_zone fin_at fin_entry fin_offset finished",
		"ELAPSED":  "at entry offset zone fin_at fin_entry fin_offset boot_hex mono fin_boot_hex fin_mono tolerance finished"}
)

// vectorClock decodes a fixture run clock; malformed fixture syntax fails the loader.
func vectorClock(t *testing.T, bootHex, raw string) (trip.RunClock, error) {
	t.Helper()
	boot, e := decodeHex(bootHex)
	if e != nil {
		t.Fatal(e)
	}
	n, e := strconv.ParseInt(raw, 10, 64)
	if e != nil {
		t.Fatal("malformed fixture monotonic count", raw)
	}
	return trip.NewRunClock(boot, n)
}

// vectorTimedTrip starts, and unless finished=false also finishes, one trip.
func vectorTimedTrip(t *testing.T, f fields) (trip.Trip, error) {
	t.Helper()
	switch f.get("finished", "true") {
	case "true":
		return vectorFinish(t, f)
	case "false":
		return vectorStart(t, startFields(f))
	}
	t.Fatal("malformed finished fixture flag")
	return trip.Trip{}, nil
}

func clockField(c *trip.RunClock) string {
	if c == nil {
		return "?"
	}
	return fmt.Sprintf("%x:%d", c.BootID(), c.ElapsedMillis())
}

func crossingFields(t *testing.T, tr trip.Trip) fields {
	t.Helper()
	start, rep, c := tr.Started().Declared(), tr.Reporting(), tr.Crossing()
	if c.Day() != rep.Day {
		t.Fatal("the crossing view must report the start reporting day")
	}
	out := fields{"id": alias(tr.Reference().ID()), "state": string(tr.State()), "day": rep.Day, "zone": rep.Zone,
		"reporting_source": rep.Source, "start_at": orgtime.FormatInstant(start.OccurredAt), "start_entry": start.EntryZone,
		"start_offset": strconv.FormatInt(start.DeviceOffsetSeconds, 10), "finish_at": "?", "finish_entry": "?", "finish_offset": "?",
		"end_day": "?", "crosses_midnight": "?", "entry_zone_changed": "?", "offset_changed": "?"}
	if end, ok := tr.Finished(); ok {
		d := end.Declared()
		out["finish_at"], out["finish_entry"] = orgtime.FormatInstant(d.OccurredAt), d.EntryZone
		out["finish_offset"] = strconv.FormatInt(d.DeviceOffsetSeconds, 10)
	}
	if c.Finished() {
		out["end_day"], out["crosses_midnight"] = c.EndDay(), strconv.FormatBool(c.CrossesMidnight())
		out["entry_zone_changed"], out["offset_changed"] = strconv.FormatBool(c.EntryZoneChanged()), strconv.FormatBool(c.OffsetChanged())
	}
	return out
}

func elapsedFields(t *testing.T, tr trip.Trip, f fields) (fields, error) {
	t.Helper()
	tolerance, e := strconv.ParseInt(f.get("tolerance", "2000"), 10, 64)
	if e != nil {
		t.Fatal("malformed fixture tolerance")
	}
	el, e := tr.Elapsed(tolerance)
	if e != nil {
		return nil, e
	}
	out := fields{"state": string(tr.State()), "status": string(el.Status()), "seconds": "?", "nanos": "?", "gap": "?",
		"start_clock": clockField(tr.Started().Clock()), "finish_clock": "?"}
	if seconds, nanos, ok := el.Duration(); ok {
		out["seconds"], out["nanos"] = strconv.FormatInt(seconds, 10), strconv.FormatInt(int64(nanos), 10)
	}
	if el.Gap() != "" {
		out["gap"] = string(el.Gap())
	}
	if end, ok := tr.Finished(); ok {
		out["finish_clock"] = clockField(end.Clock())
	}
	return out, nil
}

func runTimeVector(t *testing.T, op string, f fields) (fields, error) {
	t.Helper()
	switch op {
	case "CLOCK":
		c, e := vectorClock(t, f["boot_hex"], f["elapsed"])
		if e != nil {
			return nil, e
		}
		return fields{"boot_hex": fmt.Sprintf("%x", c.BootID()), "elapsed": strconv.FormatInt(c.ElapsedMillis(), 10)}, nil
	case "CROSSING":
		tr, e := vectorTimedTrip(t, f)
		if e != nil {
			return nil, e
		}
		return crossingFields(t, tr), nil
	case "ELAPSED":
		tr, e := vectorTimedTrip(t, f)
		if e != nil {
			return nil, e
		}
		return elapsedFields(t, tr, f)
	}
	t.Fatal("unknown operation")
	return nil, nil
}

func TestSharedTripTimeVectors(t *testing.T) {
	raw, e := os.ReadFile(timeCorpusPath)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != timeCorpusHash {
		t.Fatal("shared time corpus hash changed")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("invalid shared corpus header")
	}
	counts, ids, codes := map[string]int{}, map[string]bool{}, map[string]bool{}
	for scanner.Scan() {
		p := strings.Split(scanner.Text(), "|")
		if len(p) != 4 || strings.TrimSpace(p[0]) == "" || ids[p[0]] {
			t.Fatal("malformed/duplicate shared case")
		}
		if _, ok := wantTimeCounts[p[1]]; !ok {
			t.Fatal("unknown shared operation")
		}
		ids[p[0]] = true
		counts[p[1]]++
		args, er := fieldMap(p[2])
		if er != nil {
			t.Fatal(p[0], er)
		}
		want, er := fieldMap(p[3])
		if er != nil {
			t.Fatal(p[0], er)
		}
		for k := range args {
			if !strings.Contains(" "+timeArgs[p[1]]+" ", " "+k+" ") {
				t.Fatal("unknown fixture argument", p[0], k)
			}
		}
		for _, prefix := range []string{"", "fin_"} {
			_, boot := args[prefix+"boot_hex"]
			_, mono := args[prefix+"mono"]
			if p[1] == "ELAPSED" && boot != mono {
				t.Fatal("a fixture run clock needs both boot and count", p[0])
			}
		}
		if code, ok := want["error"]; ok {
			if len(want) != 1 {
				t.Fatal("error result carries output", p[0])
			}
			codes[code] = true
		}
		t.Run(p[0], func(t *testing.T) {
			got, err := runTimeVector(t, p[1], args)
			if err != nil {
				got = errorFields(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("shared mismatch: got %v expected %v", got, want)
			}
		})
	}
	if e = scanner.Err(); e != nil {
		t.Fatal(e)
	}
	if len(ids) != 66 || !reflect.DeepEqual(counts, wantTimeCounts) {
		t.Fatal("incomplete time corpus", len(ids), counts)
	}
	for _, code := range timeErrorCodes {
		if !codes[code] {
			t.Fatal("error code without a shared vector", code)
		}
	}
	if len(codes) != len(timeErrorCodes) {
		t.Fatal("undocumented error code in time corpus", codes)
	}
}
