package application

import (
	"context"
	"errors"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/subscription/domain"
)

// Storefront routes owner commands, provider jobs and the status read to
// the store that issued each purchase (B-BR-STORE-01/02). All services
// share one Store, so the status merges every source for the account.
type Storefront struct {
	Services []*Service
}

func (f Storefront) base() *Service {
	for _, s := range f.Services {
		if s != nil && s.Store != nil {
			return s
		}
	}
	return nil
}

func (f Storefront) service(provider string) *Service {
	if provider == "" {
		provider = domain.ProviderPlay
	}
	for _, s := range f.Services {
		if s != nil && s.store() == provider {
			return s
		}
	}
	return nil
}

// Configured reports whether at least one store can verify purchases.
func (f Storefront) Configured() bool {
	for _, s := range f.Services {
		if s.Configured() {
			return true
		}
	}
	return false
}

func (f Storefront) Status(ctx context.Context, p Principal) (domain.Status, error) {
	base := f.base()
	if base == nil {
		return domain.Status{}, domain.ErrUnavailable
	}
	return base.status(ctx, p, f.Configured())
}

// Offers lists the configured store offers for the owner, in service order.
func (f Storefront) Offers(p Principal) []Offer {
	offers := []Offer{}
	for _, s := range f.Services {
		if offer, ok := s.Offer(p); ok {
			offers = append(offers, offer)
		}
	}
	return offers
}

func (f Storefront) Admit(ctx context.Context, p Principal, c Command) (Receipt, error) {
	if c.Provider != "" && !domain.ValidProvider(c.Provider) {
		return Receipt{}, domain.ErrInvalid
	}
	s := f.service(c.Provider)
	if s == nil {
		return Receipt{}, domain.ErrUnavailable
	}
	receipt, err := s.Admit(ctx, p, c)
	if err != nil {
		return receipt, err
	}
	// The receipt status must reflect every store, not only the verifier.
	if status, e := f.Status(ctx, p); e == nil {
		receipt.Status = status
	}
	return receipt, nil
}

func (f Storefront) Export(ctx context.Context, p Principal, limit int, after time.Time, key string) ([]ExportRow, error) {
	base := f.base()
	if base == nil {
		return nil, domain.ErrUnavailable
	}
	return base.Export(ctx, p, limit, after, key)
}

// forDigest resolves the service of an already bound purchase. A purchase
// that is gone or whose store is not configured needs no provider call.
func (f Storefront) forDigest(ctx context.Context, digest string) (*Service, error) {
	base := f.base()
	if base == nil {
		return nil, domain.ErrUnavailable
	}
	jobs, ok := base.Store.(JobStore)
	if !ok {
		return nil, domain.ErrUnavailable
	}
	row, err := jobs.ForJob(ctx, digest)
	if err != nil {
		return nil, err
	}
	s := f.service(row.Provider)
	if s == nil || !s.Configured() {
		return nil, domain.ErrUnavailable
	}
	return s, nil
}

// Refresh runs a durable refresh job against the purchase's own store.
func (f Storefront) Refresh(ctx context.Context, digest, operationID string) error {
	s, err := f.forDigest(ctx, digest)
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrUnauthorized) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.Refresh(ctx, digest, operationID)
}

// Acknowledge runs a durable acknowledgement job. Only Play needs one; App
// Store purchases are committed already acknowledged.
func (f Storefront) Acknowledge(ctx context.Context, digest string) error {
	s, err := f.forDigest(ctx, digest)
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrUnauthorized) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.Acknowledge(ctx, digest)
}

// Reconcile schedules bounded refreshes for every configured store.
func (f Storefront) Reconcile(ctx context.Context) (int, error) {
	total := 0
	var failure error
	for _, s := range f.Services {
		if !s.Configured() {
			continue
		}
		n, err := s.Reconcile(ctx)
		total += n
		if err != nil && failure == nil {
			failure = err
		}
	}
	return total, failure
}
