package domain

import (
	"fmt"
	"sort"
	"time"
)

type SettlementAuthority string

const (
	AuthorityProviderVerified SettlementAuthority = "provider_verified"
	AuthorityRedirect         SettlementAuthority = "redirect"
	AuthorityAuthorization    SettlementAuthority = "authorization"
	AuthorityManual           SettlementAuthority = "manual"
	AuthorityUnknown          SettlementAuthority = "unknown"
)

func (a SettlementAuthority) Valid() bool {
	switch a {
	case AuthorityProviderVerified, AuthorityRedirect, AuthorityAuthorization, AuthorityManual, AuthorityUnknown:
		return true
	}
	return false
}

type SettlementStatus string

const (
	SettlementPending SettlementStatus = "pending"
	SettlementSettled SettlementStatus = "settled"
	SettlementUnknown SettlementStatus = "unknown"
)

func (s SettlementStatus) Valid() bool {
	switch s {
	case SettlementPending, SettlementSettled, SettlementUnknown:
		return true
	}
	return false
}

// Settlement is authoritative provider evidence. It is only a candidate until
// authority, binding and amount checks pass.
type Settlement struct {
	ID              string
	OrderID         string
	AttemptID       string
	AccountID       string
	OfferID         string
	Channel         Channel
	Provider        Provider
	Environment     Environment
	MerchantID      string
	RemotePaymentID string
	RemoteChargeID  string
	ProviderStatus  string
	Amount          Money
	Status          SettlementStatus
	Authority       SettlementAuthority
	SettledAt       time.Time
}

func (s Settlement) Validate() error {
	if !validText(s.ID, MaxText) || !validText(s.OrderID, MaxText) ||
		!validText(s.AttemptID, MaxText) || !validText(s.AccountID, MaxText) ||
		!validText(s.OfferID, MaxText) || !validText(s.MerchantID, MaxText) ||
		!validText(s.RemotePaymentID, MaxText) || !validText(s.ProviderStatus, MaxText) {
		return fmt.Errorf("%w: settlement identity", ErrInvalid)
	}
	if err := s.Channel.Validate(); err != nil {
		return err
	}
	if err := s.Provider.Validate(); err != nil {
		return err
	}
	if s.Channel.Provider() != s.Provider {
		return fmt.Errorf("%w: settlement provider", ErrBinding)
	}
	if err := s.Environment.Validate(); err != nil {
		return err
	}
	if err := s.Amount.Positive(); err != nil {
		return err
	}
	if !s.Status.Valid() || !s.Authority.Valid() {
		return fmt.Errorf("%w: settlement state", ErrInvalid)
	}
	if s.SettledAt.IsZero() {
		return fmt.Errorf("%w: settlement time", ErrInvalid)
	}
	return nil
}

const (
	PeriodInitial = "initial"
	PeriodEarly   = "early-renewal"
	PeriodLate    = "late-renewal-reset-anchor"
)

type Period struct {
	Start     time.Time
	End       time.Time
	AnchorDay int
	Reason    string
}

// NextPeriod computes the next paid period from authoritative settlement time,
// current paid-through and the contract anchor.
func NextPeriod(settledAt, currentEnd time.Time, anchorDay int) (Period, error) {
	if settledAt.IsZero() {
		return Period{}, fmt.Errorf("%w: settlement time", ErrInvalid)
	}
	location, err := time.LoadLocation(BillingTimezone)
	if err != nil {
		return Period{}, fmt.Errorf("%w: billing timezone", ErrUnavailable)
	}
	start := settledAt.UTC()
	anchor := settledAt.In(location).Day()
	reason := PeriodInitial
	if !currentEnd.IsZero() {
		if anchorDay < 1 || anchorDay > 31 {
			return Period{}, fmt.Errorf("%w: period anchor", ErrInvalid)
		}
		if !settledAt.After(currentEnd) {
			start = currentEnd.UTC()
			anchor = anchorDay
			reason = PeriodEarly
		} else {
			reason = PeriodLate
		}
	}
	end := addMonth(start, anchor, location)
	if !end.After(start) {
		return Period{}, fmt.Errorf("%w: period duration", ErrInvalid)
	}
	return Period{Start: start, End: end, AnchorDay: anchor, Reason: reason}, nil
}

