// Package linking stores reviewed relationships between charging places and
// canonical directory stations, plus separate display/entrance/road points.
// Proximity alone never creates a reviewed link.
package linking

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type State string

const (
	Reviewed   State = "REVIEWED"
	Unresolved State = "UNRESOLVED"
	Rejected   State = "REJECTED"
)

type Method string

const (
	MethodCNPJ        Method = "CNPJ"
	MethodAddressName Method = "ADDRESS_NAME"
	MethodManual      Method = "MANUAL"
)

type Link struct {
	ID          string
	PlaceID     string
	StationID   string
	State       State
	Method      Method
	Evidence    map[string]any
	ReviewerRef string
}

type AccessKind string

const (
	Display  AccessKind = "DISPLAY"
	Entrance AccessKind = "ENTRANCE"
	RoadSnap AccessKind = "ROAD_SNAP"
)

type AccessPoint struct {
	ID              string
	StationID       string
	Kind            AccessKind
	Longitude       float64
	Latitude        float64
	Quality         string
	SourceReference string
	ReviewerRef     string
}

type Store struct {
	Pool *pgxpool.Pool
}

func newID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func uuid(text string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(text); err != nil || !id.Valid {
		return pgtype.UUID{}, errors.New("linking: invalid uuid")
	}
	return id, nil
}

func validState(state State) bool {
	return state == Reviewed || state == Unresolved || state == Rejected
}

func validMethod(method Method) bool {
	return method == MethodCNPJ || method == MethodAddressName || method == MethodManual
}

func validKind(kind AccessKind) bool {
	return kind == Display || kind == Entrance || kind == RoadSnap
}

func validQuality(quality string) bool {
	return quality == "reviewed" || quality == "osm-mapped"
}

// Review records one explicit decision for a place. An existing active
// decision is superseded in the same transaction; a failed insert restores
// it because the whole transaction rolls back.
func (s Store) Review(ctx context.Context, link Link) (string, error) {
	if s.Pool == nil {
		return "", errors.New("linking: store unavailable")
	}
	place, err := uuid(link.PlaceID)
	if err != nil {
		return "", err
	}
	station, err := uuid(link.StationID)
	if err != nil {
		return "", err
	}
	if !validState(link.State) || !validMethod(link.Method) || link.ReviewerRef == "" || len(link.ReviewerRef) > 200 {
		return "", errors.New("linking: invalid review")
	}
	evidence := link.Evidence
	if evidence == nil {
		evidence = map[string]any{}
	}
	rawEvidence, err := json.Marshal(evidence)
	if err != nil {
		return "", errors.New("linking: invalid evidence")
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	newID, err := uuid(id)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize reviews for one place, including the first insert.
	if _, err = tx.Exec(ctx, "SELECT id FROM charging_places WHERE id=$1 FOR UPDATE", place); err != nil {
		return "", fmt.Errorf("linking: place unavailable: %w", err)
	}
	var active pgtype.UUID
	err = tx.QueryRow(ctx, "SELECT id FROM charging_place_station_links WHERE place_id=$1 AND superseded_by IS NULL FOR UPDATE", place).Scan(&active)
	if err == nil {
		if _, err = tx.Exec(ctx, "UPDATE charging_place_station_links SET superseded_by=$2 WHERE id=$1", active, newID); err != nil {
			return "", err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO charging_place_station_links
		(id,place_id,station_id,state,method,evidence,reviewer_ref) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		newID, place, station, string(link.State), string(link.Method), rawEvidence, link.ReviewerRef); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s Store) Active(ctx context.Context, placeID string) (Link, bool, error) {
	place, err := uuid(placeID)
	if err != nil {
		return Link{}, false, err
	}
	var link Link
	var evidence []byte
	var station pgtype.UUID
	err = s.Pool.QueryRow(ctx, `SELECT id,place_id,station_id,state,method,evidence,reviewer_ref
		FROM charging_place_station_links WHERE place_id=$1 AND superseded_by IS NULL`, place).
		Scan(&link.ID, &link.PlaceID, &station, &link.State, &link.Method, &evidence, &link.ReviewerRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, false, nil
	}
	if err != nil {
		return Link{}, false, err
	}
	link.StationID = station.String()
	if len(evidence) > 0 {
		if err := json.Unmarshal(evidence, &link.Evidence); err != nil {
			return Link{}, false, err
		}
	}
	return link, true, nil
}

// RecordAccessPoint adds one reviewed display/entrance/road point and
// supersedes only the previous active point of the same kind.
func (s Store) RecordAccessPoint(ctx context.Context, point AccessPoint) (string, error) {
	if s.Pool == nil {
		return "", errors.New("linking: store unavailable")
	}
	station, err := uuid(point.StationID)
	if err != nil {
		return "", err
	}
	if !validKind(point.Kind) || !validQuality(point.Quality) || point.SourceReference == "" ||
		len(point.SourceReference) > 256 || point.ReviewerRef == "" || len(point.ReviewerRef) > 200 {
		return "", errors.New("linking: invalid access point")
	}
	if point.Latitude < -90 || point.Latitude > 90 || point.Longitude < -180 || point.Longitude > 180 {
		return "", errors.New("linking: coordinate out of range")
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	newID, err := uuid(id)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, "SELECT id FROM directory_stations WHERE id=$1 FOR UPDATE", station); err != nil {
		return "", fmt.Errorf("linking: station unavailable: %w", err)
	}
	var active pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM directory_access_points
		WHERE station_id=$1 AND kind=$2 AND superseded_by IS NULL FOR UPDATE`, station, string(point.Kind)).Scan(&active)
	if err == nil {
		if _, err = tx.Exec(ctx, "UPDATE directory_access_points SET superseded_by=$2 WHERE id=$1", active, newID); err != nil {
			return "", err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO directory_access_points
		(id,station_id,kind,point,quality,source_reference,reviewer_ref)
		VALUES($1,$2,$3,ST_SetSRID(ST_MakePoint($4,$5),4326)::geography,$6,$7,$8)`,
		newID, station, string(point.Kind), point.Longitude, point.Latitude, point.Quality, point.SourceReference, point.ReviewerRef); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s Store) ActiveAccessPoints(ctx context.Context, stationID string) ([]AccessPoint, error) {
	station, err := uuid(stationID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT id,station_id,kind,
		ST_X(point::geometry),ST_Y(point::geometry),quality,source_reference,reviewer_ref
		FROM directory_access_points WHERE station_id=$1 AND superseded_by IS NULL ORDER BY kind`, station)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AccessPoint, 0, 3)
	for rows.Next() {
		var point AccessPoint
		var id, station pgtype.UUID
		if err := rows.Scan(&id, &station, &point.Kind, &point.Longitude, &point.Latitude, &point.Quality, &point.SourceReference, &point.ReviewerRef); err != nil {
			return nil, err
		}
		point.ID = id.String()
		point.StationID = station.String()
		result = append(result, point)
	}
	return result, rows.Err()
}
