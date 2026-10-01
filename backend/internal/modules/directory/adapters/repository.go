package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	directory "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/directory"
	domain "github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/directory/domain"
)

// Repository implements domain.Store on a pgx pool.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository wires the owned generated queries to a pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// NewUUIDv4 generates a random version-4 UUID in canonical text form from
// crypto/rand. Stored identifiers always originate here, never from source
// input, so a malicious source cannot fix primary keys.
func NewUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	hexed := hex.EncodeToString(b[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32], nil
}

func mustUUID(text string) (pgtype.UUID, error) {
	clean := strings.ReplaceAll(text, "-", "")
	raw, err := hex.DecodeString(clean)
	if err != nil || len(raw) != 16 {
		return pgtype.UUID{}, errors.New("adapters: malformed UUID")
	}
	var id pgtype.UUID
	copy(id.Bytes[:], raw)
	id.Valid = true
	return id, nil
}

func mapStation(r directory.ResolveActiveIdentifierRow) domain.Station {
	return domain.Station{
		ID:               uuidString(r.ID),
		DisplayName:      r.DisplayName,
		MunicipalityCode: r.MunicipalityCode.String,
		State:            r.State.String,
		Status:           r.Status,
		CurrentPointWKT:  wktString(r.CurrentPointWkt),
		CurrentQuality:   r.CurrentQuality.String,
		CreatedAt:        r.CreatedAt.Time,
	}
}

// wktString decodes the ST_AsText columns sqlc types as interface{}.
func wktString(v any) string {
	s, _ := v.(string)
	return s
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	hexed := hex.EncodeToString(id.Bytes[:])
	return hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32]
}