func addMonth(start time.Time, anchor int, location *time.Location) time.Time {
	local := start.In(location)
	year, month := local.Year(), local.Month()
	if month == time.December {
		year++
		month = time.January
	} else {
		month++
	}
	day := anchor
	if last := daysInMonth(year, month); day > last {
		day = last
	}
	return time.Date(year, month, day, local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), location).UTC()
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

type GrantState string

const (
	GrantActive  GrantState = "ACTIVE"
	GrantRevoked GrantState = "REVOKED"
)

func (s GrantState) Valid() bool {
	return s == GrantActive || s == GrantRevoked
}

type RefundState string

const (
	RefundRequested RefundState = "REQUESTED"
	RefundPending   RefundState = "PENDING"
	RefundSucceeded RefundState = "SUCCEEDED"
	RefundFailed    RefundState = "FAILED"
	RefundDenied    RefundState = "DENIED"
)

var refundAllowed = map[RefundState]map[RefundState]bool{
	RefundRequested: {
		RefundPending:   true,
		RefundSucceeded: true,
		RefundFailed:    true,
		RefundDenied:    true,
	},
	RefundPending: {
		RefundSucceeded: true,
		RefundFailed:    true,
	},
	RefundSucceeded: {},
	RefundFailed:    {},
	RefundDenied:    {},
}

func (s RefundState) Valid() bool {
	_, ok := refundAllowed[s]
	return ok
}

func canTransitionRefund(from, to RefundState) bool {
	return refundAllowed[from][to]
}

// RefundTransitionAllowed exposes the frozen refund state machine for
// persistence and application layers.
func RefundTransitionAllowed(from, to RefundState) bool {
	return canTransitionRefund(from, to)
}

type Refund struct {
	ID          string
	GrantID     string
	Amount      Money
	State       RefundState
	RequestedAt time.Time
	UpdatedAt   time.Time
	ResolvedAt  time.Time
}

type DisputeState string

const (
	DisputePending     DisputeState = "PENDING"
	DisputeUnderReview DisputeState = "UNDER_REVIEW"
	DisputeWon         DisputeState = "WON"
	DisputeLost        DisputeState = "LOST"
	DisputeClosed      DisputeState = "CLOSED"
)

var disputeAllowed = map[DisputeState]map[DisputeState]bool{
	DisputePending: {
		DisputeUnderReview: true,
		DisputeWon:         true,
		DisputeLost:        true,
		DisputeClosed:      true,
	},
	DisputeUnderReview: {
		DisputeWon:    true,
		DisputeLost:   true,
		DisputeClosed: true,
	},
	DisputeWon:    {},
	DisputeLost:   {},
	DisputeClosed: {},
}

func (s DisputeState) Valid() bool {
	_, ok := disputeAllowed[s]
	return ok
}

func canTransitionDispute(from, to DisputeState) bool {
	return disputeAllowed[from][to]
}

// DisputeTransitionAllowed exposes the frozen dispute state machine for
// persistence and application layers.
func DisputeTransitionAllowed(from, to DisputeState) bool {
	return canTransitionDispute(from, to)
}

type Dispute struct {
	ID         string
	GrantID    string
	Amount     Money
	State      DisputeState
	OpenedAt   time.Time
	UpdatedAt  time.Time
	ResolvedAt time.Time
}

type Grant struct {
	ID              string
	OrderID         string
	SettlementID    string
	RemotePaymentID string
	Source          Channel
	Paid            Money
	Start           time.Time
	End             time.Time
	AnchorDay       int
	Capabilities    []string
	State           GrantState
	PolicyVersion   int
	Refunds         []Refund
	Dispute         *Dispute
}

