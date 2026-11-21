package adapters

import (
	"context"
	"errors"

	db "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/orgbilling"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/kernel"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/orgbilling/domain"
	payment "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/application"
	paydomain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/database"
	"github.com/jackc/pgx/v5"
)

type companyPixResources interface {
	RetrieveCompanyPix(context.Context, string) (payment.CompanyPixProof, error)
}

type asaasBindingSource interface {
	ReadAsaasBinding(context.Context, kernel.OrgScope, domain.BillingNotice, string) (domain.BillingBinding, error)
}

func (intake BillingIntake) ReadAsaasBinding(ctx context.Context, scope kernel.OrgScope, notice domain.BillingNotice, reference string) (domain.BillingBinding, error) {
	fromReference, err := domain.AsaasReferenceScope(reference)
	if err != nil || fromReference != scope || notice.Provider != paydomain.ProviderAsaas || notice.Validate() != nil {
		return domain.BillingBinding{}, domain.ErrBillingBinding
	}
	if intake.Pool == nil {
		return domain.BillingBinding{}, paydomain.ErrUnavailable
	}
	var binding domain.BillingBinding
	err = database.WithOrgTx(ctx, intake.Pool, scope, func(tx pgx.Tx) error {
		var err error
		binding, err = readChargeBinding(ctx, db.New(tx), scope, notice)
		if err != nil {
			return err
		}
		if binding.AsaasExternalRef != reference {
			return domain.ErrBillingBinding
		}
		return nil
	})
	if err != nil {
		return domain.BillingBinding{}, err
	}
	return binding, nil
}

type AsaasSettlement struct {
	Resources        companyPixResources
	Bindings         asaasBindingSource
	Record           settlementRecorder
	Personal         settlementProcessor
	Inbox            settlementInbox
	Merchant         string
	Environment      paydomain.Environment
	AutomaticAllowed bool
}

func (s AsaasSettlement) Process(ctx context.Context, event payment.InboxRecord) error {
	if s.Resources == nil || s.Bindings == nil || s.Record == nil || s.Personal == nil || s.Inbox == nil {
		return paydomain.ErrUnavailable
	}
	if event.Provider != paydomain.ProviderAsaas || event.EventType != "PAYMENT_RECEIVED" || event.ResourceType != "payment" {
		return s.Personal.Process(ctx, event)
	}
	if event.Environment != s.Environment || event.Merchant != s.Merchant {
		return s.Inbox.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "asaas-merchant-binding")
	}
	proof, err := s.Resources.RetrieveCompanyPix(ctx, event.ResourceID)
	switch {
	case errors.Is(err, paydomain.ErrInvalid):
		return s.Inbox.MarkInbox(ctx, event.InboxEvent, "CONFLICT", "asaas-payment-reference")
	case errors.Is(err, paydomain.ErrUnavailable), errors.Is(err, payment.ErrAmbiguous),
		errors.Is(err, payment.ErrProviderUnauthorized):
		return err
	case err != nil:
		// The router drains in order and stops at an error, so a deterministic
		// provider answer keeps the personal processor's terminal handling.
		return s.Personal.Process(ctx, event)
	}
	if !domain.IsCompanyAsaasReference(proof.ExternalReference) {
		return s.Personal.Process(ctx, event)
	}
	fact, err := s.billingFact(ctx, event, proof)
	if err == nil {
		_, err = s.Record.RecordSettlement(ctx, fact.Notice, fact)
	}
	return finishOrganizationSettlement(ctx, s.Inbox, event, err)
}

func (s AsaasSettlement) billingFact(ctx context.Context, event payment.InboxRecord, proof payment.CompanyPixProof) (domain.CanonicalBillingFact, error) {
	scope, err := domain.AsaasReferenceScope(proof.ExternalReference)
	if err != nil || proof.ID != event.ResourceID || proof.Environment != s.Environment ||
		proof.Method != "PIX" || proof.Deleted || proof.Adjusted || proof.RefundedOrDisputed {
		return domain.CanonicalBillingFact{}, domain.ErrBillingBinding
	}
	if proof.Status != "RECEIVED" || proof.PaidOn.IsZero() || event.ReceivedAt.Before(proof.PaidOn) {
		return domain.CanonicalBillingFact{}, domain.ErrBillingUnsettled
	}
	notice := domain.BillingNotice{Provider: event.Provider, Environment: event.Environment, Merchant: event.Merchant,
		EventID: event.EventID, PayloadHash: event.PayloadHash, ResourceRef: event.ResourceID}
	binding, err := s.Bindings.ReadAsaasBinding(ctx, scope, notice, proof.ExternalReference)
	if err != nil {
		return domain.CanonicalBillingFact{}, err
	}
	channel, contract := paydomain.ChannelPixManual, proof.ID
	if proof.AuthorizationID != "" {
		if !s.AutomaticAllowed {
			// A disabled channel is a deployment fact, not a transient fault.
			return domain.CanonicalBillingFact{}, domain.ErrBillingBinding
		}
		channel, contract = paydomain.ChannelPixAutomatic, proof.AuthorizationID
	} else if proof.SubscriptionID != "" {
		return domain.CanonicalBillingFact{}, domain.ErrBillingBinding
	}
	return domain.CanonicalBillingFact{
		Notice: notice, Scope: scope, SubscriptionID: binding.SubscriptionID, ResourceRef: proof.ID,
		ChargeRef: proof.ID, Channel: channel, ContractRef: contract, CustomerRef: proof.CustomerID,
		ProviderOfferRef: binding.Offer.ProviderOfferRef, Outcome: domain.BillingSettled,
		At: event.ReceivedAt.UTC(), ProviderPaidOn: proof.PaidOn, AsaasExternalRef: proof.ExternalReference,
		PaidTerm: binding.ExpectedTerm, Gross: proof.Amount,
	}, nil
}
