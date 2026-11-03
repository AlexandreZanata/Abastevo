package prepared

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	charging "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/charging"
	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/modules/charging/adapters/linking"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NationalImport carries owner-reviewed receipt and geography pins.
type NationalImport struct {
	Files           fs.FS
	RightsSHA256    string
	ManifestSHA256  string
	Geography       []byte
	GeographySHA256 string
	BoundarySHA256  string
	Override        *CatalogCoverageOverride
}

// ImportNationalCatalog stages and activates one verified catalog in the
// same transaction. Callers must obtain operator approval before invoking it.
func ImportNationalCatalog(ctx context.Context, pool *pgxpool.Pool, input NationalImport) (string, error) {
	if pool == nil || input.Files == nil {
		return "", errors.New("prepared: national import unavailable")
	}
	bundle, err := VerifyNationalBundle(input.Files, input.RightsSHA256, input.ManifestSHA256)
	if err != nil {
		return "", err
	}
	projected, err := ProjectNationalCatalog(bundle)
	if err != nil {
		return "", err
	}
	places, counts, err := BindCatalogGeography(bundle, projected, input.Geography, input.GeographySHA256, input.BoundarySHA256)
	if err != nil {
		return "", err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte("abastevo:charging-batch:v1\x00"+input.RightsSHA256+input.GeographySHA256)))
	tx, err := pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var current pgtype.UUID
	var held bool
	if err := tx.QueryRow(ctx, "SELECT batch_id,held FROM charging_catalog_current WHERE singleton FOR UPDATE").Scan(&current, &held); err != nil {
		return "", err
	}
	if held {
		return "", errors.New("prepared: national catalog publication is held")
	}
	var existing pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM charging_batches WHERE profile_id=$1 AND source_revision=$2`,
		NationalCatalogProfile, digest).Scan(&existing)
	if err == nil {
		if existing != current {
			return "", errors.New("prepared: superseded national catalog cannot be replayed")
		}
		return hexUUID(existing), tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	previous, err := previousCatalogIdentities(ctx, tx)
	if err != nil {
		return "", err
	}
	identities := make([]CatalogIdentity, len(places))
	entities := len(places)
	for i, place := range places {
		identities[i] = place.Identity
		entities += len(place.Groups)
		if entities > MaxRecords {
			return "", errOversize
		}
	}
	publicIDs, err := ResolveCatalogIdentities(identities, previous)
	if err != nil {
		return "", err
	}
	batch, err := newUUID()
	if err != nil {
		return "", err
	}
	q := charging.New(tx)
	if _, err := q.RecordChargingBatch(ctx, charging.RecordChargingBatchParams{
		ID: batch, Format: "charging-prepared-v1", ProfileID: NationalCatalogProfile, SourceRevision: digest,
		ManifestSha256: digest, InputCount: int32(entities), AcceptedCount: int32(entities),
	}); err != nil {
		return "", err
	}
	census, err := json.Marshal(counts)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO charging_catalog_generations(batch_id,rights_sha256,offer_sha256,uf_counts)
		VALUES($1,$2,$3,$4)`, batch, bundle.RightsSHA256, bundle.OfferSHA256, census); err != nil {
		return "", err
	}
	for i, place := range places {
		if err := stageNationalPlace(ctx, tx, batch, place, publicIDs[i]); err != nil {
			return "", err
		}
	}
	if err := ActivateCatalog(ctx, tx, batch, input.Override); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return hexUUID(batch), nil
}