func (g Grant) ActiveAt(now time.Time) bool {
	return g.State == GrantActive && !now.Before(g.Start) && now.Before(g.End)
}

func (g Grant) Refunded() Money {
	total := BRL(0)
	for _, refund := range g.Refunds {
		if refund.State != RefundSucceeded {
			continue
		}
		next, err := total.Add(refund.Amount)
		if err != nil {
			return BRL(0)
		}
		total = next
	}
	return total
}

func (g Grant) PendingRefund() Money {
	total := BRL(0)
	for _, refund := range g.Refunds {
		if refund.State != RefundRequested && refund.State != RefundPending {
			continue
		}
		next, err := total.Add(refund.Amount)
		if err != nil {
			return BRL(0)
		}
		total = next
	}
	return total
}

func (g Grant) Refundable() Money {
	refunded, err := g.Paid.Sub(g.Refunded())
	if err != nil {
		return BRL(0)
	}
	pending, err := refunded.Sub(g.PendingRefund())
	if err != nil {
		return BRL(0)
	}
	return pending
}

func (g Grant) clone() Grant {
	g.Capabilities = copyStrings(g.Capabilities)
	g.Refunds = append([]Refund(nil), g.Refunds...)
	if g.Dispute != nil {
		dispute := *g.Dispute
		g.Dispute = &dispute
	}
	return g
}

func (g *Grant) revoke() {
	g.State = GrantRevoked
}

type Entitlement struct {
	AccountID     string
	OfferID       string
	PolicyVersion int
	Grants        []Grant
	Excess        []Settlement
}

func NewEntitlement(accountID, offerID string) (Entitlement, error) {
	if !validText(accountID, MaxText) || !validText(offerID, MaxText) {
		return Entitlement{}, fmt.Errorf("%w: entitlement identity", ErrInvalid)
	}
	return Entitlement{AccountID: accountID, OfferID: offerID, PolicyVersion: PolicyVersion}, nil
}

type SettlementResult struct {
	Grant  *Grant
	Excess bool
	Reason string
}

const (
	ResultGranted        = "granted"
	ResultIdempotent     = "idempotent"
	ResultOrderPaid      = "order-already-paid"
	ResultOrderCanceled  = "order-canceled"
	ResultFutureExists   = "future-period-exists"
	ResultExcessRepeated = "excess-repeated"
)

