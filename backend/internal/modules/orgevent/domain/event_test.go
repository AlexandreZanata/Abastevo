package domain_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
)

func TestPayloadPreservesOnlyReferencesAndRedactsLogSurfaces(t *testing.T) {
	_, e := sample()
	env := event.Envelope{Version: 1, OrgID: id, Consumer: "projection.refresh", Event: e}
	var decoded map[string]any
	if err := json.Unmarshal(env.Payload(), &decoded); err != nil || len(decoded) != 8 || decoded["org_id"] != id || decoded["revision"] != float64(1) {
		t.Fatal("reference envelope changed", err)
	}
	for _, value := range []any{e, env} {
		var log bytes.Buffer
		slog.New(slog.NewJSONHandler(&log, nil)).Info("event", "value", value)
		output := fmt.Sprintf("%v %+v %#v", value, value, value) + log.String()
		if strings.Contains(output, id) || !strings.Contains(output, "redacted") {
			t.Fatal("private references logged")
		}
	}
}

const id = "7b0e2a54-0000-4000-8000-000000000001"

func scope(t *testing.T, id string) kernel.OrgScope {
	t.Helper()
	org, err := kernel.ParseOrgID(id)
	if err != nil {
		t.Fatal(err)
	}
	s, err := kernel.NewOrgScope(org)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func catalog(t *testing.T) event.Catalog {
	t.Helper()
	c, err := event.NewCatalog([]event.Definition{{Name: "example.record.changed", TargetType: "example.record", Consumers: []string{"projection.refresh", "review.prepare"}}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func sample() (audit.Event, event.Event) {
	return audit.Event{OperationID: id, ActorID: id, TargetID: id, TargetType: "example.record", Action: "example.record.changed", RequestHash: strings.Repeat("a", 64)},
		event.Event{ID: id, Name: "example.record.changed", TargetType: "example.record", TargetID: id, Revision: 1}
}
func TestRegisteredIntentBindsReplayAndOrganization(t *testing.T) {
	c := catalog(t)
	a, e := sample()
	s := scope(t, id)
	p, bound, err := c.Prepare(s, a, e, []string{"review.prepare", "projection.refresh"})
	if err != nil {
		t.Fatal(err)
	}
	q, same, err := c.Prepare(s, a, e, []string{"projection.refresh", "review.prepare"})
	if err != nil || same != bound || p.Consumers[0] != q.Consumers[0] {
		t.Fatal("order changed digest", err)
	}
	if bound.RequestHash == a.RequestHash {
		t.Fatal("event plan was not bound")
	}
	e.Revision++
	_, changed, err := c.Prepare(s, a, e, p.Consumers)
	if err != nil || changed.RequestHash == bound.RequestHash {
		t.Fatal("changed event replayed", err)
	}
	e.Revision--
	_, fewer, err := c.Prepare(s, a, e, []string{"projection.refresh"})
	if err != nil || fewer.RequestHash == bound.RequestHash {
		t.Fatal("consumer set not bound", err)
	}
	env := event.Envelope{Version: 1, OrgID: id, Consumer: "projection.refresh", Event: e}
	if _, err := c.ValidateEnvelope(env); err != nil {
		t.Fatal(err)
	}
	other := env
	other.OrgID = "7b0e2a54-0000-4000-8000-000000000002"
	if env.DedupeKey() == other.DedupeKey() {
		t.Fatal("foreign scope shares dedupe")
	}
	for _, bad := range []event.Envelope{
		{Version: 2, OrgID: id, Consumer: env.Consumer, Event: e},
		{Version: 1, OrgID: id, Consumer: "credential.private", Event: e},
		{Version: 1, OrgID: "", Consumer: env.Consumer, Event: e},
	} {
		if _, err := c.ValidateEnvelope(bad); !errors.Is(err, event.ErrInvalid) {
			t.Fatal(err)
		}
	}
}
func TestCatalogRejectsFreeTextAndCopiesRegistration(t *testing.T) {
	for _, defs := range [][]event.Definition{
		nil,
		{{Name: "example.record.changed", TargetType: "example.record", Consumers: []string{"secret in payload"}}},
		{{Name: "bad", TargetType: "example.record", Consumers: []string{"projection.refresh"}}},
		{{Name: "example.record.changed", TargetType: "example.record", Consumers: []string{"projection.refresh", "projection.refresh"}}},
	} {
		if _, err := event.NewCatalog(defs); !errors.Is(err, event.ErrInvalid) {
			t.Fatal(err)
		}
	}
	defs := []event.Definition{{Name: "example.record.changed", TargetType: "example.record", Consumers: []string{"projection.refresh"}}}
	c, err := event.NewCatalog(defs)
	if err != nil {
		t.Fatal(err)
	}
	defs[0].Consumers[0] = "changed.consumer"
	a, e := sample()
	if _, _, err := c.Prepare(scope(t, id), a, e, []string{"projection.refresh"}); err != nil {
		t.Fatal("mutable catalog", err)
	}
}

func TestInvalidPlansNeverReachTheTransaction(t *testing.T) {
	for _, kind := range []string{"scope", "operation", "target", "type", "actor", "delegation", "hash", "empty", "budget", "duplicate", "unknown consumer", "unknown event", "invalid event", "invalid target", "zero revision"} {
		t.Run(kind, func(t *testing.T) {
			a, e := sample()
			s := scope(t, id)
			consumers := []string{"projection.refresh"}
			switch kind {
			case "scope":
				s = kernel.OrgScope{}
			case "operation":
				a.OperationID = ""
			case "target":
				a.TargetID = ""
			case "type":
				a.TargetType = ""
			case "actor":
				a.ActorID = ""
			case "delegation":
				a.DelegationID = "private-text"
			case "hash":
				a.RequestHash = strings.Repeat("A", 64)
			case "empty":
				consumers = nil
			case "budget":
				consumers = make([]string, event.MaxConsumers+1)
			case "duplicate":
				consumers = []string{"projection.refresh", "projection.refresh"}
			case "unknown consumer":
				consumers = []string{"unknown.consumer"}
			case "unknown event":
				e.Name = "unknown.record.changed"
			case "invalid event":
				e.ID = ""
				a.OperationID = ""
			case "invalid target":
				e.TargetID = ""
				a.TargetID = ""
			case "zero revision":
				e.Revision = 0
			}
			if _, _, err := catalog(t).Prepare(s, a, e, consumers); !errors.Is(err, event.ErrInvalid) {
				t.Fatal("unsafe plan accepted", err)
			}
		})
	}
	def := event.Definition{Name: "example.record.changed", TargetType: "example.record", Consumers: []string{"projection.refresh"}}
	for _, kind := range []string{"duplicate", "target", "empty", "budget", "long name", "long consumer"} {
		t.Run("catalog "+kind, func(t *testing.T) {
			d := def
			switch kind {
			case "target":
				d.TargetType = "private text"
			case "empty":
				d.Consumers = nil
			case "budget":
				d.Consumers = make([]string, event.MaxConsumers+1)
			case "long name":
				d.Name = strings.Repeat("a", 65) + ".record.changed"
			case "long consumer":
				d.Consumers = []string{strings.Repeat("a", 65)}
			}
			defs := []event.Definition{d}
			if kind == "duplicate" {
				defs = append(defs, d)
			}
			if _, err := event.NewCatalog(defs); !errors.Is(err, event.ErrInvalid) {
				t.Fatal(err)
			}
		})
	}
	c := catalog(t)
	names := c.Consumers()
	names[0] = "changed.consumer"
	if c.Consumers()[0] != "projection.refresh" {
		t.Fatal("catalog consumer view aliases registration")
	}
	a, e := sample()
	a.DelegationID = id
	if _, _, err := c.Prepare(scope(t, id), a, e, []string{"projection.refresh"}); err != nil {
		t.Fatal(err)
	}
	for _, org := range []string{"00000000-0000-0000-0000-000000000000", strings.ToUpper(id)} {
		if _, err := c.ValidateEnvelope(event.Envelope{Version: 1, OrgID: org, Consumer: "projection.refresh", Event: e}); !errors.Is(err, event.ErrInvalid) {
			t.Fatal(err)
		}
	}
}
