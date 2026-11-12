// Package domain defines bounded organization audit references and vocabulary.
// It performs no I/O and shares UUID syntax with the pure kernel.
package domain

import (
	"errors"
	"regexp"
	"slices"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
)

var (
	ErrInvalid     = errors.New("orgaudit: invalid audit metadata")
	ErrReused      = errors.New("orgaudit: operation ID reused")
	ErrDenied      = errors.New("orgaudit: audit read denied")
	ErrUnavailable = errors.New("orgaudit: unavailable")
	codePattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
	hashPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Event contains references and registered codes, with a digest of the entire
// canonical command. ActorID is the verified account acting on the command.
type Event struct {
	OperationID  string
	ActorID      string
	DelegationID string
	Action       string
	TargetType   string
	TargetID     string
	ReasonCode   string
	RequestHash  string
}

// Record adds the server-generated recording time.
type Record struct {
	Event Event
	At    time.Time
}

// Definition is source-owned vocabulary registered by a module at composition.
// Human explanations belong to the referenced business record.
type Definition struct {
	Action        string
	TargetType    string
	Reasons       []string
	RequireReason bool
}

// Catalog holds an immutable copy of registered action definitions.
type Catalog struct{ definitions map[string]Definition }

func NewCatalog(definitions []Definition) (Catalog, error) {
	if len(definitions) == 0 {
		return Catalog{}, ErrInvalid
	}
	catalog := Catalog{definitions: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		if !validCode(definition.Action) || !validCode(definition.TargetType) ||
			(definition.RequireReason && len(definition.Reasons) == 0) {
			return Catalog{}, ErrInvalid
		}
		if _, exists := catalog.definitions[definition.Action]; exists {
			return Catalog{}, ErrInvalid
		}
		for _, reason := range definition.Reasons {
			if !validCode(reason) {
				return Catalog{}, ErrInvalid
			}
		}
		definition.Reasons = slices.Clone(definition.Reasons)
		catalog.definitions[definition.Action] = definition
	}
	return catalog, nil
}

func (catalog Catalog) Validate(event Event) error {
	definition, exists := catalog.definitions[event.Action]
	if !exists || event.TargetType != definition.TargetType ||
		!ValidID(event.OperationID) || !ValidID(event.ActorID) || !ValidID(event.TargetID) ||
		(event.DelegationID != "" && !ValidID(event.DelegationID)) || !hashPattern.MatchString(event.RequestHash) {
		return ErrInvalid
	}
	if event.ReasonCode == "" {
		if definition.RequireReason {
			return ErrInvalid
		}
	} else if !slices.Contains(definition.Reasons, event.ReasonCode) {
		return ErrInvalid
	}
	return nil
}

func validCode(value string) bool {
	return len(value) <= 64 && codePattern.MatchString(value)
}

// ValidID checks canonical non-nil UUID syntax without inferring authority.
func ValidID(value string) bool {
	id, err := kernel.ParseOrgID(value)
	return err == nil && id.String() == value
}

// Filter is an internal keyset page. HTTP cursor sealing belongs to the API.
type Filter struct {
	Limit             int
	BeforeAt          time.Time
	BeforeOperationID string
}

func (filter Filter) Normalize() (Filter, error) {
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	if filter.Limit < 1 || filter.Limit > 100 ||
		filter.BeforeAt.IsZero() != (filter.BeforeOperationID == "") ||
		(filter.BeforeOperationID != "" && !ValidID(filter.BeforeOperationID)) {
		return Filter{}, ErrInvalid
	}
	filter.BeforeAt = filter.BeforeAt.UTC()
	return filter, nil
}