func (e *Entitlement) ApplySettlement(offer Offer, order *Order, attempt *Attempt, settlement Settlement, now time.Time) (SettlementResult, error) {
	if e == nil || order == nil || attempt == nil || now.IsZero() {
		return SettlementResult{}, ErrInvalid
	}
	if err := e.Validate(); err != nil {
		return SettlementResult{}, err
	}
	if err := offer.Validate(); err != nil {
		return SettlementResult{}, err
	}
	if err := settlement.Validate(); err != nil {
		return SettlementResult{}, err
	}
	if settlement.SettledAt.After(now) {
		return SettlementResult{}, fmt.Errorf("%w: future settlement", ErrInvalid)
	}
	if e.AccountID != settlement.AccountID || e.OfferID != settlement.OfferID ||
		offer.ID != e.OfferID || order.OfferID != e.OfferID {
		return SettlementResult{}, fmt.Errorf("%w: entitlement offer/account", ErrBinding)
	}
	if order.AccountID != e.AccountID || order.ID != settlement.OrderID ||
		order.Channel != settlement.Channel || !order.Amount.Equal(settlement.Amount) ||
		!order.Amount.Equal(offer.Price) || !sameStringSet(order.Capabilities, offer.Capabilities) {
		return SettlementResult{}, fmt.Errorf("%w: order settlement", ErrBinding)
	}
	if settlement.SettledAt.Before(order.QuotedAt) {
		return SettlementResult{}, fmt.Errorf("%w: settlement before quote", ErrBinding)
	}
	if attempt.ID != settlement.AttemptID || attempt.OrderID != order.ID ||
		attempt.Provider != settlement.Provider || attempt.State != AttemptSucceeded {
		return SettlementResult{}, fmt.Errorf("%w: attempt settlement", ErrBinding)
	}
	if attempt.DispatchedAt.IsZero() || settlement.SettledAt.Before(attempt.DispatchedAt) {
		return SettlementResult{}, fmt.Errorf("%w: settlement before dispatch", ErrBinding)
	}
	if settlement.Status != SettlementSettled || settlement.Authority != AuthorityProviderVerified {
		return SettlementResult{}, fmt.Errorf("%w: settlement is not verified", ErrBinding)
	}

	if existing, ok := e.grantBySettlement(settlement.ID); ok {
		if existing.OrderID != settlement.OrderID || existing.RemotePaymentID != settlement.RemotePaymentID ||
			!existing.Paid.Equal(settlement.Amount) {
			return SettlementResult{}, fmt.Errorf("%w: settlement replay", ErrConflict)
		}
		copy := existing.clone()
		return SettlementResult{Grant: &copy, Reason: ResultIdempotent}, nil
	}
	if _, ok := e.excessBySettlement(settlement.ID); ok {
		return SettlementResult{Excess: true, Reason: ResultExcessRepeated}, nil
	}
	if existing, ok := e.grantByRemote(settlement.RemotePaymentID); ok {
		if existing.SettlementID != settlement.ID {
			return SettlementResult{}, fmt.Errorf("%w: remote payment already bound", ErrConflict)
		}
		copy := existing.clone()
		return SettlementResult{Grant: &copy, Reason: ResultIdempotent}, nil
	}
	for _, excess := range e.Excess {
		if excess.RemotePaymentID == settlement.RemotePaymentID && excess.ID != settlement.ID {
			return SettlementResult{}, fmt.Errorf("%w: remote payment already recorded", ErrConflict)
		}
	}

	switch order.State {
	case OrderPaid:
		if order.SettlementID == settlement.ID {
			return SettlementResult{}, fmt.Errorf("%w: paid order has no grant", ErrConflict)
		}
		e.recordExcess(settlement)
		return SettlementResult{Excess: true, Reason: ResultOrderPaid}, nil
	case OrderCanceled:
		e.recordExcess(settlement)
		return SettlementResult{Excess: true, Reason: ResultOrderCanceled}, nil
	case OrderQuoted:
		return SettlementResult{}, fmt.Errorf("%w: settlement without pending order", ErrTransition)
	case OrderPending, OrderExpired:
	default:
		return SettlementResult{}, fmt.Errorf("%w: order state", ErrTransition)
	}

	if e.futureGrantCount(now) > 0 {
		if err := order.MarkPaid(settlement.ID, now); err != nil {
			return SettlementResult{}, err
		}
		e.recordExcess(settlement)
		return SettlementResult{Excess: true, Reason: ResultFutureExists}, nil
	}

	paidThrough := e.PaidThrough()
	anchor := 0
	if !paidThrough.IsZero() {
		if last, ok := e.grantEndingAt(paidThrough); ok {
			anchor = last.AnchorDay
		}
	}
	period, err := NextPeriod(settlement.SettledAt, paidThrough, anchor)
	if err != nil {
		return SettlementResult{}, err
	}
	grant := Grant{
		ID:              settlement.ID,
		OrderID:         order.ID,
		SettlementID:    settlement.ID,
		RemotePaymentID: settlement.RemotePaymentID,
		Source:          settlement.Channel,
		Paid:            settlement.Amount,
		Start:           period.Start,
		End:             period.End,
		AnchorDay:       period.AnchorDay,
		Capabilities:    copyStrings(order.Capabilities),
		State:           GrantActive,
		PolicyVersion:   PolicyVersion,
	}
	e.Grants = append(e.Grants, grant)
	if err := order.MarkPaid(settlement.ID, now); err != nil {
		e.Grants = e.Grants[:len(e.Grants)-1]
		return SettlementResult{}, err
	}
	copy := grant.clone()
	return SettlementResult{Grant: &copy, Reason: ResultGranted}, nil
}

