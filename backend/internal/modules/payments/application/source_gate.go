package application

import (
	"context"
	"sort"
	"time"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/payments/domain"
)

// allowNewRecurringSource is the runtime fence: one automatic renewal source
// per account/offer, a confirmed-ready switch target, and no new debit before
// the current paid-through boundary.
func allowNewRecurringSource(ctx context.Context, store SourceGateStore, catalog domain.Catalog, accountID, offerID string, channel domain.Channel, quote domain.Quote, now time.Time) error {
	if channel != domain.ChannelCard && channel != domain.ChannelPixAutomatic {
		return nil
	}
	if _, found, err := activeFamilySource(ctx, store, catalog, accountID, offerID); err != nil {
		return err
	} else if found {
		return domain.ErrConflict
	}
	end, err := activeFamilyGrantEnd(ctx, store, catalog, accountID, offerID)
	if err != nil {
		return err
	}
	if record, found, err := openFamilySwitch(ctx, store, catalog, accountID, offerID); err != nil {
		return err
	} else if found {
		if record.NewSource != channel || record.TargetCatalogVersion != quote.CatalogVersion ||
			record.OfferID != quote.OfferID || !record.TargetAmount.Equal(quote.Amount) ||
			!sameStrings(record.TargetCapabilities, quote.Capabilities) {
			return domain.ErrStale
		}
		if record.State != SourceSwitchReady {
			return domain.ErrConflict
		}
		first := record.FirstDebitAt
		if end.After(first) {
			first = end
		}
		if now.Before(first) {
			return domain.ErrBusy
		}
		return nil
	}
	if end.After(now) {
		return domain.ErrBusy
	}
	return nil
}

func activeFamilySource(ctx context.Context, store SourceGateStore, catalog domain.Catalog, accountID, offerID string) (RecurringSource, bool, error) {
	offers, err := familyOffers(catalog, offerID)
	if err != nil {
		return RecurringSource{}, false, err
	}
	var found RecurringSource
	for _, familyOfferID := range offers {
		source, ok, err := store.ActiveRecurringSource(ctx, accountID, familyOfferID)
		if err != nil {
			return RecurringSource{}, false, err
		}
		if !ok {
			continue
		}
		if found.Source != "" {
			return RecurringSource{}, false, domain.ErrConflict
		}
		found = source
	}
	return found, found.Source != "", nil
}

func familyOffers(catalog domain.Catalog, offerID string) ([]string, error) {
	offer, ok := catalog.Offers[offerID]
	if !ok {
		return nil, domain.ErrUnknownOffer
	}
	ids := make([]string, 0, len(catalog.Offers))
	for id, candidate := range catalog.Offers {
		if candidate.Family == offer.Family {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func activeFamilyGrantEnd(ctx context.Context, store SourceGateStore, catalog domain.Catalog, accountID, offerID string) (time.Time, error) {
	offers, err := familyOffers(catalog, offerID)
	if err != nil {
		return time.Time{}, err
	}
	var end time.Time
	for _, familyOfferID := range offers {
		value, err := store.ActiveGrantEnd(ctx, accountID, familyOfferID)
		if err != nil {
			return time.Time{}, err
		}
		if value.After(end) {
			end = value
		}
	}
	return end, nil
}

func openFamilySwitch(ctx context.Context, store SourceGateStore, catalog domain.Catalog, accountID, offerID string) (SourceSwitchRecord, bool, error) {
	offers, err := familyOffers(catalog, offerID)
	if err != nil {
		return SourceSwitchRecord{}, false, err
	}
	var found SourceSwitchRecord
	for _, familyOfferID := range offers {
		record, ok, err := store.OpenSourceSwitch(ctx, accountID, familyOfferID)
		if err != nil {
			return SourceSwitchRecord{}, false, err
		}
		if !ok {
			continue
		}
		if found.ID != "" {
			return SourceSwitchRecord{}, false, domain.ErrConflict
		}
		found = record
	}
	return found, found.ID != "", nil
}
