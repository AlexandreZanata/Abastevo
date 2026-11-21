// Package domain owns organization plans and billing rules.
package domain

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	capability "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgcapability/domain"
)

var (
	ErrInvalidPlan    = errors.New("orgbilling: invalid plan")
	ErrPlanNotFound   = errors.New("orgbilling: plan version not found")
	ErrLimitUndefined = errors.New("orgbilling: limit undefined")
	planCode          = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
)

type PlanKey struct {
	Product  string
	Plan     string
	Revision int64
}

func (key PlanKey) Validate() error {
	if key.Revision < 1 || len(key.Product) > 64 || len(key.Plan) > 64 ||
		!planCode.MatchString(key.Product) || !planCode.MatchString(key.Plan) {
		return ErrInvalidPlan
	}
	return nil
}

// Meter identifies a measurable resource. An absent ceiling and a zero
// ceiling remain distinct in storage and in entitlement evaluation.
type Meter string

const (
	ActiveVehicles Meter = "active_vehicles"
	ActiveStations Meter = "active_stations"
	StorageBytes   Meter = "storage_bytes"
	APICalls       Meter = "api_calls"
)

func (meter Meter) known() bool {
	switch meter {
	case ActiveVehicles, ActiveStations, StorageBytes, APICalls:
		return true
	default:
		return false
	}
}

// Plan contains catalog facts only. Callers still need organization and
// membership authority before evaluating any subscription entitlement.
type Plan struct {
	key          PlanKey
	name         string
	capabilities map[string]struct{}
	limits       map[Meter]int64
}

func NewPlan(key PlanKey, name string, capabilities []string, limits map[Meter]int64, known capability.Catalog) (Plan, error) {
	if key.Validate() != nil || name == "" || len(name) > 120 || !utf8.ValidString(name) ||
		strings.TrimSpace(name) != name || strings.ContainsFunc(name, unicode.IsControl) {
		return Plan{}, ErrInvalidPlan
	}

	registered := make(map[string]struct{}, len(capabilities))
	for _, code := range capabilities {
		if known.Validate(code) != nil {
			return Plan{}, ErrInvalidPlan
		}
		if _, exists := registered[code]; exists {
			return Plan{}, ErrInvalidPlan
		}
		registered[code] = struct{}{}
	}

	ceilings := make(map[Meter]int64, len(limits))
	for meter, ceiling := range limits {
		if !meter.known() || ceiling < 0 {
			return Plan{}, ErrInvalidPlan
		}
		ceilings[meter] = ceiling
	}
	return Plan{key: key, name: name, capabilities: registered, limits: ceilings}, nil
}

func (plan Plan) Key() PlanKey { return plan.key }
func (plan Plan) Name() string { return plan.name }

func (plan Plan) Includes(code string) bool {
	_, exists := plan.capabilities[code]
	return exists
}

func (plan Plan) Limit(meter Meter) (int64, error) {
	ceiling, exists := plan.limits[meter]
	if !exists {
		return 0, ErrLimitUndefined
	}
	return ceiling, nil
}
