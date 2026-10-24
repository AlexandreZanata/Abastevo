package domain

import (
	"fmt"
	"time"
)

type ContractState string

const (
	ContractPending  ContractState = "PENDING"
	ContractActive   ContractState = "ACTIVE"
	ContractPastDue  ContractState = "PAST_DUE"
	ContractCanceled ContractState = "CANCELED"
	ContractExpired  ContractState = "EXPIRED"
)

var contractAllowed = map[ContractState]map[ContractState]bool{
	ContractPending: {
		ContractActive:   true,
		ContractCanceled: true,
		ContractExpired:  true,
	},
	ContractActive: {
		ContractPastDue:  true,
		ContractCanceled: true,
		ContractExpired:  true,
	},
	ContractPastDue: {
		ContractActive:   true,
		ContractCanceled: true,
		ContractExpired:  true,
	},
	ContractCanceled: {},
	ContractExpired:  {},
}

func (s ContractState) Valid() bool {
	_, ok := contractAllowed[s]
	return ok
}

func canTransitionContract(from, to ContractState) bool {
	return contractAllowed[from][to]
}

type MandateState string

const (
	MandateCreated  MandateState = "CREATED"
	MandateActive   MandateState = "ACTIVE"
	MandateRefused  MandateState = "REFUSED"
	MandateCanceled MandateState = "CANCELED"
	MandateExpired  MandateState = "EXPIRED"
)

var mandateAllowed = map[MandateState]map[MandateState]bool{
	MandateCreated: {
		MandateActive:   true,
		MandateRefused:  true,
		MandateCanceled: true,
		MandateExpired:  true,
	},
	MandateActive: {
		MandateCanceled: true,
		MandateExpired:  true,
	},
	MandateRefused:  {},
	MandateCanceled: {},
	MandateExpired:  {},
}

func (s MandateState) Valid() bool {
	_, ok := mandateAllowed[s]
	return ok
}

func canTransitionMandate(from, to MandateState) bool {
	return mandateAllowed[from][to]
}

type Mandate struct {
	ID        string
	State     MandateState
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Contract struct {
	ID        string
	AccountID string
	OfferID   string
	Source    Channel
	State     ContractState
	Mandate   *Mandate
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewContract(id, accountID, offerID string, source Channel, mandateID string) (Contract, error) {
	if !validText(id, MaxText) || !validText(accountID, MaxText) || !validText(offerID, MaxText) {
		return Contract{}, fmt.Errorf("%w: contract identity", ErrInvalid)
	}
	if err := source.Validate(); err != nil {
		return Contract{}, err
	}
	if !source.Recurring() {
		return Contract{}, fmt.Errorf("%w: contract source", ErrChannelUnsupported)
	}
	contract := Contract{
		ID:        id,
		AccountID: accountID,
		OfferID:   offerID,
		Source:    source,
		State:     ContractPending,
	}
	switch source {
	case ChannelPixAutomatic:
		if !validText(mandateID, MaxText) {
			return Contract{}, fmt.Errorf("%w: mandate identity", ErrInvalid)
		}
		contract.Mandate = &Mandate{ID: mandateID, State: MandateCreated}
	case ChannelCard:
		if mandateID != "" {
			return Contract{}, fmt.Errorf("%w: card mandate", ErrInvalid)
		}
	default:
		return Contract{}, fmt.Errorf("%w: contract source", ErrChannelUnsupported)
	}
	return contract, nil
}

func (c *Contract) Activate(now time.Time) error {
	if c == nil || now.IsZero() {
		return ErrInvalid
	}
	if !canTransitionContract(c.State, ContractActive) {
		return fmt.Errorf("%w: contract %s -> %s", ErrTransition, c.State, ContractActive)
	}
	if c.Source == ChannelPixAutomatic {
		if c.Mandate == nil || c.Mandate.State != MandateActive {
			return fmt.Errorf("%w: mandate not active", ErrBinding)
		}
	}
	c.State = ContractActive
	c.UpdatedAt = now.UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now.UTC()
	}
	return nil
}

func (c *Contract) MarkPastDue(now time.Time) error {
	if c == nil || now.IsZero() {
		return ErrInvalid
	}
	if !canTransitionContract(c.State, ContractPastDue) {
		return fmt.Errorf("%w: contract %s -> %s", ErrTransition, c.State, ContractPastDue)
	}
	c.State = ContractPastDue
	c.UpdatedAt = now.UTC()
	return nil
}

func (c *Contract) Cancel(now time.Time) error {
	if c == nil || now.IsZero() {
		return ErrInvalid
	}
	if !canTransitionContract(c.State, ContractCanceled) {
		return fmt.Errorf("%w: contract %s -> %s", ErrTransition, c.State, ContractCanceled)
	}
	c.State = ContractCanceled
	c.UpdatedAt = now.UTC()
	if c.Source == ChannelPixAutomatic && c.Mandate != nil && (c.Mandate.State == MandateCreated || c.Mandate.State == MandateActive) {
		if err := c.Mandate.Cancel(now); err != nil {
			return err
		}
	}
	return nil
}

func (c *Contract) Expire(now time.Time) error {
	if c == nil || now.IsZero() {
		return ErrInvalid
	}
	if !canTransitionContract(c.State, ContractExpired) {
		return fmt.Errorf("%w: contract %s -> %s", ErrTransition, c.State, ContractExpired)
	}
	c.State = ContractExpired
	c.UpdatedAt = now.UTC()
	if c.Source == ChannelPixAutomatic && c.Mandate != nil && (c.Mandate.State == MandateCreated || c.Mandate.State == MandateActive) {
		if err := c.Mandate.Expire(now); err != nil {
			return err
		}
	}
	return nil
}

func (c Contract) Validate() error {
	if !validText(c.ID, MaxText) || !validText(c.AccountID, MaxText) || !validText(c.OfferID, MaxText) {
		return fmt.Errorf("%w: contract identity", ErrInvalid)
	}
	if err := c.Source.Validate(); err != nil {
		return err
	}
	if !c.Source.Recurring() || !c.State.Valid() {
		return fmt.Errorf("%w: contract state", ErrInvalid)
	}
	switch c.Source {
	case ChannelPixAutomatic:
		if c.Mandate == nil {
			return fmt.Errorf("%w: missing mandate", ErrInvalid)
		}
		if err := c.Mandate.Validate(); err != nil {
			return err
		}
		switch c.State {
		case ContractActive, ContractPastDue:
			if c.Mandate.State != MandateActive {
				return fmt.Errorf("%w: active contract mandate", ErrConflict)
			}
		case ContractCanceled, ContractExpired:
			if c.Mandate.State == MandateCreated || c.Mandate.State == MandateActive {
				return fmt.Errorf("%w: terminal contract mandate", ErrConflict)
			}
		}
	case ChannelCard:
		if c.Mandate != nil {
			return fmt.Errorf("%w: unexpected mandate", ErrInvalid)
		}
	}
	return nil
}

func (m *Mandate) Validate() error {
	if !validText(m.ID, MaxText) || !m.State.Valid() {
		return fmt.Errorf("%w: mandate", ErrInvalid)
	}
	return nil
}

func (m *Mandate) Activate(now time.Time) error {
	return m.transition(MandateActive, now)
}

func (m *Mandate) Refuse(now time.Time) error {
	return m.transition(MandateRefused, now)
}

func (m *Mandate) Cancel(now time.Time) error {
	return m.transition(MandateCanceled, now)
}

func (m *Mandate) Expire(now time.Time) error {
	return m.transition(MandateExpired, now)
}

func (m *Mandate) transition(to MandateState, now time.Time) error {
	if m == nil || now.IsZero() {
		return ErrInvalid
	}
	if m.State == to {
		return nil
	}
	if !canTransitionMandate(m.State, to) {
		return fmt.Errorf("%w: mandate %s -> %s", ErrTransition, m.State, to)
	}
	m.State = to
	m.UpdatedAt = now.UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now.UTC()
	}
	return nil
}