func (e *Entitlement) recordExcess(settlement Settlement) {
	if _, ok := e.excessBySettlement(settlement.ID); ok {
		return
	}
	e.Excess = append(e.Excess, settlement)
}

func (e Entitlement) Validate() error {
	if !validText(e.AccountID, MaxText) || !validText(e.OfferID, MaxText) || e.PolicyVersion != PolicyVersion {
		return fmt.Errorf("%w: entitlement identity", ErrInvalid)
	}
	grants := make([]Grant, len(e.Grants))
	copy(grants, e.Grants)
	sort.Slice(grants, func(i, j int) bool { return grants[i].Start.Before(grants[j].Start) })
	grantIDs := make(map[string]bool, len(grants))
	settlementIDs := make(map[string]bool, len(grants))
	orderIDs := make(map[string]bool, len(grants))
	remoteIDs := make(map[string]bool, len(grants))
	refundIDs := make(map[string]bool)
	for _, grant := range grants {
		if !validText(grant.ID, MaxText) || !validText(grant.OrderID, MaxText) ||
			!validText(grant.SettlementID, MaxText) || !validText(grant.RemotePaymentID, MaxText) {
			return fmt.Errorf("%w: grant identity", ErrInvalid)
		}
		if grantIDs[grant.ID] || settlementIDs[grant.SettlementID] || orderIDs[grant.OrderID] ||
			remoteIDs[grant.RemotePaymentID] {
			return fmt.Errorf("%w: duplicate grant identity", ErrConflict)
		}
		grantIDs[grant.ID] = true
		settlementIDs[grant.SettlementID] = true
		orderIDs[grant.OrderID] = true
		remoteIDs[grant.RemotePaymentID] = true
		if grant.Source != ChannelCard && grant.Source != ChannelPixManual && grant.Source != ChannelPixAutomatic {
			return fmt.Errorf("%w: grant source", ErrInvalid)
		}
		if err := grant.Paid.Positive(); err != nil {
			return err
		}
		if !grant.Start.Before(grant.End) || grant.AnchorDay < 1 || grant.AnchorDay > 31 {
			return fmt.Errorf("%w: grant period", ErrInvalid)
		}
		if !grant.State.Valid() || grant.PolicyVersion != PolicyVersion || len(grant.Capabilities) == 0 {
			return fmt.Errorf("%w: grant state", ErrInvalid)
		}
		refunded := BRL(0)
		pending := BRL(0)
		for _, refund := range grant.Refunds {
			if !validText(refund.ID, MaxText) || !validText(refund.GrantID, MaxText) ||
				refund.GrantID != grant.ID || !refund.State.Valid() || refund.RequestedAt.IsZero() {
				return fmt.Errorf("%w: refund", ErrInvalid)
			}
			if refundIDs[refund.ID] {
				return fmt.Errorf("%w: duplicate refund", ErrConflict)
			}
			refundIDs[refund.ID] = true
			if err := refund.Amount.Positive(); err != nil {
				return err
			}
			switch refund.State {
			case RefundSucceeded:
				next, err := refunded.Add(refund.Amount)
				if err != nil {
					return err
				}
				refunded = next
			case RefundRequested, RefundPending:
				next, err := pending.Add(refund.Amount)
				if err != nil {
					return err
				}
				pending = next
			}
		}
		total, err := refunded.Add(pending)
		if err != nil || total.Centavos > grant.Paid.Centavos {
			return fmt.Errorf("%w: refund total", ErrRefundBounds)
		}
		if grant.Dispute != nil {
			dispute := grant.Dispute
			if !validText(dispute.ID, MaxText) || dispute.GrantID != grant.ID ||
				!dispute.State.Valid() || dispute.OpenedAt.IsZero() {
				return fmt.Errorf("%w: dispute", ErrInvalid)
			}
			if err := dispute.Amount.Positive(); err != nil || dispute.Amount.Centavos > grant.Paid.Centavos {
				return fmt.Errorf("%w: dispute amount", ErrInvalid)
			}
		}
	}
	for i := 0; i < len(grants); i++ {
		if grants[i].State != GrantActive {
			continue
		}
		for j := i + 1; j < len(grants); j++ {
			if grants[j].State != GrantActive {
				continue
			}
			if grants[i].Start.Before(grants[j].End) && grants[j].Start.Before(grants[i].End) {
				return fmt.Errorf("%w: overlapping active grants", ErrConflict)
			}
		}
	}
	excessIDs := make(map[string]bool, len(e.Excess))
	excessRemoteIDs := make(map[string]bool, len(e.Excess))
	for _, settlement := range e.Excess {
		if err := settlement.Validate(); err != nil {
			return err
		}
		if settlementIDs[settlement.ID] || excessIDs[settlement.ID] || remoteIDs[settlement.RemotePaymentID] ||
			excessRemoteIDs[settlement.RemotePaymentID] {
			return fmt.Errorf("%w: duplicate excess settlement", ErrConflict)
		}
		excessIDs[settlement.ID] = true
		excessRemoteIDs[settlement.RemotePaymentID] = true
	}
	return nil
}

