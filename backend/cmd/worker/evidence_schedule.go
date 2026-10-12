package main

import (
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/jobs"
)

func evidenceSweepSchedule() jobs.Schedule {
	return jobs.Schedule{
		Name: "evidence-sweep-every-minute", Kind: "evidence-sweep", Version: 1,
		Interval: time.Minute, Enabled: true,
		Build: func(string) map[string]any { return map[string]any{"version": 1} },
	}
}
