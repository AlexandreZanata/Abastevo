// Package domain defines registered, reference-only enterprise follow-up intent.
package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"slices"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	audit "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgaudit/domain"
)

const (
	JobKind         = "org-event"
	Version         = 1
	MaxAttempts     = 5
	MaxConsumers    = 50 // Existing bounded operation-batch precedent.
	MaxPayloadBytes = 1024
)

var (
	ErrInvalid     = errors.New("orgevent: invalid intent")
	ErrReused      = errors.New("orgevent: operation reused")
	ErrUnavailable = errors.New("orgevent: unavailable")
	ErrDenied      = errors.New("orgevent: intent denied")
	ErrDisabled    = errors.New("orgevent: producer disabled")
	code           = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*)*$`)
	eventName      = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z][a-z0-9]*){2,}$`)
	hash           = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Event struct {
	ID         string `json:"event_id"`
	Name       string `json:"event_name"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Revision   int64  `json:"revision"`
}
type Envelope struct {
	Version  int    `json:"version"`
	OrgID    string `json:"org_id"`
	Consumer string `json:"consumer"`
	Event
}

func (Event) String() string          { return "orgevent: references redacted" }
func (Event) GoString() string        { return "orgevent: references redacted" }
func (Event) LogValue() slog.Value    { return slog.StringValue("orgevent: references redacted") }
func (Envelope) String() string       { return "orgevent: envelope redacted" }
func (Envelope) GoString() string     { return "orgevent: envelope redacted" }
func (Envelope) LogValue() slog.Value { return slog.StringValue("orgevent: envelope redacted") }
func (e Envelope) DedupeKey() string {
	return "org-event:v1:" + e.OrgID + ":" + e.ID + ":" + e.Consumer
}
func (e Envelope) Payload() []byte { raw, _ := json.Marshal(e); return raw }

type Definition struct {
	Name, TargetType string
	Consumers        []string
}
type Catalog struct{ definitions map[string]Definition }
type Plan struct {
	Event     Event
	Consumers []string
}

func validCode(s string) bool { return len(s) <= 64 && code.MatchString(s) }
func NewCatalog(definitions []Definition) (Catalog, error) {
	if len(definitions) == 0 {
		return Catalog{}, ErrInvalid
	}
	c := Catalog{definitions: make(map[string]Definition, len(definitions))}
	for _, d := range definitions {
		if len(d.Name) > 64 || !eventName.MatchString(d.Name) || !validCode(d.TargetType) || len(d.Consumers) == 0 || len(d.Consumers) > MaxConsumers {
			return Catalog{}, ErrInvalid
		}
		if _, exists := c.definitions[d.Name]; exists {
			return Catalog{}, ErrInvalid
		}
		d.Consumers = slices.Clone(d.Consumers)
		slices.Sort(d.Consumers)
		for i, name := range d.Consumers {
			if !validCode(name) || (i > 0 && name == d.Consumers[i-1]) {
				return Catalog{}, ErrInvalid
			}
		}
		c.definitions[d.Name] = d
	}
	return c, nil
}
func (c Catalog) Consumers() []string {
	var names []string
	for _, d := range c.definitions {
		names = append(names, d.Consumers...)
	}
	slices.Sort(names)
	return slices.Compact(names)
}
func (c Catalog) ValidateEnvelope(e Envelope) (kernel.OrgScope, error) {
	org, err := kernel.ParseOrgID(e.OrgID)
	if err != nil || org.String() != e.OrgID {
		return kernel.OrgScope{}, ErrInvalid
	}
	scope, err := kernel.NewOrgScope(org)
	if err != nil {
		return kernel.OrgScope{}, ErrInvalid
	}
	d, ok := c.definitions[e.Name]
	if !ok || e.Version != Version || d.TargetType != e.TargetType || !audit.ValidID(e.ID) || !audit.ValidID(e.TargetID) || e.Revision < 1 || !slices.Contains(d.Consumers, e.Consumer) {
		return kernel.OrgScope{}, ErrInvalid
	}
	return scope, nil
}

// Prepare binds the original command digest and canonical event/consumer plan.
// Source authority is resolved by the owning command before entering this port.
func (c Catalog) Prepare(scope kernel.OrgScope, a audit.Event, e Event, consumers []string) (Plan, audit.Event, error) {
	if scope.Validate() != nil || e.ID != a.OperationID || e.TargetID != a.TargetID || e.TargetType != a.TargetType ||
		!audit.ValidID(a.ActorID) || (a.DelegationID != "" && !audit.ValidID(a.DelegationID)) || !hash.MatchString(a.RequestHash) ||
		len(consumers) == 0 || len(consumers) > MaxConsumers {
		return Plan{}, audit.Event{}, ErrInvalid
	}
	plan := Plan{Event: e, Consumers: slices.Clone(consumers)}
	slices.Sort(plan.Consumers)
	for i, name := range plan.Consumers {
		if i > 0 && name == plan.Consumers[i-1] {
			return Plan{}, audit.Event{}, ErrInvalid
		}
		if _, err := c.ValidateEnvelope(Envelope{Version: Version, OrgID: scope.OrgID().String(), Consumer: name, Event: e}); err != nil {
			return Plan{}, audit.Event{}, ErrInvalid
		}
	}
	raw, _ := json.Marshal(struct {
		OrgID, CommandHash string
		Plan               Plan
	}{scope.OrgID().String(), a.RequestHash, plan})
	sum := sha256.Sum256(append([]byte("org-event.command.v1\x00"), raw...))
	a.RequestHash = hex.EncodeToString(sum[:])
	return plan, a, nil
}
