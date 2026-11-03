package linking

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

const sourceLinkReviewer = "source:evc-04"

type sourceLinkEvidence struct {
	Rule     string `json:"rule"`
	OSMType  string `json:"osm_type,omitempty"`
	OSMID    string `json:"osm_id,omitempty"`
	MaxMeter int    `json:"max_distance_m,omitempty"`
}

// ApplySourceLink records a decision for a prepared place. Its bool indicates
// whether the resulting active decision can be exposed as a reviewed link.
func (s Store) ApplySourceLink(ctx context.Context, placeID string, facts StationSourceFacts) (Link, bool, error) {
	if s.Pool == nil {
		return Link{}, false, errors.New("linking: store unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Link{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	link, reviewed, err := ApplySourceLinkTx(ctx, tx, placeID, facts)
	if err != nil {
		return Link{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Link{}, false, err
	}
	return link, reviewed, nil
}

// ApplySourceLinkTx can share the publication transaction. A caller that
// receives an error must roll it back along with the prepared generation.
func ApplySourceLinkTx(ctx context.Context, tx pgx.Tx, placeID string, facts StationSourceFacts) (Link, bool, error) {
	place, err := uuid(placeID)
	if err != nil {
		return Link{}, false, err
	}
	if err := lockPlace(ctx, tx, place); err != nil {
		return Link{}, false, err
	}
	active, hasActive, err := readActiveLink(ctx, tx, place)
	if err != nil {
		return Link{}, false, err
	}
	if hasActive && active.ReviewerRef != sourceLinkReviewer {
		return active, active.State == Reviewed, nil
	}
	candidate, found, err := findStationCandidate(ctx, tx, placeID, facts)
	if err != nil {
		return Link{}, false, err
	}

	link := Link{PlaceID: placeID, ReviewerRef: sourceLinkReviewer}
	var evidence sourceLinkEvidence
	if found {
		link.StationID, link.State, link.Method = candidate.StationID, Reviewed, MethodAddressName
		evidence.Rule = candidate.Rule
		if candidate.Rule == ruleCombinedOSM {
			link.Method = MethodOSMObject
			evidence.OSMType, evidence.OSMID = facts.Combined.Type, strconv.FormatInt(facts.Combined.ID, 10)
		} else {
			evidence.MaxMeter = 75
		}
	} else {
		if !hasActive || active.State == Unresolved {
			return active, false, nil
		}
		link.StationID, link.State, link.Method = active.StationID, Unresolved, active.Method
		evidence.Rule = "source-evidence-unresolved"
	}

	raw, err := json.Marshal(evidence)
	if err != nil {
		return Link{}, false, err
	}
	if err := json.Unmarshal(raw, &link.Evidence); err != nil {
		return Link{}, false, err
	}
	if hasActive && active.StationID == link.StationID && active.State == link.State && active.Method == link.Method &&
		active.Evidence["rule"] == link.Evidence["rule"] && active.Evidence["osm_type"] == link.Evidence["osm_type"] &&
		active.Evidence["osm_id"] == link.Evidence["osm_id"] {
		return active, active.State == Reviewed, nil
	}
	station, err := uuid(link.StationID)
	if err != nil {
		return Link{}, false, err
	}
	link.ID, err = writeLink(ctx, tx, place, station, active.ID, link, raw)
	if err != nil {
		return Link{}, false, err
	}
	return link, link.State == Reviewed, nil
}
