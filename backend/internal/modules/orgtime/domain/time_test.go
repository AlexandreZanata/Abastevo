package domain_test

import (
	"bufio"
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
)

func fields(t *testing.T, raw string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, item := range strings.Split(raw, ";") {
		key, val, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			t.Fatal("invalid vector field")
		}
		if _, found := out[key]; found {
			t.Fatal("duplicate vector field")
		}
		out[key] = val
	}
	return out
}
func number(t *testing.T, raw string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func execute(t *testing.T, op string, a map[string]string) (map[string]string, error) {
	t.Helper()
	switch op {
	case "INSTANT":
		v, err := orgtime.ParseInstant(a["text"])
		return map[string]string{"value": orgtime.FormatInstant(v)}, err
	case "RECORD":
		occurred, err := orgtime.ParseInstant(a["occurred"])
		if err != nil {
			return nil, err
		}
		received, err := orgtime.ParseInstant(a["received"])
		if err != nil {
			return nil, err
		}
		var unit *string
		if a["unit"] != "?" {
			s := a["unit"]
			unit = &s
		}
		v, err := orgtime.Capture(orgtime.Declaration{
			OccurredAt: occurred, EntryZone: a["entry"], DeviceOffsetSeconds: number(t, a["offset"]),
		}, received, orgtime.ReportingZones{Organization: a["org"], Unit: unit})
		return map[string]string{
			"occurred": orgtime.FormatInstant(v.OccurredAt), "received": orgtime.FormatInstant(v.ReceivedAt),
			"authority": orgtime.FormatInstant(v.AuthorityAt()), "entry": v.EntryZone,
			"offset": strconv.FormatInt(v.DeviceOffsetSeconds, 10), "day": v.ReportingDay,
			"zone": v.ReportingZone, "source": v.ReportingSource,
		}, err
	case "PERIOD":
		at, err := orgtime.ParseInstant(a["at"])
		if err != nil {
			return nil, err
		}
		start, err := orgtime.ParseInstant(a["start"])
		if err != nil {
			return nil, err
		}
		end, err := orgtime.ParseInstant(a["end"])
		if err != nil {
			return nil, err
		}
		v, err := orgtime.InPeriod(at, start, end)
		return map[string]string{"inside": strconv.FormatBool(v)}, err
	case "SKEW":
		device, err := orgtime.ParseInstant(a["device"])
		if err != nil {
			return nil, err
		}
		server, err := orgtime.ParseInstant(a["server"])
		if err != nil {
			return nil, err
		}
		var limit *int64
		if a["limit"] != "?" {
			n := number(t, a["limit"])
			limit = &n
		}
		v, err := orgtime.AssessClockSkew(device, server, limit)
		return map[string]string{
			"seconds": strconv.FormatInt(v.Seconds, 10), "nanos": strconv.FormatInt(int64(v.Nanos), 10),
			"flag": v.Flag, "limit": strconv.FormatInt(v.LimitSeconds, 10),
		}, err
	default:
		t.Fatal("unknown vector operation", op)
	}
	return nil, nil
}
func TestSharedEnterpriseTimeVectors(t *testing.T) {
	f, err := os.Open("../../../../../contracts/testdata/enterprise/time-v1.psv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() || scanner.Text() != "id|operation|args|expected" {
		t.Fatal("invalid header")
	}
	seen := map[string]bool{}
	for scanner.Scan() {
		row := strings.Split(scanner.Text(), "|")
		if len(row) != 4 || row[0] == "" || seen[row[0]] {
			t.Fatal("invalid/duplicate vector")
		}
		seen[row[0]] = true
		t.Run(row[0], func(t *testing.T) {
			got, err := execute(t, row[1], fields(t, row[2]))
			if err != nil {
				if !errors.Is(err, orgtime.ErrInvalid) {
					t.Fatal(err)
				}
				got = map[string]string{"error": "invalid-time"}
			}
			if want := fields(t, row[3]); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v; want %v", got, want)
			}
		})
	}
	if scanner.Err() != nil || len(seen) != 93 {
		t.Fatalf("rows=%d error=%v", len(seen), scanner.Err())
	}
}
