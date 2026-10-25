package domain

import (
	"fmt"
	"time"
)

type OrderState string

const (
	OrderQuoted   OrderState = "QUOTED"
	OrderPending  OrderState = "PENDING"
	OrderPaid     OrderState = "PAID"
	OrderExpired  OrderState = "EXPIRED"
	OrderCanceled OrderState = "CANCELED"
)

var orderAllowed = map[OrderState]map[OrderState]bool{
	OrderQuoted: {
		OrderPending:  true,
		OrderExpired:  true,
		OrderCanceled: true,
	},
	OrderPending: {
		OrderPaid:     true,
		OrderExpired:  true,
		OrderCanceled: true,
	},
	OrderExpired: {
		OrderPaid: true,
	},
	OrderPaid:     {},
	OrderCanceled: {},
}

func (s OrderState) Valid() bool {
	_, ok := orderAllowed[s]
	return ok
}

func canTransitionOrder(from, to OrderState) bool {
	return orderAllowed[from][to]
}

func transitionOrder(from, to OrderState) error {
	if !canTransitionOrder(from, to) {
		return fmt.Errorf("%w: order %s -> %s", ErrTransition, from, to)
	}
	return nil
}

type Order struct {
	ID             string
	AccountID      string
	OfferID        string
	Family         string
	CatalogVersion string
	Channel        Channel
	Term           Term
	Amount         Money
	Capabilities   []string
	State          OrderState

	QuotedAt     time.Time
	ExpiresAt    time.Time
	UpdatedAt    time.Time
	PaidAt       time.Time
	SettlementID string
}

func NewOrder(id, accountID string, quote Quote, now time.Time) (Order, error) {
	if !validText(id, MaxText) || !validText(accountID, MaxText) {
		return Order{}, fmt.Errorf("%w: order identity", ErrInvalid)
	}
	if now.IsZero() || quote.QuotedAt.IsZero() || !quote.ExpiresAt.After(now) {
		return Order{}, fmt.Errorf("%w: order time", ErrStale)
	}
	if !validText(quote.OfferID, MaxText) || !validText(quote.Family, MaxText) ||
		!validText(quote.CatalogVersion, MaxText) {
		return Order{}, fmt.Errorf("%w: order quote", ErrInvalid)
	}
	if err := quote.Channel.Validate(); err != nil {
		return Order{}, err
	}
	if err := quote.Term.Validate(); err != nil {
		return Order{}, err
	}
	if err := quote.Amount.Positive(); err != nil {
		return Order{}, err
	}
	return Order{
		ID:             id,
		AccountID:      accountID,
		OfferID:        quote.OfferID,
		Family:         quote.Family,
		CatalogVersion: quote.CatalogVersion,
		Channel:        quote.Channel,
		Term:           quote.Term,
		Amount:         quote.Amount,
		Capabilities:   copyStrings(quote.Capabilities),
		State:          OrderQuoted,
		QuotedAt:       quote.QuotedAt.UTC(),
		ExpiresAt:      quote.ExpiresAt.UTC(),
		UpdatedAt:      now.UTC(),
	}, nil
}

func (o *Order) MarkPending(now time.Time) error {
	if o == nil {
		return ErrInvalid
	}
	if now.IsZero() || now.Before(o.QuotedAt) {
		return fmt.Errorf("%w: order time", ErrInvalid)
	}
	if !now.Before(o.ExpiresAt) {
		return fmt.Errorf("%w: quote expired", ErrStale)
	}
	if err := transitionOrder(o.State, OrderPending); err != nil {
		return err
	}
	o.State = OrderPending
	o.UpdatedAt = now.UTC()
	return nil
}

func (o *Order) MarkPaid(settlementID string, now time.Time) error {
	if o == nil || !validText(settlementID, MaxText) || now.IsZero() || now.Before(o.QuotedAt) {
		return fmt.Errorf("%w: order payment", ErrInvalid)
	}
	if o.State == OrderPaid {
		if o.SettlementID == settlementID {
			return nil
		}
		return fmt.Errorf("%w: order already paid", ErrConflict)
	}
	if err := transitionOrder(o.State, OrderPaid); err != nil {
		return err
	}
	o.State = OrderPaid
	o.PaidAt = now.UTC()
	o.UpdatedAt = now.UTC()
	o.SettlementID = settlementID
	return nil
}

func (o *Order) Expire(now time.Time) error {
	if o == nil || now.IsZero() {
		return ErrInvalid
	}
	if !now.Before(o.ExpiresAt) {
		if err := transitionOrder(o.State, OrderExpired); err != nil {
			return err
		}
		o.State = OrderExpired
		o.UpdatedAt = now.UTC()
		return nil
	}
	return fmt.Errorf("%w: quote still live", ErrTransition)
}

func (o *Order) Cancel(now time.Time) error {
	if o == nil || now.IsZero() {
		return ErrInvalid
	}
	if err := transitionOrder(o.State, OrderCanceled); err != nil {
		return err
	}
	o.State = OrderCanceled
	o.UpdatedAt = now.UTC()
	return nil
}

func (o Order) Expired(now time.Time) bool {
	return !now.Before(o.ExpiresAt)
}

type AttemptState string

const (
	AttemptCreated     AttemptState = "CREATED"
	AttemptDispatched  AttemptState = "DISPATCHED"
	AttemptSucceeded   AttemptState = "SUCCEEDED"
	AttemptFailed      AttemptState = "FAILED"
	AttemptUnknown     AttemptState = "UNKNOWN"
	AttemptQuarantined AttemptState = "QUARANTINED"
	AttemptCanceled    AttemptState = "CANCELED"
)

