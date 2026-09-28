package kernel

import (
	"errors"
	"strings"
)

// Sale condition kind. Exactly the OpenAPI PriceCondition enum.
type ConditionKind string

const (
	Standard ConditionKind = "STANDARD"
	Cash     ConditionKind = "CASH"
	Debit    ConditionKind = "DEBIT"
	Credit   ConditionKind = "CREDIT"
	App      ConditionKind = "APP"
	Loyalty  ConditionKind = "LOYALTY"
	Other    ConditionKind = "OTHER"
)

var (
	ErrUnknownCondition   = errors.New("kernel: unknown condition kind")
	ErrConditionQualifier = errors.New("kernel: invalid condition qualifier")
)

// Condition is a validated sale condition with an optional program
// qualifier. STANDARD forbids any qualifier; unknown kinds never become
// STANDARD; an empty qualifier string is invalid (use nil for none).
type Condition struct {
	Kind        ConditionKind
	QualifierID *string
}

// ParseCondition validates a kind with an optional qualifier id.
func ParseCondition(kind string, qualifierID *string) (Condition, error) {
	k := ConditionKind(strings.ToUpper(strings.TrimSpace(kind)))
	switch k {
	case Standard, Cash, Debit, Credit, App, Loyalty, Other:
	default:
		return Condition{}, ErrUnknownCondition
	}
	if qualifierID != nil && strings.TrimSpace(*qualifierID) == "" {
		return Condition{}, ErrConditionQualifier
	}
	if k == Standard && qualifierID != nil {
		return Condition{}, ErrConditionQualifier
	}
	return Condition{Kind: k, QualifierID: qualifierID}, nil
}
