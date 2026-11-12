package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

func exampleEvent() domain.Event {
	return domain.Event{
		OperationID: "7b0e2a54-0000-4000-8000-000000000010",
		ActorID:     "7b0e2a54-0000-4000-8000-000000000011",
		TargetID:    "7b0e2a54-0000-4000-8000-000000000012",
		Action:      "org.settings.changed", TargetType: "org.settings",
		ReasonCode: "correction", RequestHash: strings.Repeat("a", 64),
	}
}

func TestCatalogValidatesBoundedReferenceOnlyEvents(t *testing.T) {
	reasons := []string{"correction"}
	definitions := []domain.Definition{{Action: "org.settings.changed", TargetType: "org.settings", Reasons: reasons, RequireReason: true}}
	catalog, err := domain.NewCatalog(definitions)
	if err != nil {
		t.Fatal(err)
	}
	// The registered vocabulary must be independent from mutable caller slices.
	reasons[0] = "changed"
	definitions[0].TargetType = "changed"
	event := exampleEvent()
	if err := catalog.Validate(event); err != nil {
		t.Fatal(err)
	}
	event.DelegationID = event.ActorID
	if err := catalog.Validate(event); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*domain.Event){
		"missing actor":           func(e *domain.Event) { e.ActorID = "" },
		"nil operation":           func(e *domain.Event) { e.OperationID = "00000000-0000-0000-0000-000000000000" },
		"malformed target":        func(e *domain.Event) { e.TargetID = "document bytes" },
		"uppercase ID":            func(e *domain.Event) { e.ActorID = strings.ToUpper(e.ActorID) },
		"malformed delegation":    func(e *domain.Event) { e.DelegationID = "Bearer synthetic" },
		"unknown action":          func(e *domain.Event) { e.Action = "org.settings.deleted" },
		"credential action":       func(e *domain.Event) { e.Action = "Bearer synthetic" },
		"wrong target type":       func(e *domain.Event) { e.TargetType = "document" },
		"missing required reason": func(e *domain.Event) { e.ReasonCode = "" },
		"free text reason":        func(e *domain.Event) { e.ReasonCode = "a receipt description" },
		"coordinates":             func(e *domain.Event) { e.ReasonCode = "-1.23,-4.56" },
		"payload reason":          func(e *domain.Event) { e.ReasonCode = "{\"content\":\"synthetic\"}" },
		"unknown reason":          func(e *domain.Event) { e.ReasonCode = "changed" },
		"missing digest":          func(e *domain.Event) { e.RequestHash = "" },
		"invalid digest":          func(e *domain.Event) { e.RequestHash = strings.Repeat("g", 64) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			e := exampleEvent()
			change(&e)
			if err := catalog.Validate(e); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("invalid event returned %v", err)
			}
		})
	}
	optional, err := domain.NewCatalog([]domain.Definition{{Action: "org.created", TargetType: "org"}})
	if err != nil {
		t.Fatal(err)
	}
	event.Action, event.TargetType, event.ReasonCode = "org.created", "org", ""
	if err := optional.Validate(event); err != nil {
		t.Fatal(err)
	}
	if err := (domain.Catalog{}).Validate(event); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal("empty catalog granted an action")
	}
}

func TestCatalogRejectsInvalidDefinitions(t *testing.T) {
	valid := domain.Definition{Action: "org.created", TargetType: "org"}
	for _, definitions := range [][]domain.Definition{
		nil, {valid, valid}, {{Action: "invalid action", TargetType: "org"}},
		{{Action: "org.created", TargetType: strings.Repeat("a", 65)}},
		{{Action: "org.created", TargetType: "org", RequireReason: true}},
		{{Action: "org.created", TargetType: "org", Reasons: []string{"free text"}}},
	} {
		if _, err := domain.NewCatalog(definitions); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("invalid catalog returned %v", err)
		}
	}
}

func TestFilterValidatesCursorAndBounds(t *testing.T) {
	filter, err := (domain.Filter{}).Normalize()
	if err != nil || filter.Limit != 20 {
		t.Fatal("default page was not bounded")
	}
	stamp := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	id := exampleEvent().OperationID
	for _, f := range []domain.Filter{{Limit: -1}, {Limit: 101}, {BeforeAt: stamp}, {BeforeOperationID: id}, {BeforeAt: stamp, BeforeOperationID: "bad"}} {
		if _, err := f.Normalize(); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("invalid filter returned %v", err)
		}
	}
	if _, err := (domain.Filter{Limit: 100, BeforeAt: stamp, BeforeOperationID: id}).Normalize(); err != nil {
		t.Fatal(err)
	}
}
