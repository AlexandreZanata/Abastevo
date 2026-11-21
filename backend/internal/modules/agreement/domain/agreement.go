// Package domain holds the pure Phase F7 agreement rules: the bilateral
// station agreement state machine, its negotiated price terms and the
// bounded single-use fueling authorizations (EMP-AGREE-001..006,
// BR-AGREE-001/002). State names follow the station-commerce state catalog.
package domain

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

var (
	ErrTransitionRefused = errors.New("agreement: transition refused")
	ErrTermsRequired     = errors.New("agreement: negotiated price terms are required")
	ErrTermDuplicate     = errors.New("agreement: duplicate negotiated price term")
	ErrWindowInvalid     = errors.New("agreement: validity window is invalid")
	ErrLabelInvalid      = errors.New("agreement: organization label is invalid")
	ErrConditionsInvalid = errors.New("agreement: conditions are invalid")
	ErrProductInvalid    = errors.New("agreement: product or unit is invalid")
	ErrPriceInvalid      = errors.New("agreement: negotiated price is invalid")
)

// State is the stored agreement state (state-model.md).
type State string

const (
	Proposed  State = "PROPOSED"
	Active    State = "ACTIVE"
	Suspended State = "SUSPENDED"
	Ended     State = "ENDED"
	Declined  State = "DECLINED"
	// Expired is derived at read time; it is never stored.
	Expired State = "EXPIRED"
)

type private struct{}

func (private) String() string               { return "AgreementValue(redacted)" }
func (private) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, "AgreementValue(redacted)") }
func (private) MarshalJSON() ([]byte, error) { return []byte(`"AgreementValue(redacted)"`), nil }
func (private) LogValue() slog.Value         { return slog.StringValue("AgreementValue(redacted)") }

// Product is one closed commercial fuel product (the official vocabulary).
var products = map[string]bool{
	"ETHANOL": true, "GASOLINE_REGULAR": true, "GASOLINE_ADDITIVED": true,
	"DIESEL_S500": true, "DIESEL_S10": true, "CNG": true, "LPG_P13": true,
}

var units = map[string]bool{"L": true, "M3": true, "KG_13": true}

// ValidProduct reports whether the closed commercial vocabulary admits the pair.
func ValidProduct(product, unit string) bool {
	return products[product] && units[unit]
}

// Term is one negotiated price entry of an agreement (EMP-AGREE-003).
type Term struct {
	private
	Product       string
	Unit          string
	PriceMilliBRL int64
	Conditions    string
}

// Validate enforces the exact product, unit and price bounds.
func (t Term) Validate() error {
	if !ValidProduct(t.Product, t.Unit) {
		return ErrProductInvalid
	}
	if t.PriceMilliBRL < 1 || t.PriceMilliBRL > 1000000 {
		return ErrPriceInvalid
	}
	if len([]rune(strings.TrimSpace(t.Conditions))) > 120 {
		return ErrConditionsInvalid
	}
	return nil
}

// Agreement is the pure agreement head. Only the recorded transitions move
// it; every transition is a command with an audited revision.
type Agreement struct {
	private
	state    State
	revision int64
	from     time.Time
	until    time.Time
	terms    []Term
}

// Propose validates the proposed terms and window and opens the PROPOSED
// head (UC-AGREE-01). An agreement with no terms shares nothing and is
// refused; duplicate product and unit pairs are refused.
func Propose(from, until time.Time, terms []Term) (Agreement, error) {
	if from.IsZero() || until.IsZero() || !until.After(from) {
		return Agreement{}, ErrWindowInvalid
	}
	if len(terms) == 0 {
		return Agreement{}, ErrTermsRequired
	}
	seen := map[string]bool{}
	for _, term := range terms {
		if err := term.Validate(); err != nil {
			return Agreement{}, err
		}
		key := term.Product + "/" + term.Unit
		if seen[key] {
			return Agreement{}, ErrTermDuplicate
		}
		seen[key] = true
	}
	return Agreement{state: Proposed, revision: 1, from: from, until: until, terms: append([]Term(nil), terms...)}, nil
}

func (a Agreement) State() State          { return a.state }
func (a Agreement) Revision() int64       { return a.revision }
func (a Agreement) Terms() []Term         { return append([]Term(nil), a.terms...) }
func (a Agreement) ValidFrom() time.Time  { return a.from }
func (a Agreement) ValidUntil() time.Time { return a.until }

