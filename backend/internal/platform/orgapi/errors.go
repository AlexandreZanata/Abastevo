// Package orgapi defines the shared, fail-closed organization HTTP boundary.
package orgapi

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/httpapi"
)

type Code string

const (
	Malformed               Code = "MALFORMED"
	Unauthenticated         Code = "UNAUTHENTICATED"
	NotFound                Code = "NOT_FOUND"
	Forbidden               Code = "FORBIDDEN"
	MembershipSuspended     Code = "MEMBERSHIP_SUSPENDED"
	MembershipEnded         Code = "MEMBERSHIP_ENDED"
	OrgSuspended            Code = "ORG_SUSPENDED"
	OrgArchived             Code = "ORG_ARCHIVED"
	OrgSetupIncomplete      Code = "ORG_SETUP_INCOMPLETE"
	FreshAuthRequired       Code = "FRESH_AUTH_REQUIRED"
	EntitlementRequired     Code = "ENTITLEMENT_REQUIRED"
	LimitReached            Code = "LIMIT_REACHED"
	Conflict                Code = "CONFLICT"
	OperationReused         Code = "OPERATION_ID_REUSED"
	ValidationFailed        Code = "VALIDATION_FAILED"
	SelfApproval            Code = "SELF_APPROVAL"
	PeriodLocked            Code = "PERIOD_LOCKED"
	LastOwner               Code = "LAST_OWNER"
	LastUnit                Code = "LAST_UNIT"
	StructureInUse          Code = "STRUCTURE_IN_USE"
	DuplicateReviewRequired Code = "DUPLICATE_REVIEW_REQUIRED"
	OwnerRequiresPersonal   Code = "OWNER_REQUIRES_PERSONAL_ACCOUNT"
	RateLimited             Code = "RATE_LIMITED"
	Unavailable             Code = "UNAVAILABLE"
)

func (code Code) Error() string { return string(code) }

var statuses = map[Code]int{
	Malformed: 400, Unauthenticated: 401, NotFound: 404,
	Forbidden: 403, MembershipSuspended: 403, MembershipEnded: 403, OrgSuspended: 403, OrgArchived: 403,
	FreshAuthRequired: 401, EntitlementRequired: 403, LimitReached: 403,
	Conflict: 409, OperationReused: 409, SelfApproval: 409, PeriodLocked: 409, LastOwner: 409,
	ValidationFailed: 422, RateLimited: 429, Unavailable: 503,
	LastUnit: 409, StructureInUse: 409, DuplicateReviewRequired: 409,
	OrgSetupIncomplete:    409,
	OwnerRequiresPersonal: 409,
}

type permissionFailure struct{ key string }

type setupFailure struct{ fields []string }

type fieldFailure struct{ field, reason string }

func (fieldFailure) Error() string { return string(ValidationFailed) }
func (fieldFailure) Unwrap() error { return ValidationFailed }

// UnitDuplicate accepts only the source-owned active uniqueness fields.
func UnitDuplicate(field string) error {
	if field != "name" && field != "code" {
		return Unavailable
	}
	return fieldFailure{field, "DUPLICATE"}
}

// EmployeeRegistrationDuplicate has no caller-selected field or private value.
func EmployeeRegistrationDuplicate() error { return fieldFailure{"registration_number", "DUPLICATE"} }

// StructureCounts contains category totals, never source records or identities.
type StructureCounts struct{ Assets, Employees, Budgets, Operations int64 }
type structureFailure struct{ counts StructureCounts }

func (structureFailure) Error() string { return string(StructureInUse) }
func (structureFailure) Unwrap() error { return StructureInUse }
func StructureInUseCounts(v StructureCounts) error {
	if v.Assets < 0 || v.Employees < 0 || v.Budgets < 0 || v.Operations < 0 || v == (StructureCounts{}) {
		return Unavailable
	}
	return structureFailure{v}
}

// IdentifierValidation exposes only registered source fields and reason codes.
// Raw identifiers and provider/SQL text are never accepted as details.
func IdentifierValidation(field, reason string) error {
	if field != "identifier" && field != "cnpj" {
		return Unavailable
	}
	switch reason {
	case "INVALID_CPF", "INVALID_CNPJ", "DUPLICATE":
	default:
		return Unavailable
	}
	return fieldFailure{field, reason}
}

func (setupFailure) Error() string { return string(OrgSetupIncomplete) }
func (setupFailure) Unwrap() error { return OrgSetupIncomplete }

// IncompleteSetup accepts source checklist keys only, never free text or
// resource identifiers. The caller cannot mutate a recorded error afterward.
func IncompleteSetup(fields []string) error {
	if len(fields) == 0 || len(fields) > 5 {
		return Unavailable
	}
	seen := map[string]bool{}
	for _, field := range fields {
		switch field {
		case "name", "time_zone", "active_unit", "active_owner", "terms":
		default:
			return Unavailable
		}
		if seen[field] {
			return Unavailable
		}
		seen[field] = true
	}
	return setupFailure{append([]string(nil), fields...)}
}

func (e permissionFailure) Error() string { return string(Forbidden) }
func (e permissionFailure) Unwrap() error { return Forbidden }

// MissingPermission accepts a canonical source key, never request data. AUTH
// supplies only the registered key carried by a bounded permission denial.
func MissingPermission(key string) error {
	if len(key) > 128 || !permissionPattern.MatchString(key) {
		return Unavailable
	}
	return permissionFailure{key: key}
}

// Registered returns the bare registered code carried by err, without any
// wrapping text, so a module can keep a refusal's status but not its detail.
func Registered(err error) (Code, bool) {
	var reported Code
	if errors.As(err, &reported) {
		if _, known := statuses[reported]; known {
			return reported, true
		}
	}
	return "", false
}

// WriteError emits registered codes and static text. Raw provider errors stay private.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	code := Unavailable
	if reported, ok := Registered(err); ok {
		code = reported
	}
	recordCode(r, code)
	var limited *RateLimitError
	if code == RateLimited && errors.As(err, &limited) && limited != nil && limited.RetryAfter > 0 {
		seconds := limited.RetryAfter / time.Second
		if limited.RetryAfter%time.Second != 0 {
			seconds++
		}
		w.Header().Set("Retry-After", strconv.FormatInt(int64(seconds), 10))
	}
	w.Header().Del("ETag")
	var details []httpapi.Detail
	var denied permissionFailure
	if code == Forbidden && errors.As(err, &denied) {
		details = []httpapi.Detail{{Field: "permission", Code: denied.key}}
	}
	var setup setupFailure
	if code == OrgSetupIncomplete && errors.As(err, &setup) {
		for _, field := range setup.fields {
			details = append(details, httpapi.Detail{Field: field, Code: "REQUIRED"})
		}
	}
	var field fieldFailure
	if code == ValidationFailed && errors.As(err, &field) {
		details = []httpapi.Detail{{Field: field.field, Code: field.reason}}
	}
	var structure structureFailure
	if code == StructureInUse && errors.As(err, &structure) {
		counts := structure.counts
		for _, category := range []struct {
			field string
			count int64
		}{{"active_assets", counts.Assets}, {"active_employees", counts.Employees}, {"open_budgets", counts.Budgets}, {"open_operations", counts.Operations}} {
			details = append(details, httpapi.Detail{Field: category.field, Code: "IN_USE", Count: strconv.FormatInt(category.count, 10)})
		}
	}
	httpapi.WriteError(w, r, statuses[code], string(code), "Organization request unavailable or refused", details)
}
