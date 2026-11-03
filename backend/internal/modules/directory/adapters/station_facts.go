package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ProjectSourceFacts publishes only complete evidence bound to this identity.
// The station lock serializes importers and accepted correction projection.
func (c RegistryCanonicalizer) ProjectSourceFacts(ctx context.Context, stationID, assertionID string) error {
	station, err := mustUUID(stationID)
	if err != nil {
		return err
	}
	assertion, err := mustUUID(assertionID)
	if err != nil {
		return err
	}
	tx, err := c.Repo.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := directory.New(tx)
	current, err := q.LockStationFacts(ctx, station)
	if err != nil {
		return err
	}
	source, err := q.SourceStationFacts(ctx, directory.SourceStationFactsParams{StationID: station, AssertionID: assertion})
	if err != nil {
		return fmt.Errorf("directory: complete source facts unavailable: %w", err)
	}
	if current.FactsSourceClock.Valid && (source.SourceClock.Time.Before(current.FactsSourceClock.Time) || (source.SourceClock.Time.Equal(current.FactsSourceClock.Time) && source.Checksum <= current.FactsSourceChecksum.String)) {
		return tx.Commit(ctx)
	}
	correction, err := acceptedStationName(ctx, q, station)
	if err != nil {
		return err
	}
	display := current.DisplayName
	if correction != "" || current.SourceNameOwned || current.DisplayName == source.DisplayName || (current.LegalName.Valid && current.DisplayName == domain.StationDisplayName(current.LegalName.String, current.TradeName.String, source.SourceKey, "")) {
		if source.LegalName.Valid {
			display = domain.StationDisplayName(source.LegalName.String, source.TradeName.String, source.SourceKey, correction)
		} else if correction != "" {
			display = domain.StationDisplayName("", "", "", correction)
		} else {
			display = source.DisplayName
		}
	}
	var incoming, previous map[string]string
	if err := json.Unmarshal(source.Address, &incoming); err != nil {
		return err
	}
	if err := json.Unmarshal(current.Address, &previous); err != nil {
		return err
	}
	address, addressSource := current.Address, pgtype.Text{}
	if current.SourceAddressOwned || len(previous) == 0 || reflect.DeepEqual(incoming, previous) {
		candidate := domain.StationAddress{Source: source.AddressSource.String, Components: incoming}
		if candidate.Source == "" {
			candidate = domain.ANPStationAddress(incoming)
		}
		selected := domain.SelectStationAddress(candidate, domain.StationAddress{Source: current.AddressSource.String, Components: previous})
		if selected.Source != "" {
			address, err = json.Marshal(selected.Components)
			if err != nil {
				return err
			}
			addressSource = pgtype.Text{String: selected.Source, Valid: true}
		}
	}
	if err := q.UpdateStationSourceFacts(ctx, directory.UpdateStationSourceFactsParams{ID: station, LegalName: source.LegalName, TradeName: source.TradeName, DisplayName: display, Address: address, AddressSource: addressSource, SourceClock: source.SourceClock, SourceChecksum: pgtype.Text{String: source.Checksum, Valid: true}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func acceptedStationName(ctx context.Context, q *directory.Queries, station pgtype.UUID) (string, error) {
	value, err := q.AcceptedStationName(ctx, station)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// Called inside the audited decision transaction; source columns stay intact.
func projectAcceptedStationName(ctx context.Context, q *directory.Queries, station pgtype.UUID) error {
	if _, err := q.LockStationFacts(ctx, station); err != nil {
		return err
	}
	name, err := acceptedStationName(ctx, q, station)
	if err != nil || name == "" {
		return err
	}
	name = domain.StationDisplayName("", "", "", name)
	return q.UpdateStationAcceptedName(ctx, directory.UpdateStationAcceptedNameParams{ID: station, DisplayName: name})
}