func (a Agreement) move(now time.Time, to State) (Agreement, error) {
	allowed := false
	switch a.state {
	case Proposed:
		allowed = to == Active || to == Declined || to == Ended
	case Active:
		allowed = to == Suspended || to == Ended
	case Suspended:
		allowed = to == Ended
	}
	if !allowed {
		return Agreement{}, ErrTransitionRefused
	}
	if now.IsZero() {
		return Agreement{}, ErrTransitionRefused
	}
	return Agreement{state: to, revision: a.revision + 1, from: a.from, until: a.until, terms: a.terms}, nil
}

// Accept is the station party's explicit acceptance.
func (a Agreement) Accept(now time.Time) (Agreement, error) { return a.move(now, Active) }

// Decline is the station party's explicit decline of a proposal.
func (a Agreement) Decline(now time.Time) (Agreement, error) { return a.move(now, Declined) }

// Suspend stops new eligible transactions under the recorded policy.
func (a Agreement) Suspend(now time.Time) (Agreement, error) { return a.move(now, Suspended) }

// End closes a proposed, active or suspended agreement; history stays.
func (a Agreement) End(now time.Time) (Agreement, error) { return a.move(now, Ended) }

// Usable reports whether an agreement admits new eligible transactions at
// the given instant: only an ACTIVE agreement inside its validity.
func (a Agreement) Usable(now time.Time) bool {
	return a.state == Active && !now.Before(a.from) && !now.After(a.until)
}

// DerivedState is the read-time state: an ACTIVE agreement past its
// validity reads EXPIRED; every stored terminal state stays as recorded.
func DerivedState(state State, now, from, until time.Time) State {
	if state == Active && !until.IsZero() && now.After(until) {
		return Expired
	}
	return state
}

// AgreementOption rebuilds one stored agreement head (the adapters' read
// path); it never mutates the recorded transitions.
type AgreementOption func(*Agreement)

// AgreementOf is the stored-head constructor.
func AgreementOf(state State, opts ...AgreementOption) Agreement {
	a := Agreement{state: state, revision: 1}
	for _, opt := range opts {
		opt(&a)
	}
	return a
}

// WithWindow sets the recorded validity window.
func WithWindow(from, until time.Time) AgreementOption {
	return func(a *Agreement) { a.from, a.until = from, until }
}

// WithRevision sets the stored revision.
func WithRevision(revision int64) AgreementOption {
	return func(a *Agreement) { a.revision = revision }
}

// WithTerms sets the stored negotiated terms.
func WithTerms(terms ...Term) AgreementOption {
	return func(a *Agreement) { a.terms = append([]Term(nil), terms...) }
}

// ErrorCode is the frozen golden-vector code of a domain refusal; the
// adapters map these codes to the bounded public envelopes.
func ErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrTermsRequired):
		return "AGREEMENT_TERMS_REQUIRED"
	case errors.Is(err, ErrTermDuplicate):
		return "AGREEMENT_TERM_DUPLICATE"
	case errors.Is(err, ErrWindowInvalid):
		return "AGREEMENT_WINDOW_INVALID"
	case errors.Is(err, ErrLabelInvalid):
		return "AGREEMENT_LABEL_INVALID"
	case errors.Is(err, ErrConditionsInvalid):
		return "AGREEMENT_CONDITIONS_INVALID"
	case errors.Is(err, ErrProductInvalid):
		return "AGREEMENT_PRODUCT_INVALID"
	case errors.Is(err, ErrPriceInvalid):
		return "AGREEMENT_PRICE_INVALID"
	case errors.Is(err, ErrTransitionRefused):
		return "AGREEMENT_TRANSITION_REFUSED"
	case errors.Is(err, ErrAuthorizationExpiryInvalid):
		return "AUTHORIZATION_EXPIRY_INVALID"
	case errors.Is(err, ErrAuthorizationLimitsInvalid):
		return "AUTHORIZATION_LIMITS_INVALID"
	case errors.Is(err, ErrAuthorizationProductInvalid):
		return "AUTHORIZATION_PRODUCT_INVALID"
	case errors.Is(err, ErrAuthorizationTransitionRefused):
		return "AUTHORIZATION_TRANSITION_REFUSED"
	case errors.Is(err, ErrAuthorizationExpired):
		return "AUTHORIZATION_EXPIRED"
	case errors.Is(err, ErrAuthorizationProductMismatch):
		return "AUTHORIZATION_PRODUCT_MISMATCH"
	case errors.Is(err, ErrAuthorizationQuantityExceeded):
		return "AUTHORIZATION_QUANTITY_EXCEEDED"
	case errors.Is(err, ErrAuthorizationTotalExceeded):
		return "AUTHORIZATION_TOTAL_EXCEEDED"
	case errors.Is(err, ErrAuthorizationLocked):
		return "AUTHORIZATION_LOCKED"
	}
	return ""
}