var attemptAllowed = map[AttemptState]map[AttemptState]bool{
	AttemptCreated: {
		AttemptDispatched: true,
		AttemptCanceled:   true,
	},
	AttemptDispatched: {
		AttemptSucceeded: true,
		AttemptFailed:    true,
		AttemptUnknown:   true,
	},
	AttemptUnknown: {
		AttemptSucceeded:   true,
		AttemptFailed:      true,
		AttemptQuarantined: true,
	},
	AttemptSucceeded:   {},
	AttemptFailed:      {},
	AttemptQuarantined: {},
	AttemptCanceled:    {},
}

func (s AttemptState) Valid() bool {
	_, ok := attemptAllowed[s]
	return ok
}

func canTransitionAttempt(from, to AttemptState) bool {
	return attemptAllowed[from][to]
}

func transitionAttempt(from, to AttemptState) error {
	if !canTransitionAttempt(from, to) {
		return fmt.Errorf("%w: attempt %s -> %s", ErrTransition, from, to)
	}
	return nil
}

type Attempt struct {
	ID        string
	OrderID   string
	Provider  Provider
	Operation string
	State     AttemptState

	RemoteID     string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DispatchedAt time.Time
	ResolvedAt   time.Time
}

func NewAttempt(id, orderID string, provider Provider, operation string) (Attempt, error) {
	if !validText(id, MaxText) || !validText(orderID, MaxText) || !validText(operation, MaxText) {
		return Attempt{}, fmt.Errorf("%w: attempt identity", ErrInvalid)
	}
	if err := provider.Validate(); err != nil {
		return Attempt{}, err
	}
	return Attempt{
		ID:        id,
		OrderID:   orderID,
		Provider:  provider,
		Operation: operation,
		State:     AttemptCreated,
	}, nil
}

func (a *Attempt) Dispatch(now time.Time) error {
	if a == nil || now.IsZero() {
		return ErrInvalid
	}
	if err := transitionAttempt(a.State, AttemptDispatched); err != nil {
		return err
	}
	a.State = AttemptDispatched
	a.CreatedAt = now.UTC()
	a.UpdatedAt = now.UTC()
	a.DispatchedAt = now.UTC()
	return nil
}

func (a *Attempt) Succeed(remoteID string, now time.Time) error {
	if a == nil || !validText(remoteID, MaxText) || now.IsZero() {
		return fmt.Errorf("%w: attempt success", ErrInvalid)
	}
	if a.State == AttemptSucceeded {
		if a.RemoteID == remoteID {
			return nil
		}
		return fmt.Errorf("%w: attempt remote id", ErrConflict)
	}
	if err := transitionAttempt(a.State, AttemptSucceeded); err != nil {
		return err
	}
	a.State = AttemptSucceeded
	a.RemoteID = remoteID
	a.ResolvedAt = now.UTC()
	a.UpdatedAt = now.UTC()
	return nil
}

func (a *Attempt) Fail(now time.Time) error {
	if a == nil || now.IsZero() {
		return ErrInvalid
	}
	if a.State == AttemptFailed {
		return nil
	}
	if err := transitionAttempt(a.State, AttemptFailed); err != nil {
		return err
	}
	a.State = AttemptFailed
	a.ResolvedAt = now.UTC()
	a.UpdatedAt = now.UTC()
	return nil
}

func (a *Attempt) Unknown(remoteID string, now time.Time) error {
	if a == nil || now.IsZero() {
		return ErrInvalid
	}
	if remoteID != "" && !validText(remoteID, MaxText) {
		return fmt.Errorf("%w: attempt remote id", ErrInvalid)
	}
	if err := transitionAttempt(a.State, AttemptUnknown); err != nil {
		return err
	}
	a.State = AttemptUnknown
	a.RemoteID = remoteID
	a.UpdatedAt = now.UTC()
	return nil
}

func (a *Attempt) ResolveSuccess(remoteID string, now time.Time) error {
	if a == nil || !validText(remoteID, MaxText) || now.IsZero() {
		return fmt.Errorf("%w: attempt resolution", ErrInvalid)
	}
	if a.State != AttemptUnknown {
		return fmt.Errorf("%w: attempt not unknown", ErrTransition)
	}
	a.State = AttemptSucceeded
	a.RemoteID = remoteID
	a.ResolvedAt = now.UTC()
	a.UpdatedAt = now.UTC()
	return nil
}

func (a *Attempt) ResolveFailure(now time.Time) error {
	if a == nil || now.IsZero() {
		return ErrInvalid
	}
	if a.State != AttemptUnknown {
		return fmt.Errorf("%w: attempt not unknown", ErrTransition)
	}
	a.State = AttemptFailed
	a.ResolvedAt = now.UTC()
	a.UpdatedAt = now.UTC()
	return nil
}

func (a *Attempt) Quarantine(now time.Time) error {
	if a == nil || now.IsZero() {
		return ErrInvalid
	}
	if err := transitionAttempt(a.State, AttemptQuarantined); err != nil {
		return err
	}
	a.State = AttemptQuarantined
	a.ResolvedAt = now.UTC()
	a.UpdatedAt = now.UTC()
	return nil
}

func (a *Attempt) Cancel(now time.Time) error {
	if a == nil || now.IsZero() {
		return ErrInvalid
	}
	if err := transitionAttempt(a.State, AttemptCanceled); err != nil {
		return err
	}
	a.State = AttemptCanceled
	a.ResolvedAt = now.UTC()
	a.UpdatedAt = now.UTC()
	return nil
}
