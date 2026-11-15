//go:build integration

package adapters_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	orgtime "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgtime/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/orgtest"
)

const (
	lateSQLCorpus = "../../../../../contracts/testdata/enterprise/sync/late-intake-v1.psv"
	lateSQLHash   = "feb23b2d4c147569ac49977e060b5fdb110c3817cf54c457bb7652e244214dc8"
)

// The database's org_sync_late_disposition is the second consumer of the
// shared late-intake oracle (EDR-197/198): every DECIDE row gives the same
// disposition as Go DecideLate, or NULL for invalid evidence, under UTC and
// under a non-UTC session zone with daylight saving (EDR-119 elapsed seconds).
func TestLateIntakeSQLDispositionMatchesTheSharedOracle(t *testing.T) {
	raw, err := os.ReadFile(lateSQLCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != lateSQLHash {
		t.Fatal("late-intake oracle changed without updating its pinned hash")
	}
	f := orgtest.New(t)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if lines[0] != "id|operation|args|expected" {
		t.Fatal("oracle header")
	}
	decided := 0
	for _, zone := range []string{"UTC", "America/New_York", "America/Sao_Paulo"} {
		conn, err := f.Admin.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(t.Context(), "SET TIME ZONE '"+zone+"'"); err != nil {
			t.Fatal(err)
		}
		for _, line := range lines[1:] {
			cols := strings.Split(line, "|")
			if len(cols) != 4 {
				t.Fatal("oracle row", line)
			}
			if cols[1] != "DECIDE" {
				continue
			}
			args, expected := map[string]string{}, map[string]string{}
			for _, part := range strings.Split(cols[2], ";") {
				k, v, _ := strings.Cut(part, "=")
				args[k] = v
			}
			for _, part := range strings.Split(cols[3], ";") {
				k, v, _ := strings.Cut(part, "=")
				expected[k] = v
			}
			instant := func(key string) *time.Time {
				if args[key] == "?" {
					return nil
				}
				at, err := orgtime.ParseInstant(args[key])
				if err != nil {
					t.Fatal("oracle instant", line)
				}
				return &at
			}
			window, err := strconv.Atoi(args["window"])
			if err != nil {
				t.Fatal(err)
			}
			var got *string
			if err := conn.QueryRow(t.Context(), `SELECT org_sync_late_disposition($1,$2,$3,$4,$5)`, instant("ended"), window, instant("declared"), instant("claimed"), instant("received")).Scan(&got); err != nil {
				t.Fatal(cols[0], err)
			}
			if expected["error"] == "INVALID" {
				if got != nil {
					t.Fatal("SQL decided invalid late evidence", cols[0], *got)
				}
			} else if got == nil || *got != expected["disposition"] {
				t.Fatal("SQL late disposition disagrees with the shared oracle", zone, cols[0], got)
			}
			decided++
		}
		if _, err := conn.Exec(t.Context(), "RESET TIME ZONE"); err != nil {
			t.Fatal(err)
		}
		conn.Release()
	}
	if decided != 3*47 {
		t.Fatal("SQL consumer did not decide every DECIDE row in every zone", decided)
	}
}

const (
	stateSQLCorpus = "../../../../../contracts/testdata/enterprise/sync/state-intake-v1.psv"
	stateSQLHash   = "ecb5ec152d738be26caa644abe1c2f3700b3445e4c6e8a969084a4cdb5004d5c"
)

// org_sync_plan_disposition is the second consumer of the plan oracle
// (EDR-201): every row agrees with Go DecidePlan, NULL for invalid evidence.
func TestStateIntakeSQLPlanDispositionMatchesTheSharedOracle(t *testing.T) {
	raw, err := os.ReadFile(stateSQLCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != stateSQLHash {
		t.Fatal("state-intake oracle changed without updating its pinned hash")
	}
	f := orgtest.New(t)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	decided := 0
	for _, line := range lines[1:] {
		cols := strings.Split(line, "|")
		args, expected := map[string]string{}, map[string]string{}
		for _, part := range strings.Split(cols[2], ";") {
			k, v, _ := strings.Cut(part, "=")
			args[k] = v
		}
		for _, part := range strings.Split(cols[3], ";") {
			k, v, _ := strings.Cut(part, "=")
			expected[k] = v
		}
		instant := func(key string) *time.Time {
			if args[key] == "?" {
				return nil
			}
			at, err := orgtime.ParseInstant(args[key])
			if err != nil {
				t.Fatal("oracle instant", line)
			}
			return &at
		}
		revision, err := strconv.ParseInt(args["revision"], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		var got *string
		if err := f.Admin.QueryRow(t.Context(), `SELECT org_sync_plan_disposition($1,$2,$3,$4,$5)`, args["state"], revision, instant("changed"), instant("declared"), instant("received")).Scan(&got); err != nil {
			t.Fatal(cols[0], err)
		}
		if expected["error"] == "INVALID" {
			if got != nil {
				t.Fatal("SQL decided invalid plan evidence", cols[0], *got)
			}
		} else if got == nil || *got != expected["disposition"] {
			t.Fatal("SQL plan disposition disagrees with the shared oracle", cols[0], got)
		}
		decided++
	}
	if decided != 20 {
		t.Fatal("SQL plan consumer did not decide every row", decided)
	}
}
