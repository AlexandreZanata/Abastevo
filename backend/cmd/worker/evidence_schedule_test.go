package main

import (
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func TestEvidenceScheduleRunsEveryMinute(t *testing.T) {
	s := evidenceSweepSchedule()
	if !s.Enabled || s.Kind != "evidence-sweep" || s.Version != 1 || s.Interval != time.Minute || s.Build == nil {
		t.Fatal("unsafe evidence schedule", s)
	}
	at := time.Date(2026, 10, 12, 10, 1, 1, 0, time.UTC)
	first := jobs.Period(s.Interval, at)
	if first != jobs.Period(s.Interval, at.Add(58*time.Second)) || first == jobs.Period(s.Interval, at.Add(time.Minute)) {
		t.Fatal("evidence cadence was coarsened by dedupe")
	}
	if payload := s.Build(first); payload["version"] != s.Version || len(payload) != 1 {
		t.Fatal("handler envelope changed", payload)
	}
}