// ResolveCNPJ returns the canonical station for a normalized CNPJ,
// creating exactly one station and binding the identifier atomically.
// cnpjNormalized must already be kernel-validated (the P02-T04 import flow
// enforces this); the repository never reinterprets source text.
// Concurrent callers with the same CNPJ converge: the unique-index loser
// re-reads the winner's row instead of duplicating the station.
func (r *Repository) ResolveCNPJ(ctx context.Context, cnpjNormalized, displayName string, address map[string]string) (domain.Station, error) {
	q := directory.New(r.pool)
	if row, err := q.ResolveActiveIdentifier(ctx, directory.ResolveActiveIdentifierParams{
		Kind:  "CNPJ",
		Value: cnpjNormalized,
	}); err == nil {
		return mapStation(row), nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Station{}, err
	}
	addrJSON, err := json.Marshal(address)
	if err != nil {
		return domain.Station{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Station{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tq := directory.New(tx)
	stationID, err := NewUUIDv4()
	if err != nil {
		return domain.Station{}, err
	}
	stationUUID, err := mustUUID(stationID)
	if err != nil {
		return domain.Station{}, err
	}
	if _, err := tq.CreateStation(ctx, directory.CreateStationParams{
		ID:               stationUUID,
		DisplayName:      displayName,
		Address:          addrJSON,
		MunicipalityCode: pgtype.Text{},
		State:            pgtype.Text{},
	}); err != nil {
		return domain.Station{}, err
	}
	identifierID, err := NewUUIDv4()
	if err != nil {
		return domain.Station{}, err
	}
	identifierUUID, err := mustUUID(identifierID)
	if err != nil {
		return domain.Station{}, err
	}
	if _, err := tq.CreateIdentifier(ctx, directory.CreateIdentifierParams{
		ID:               identifierUUID,
		StationID:        stationUUID,
		Kind:             "CNPJ",
		Value:            cnpjNormalized,
		SourceRevisionID: pgtype.Text{},
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Lost the race: another transaction bound this CNPJ first.
			// Roll back our orphan station, then resolve the winner.
			_ = tx.Rollback(ctx)
			row, rerr := q.ResolveActiveIdentifier(ctx, directory.ResolveActiveIdentifierParams{
				Kind:  "CNPJ",
				Value: cnpjNormalized,
			})
			if rerr != nil {
				return domain.Station{}, rerr
			}
			return mapStation(row), nil
		}
		return domain.Station{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Station{}, err
	}
	row, err := q.ResolveActiveIdentifier(ctx, directory.ResolveActiveIdentifierParams{
		Kind:  "CNPJ",
		Value: cnpjNormalized,
	})
	if err != nil {
		return domain.Station{}, err
	}
	return mapStation(row), nil
}

// Station loads one station by ID.
func (r *Repository) Station(ctx context.Context, id string) (domain.Station, error) {
	uid, err := mustUUID(id)
	if err != nil {
		return domain.Station{}, domain.ErrUnknownStation
	}
	row, err := directory.New(r.pool).GetStation(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Station{}, domain.ErrUnknownStation
		}
		return domain.Station{}, err
	}
	return domain.Station{
		ID:               uuidString(row.ID),
		DisplayName:      row.DisplayName,
		MunicipalityCode: row.MunicipalityCode.String,
		State:            row.State.String,
		Status:           row.Status,
		CurrentPointWKT:  wktString(row.CurrentPointWkt),
		CurrentQuality:   row.CurrentQuality.String,
		CreatedAt:        row.CreatedAt.Time,
	}, nil
}

// FixDistance measures PostGIS metres from a station's precise point
// to a transient fix for submit-time proximity (P16-T03B). The fix
// never persists: only the returned distance (reduced to a band by
// the caller) survives the call. Unknown stations and missing points
// yield ErrUnknownStation, failing the proximity path closed.
func (r *Repository) FixDistance(ctx context.Context, stationID string, lon, lat float64) (float64, error) {
	uid, err := mustUUID(stationID)
	if err != nil {
		return 0, domain.ErrUnknownStation
	}
	row, err := directory.New(r.pool).FixDistanceM(ctx, directory.FixDistanceMParams{
		StationID: uid,
		Lon:       lon,
		Lat:       lat,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrUnknownStation
		}
		return 0, err
	}
	distance, ok := row.DistanceM.(float64)
	if !ok {
		return 0, errors.New("adapters: bad fix distance")
	}
	return distance, nil
}

// StationPairDistanceM measures PostGIS metres between two precise
// station points for teleport review (P16-T03B). Missing points on
// either side yield ErrUnknownStation: no baseline, no teleport.
func (r *Repository) StationPairDistanceM(ctx context.Context, a, b string) (float64, error) {
	uidA, err := mustUUID(a)
	if err != nil {
		return 0, domain.ErrUnknownStation
	}
	uidB, err := mustUUID(b)
	if err != nil {
		return 0, domain.ErrUnknownStation
	}
	distanceM, err := directory.New(r.pool).StationPairDistanceM(ctx, directory.StationPairDistanceMParams{
		StationID: uidA,
		OtherID:   uidB,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrUnknownStation
		}
		return 0, err
	}
	// Single-column PostGIS distance decodes as an untyped numeric.
	distance, ok := distanceM.(float64)
	if !ok {
		return 0, errors.New("adapters: bad station distance")
	}
	return distance, nil
}

// RecordLocation appends one location revision. A missing point stays NULL
// in storage; quality records why (unknown, city-centroid, reviewed).
func (r *Repository) RecordLocation(ctx context.Context, rev domain.LocationRevision) (domain.LocationRevision, error) {
	// Any spelled quality records; only reviewed ones may project later.
	if _, err := domain.ParseQuality(rev.Quality); err != nil {
		return domain.LocationRevision{}, err
	}
	stationUUID, err := mustUUID(rev.StationID)
	if err != nil {
		return domain.LocationRevision{}, domain.ErrUnknownStation
	}
	id, err := NewUUIDv4()
	if err != nil {
		return domain.LocationRevision{}, err
	}
	uid, err := mustUUID(id)
	if err != nil {
		return domain.LocationRevision{}, err
	}
	var supersedes pgtype.UUID
	if rev.SupersedesID != "" {
		supersedes, err = mustUUID(rev.SupersedesID)
		if err != nil {
			return domain.LocationRevision{}, err
		}
	}
	row, err := directory.New(r.pool).CreateLocationRevision(ctx, directory.CreateLocationRevisionParams{
		ID:              uid,
		StationID:       stationUUID,
		PointWkt:        rev.PointWKT,
		Quality:         rev.Quality,
		Provider:        rev.Provider,
		SourceReference: rev.SourceReference,
		SupersedesID:    supersedes,
	})
	if err != nil {
		return domain.LocationRevision{}, err
	}
	rev.ID = uuidString(row.ID)
	rev.ObtainedAt = row.ObtainedAt.Time
	return rev, nil
}

// ProjectLocation sets the reviewed projection from a stored revision.
// Non-reviewed or point-less revisions fail in the domain before any write.
func (r *Repository) ProjectLocation(ctx context.Context, stationID, revisionID string) (domain.Station, error) {
	stationUUID, err := mustUUID(stationID)
	if err != nil {
		return domain.Station{}, domain.ErrUnknownStation
	}
	revUUID, err := mustUUID(revisionID)
	if err != nil {
		return domain.Station{}, err
	}
	q := directory.New(r.pool)
	revs, err := q.ListLocationRevisions(ctx, stationUUID)
	if err != nil {
		return domain.Station{}, err
	}
	var found *directory.ListLocationRevisionsRow
	for i := range revs {
		if revs[i].ID == revUUID {
			found = &revs[i]
			break
		}
	}
	if found == nil {
		return domain.Station{}, domain.ErrUnknownStation
	}
	rev := domain.LocationRevision{
		ID:        revisionID,
		StationID: stationID,
		PointWKT:  wktString(found.PointWkt),
		Quality:   found.Quality,
	}
	if err := rev.Projected(); err != nil {
		return domain.Station{}, err
	}
	if err := q.UpdateStationProjection(ctx, directory.UpdateStationProjectionParams{
		ID:         stationUUID,
		PointWkt:   wktString(found.PointWkt),
		Quality:    pgtype.Text{String: found.Quality, Valid: true},
		RevisionID: revUUID,
	}); err != nil {
		return domain.Station{}, err
	}
	return r.Station(ctx, stationID)
}

// RetireIdentifier ends an identifier's validity for alias corrections.
// History stays; the row is never deleted.
func (r *Repository) RetireIdentifier(ctx context.Context, kind, normalizedValue string) error {
	n, err := directory.New(r.pool).RetireIdentifier(ctx, directory.RetireIdentifierParams{
		Kind:  kind,
		Value: normalizedValue,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrUnknownIdentifier
	}
	return nil
}

// Revisions lists a station's location history, oldest first.
func (r *Repository) Revisions(ctx context.Context, stationID string) ([]domain.LocationRevision, error) {
	stationUUID, err := mustUUID(stationID)
	if err != nil {
		return nil, domain.ErrUnknownStation
	}
	rows, err := directory.New(r.pool).ListLocationRevisions(ctx, stationUUID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.LocationRevision, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.LocationRevision{
			ID:              uuidString(row.ID),
			StationID:       stationID,
			PointWKT:        wktString(row.PointWkt),
			Quality:         row.Quality,
			Provider:        row.Provider,
			SourceReference: row.SourceReference,
			ObtainedAt:      row.ObtainedAt.Time,
			SupersedesID:    uuidString(row.SupersedesID),
		})
	}
	return out, nil
}
