package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSafetyReserveBoundaries(t *testing.T) {
	if err := budget(12*1024*1024, 5.2, 8, 50*1024*1024*1024); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		mem  uint64
		load float64
		cpu  int
		disk uint64
	}{{1, 0, 8, 1 << 40}, {1 << 30, 5.21, 8, 1 << 40}, {1 << 30, 0, 0, 1 << 40}, {1 << 30, 0, 8, 1}} {
		if budget(v.mem, v.load, v.cpu, v.disk) == nil {
			t.Fatal("unsafe budget accepted")
		}
	}
}
func TestChildFailureRedactsSecretAndSourceText(t *testing.T) {
	t.Setenv("ANPFUEL_DATABASE_URL", "synthetic-secret-never-log")
	err := child(context.Background(), "synthetic_failure", "/nonexistent-owned-test-binary", nil, true)
	if err == nil || strings.Contains(err.Error(), os.Getenv("ANPFUEL_DATABASE_URL")) {
		t.Fatal("child failed without safe error")
	}
}

func TestChildCountersAreBoundedAndAllowlisted(t *testing.T) {
	var output countsBuffer
	_, _ = output.Write([]byte("accepted=7 preserved_curated=2 secret=synthetic-private-value source_key=04218406000104 accepted=invalid\n"))
	result := output.counts()
	if len(result) != 2 || result["accepted"] != 7 || result["preserved_curated"] != 2 {
		t.Fatalf("unsafe counters %+v", result)
	}
	_, _ = output.Write([]byte(strings.Repeat("X", 10000)))
	if output.Len() != 4096 {
		t.Fatal("child output exceeded cap")
	}
}

func TestWaitForBudgetDefersTransientRefusal(t *testing.T) {
	calls := 0
	var refusals []string
	check := func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("host load1 exceeds conservative 0.65 x CPU-count reserve")
		}
		return nil
	}
	err := waitForBudget(context.Background(), check, time.Second, time.Millisecond, func(err error) { refusals = append(refusals, err.Error()) })
	if err != nil || calls != 3 || len(refusals) != 2 {
		t.Fatalf("transient refusal not deferred: err=%v calls=%d refusals=%d", err, calls, len(refusals))
	}
}

func TestWaitForBudgetFailsClosedAfterBoundedWait(t *testing.T) {
	refused := errors.New("host available RAM below 12GiB reserve")
	calls := 0
	started := time.Now()
	err := waitForBudget(context.Background(), func(context.Context) error { calls++; return refused }, 20*time.Millisecond, 5*time.Millisecond, func(error) {})
	if !errors.Is(err, refused) || calls < 2 || time.Since(started) > time.Second {
		t.Fatalf("persistent refusal not returned after bounded wait: err=%v calls=%d", err, calls)
	}
}

func TestWaitForBudgetStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitForBudget(ctx, func(context.Context) error { return errors.New("refused") }, time.Minute, time.Second, func(error) {})
	if err == nil {
		t.Fatal("cancelled wait accepted")
	}
}

func TestWaitForBudgetZeroWaitChecksOnce(t *testing.T) {
	calls := 0
	err := waitForBudget(context.Background(), func(context.Context) error { calls++; return errors.New("refused") }, 0, time.Millisecond, func(error) {})
	if err == nil || calls != 1 {
		t.Fatalf("zero wait must check exactly once: err=%v calls=%d", err, calls)
	}
}
