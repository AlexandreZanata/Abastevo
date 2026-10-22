package prepared

import (
	"context"
	"crypto/rand"
	"fmt"

	charging "github.com/AlexandreZanata/brazil-fuel-prices/backend/db/queries/charging"
	"github.com/jackc/pgx/v5/pgtype"
)

func newUUID() (pgtype.UUID, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return pgtype.UUID{}, err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return pgtype.UUID{Bytes: raw, Valid: true}, nil
}

func hexUUID(id pgtype.UUID) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", id.Bytes[0:4], id.Bytes[4:6], id.Bytes[6:8], id.Bytes[8:10], id.Bytes[10:16])
}

// StoreBatch stages one verified batch atomically. The batch must come
// from LoadBatch (verified identities, census and integrity); storage
// rechecks structural bounds and same-batch census, never cryptographic
// identities (already verified) and never Directory links (stay null
// until reviewed). A census mismatch rolls back everything.
func StoreBatch(ctx context.Context, q *charging.Queries, batch Batch, manifestSHA256 string, inputCount, acceptedCount int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	batchUUID, err := newUUID()
	if err != nil {
		return "", err
	}
	batchID := hexUUID(batchUUID)
	if _, err := q.RecordChargingBatch(ctx, charging.RecordChargingBatchParams{
		ID: batchUUID, Format: batch.Format, ProfileID: batch.ProfileID,
		SourceRevision: batch.ManifestHash, ManifestSha256: manifestSHA256,
		InputCount: int32(inputCount), AcceptedCount: int32(acceptedCount),
	}); err != nil {
		return "", fmt.Errorf("prepared: batch record refused: %w", err)
	}
	placeIDs := map[string]pgtype.UUID{}
	ordered := make([]Entity, 0, len(batch.Entities))
	for _, kind := range []string{"PLACE", "CONNECTION_GROUP", "VEHICLE_REFERENCE"} {
		for _, entity := range batch.Entities {
			if entity.Kind == kind {
				ordered = append(ordered, entity)
			}
		}
	}
	if len(ordered) != len(batch.Entities) {
		return "", fmt.Errorf("prepared: unknown kind")
	}
	for _, entity := range ordered {
		entityUUID, err := newUUID()
		if err != nil {
			return "", err
		}
		switch entity.Kind {
		case "PLACE":
			payload := entity.Payload
			name, _ := payload["name"].(string)
			latitude, _ := payload["latitude_decimal"].(string)
			longitude, _ := payload["longitude_decimal"].(string)
			publicAccess, _ := payload["public_access"].(string)
			var points int32
			if pointsFloat, ok := payload["reported_point_count"].(float64); ok {
				points = int32(pointsFloat)
			}
			if _, err := q.StageChargingPlace(ctx, charging.StageChargingPlaceParams{
				ID: entityUUID, BatchID: batchUUID, EntityKey: entity.Key, ExternalID: entity.ExternalID,
				Name: name, Country: "BR", Latitude: latitude, Longitude: longitude,
				PublicAccess: publicAccess, ReportedPoints: points,
			}); err != nil {
				return "", fmt.Errorf("prepared: place refused: %w", err)
			}
			placeIDs[entity.Key] = entityUUID
		case "CONNECTION_GROUP":
			parentKey := ""
			if entity.ParentKey != nil {
				parentKey = *entity.ParentKey
			}
			placeUUID, ok := placeIDs[parentKey]
			if !ok {
				return "", fmt.Errorf("prepared: group without staged place")
			}
			payload := entity.Payload
			connector, _ := payload["connector_code"].(string)
			rawConnector, _ := payload["raw_connector_code"].(string)
			current, _ := payload["current"].(string)
			var power, quantity pgtype.Int4
			if powerFloat, ok := payload["power_w"].(float64); ok {
				power = pgtype.Int4{Int32: int32(powerFloat), Valid: true}
			}
			if quantityFloat, ok := payload["reported_quantity"].(float64); ok {
				quantity = pgtype.Int4{Int32: int32(quantityFloat), Valid: true}
			}
			if _, err := q.StageChargingConnectionGroup(ctx, charging.StageChargingConnectionGroupParams{
				ID: entityUUID, BatchID: batchUUID, PlaceID: placeUUID, EntityKey: entity.Key,
				ExternalID: entity.ExternalID, ConnectorCode: connector, RawConnectorCode: rawConnector,
				Current: current, PowerW: power, ReportedQuantity: quantity,
			}); err != nil {
				return "", fmt.Errorf("prepared: group refused: %w", err)
			}
		case "VEHICLE_REFERENCE":
			payload := entity.Payload
			model, _ := payload["model_code"].(string)
			if _, err := q.StageChargingVehicleReference(ctx, charging.StageChargingVehicleReferenceParams{
				ID: entityUUID, BatchID: batchUUID, EntityKey: entity.Key, ModelCode: model,
			}); err != nil {
				return "", fmt.Errorf("prepared: vehicle refused: %w", err)
			}
		default:
			return "", fmt.Errorf("prepared: unknown kind %s", entity.Kind)
		}
	}
	counts, err := q.ChargingBatchCounts(ctx, batchUUID)
	if err != nil {
		return "", err
	}
	if int(counts.Places)+int(counts.Groups)+int(counts.Vehicles) != len(batch.Entities) {
		return "", fmt.Errorf("prepared: staged census diverged")
	}
	return batchID, nil
}