func (e Entitlement) PaidThrough() time.Time {
	var latest time.Time
	for _, grant := range e.Grants {
		if grant.State == GrantActive && grant.End.After(latest) {
			latest = grant.End
		}
	}
	return latest.UTC()
}

func (e Entitlement) futureGrantCount(now time.Time) int {
	var currentEnd time.Time
	for _, grant := range e.Grants {
		if grant.ActiveAt(now) {
			if grant.End.After(currentEnd) {
				currentEnd = grant.End
			}
		}
	}
	count := 0
	for _, grant := range e.Grants {
		if grant.State != GrantActive {
			continue
		}
		if currentEnd.IsZero() {
			if grant.Start.After(now) {
				count++
			}
		} else if !grant.Start.Before(currentEnd) {
			count++
		}
	}
	return count
}

func (e Entitlement) grantBySettlement(id string) (Grant, bool) {
	for _, grant := range e.Grants {
		if grant.SettlementID == id {
			return grant, true
		}
	}
	return Grant{}, false
}

func (e Entitlement) grantByRemote(id string) (Grant, bool) {
	for _, grant := range e.Grants {
		if grant.RemotePaymentID == id {
			return grant, true
		}
	}
	return Grant{}, false
}

func (e Entitlement) excessBySettlement(id string) (Settlement, bool) {
	for _, settlement := range e.Excess {
		if settlement.ID == id {
			return settlement, true
		}
	}
	return Settlement{}, false
}

func (e Entitlement) grantEndingAt(end time.Time) (Grant, bool) {
	for _, grant := range e.Grants {
		if grant.State == GrantActive && grant.End.Equal(end) {
			return grant, true
		}
	}
	return Grant{}, false
}

func (e Entitlement) grantByID(id string) *Grant {
	for i := range e.Grants {
		if e.Grants[i].ID == id {
			return &e.Grants[i]
		}
	}
	return nil
}

type AccessState string

const (
	AccessNone    AccessState = "NONE"
	AccessActive  AccessState = "ACTIVE"
	AccessExpired AccessState = "EXPIRED"
	AccessRevoked AccessState = "REVOKED"
	AccessFuture  AccessState = "FUTURE"
)

