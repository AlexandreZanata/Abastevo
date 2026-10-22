package com.anpfuel.data.local.charging

import com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity
import com.anpfuel.data.local.entity.ChargingSnapshotPlaceEntity
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingConnectionGroup
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingPackMeta
import org.json.JSONArray
import org.json.JSONObject

/**
 * EE-21 storage mapping for validated snapshot rows. Decode failures
 * fail closed (corrupt stored JSON never becomes a place); group JSON
 * uses the fixed key order written by [ChargingSnapshotImport].
 */
object ChargingSnapshotMapper {

    fun toEntity(row: StoredChargingPlace, savedAtMillis: Long): ChargingSnapshotPlaceEntity =
        ChargingSnapshotPlaceEntity(
            placeId = row.placeId, entityKey = row.entityKey, externalId = row.externalId,
            generation = row.generation, profile = row.profile,
            sourceRevision = row.sourceRevision, origin = row.origin, license = row.license,
            name = row.name, country = row.country, latitude = row.latitude,
            longitude = row.longitude, publicAccess = row.publicAccess,
            reportedPoints = row.reportedPoints, groupsJson = row.groupsJson,
            tombstone = row.tombstone, savedAtMillis = savedAtMillis,
        )

    fun toMetaEntity(meta: StoredPackMeta, key: String): ChargingSnapshotMetaEntity =
        ChargingSnapshotMetaEntity(
            key = key, generation = meta.generation, profile = meta.profile,
            sourceRevision = meta.sourceRevision, origin = meta.origin, license = meta.license,
            schemaVersion = meta.schemaVersion, baseGeneration = meta.baseGeneration,
            placeCount = meta.placeCount, groupCount = meta.groupCount, cursor = meta.cursor,
            state = meta.state, savedAtMillis = meta.savedAtMillis,
        )

    fun toDomain(entity: ChargingSnapshotPlaceEntity): ChargingPlace {
        // Identity columns are required on live rows (tombstones never
        // reach this mapping); anything else is stored corruption.
        val entityKey = entity.entityKey ?: throw IllegalStateException("corrupt snapshot identity for ${entity.placeId}")
        val externalId = entity.externalId ?: throw IllegalStateException("corrupt snapshot identity for ${entity.placeId}")
        val groups = try {
            val array = JSONArray(entity.groupsJson)
            (0 until array.length()).map { index ->
                val row = array.getJSONObject(index)
                ChargingConnectionGroup(
                    id = row.getString("id"),
                    entityKey = row.getString("entity_key"),
                    externalId = row.getString("external_id"),
                    connector = ChargingConnector.valueOf(row.getString("connector_code")),
                    rawConnectorCode = row.getString("raw_connector_code"),
                    current = ChargingCurrent.valueOf(row.getString("current")),
                    powerW = if (row.isNull("power_w")) null else row.getLong("power_w"),
                    reportedQuantity = if (row.isNull("reported_quantity")) null else row.getLong("reported_quantity"),
                )
            }
        } catch (e: Exception) {
            throw IllegalStateException("corrupt snapshot groups for ${entity.placeId}")
        }
        return ChargingPlace(
            id = entity.placeId, entityKey = entityKey,
            externalId = externalId, name = entity.name, country = entity.country,
            latitude = entity.latitude, longitude = entity.longitude,
            publicAccess = ChargingAccess.valueOf(entity.publicAccess),
            reportedPoints = entity.reportedPoints, groups = groups,
        )
    }

    fun toPackMeta(meta: ChargingSnapshotMetaEntity, generatedAt: String): ChargingPackMeta =
        ChargingPackMeta(
            profile = meta.profile, sourceRevision = meta.sourceRevision, origin = meta.origin,
            license = meta.license, schemaVersion = meta.schemaVersion,
            baseGeneration = meta.baseGeneration, generatedAt = generatedAt,
        )
}
