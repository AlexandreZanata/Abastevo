package domain_test

import (
	"errors"
	"strings"
	"testing"

	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
	event "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgevent/domain"
)

func TestLegalVocabularyStillRequiresExactProducerConsumerAndTargetRegistration(t *testing.T) {
	definition := event.Definition{Name: "org.legal_entity.declared", TargetType: "legal_entity", Consumers: []string{"legal_projection.refresh"}}
	catalog, err := event.NewCatalog([]event.Definition{definition})
	if err != nil {
		t.Fatal(err)
	}
	a := audit.Event{OperationID: id, ActorID: id, TargetID: id, Action: definition.Name, TargetType: definition.TargetType, RequestHash: strings.Repeat("a", 64)}
	e := event.Event{ID: id, Name: definition.Name, TargetType: definition.TargetType, TargetID: id, Revision: 1}
	auditCatalog, err := audit.NewCatalog([]audit.Definition{{Action: a.Action, TargetType: a.TargetType}})
	if err != nil {
		t.Fatal(err)
	}
	if err := auditCatalog.Validate(a); err != nil {
		t.Fatal(err)
	}
	if _, _, err := catalog.Prepare(scope(t, id), a, e, definition.Consumers); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*audit.Event, *event.Event, *[]string){
		func(a *audit.Event, e *event.Event, c *[]string) { e.Name = "org.legal_entity.verified" },
		func(a *audit.Event, e *event.Event, c *[]string) { e.TargetType = "other_entity" },
		func(a *audit.Event, e *event.Event, c *[]string) { *c = []string{"unregistered_consumer.refresh"} },
	} {
		a2, e2, c := a, e, append([]string(nil), definition.Consumers...)
		change(&a2, &e2, &c)
		if _, _, err := catalog.Prepare(scope(t, id), a2, e2, c); err == nil {
			t.Fatal("unregistered legal event admitted")
		}
	}
	for _, name := range []string{"org.*.declared", "org.legal entity.declared", "_org.legal_entity.declared", "org..declared"} {
		if _, err := event.NewCatalog([]event.Definition{{Name: name, TargetType: definition.TargetType, Consumers: definition.Consumers}}); !errors.Is(err, event.ErrInvalid) {
			t.Fatal("unsafe vocabulary accepted")
		}
	}
}