func previousCatalogIdentities(ctx context.Context, tx pgx.Tx) (map[string]CatalogPublicIdentity, error) {
	rows, err := tx.Query(ctx, `SELECT source.key,primary_alias.public_id::text,
		ARRAY(SELECT alias.public_id::text FROM charging_catalog_place_aliases alias
			WHERE alias.batch_id=p.batch_id AND alias.place_id=p.id AND NOT alias.is_primary ORDER BY alias.public_id)
		FROM charging_catalog_activations activation
		JOIN charging_places p ON p.batch_id=activation.batch_id
		JOIN charging_catalog_place_aliases primary_alias
			ON primary_alias.batch_id=p.batch_id AND primary_alias.place_id=p.id AND primary_alias.is_primary
		CROSS JOIN LATERAL jsonb_array_elements_text(p.catalog_details->'root_source_keys') source(key)
		ORDER BY activation.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	previous := map[string]CatalogPublicIdentity{}
	for rows.Next() {
		var key string
		var identity CatalogPublicIdentity
		if err := rows.Scan(&key, &identity.ID, &identity.Aliases); err != nil {
			return nil, err
		}
		if _, found := previous[key]; !found {
			previous[key] = identity
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	legacy, err := tx.Query(ctx, `SELECT 'ocm:poi:'||p.external_id,
		array_agg(p.id::text ORDER BY b.stored_at DESC,p.id)
		FROM charging_places p JOIN charging_batches b ON b.id=p.batch_id
		WHERE b.profile_id='ocm-br-20261021-v1' AND p.external_id ~ '^[1-9][0-9]*$'
		GROUP BY p.external_id`)
	if err != nil {
		return nil, err
	}
	defer legacy.Close()
	for legacy.Next() {
		var key string
		var ids []string
		if err := legacy.Scan(&key, &ids); err != nil {
			return nil, err
		}
		prior, found := previous[key]
		if !found {
			prior.ID, ids = ids[0], ids[1:]
		}
		prior.Aliases = append(prior.Aliases, ids...)
		previous[key] = prior
	}
	return previous, legacy.Err()
}

func previousPlacesForPublicIDs(ctx context.Context, tx pgx.Tx, publicIDs []string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT COALESCE(prior.place_id, legacy.id)::text
		FROM unnest($1::text[]) AS public(id)
		LEFT JOIN LATERAL (
			SELECT a.place_id FROM charging_catalog_place_aliases a
			JOIN charging_catalog_activations activation ON activation.batch_id=a.batch_id
			WHERE a.public_id=public.id::uuid
			ORDER BY activation.id DESC LIMIT 1
		) prior ON TRUE
		LEFT JOIN (
			SELECT p.id FROM charging_places p JOIN charging_batches b ON b.id=p.batch_id
			WHERE b.profile_id='ocm-br-20261021-v1'
		) legacy ON legacy.id=public.id::uuid
		WHERE COALESCE(prior.place_id,legacy.id) IS NOT NULL`, publicIDs)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func stageNationalPlace(ctx context.Context, tx pgx.Tx, batch pgtype.UUID, place NationalPlace, public CatalogPublicIdentity) error {
	id, err := newUUID()
	if err != nil {
		return err
	}
	details, err := json.Marshal(map[string]any{
		"confidence": place.Confidence, "uf": place.UF, "sources": place.Sources,
		"root_source_keys": place.Identity.SourceKeys, "point_source_key": place.PointSourceKey,
		"access": place.Access, "address": place.Address, "brand": place.Brand,
		"operator": place.Operator, "network": place.Network,
	})
	if err != nil {
		return err
	}
	name, access := "", "UNKNOWN"
	if place.Name != nil {
		name = *place.Name
	}
	if place.Access == "PUBLIC" {
		access = "PUBLIC"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO charging_places
		(id,batch_id,entity_key,external_id,name,country,latitude,longitude,public_access,reported_points,catalog_details)
		VALUES($1,$2,$3,$4,$5,'BR',$6,$7,$8,0,$9)`, id, batch, strings.TrimPrefix(place.Identity.Key, "ev-site-v1:"),
		place.Identity.Key, name, place.Latitude, place.Longitude, access, details); err != nil {
		return err
	}
	publicIDs := append([]string{public.ID}, public.Aliases...)
	for _, publicID := range publicIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO charging_catalog_place_aliases(batch_id,public_id,place_id,is_primary)
			VALUES($1,$2,$3,$4)`, batch, publicID, id, publicID == public.ID); err != nil {
			return err
		}
	}
	for _, group := range place.Groups {
		groupID, err := newUUID()
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%x", sha256.Sum256([]byte(group.Key)))
		if _, err := tx.Exec(ctx, `INSERT INTO charging_connection_groups
			(id,batch_id,place_id,entity_key,external_id,connector_code,raw_connector_code,current,power_w,reported_quantity,catalog_source_key)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, groupID, batch, id, key, group.Key,
			group.Family, group.RawSocket, group.Current, group.PowerW, group.Quantity, group.SourceKey); err != nil {
			return err
		}
	}
	prior, err := previousPlacesForPublicIDs(ctx, tx, publicIDs)
	if err != nil {
		return err
	}
	if err := linking.CarryCatalogReviewTx(ctx, tx, hexUUID(id), prior); err != nil {
		return err
	}
	facts := linking.StationSourceFacts{TradeName: name}
	if place.Brand != nil {
		facts.Brand = *place.Brand
	}
	if place.Combined != nil {
		facts.Combined = &linking.OSMObject{Type: place.Combined.Type, ID: place.Combined.ID}
	}
	_, _, err = linking.ApplySourceLinkTx(ctx, tx, hexUUID(id), facts)
	return err
}
