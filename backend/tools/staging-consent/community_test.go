package main

import (
	"context"
	"testing"
)

func TestCommunityProbeRefusesMissingStationBeforeNetwork(t *testing.T) {
	for _, station := range []string{"", "not-a-station"} {
		p := probe{}
		if err := p.runCommunity(context.Background(), station); err == nil || p.requests != 0 {
			t.Fatalf("unsafe input performed requests: %q %v", station, err)
		}
	}
}
