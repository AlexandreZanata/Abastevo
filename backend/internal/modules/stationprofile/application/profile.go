package application

import (
	"context"
	"errors"
)

// PublicProfile is the anonymous read DTO: station identity plus the
// versioned business projection and current operator link. No private
// proof, personal identity, prices or grants — unclaimed profiles read
// honestly with empty business fields and no badge.
type PublicProfile struct {
	StationID       string
	DisplayName     string
	LocationQuality string
	Latitude        *float64
	Longitude       *float64
	PolicyVersion   string
	Revision        int
	Business        map[string]string
	OperatorCNPJ    string
	OperatorSource  string
	HasBadge        bool
}

// ProfileStore persists profiles and operator revisions.
type ProfileStore interface {
	EnsureUnclaimed(ctx context.Context, stationID string) error
	Profile(ctx context.Context, stationID string) (StoredProfile, error)
	RecordOperator(ctx context.Context, id, stationID, cnpj, source, ref string) error
	CloseOperator(ctx context.Context, id string) (int64, error)
	CurrentOperator(ctx context.Context, stationID string) (StoredOperator, bool, error)
	UpdateProjection(ctx context.Context, stationID string, expectedRevision int, fields map[string]string) (StoredProfile, error)
}

// StoredProfile is the persisted projection.
type StoredProfile struct {
	PolicyVersion string
	Revision      int
	Business      map[string]string
}

// StoredOperator is the persisted open link.
type StoredOperator struct {
	ID        string
	CNPJ      string
	Source    string
	Reference string
}

var (
	ErrProfileStation   = errors.New("stationprofile: station id is required")
	ErrProfileOperator  = errors.New("stationprofile: operator evidence is required")
	ErrStationUnknown   = errors.New("stationprofile: station unknown")
	ErrOpenRevisionBusy = errors.New("stationprofile: open revision already exists")
)

// StationReader resolves canonical station identity/display for the
// public DTO without importing another module's adapter.
type StationReader func(ctx context.Context, stationID string) (display, quality string, lat, lon *float64, err error)

// EnsureUnclaimed creates the empty public profile for a station,
// idempotently: replays converge on the existing row.
func EnsureUnclaimed(ctx context.Context, store ProfileStore, stationID string) error {
	if stationID == "" {
		return ErrProfileStation
	}
	return store.EnsureUnclaimed(ctx, stationID)
}

// LinkOperator closes the current open revision (if any) and records
// the new effective link. Only permitted sources establish operation;
// concurrent links converge on the open-revision guard instead of
// forking history.
func LinkOperator(ctx context.Context, store ProfileStore, newID func() string, stationID, cnpj, source, ref string) error {
	if stationID == "" || cnpj == "" {
		return ErrProfileOperator
	}
	if !validSource(source) {
		return ErrProfileOperator
	}
	current, found, err := store.CurrentOperator(ctx, stationID)
	if err != nil {
		return err
	}
	if found {
		if current.CNPJ == cnpj {
			return nil
		}
		if _, err := store.CloseOperator(ctx, current.ID); err != nil {
			return err
		}
	}
	if err := store.RecordOperator(ctx, newID(), stationID, cnpj, source, ref); err != nil {
		// Lost the race: another writer opened the revision first.
		// Converge when it carries the same CNPJ; anything else stays
		// an explicit conflict for review instead of forking history.
		if errors.Is(err, ErrOpenRevisionBusy) {
			current, found, rerr := store.CurrentOperator(ctx, stationID)
			if rerr != nil {
				return rerr
			}
			if found && current.CNPJ == cnpj {
				return nil
			}
		}
		return err
	}
	return nil
}

// ReadProfile assembles the public DTO. Missing profiles read as
// honest unclaimed (empty business, no badge) once ensured; unknown
// stations stay unknown via the reader.
func ReadProfile(ctx context.Context, store ProfileStore, read StationReader, stationID string) (PublicProfile, error) {
	if stationID == "" {
		return PublicProfile{}, ErrProfileStation
	}
	display, quality, lat, lon, err := read(ctx, stationID)
	if err != nil {
		return PublicProfile{}, err
	}
	stored, err := store.Profile(ctx, stationID)
	if err != nil {
		return PublicProfile{}, err
	}
	operator, found, err := store.CurrentOperator(ctx, stationID)
	if err != nil {
		return PublicProfile{}, err
	}
	profile := PublicProfile{
		StationID: stationID, DisplayName: display, LocationQuality: quality,
		Latitude: lat, Longitude: lon,
		PolicyVersion: stored.PolicyVersion, Revision: stored.Revision,
		Business: stored.Business, HasBadge: false,
	}
	if found {
		profile.OperatorCNPJ = operator.CNPJ
		profile.OperatorSource = operator.Source
	}
	return profile, nil
}

func validSource(source string) bool {
	return source == "registry" || source == "dou" || source == "review"
}
