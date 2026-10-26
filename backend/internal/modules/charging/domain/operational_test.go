package charging

import (
	"testing"
	"time"
)

func TestOperationalEventShapesAndClock(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	place := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	group := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	valid := OperationalEvent{Event: "WORKED", TargetScope: "SITE", ObservedAt: now.Format(time.RFC3339)}
	if err := ValidateOperationalEvent(nil, nil, now); err != nil {
		t.Fatal("legacy observation rejected", err)
	}
	for _, event := range []string{"WORKED", "DID_NOT_WORK", "IN_USE", "ACCESS_RESTRICTED"} {
		sample := valid
		sample.Event = event
		if err := ValidateOperationalEvent(&sample, &place, now); err != nil {
			t.Fatal(event, err)
		}
	}
	for _, scope := range []string{"EQUIPMENT", "CONNECTOR"} {
		sample := valid
		sample.TargetScope = scope
		sample.ConnectionGroupID = &group
		if err := ValidateOperationalEvent(&sample, &place, now); err != nil {
			t.Fatal(scope, err)
		}
	}
	tests := map[string]func(*OperationalEvent){
		"unknown event":   func(e *OperationalEvent) { e.Event = "LIVE_AVAILABLE" },
		"unknown scope":   func(e *OperationalEvent) { e.TargetScope = "NEARBY" },
		"missing target":  func(e *OperationalEvent) { e.TargetScope = "CONNECTOR" },
		"site with group": func(e *OperationalEvent) { e.ConnectionGroupID = &group },
		"invalid clock":   func(e *OperationalEvent) { e.ObservedAt = "2026-99-99" },
		"future instant":  func(e *OperationalEvent) { e.ObservedAt = now.Add(time.Nanosecond).Format(time.RFC3339Nano) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			sample := valid
			mutate(&sample)
			if ValidateOperationalEvent(&sample, &place, now) == nil {
				t.Fatal("invalid event admitted")
			}
		})
	}
	if ValidateOperationalEvent(&valid, nil, now) == nil {
		t.Fatal("operational event without actual place admitted")
	}
	bad := "not-a-uuid"
	if ValidateOperationalEvent(&valid, &bad, now) == nil {
		t.Fatal("invalid place admitted")
	}
	past := valid
	past.ObservedAt = now.Add(-24 * time.Hour).Format(time.RFC3339)
	if err := ValidateOperationalEvent(&past, &place, now); err != nil {
		t.Fatal("invented expiry", err)
	}
}
