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
	Malformed           Code = "MALFORMED"
	Unauthenticated     Code = "UNAUTHENTICATED"
	NotFound            Code = "NOT_FOUND"
	Forbidden           Code = "FORBIDDEN"
	MembershipSuspended Code = "MEMBERSHIP_SUSPENDED"
	MembershipEnded     Code = "MEMBERSHIP_ENDED"
	OrgSuspended        Code = "ORG_SUSPENDED"
	OrgArchived         Code = "ORG_ARCHIVED"
	OrgSetupIncomplete  Code = "ORG_SETUP_INCOMPLETE"
	FreshAuthRequired   Code = "FRESH_AUTH_REQUIRED"
	EntitlementRequired Code = "ENTITLEMENT_REQUIRED"
	LimitReached        Code = "LIMIT_REACHED"
	Conflict            Code = "CONFLICT"
	OperationReused     Code = "OPERATION_ID_REUSED"
	ValidationFailed    Code = "VALIDATION_FAILED"
	SelfApproval        Code = "SELF_APPROVAL"
	PeriodLocked        Code = "PERIOD_LOCKED"
	LastOwner           Code = "LAST_OWNER"
	RateLimited         Code = "RATE_LIMITED"
	Unavailable         Code = "UNAVAILABLE"
)

func (code Code) Error() string { return string(code) }

var statuses = map[Code]int{
	Malformed: 400, Unauthenticated: 401, NotFound: 404,
	Forbidden: 403, MembershipSuspended: 403, MembershipEnded: 403, OrgSuspended: 403, OrgArchived: 403,
	FreshAuthRequired: 401, EntitlementRequired: 403, LimitReached: 403,
	Conflict: 409, OperationReused: 409, SelfApproval: 409, PeriodLocked: 409, LastOwner: 409,
	ValidationFailed: 422, RateLimited: 429, Unavailable: 503,
	OrgSetupIncomplete: 409,
}

type permissionFailure struct{ key string }

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
	httpapi.WriteError(w, r, statuses[code], string(code), "Organization request unavailable or refused", details)
}