type CancellationState string

const (
	CancellationConfirmed     CancellationState = "CONFIRMED"
	CancellationPending       CancellationState = "PENDING"
	CancellationUnknown       CancellationState = "UNKNOWN"
	CancellationNotApplicable CancellationState = "NOT_APPLICABLE"
)

func (s CancellationState) Valid() bool {
	switch s {
	case CancellationConfirmed, CancellationPending, CancellationUnknown, CancellationNotApplicable:
		return true
	}
	return false
}

const (
	SwitchNextTerm        = "next-term"
	SwitchImmediate       = "immediate"
	SwitchOldCancellation = "old-cancellation-unknown"
)

type SwitchRequest struct {
	OldSource       Channel
	NewSource       Channel
	OldCancellation CancellationState
	CurrentEnd      time.Time
	RequestedAt     time.Time
}

type SwitchDecision struct {
	Allowed      bool
	FirstDebitAt time.Time
	Reason       string
}

func DecideSwitch(request SwitchRequest) (SwitchDecision, error) {
	if request.RequestedAt.IsZero() {
		return SwitchDecision{}, fmt.Errorf("%w: switch time", ErrInvalid)
	}
	if err := request.OldSource.Validate(); err != nil {
		return SwitchDecision{}, err
	}
	if err := request.NewSource.Validate(); err != nil {
		return SwitchDecision{}, err
	}
	if request.OldSource == request.NewSource {
		return SwitchDecision{}, fmt.Errorf("%w: same source", ErrConflict)
	}
	if !request.OldCancellation.Valid() {
		return SwitchDecision{}, fmt.Errorf("%w: cancellation state", ErrInvalid)
	}
	if !request.OldSource.Recurring() {
		if request.OldCancellation != CancellationNotApplicable {
			return SwitchDecision{}, fmt.Errorf("%w: manual source cancellation", ErrInvalid)
		}
	} else if request.OldCancellation == CancellationNotApplicable {
		return SwitchDecision{}, fmt.Errorf("%w: recurring source cancellation", ErrInvalid)
	}
	if request.OldSource.Recurring() && request.OldCancellation != CancellationConfirmed {
		return SwitchDecision{
			Allowed: false,
			Reason:  SwitchOldCancellation,
		}, nil
	}
	first := request.RequestedAt.UTC()
	reason := SwitchImmediate
	if request.CurrentEnd.After(request.RequestedAt) {
		first = request.CurrentEnd.UTC()
		reason = SwitchNextTerm
	}
	return SwitchDecision{Allowed: true, FirstDebitAt: first, Reason: reason}, nil
}

func ValidateFundingSources(contracts []Contract) error {
	active := make(map[string]bool)
	for _, contract := range contracts {
		if err := contract.Validate(); err != nil {
			return err
		}
		if contract.State != ContractActive && contract.State != ContractPastDue {
			continue
		}
		key := contract.AccountID + "\x00" + contract.OfferID
		if active[key] {
			return fmt.Errorf("%w: multiple recurring sources", ErrConflict)
		}
		active[key] = true
	}
	return nil
}
