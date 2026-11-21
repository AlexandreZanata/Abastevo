package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

var (
	ErrOfferUnapproved   = errors.New("orgbilling: organization offer is not approved")
	ErrBillingBinding    = errors.New("orgbilling: provider binding mismatch")
	ErrBillingUnsettled  = errors.New("orgbilling: payment is not settled")
	ErrBillingFutureTerm = errors.New("orgbilling: paid term is not yet effective")
)

type FutureTermError struct{ EffectiveAt time.Time }

func (FutureTermError) Error() string { return ErrBillingFutureTerm.Error() }
func (FutureTermError) Unwrap() error { return ErrBillingFutureTerm }

type BillingOffer struct {
	Plan             PlanKey
	Channel          payment.Channel
	Environment      payment.Environment
	Merchant         string
	ProviderOfferRef string
	Price            payment.Money
	ApprovalRef      string
	Term             payment.Term
}

func (offer BillingOffer) Validate() error {
	if !validReference(offer.ApprovalRef) {
		return ErrOfferUnapproved
	}
	if offer.Plan.Validate() != nil || offer.Channel.Validate() != nil || offer.Environment.Validate() != nil ||
		offer.Price.Positive() != nil || offer.Term.Validate() != nil ||
		!validProviderRef(offer.Merchant) || !validProviderRef(offer.ProviderOfferRef) {
		return ErrBillingBinding
	}
	return nil
}

type BillingBinding struct {
	Scope               kernel.OrgScope
	SubscriptionID      string
	Offer               BillingOffer
	ProviderCustomerRef string
	ProviderContractRef string
	AsaasExternalRef    string
	ExpectedTerm        Period
}

// BillingNotice comes from the authenticated durable payment inbox. The
// verifier retrieves the resource independently before normalizing its fact.
type BillingNotice struct {
	Provider    payment.Provider
	Environment payment.Environment
	Merchant    string
	EventID     string
	PayloadHash string
	ResourceRef string
}

func (notice BillingNotice) Validate() error {
	hash, err := hex.DecodeString(notice.PayloadHash)
	if notice.Provider.Validate() != nil || notice.Environment.Validate() != nil || err != nil ||
		len(hash) != sha256.Size || notice.PayloadHash != strings.ToLower(notice.PayloadHash) ||
		!validProviderRef(notice.Merchant) || !validProviderRef(notice.EventID) || !validProviderRef(notice.ResourceRef) {
		return ErrBillingBinding
	}
	return nil
}

func (notice BillingNotice) reference() string {
	key, _ := json.Marshal([4]string{string(notice.Provider), string(notice.Environment), notice.Merchant, notice.EventID})
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])
}

type BillingOutcome string

const (
	BillingSettled   BillingOutcome = "SETTLED"
	BillingUnsettled BillingOutcome = "UNSETTLED"
)

// CanonicalBillingFact is the normalized result of a trusted provider read;
// transport signatures alone do not establish settlement.
type CanonicalBillingFact struct {
	Notice           BillingNotice
	Scope            kernel.OrgScope
	SubscriptionID   string
	CustomerRef      string
	ContractRef      string
	ProviderOfferRef string
	AsaasExternalRef string
	ProviderPaidOn   time.Time
	Channel          payment.Channel
	ResourceRef      string
	ChargeRef        string
	Outcome          BillingOutcome
	At               time.Time
	Gross            payment.Money
	PaidTerm         Period
}

func (binding BillingBinding) MatchFact(notice BillingNotice, fact CanonicalBillingFact) error {
	offer := binding.Offer
	if err := offer.Validate(); err != nil {
		return err
	}
	if _, err := NewSubscription(binding.Scope, binding.SubscriptionID, offer.Plan); err != nil {
		return err
	}
	if notice.Validate() != nil || fact.Notice != notice || notice.Provider != offer.Channel.Provider() ||
		notice.Environment != offer.Environment || notice.Merchant != offer.Merchant ||
		fact.Scope != binding.Scope || fact.SubscriptionID != binding.SubscriptionID ||
		fact.Channel != offer.Channel || fact.ProviderOfferRef != offer.ProviderOfferRef ||
		!validProviderRef(binding.ProviderCustomerRef) || fact.CustomerRef != binding.ProviderCustomerRef ||
		!validProviderRef(binding.ProviderContractRef) || fact.ContractRef != binding.ProviderContractRef ||
		fact.ResourceRef != notice.ResourceRef || !validProviderRef(fact.ChargeRef) {
		return ErrBillingBinding
	}
	if notice.Provider == payment.ProviderAsaas {
		scope, err := AsaasReferenceScope(fact.AsaasExternalRef)
		if err != nil || scope != binding.Scope || fact.AsaasExternalRef != binding.AsaasExternalRef ||
			fact.ProviderPaidOn.IsZero() || fact.At.Before(fact.ProviderPaidOn) {
			return ErrBillingBinding
		}
	} else if binding.AsaasExternalRef != "" || fact.AsaasExternalRef != "" || !fact.ProviderPaidOn.IsZero() {
		return ErrBillingBinding
	}
	return nil
}

func (binding BillingBinding) SettlementEvent(notice BillingNotice, fact CanonicalBillingFact, revision int64, now time.Time) (LifecycleEvent, error) {
	if err := binding.MatchFact(notice, fact); err != nil {
		return LifecycleEvent{}, err
	}
	if revision < 0 {
		return LifecycleEvent{}, ErrBillingBinding
	}
	if fact.Outcome != BillingSettled {
		return LifecycleEvent{}, ErrBillingUnsettled
	}
	if !fact.Gross.Equal(binding.Offer.Price) {
		return LifecycleEvent{}, payment.ErrAmountMismatch
	}
	if !binding.ExpectedTerm.equal(fact.PaidTerm) || fact.At.IsZero() || now.Before(fact.At) ||
		fact.PaidTerm.Start.IsZero() || !fact.PaidTerm.Start.Before(fact.PaidTerm.End) {
		return LifecycleEvent{}, ErrBillingBinding
	}
	effectiveAt := fact.At
	if effectiveAt.Before(fact.PaidTerm.Start) {
		if now.Before(fact.PaidTerm.Start) {
			return LifecycleEvent{}, FutureTermError{EffectiveAt: fact.PaidTerm.Start.UTC()}
		}
		effectiveAt = fact.PaidTerm.Start
	}
	if !fact.PaidTerm.contains(effectiveAt) {
		return LifecycleEvent{}, ErrBillingBinding
	}
	return LifecycleEvent{
		Scope: binding.Scope, SubscriptionID: binding.SubscriptionID, ExpectedRevision: revision,
		Kind: ConfirmSettlement, Source: VerifiedBilling, At: effectiveAt.UTC(),
		Reference: notice.reference(), Period: fact.PaidTerm,
	}, nil
}

func validProviderRef(raw string) bool {
	return len(raw) > 0 && len(raw) <= 255 && !strings.ContainsFunc(raw, func(r rune) bool {
		return r < '!' || r > '~'
	})
}
