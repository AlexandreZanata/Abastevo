package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

func retentionPorts(purges ...NamedPurge) RetentionPorts {
	return RetentionPorts{
		Clock: func() time.Time {
			return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
		},
		Purges: purges,
	}
}

func TestRetainReportsEveryCategory(t *testing.T) {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	var order []string
	purge := func(name string, purged int64, oldest time.Time) NamedPurge {
		return NamedPurge{Name: name, Purge: func(context.Context) (CategoryReport, error) {
			order = append(order, name)
			return CategoryReport{Purged: purged, OldestOverdue: oldest}, nil
		}}
	}
	report, err := Retain(context.Background(), retentionPorts(
		purge("challenges", 12, base.Add(-time.Hour)),
		purge("idempotency", 0, time.Time{}),
		purge("moderation-closed", 3, base.Add(-400*24*time.Hour)),
	))
	if err != nil {
		t.Fatalf("retain = %v", err)
	}
	if len(report.Categories) != 3 {
		t.Fatalf("categories = %d", len(report.Categories))
	}
	if report.Categories[0].Category != "challenges" || report.Categories[0].Purged != 12 {
		t.Errorf("first = %+v", report.Categories[0])
	}
	if !report.Categories[0].OldestOverdue.Equal(base.Add(-time.Hour)) {
		t.Errorf("oldest = %v", report.Categories[0].OldestOverdue)
	}
	if !report.GeneratedAt.Equal(base) {
		t.Errorf("generated = %v", report.GeneratedAt)
	}
	if len(order) != 3 || order[2] != "moderation-closed" {
		t.Errorf("order = %v", order)
	}
}

func TestRetainCollectsMetricsDespiteFailure(t *testing.T) {
	boom := errors.New("category down")
	report, err := Retain(context.Background(), retentionPorts(
		NamedPurge{Name: "ok", Purge: func(context.Context) (CategoryReport, error) {
			return CategoryReport{Purged: 2}, nil
		}},
		NamedPurge{Name: "broken", Purge: func(context.Context) (CategoryReport, error) {
			return CategoryReport{}, boom
		}},
		NamedPurge{Name: "later", Purge: func(context.Context) (CategoryReport, error) {
			return CategoryReport{Purged: 5}, nil
		}},
	))
	if !errors.Is(err, boom) {
		t.Fatalf("retain error = %v", err)
	}
	// Later categories still ran and reported: one stuck category
	// cannot hide the others' metrics, and its next tick resumes.
	if len(report.Categories) != 2 {
		t.Errorf("categories = %+v", report.Categories)
	}
}

func TestRetainEmptySweep(t *testing.T) {
	report, err := Retain(context.Background(), retentionPorts())
	if err != nil || len(report.Categories) != 0 {
		t.Errorf("empty = %+v, %v", report, err)
	}
}
