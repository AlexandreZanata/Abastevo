package config

import (
	"testing"
	"time"
)

func TestShutdownDrainDelayBounds(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    time.Duration
		invalid bool
	}{
		{"", 10 * time.Second, false}, {"0s", 0, false}, {"30s", 30 * time.Second, false},
		{"-1s", 0, true}, {"30.001s", 0, true}, {"invalid", 0, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			c, err := load(envFunc(map[string]string{"ANPFUEL_SHUTDOWN_DRAIN_DELAY": tc.raw}))
			if (err != nil) != tc.invalid {
				t.Fatalf("invalid=%v err=%v", tc.invalid, err)
			}
			if err == nil && c.ShutdownDrainDelay != tc.want {
				t.Fatalf("got %v want %v", c.ShutdownDrainDelay, tc.want)
			}
		})
	}
}