const (
	ReasonActive      = "paid-through"
	ReasonInactive    = "account-inactive"
	ReasonNoPurchase  = "no-purchase"
	ReasonExpired     = "expired-zero-grace"
	ReasonRevoked     = "grant-revoked"
	ReasonStartsLater = "starts-later"
	ReasonInvalidTime = "invalid-time"
)

type Access struct {
	PolicyVersion      int
	State              AccessState
	Reason             string
	Source             Channel
	Capabilities       []string
	ValidUntil         *time.Time
	PaidThrough        *time.Time
	RetainedDataAccess bool
}

func (a Access) Granted() bool { return a.State == AccessActive }

func (e Entitlement) Evaluate(now time.Time, live bool) Access {
	result := Access{
		PolicyVersion:      PolicyVersion,
		State:              AccessNone,
		Reason:             ReasonNoPurchase,
		Capabilities:       []string{},
		RetainedDataAccess: true,
	}
	if now.IsZero() {
		result.Reason = ReasonInvalidTime
		return result
	}
	paidThrough := e.PaidThrough()
	if !paidThrough.IsZero() {
		until := paidThrough
		result.PaidThrough = &until
	}
	if !live {
		result.Reason = ReasonInactive
		return result
	}
	var active *Grant
	var revoked *Grant
	var future *Grant
	for i := range e.Grants {
		grant := &e.Grants[i]
		if grant.ActiveAt(now) {
			active = grant
			break
		}
		if grant.State == GrantRevoked && !now.Before(grant.Start) && now.Before(grant.End) {
			copy := grant
			revoked = copy
		}
		if grant.State == GrantActive && grant.Start.After(now) {
			if future == nil || grant.Start.Before(future.Start) {
				copy := grant
				future = copy
			}
		}
	}
	if active != nil {
		until := active.End.UTC()
		result.State = AccessActive
		result.Reason = ReasonActive
		result.Source = active.Source
		result.Capabilities = copyStrings(active.Capabilities)
		result.ValidUntil = &until
		return result
	}
	if revoked != nil {
		result.State = AccessRevoked
		result.Reason = ReasonRevoked
		result.Source = revoked.Source
		return result
	}
	if future != nil {
		result.State = AccessFuture
		result.Reason = ReasonStartsLater
		result.Source = future.Source
		return result
	}
	if !paidThrough.IsZero() && !now.Before(paidThrough) {
		result.State = AccessExpired
		result.Reason = ReasonExpired
	}
	return result
}

func (e *Entitlement) ReserveRefund(grantID, refundID string, amount Money, now time.Time) (Refund, error) {
	if e == nil || !validText(grantID, MaxText) || !validText(refundID, MaxText) || now.IsZero() {
		return Refund{}, fmt.Errorf("%w: refund request", ErrInvalid)
	}
	if err := amount.Positive(); err != nil {
		return Refund{}, err
	}
	grant := e.grantByID(grantID)
	if grant == nil {
		return Refund{}, fmt.Errorf("%w: refund grant", ErrBinding)
	}
	for _, existing := range e.Grants {
		for _, refund := range existing.Refunds {
			if refund.ID == refundID {
				return Refund{}, fmt.Errorf("%w: duplicate refund", ErrConflict)
			}
		}
	}
	if amount.Centavos > grant.Refundable().Centavos {
		return Refund{}, fmt.Errorf("%w: refund exceeds paid amount", ErrRefundBounds)
	}
	refund := Refund{
		ID:          refundID,
		GrantID:     grantID,
		Amount:      amount,
		State:       RefundRequested,
		RequestedAt: now.UTC(),
		UpdatedAt:   now.UTC(),
	}
	grant.Refunds = append(grant.Refunds, refund)
	return refund, nil
}

func (e *Entitlement) MarkRefundPending(grantID, refundID string, now time.Time) error {
	return e.updateRefund(grantID, refundID, RefundPending, now)
}

func (e *Entitlement) ConfirmRefund(grantID, refundID string, now time.Time) error {
	if err := e.updateRefund(grantID, refundID, RefundSucceeded, now); err != nil {
		return err
	}
	grant := e.grantByID(grantID)
	if grant == nil {
		return fmt.Errorf("%w: refund grant", ErrBinding)
	}
	if grant.Refunded().Equal(grant.Paid) {
		grant.revoke()
	}
	return nil
}

func (e *Entitlement) FailRefund(grantID, refundID string, now time.Time) error {
	return e.updateRefund(grantID, refundID, RefundFailed, now)
}

func (e *Entitlement) DenyRefund(grantID, refundID string, now time.Time) error {
	return e.updateRefund(grantID, refundID, RefundDenied, now)
}

func (e *Entitlement) updateRefund(grantID, refundID string, state RefundState, now time.Time) error {
	if e == nil || !validText(grantID, MaxText) || !validText(refundID, MaxText) || now.IsZero() {
		return fmt.Errorf("%w: refund update", ErrInvalid)
	}
	grant := e.grantByID(grantID)
	if grant == nil {
		return fmt.Errorf("%w: refund grant", ErrBinding)
	}
	for i := range grant.Refunds {
		refund := &grant.Refunds[i]
		if refund.ID != refundID {
			continue
		}
		if refund.State == state {
			return nil
		}
		if !canTransitionRefund(refund.State, state) {
			return fmt.Errorf("%w: refund %s -> %s", ErrTransition, refund.State, state)
		}
		refund.State = state
		refund.UpdatedAt = now.UTC()
		if state == RefundSucceeded || state == RefundFailed || state == RefundDenied {
			refund.ResolvedAt = now.UTC()
		}
		return nil
	}
	return fmt.Errorf("%w: refund not found", ErrBinding)
}

func (e *Entitlement) OpenDispute(grantID, disputeID string, amount Money, now time.Time) (Dispute, error) {
	if e == nil || !validText(grantID, MaxText) || !validText(disputeID, MaxText) || now.IsZero() {
		return Dispute{}, fmt.Errorf("%w: dispute request", ErrInvalid)
	}
	if err := amount.Positive(); err != nil {
		return Dispute{}, err
	}
	grant := e.grantByID(grantID)
	if grant == nil {
		return Dispute{}, fmt.Errorf("%w: dispute grant", ErrBinding)
	}
	if grant.Dispute != nil {
		return Dispute{}, fmt.Errorf("%w: dispute already exists", ErrConflict)
	}
	if amount.Centavos > grant.Paid.Centavos {
		return Dispute{}, fmt.Errorf("%w: dispute amount", ErrRefundBounds)
	}
	dispute := Dispute{
		ID:        disputeID,
		GrantID:   grantID,
		Amount:    amount,
		State:     DisputePending,
		OpenedAt:  now.UTC(),
		UpdatedAt: now.UTC(),
	}
	grant.Dispute = &dispute
	return dispute, nil
}

func (e *Entitlement) UpdateDispute(grantID, disputeID string, state DisputeState, now time.Time) error {
	if e == nil || !validText(grantID, MaxText) || !validText(disputeID, MaxText) || now.IsZero() || !state.Valid() {
		return fmt.Errorf("%w: dispute update", ErrInvalid)
	}
	grant := e.grantByID(grantID)
	if grant == nil || grant.Dispute == nil || grant.Dispute.ID != disputeID {
		return fmt.Errorf("%w: dispute not found", ErrBinding)
	}
	dispute := grant.Dispute
	if dispute.State == state {
		return nil
	}
	if !canTransitionDispute(dispute.State, state) {
		return fmt.Errorf("%w: dispute %s -> %s", ErrTransition, dispute.State, state)
	}
	dispute.State = state
	dispute.UpdatedAt = now.UTC()
	switch state {
	case DisputeWon, DisputeLost, DisputeClosed:
		dispute.ResolvedAt = now.UTC()
	}
	if state == DisputeLost {
		grant.revoke()
	}
	return nil
}
