package com.anpfuel.data.local.charging

import com.anpfuel.domain.charging.ChargingCoverage
import com.anpfuel.domain.charging.ChargingPackMeta
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingPlacePage
import java.security.MessageDigest
import org.json.JSONArray
import org.json.JSONObject

/** Admitted public snapshot profiles (EE-09). Licensed-operator or any
 * other profile refuses here, never reaching storage. */
val ADMITTED_CHARGING_PROFILES = setOf("ocm-br-20261021-v1", "pbe-2026-pdf-v1")

private val canonicalId = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")

/** Validated storage row: identity plus generation plus payload or tombstone. */
data class StoredChargingPlace(
    val placeId: String,
    val generation: Long,
    val profile: String,
    val sourceRevision: String,
    val origin: String,
    val license: String,
    val name: String,
    val country: String,
    val latitude: String?,
    val longitude: String?,
    val publicAccess: String,
    val reportedPoints: Long,
    val groupsJson: String,
    val tombstone: Boolean,
    val savedAtMillis: Long,
)

/** Validated generation metadata row. */
data class StoredPackMeta(
    val key: String,
    val generation: Long,
    val profile: String,
    val sourceRevision: String,
    val origin: String,
    val license: String,
    val schemaVersion: Int,
    val baseGeneration: Long,
    val placeCount: Long,
    val groupCount: Long,
    val cursor: String?,
    val state: String,
    val savedAtMillis: Long,
)

data class CommittedPack(
    val meta: StoredPackMeta,
    val places: List<StoredChargingPlace>,
)

/** In-progress pack assembly. Nothing here touches storage; the Room DAO
 * commits a ValidatedPack atomically. */
class PackSession internal constructor(
    val meta: ChargingPackMeta,
    val newGeneration: Long,
    val expectedHashes: List<String>,
    val expectedPlaces: Long,
    private val seenHashes: MutableList<String> = mutableListOf(),
    private val rows: MutableList<StoredChargingPlace> = mutableListOf(),
    private var cursor: String? = null,
) {
    fun resumeCursor(): String? = cursor
    fun pendingPlaces(): Int = rows.size
    internal fun append(hash: String, nextCursor: String?, newRows: List<StoredChargingPlace>) {
        seenHashes += hash
        cursor = nextCursor
        rows += newRows
    }

    internal fun snapshot(): Triple<List<String>, List<StoredChargingPlace>, String?> =
        Triple(seenHashes.toList(), rows.toList(), cursor)
}

/** Pure pack validation for atomic Room generations (EE-18). Every refusal
 * throws before any write, so a corrupt hash, wrong base/source/origin,
 * schema drift, tombstone conflict or coverage mismatch leaves the
 * committed generation — and every fuel/history/private row — untouched. */
object ChargingSnapshotImport {

    const val SCHEMA_VERSION = 1
    const val PLACES_BUDGET = 10_000
    const val PAGE_BYTES_BUDGET = 1024 * 1024
    const val SNAPSHOT_BYTES_BUDGET = 8L * 1024 * 1024

    fun packKey(profile: String): String = "pack:$profile"

    fun beginPack(
        meta: ChargingPackMeta,
        expectedOrigin: String,
        currentGeneration: Long,
        expectedHashes: List<String>,
        expectedPlaces: Long,
    ): PackSession {
        check(meta.profile in ADMITTED_CHARGING_PROFILES) { "profile ${meta.profile} not admitted" }
        check(meta.origin == expectedOrigin) { "pack origin mismatch" }
        check(meta.schemaVersion == SCHEMA_VERSION) { "pack schema ${meta.schemaVersion} unsupported" }
        check(meta.license.isNotBlank()) { "pack license missing" }
        check(meta.baseGeneration == currentGeneration) { "pack base ${meta.baseGeneration} != current $currentGeneration" }
        check(expectedPlaces in 0..PLACES_BUDGET) { "pack size out of budget" }
        check(expectedHashes.all { it.length == 64 }) { "pack manifest hash malformed" }
        return PackSession(meta, currentGeneration + 1, expectedHashes, expectedPlaces)
    }

    fun addPage(session: PackSession, rawBytes: ByteArray, page: ChargingPlacePage, sha256Hex: String) {
        check(rawBytes.size <= PAGE_BYTES_BUDGET) { "page over byte budget" }
        check(sha256Hex == sha256Hex(rawBytes)) { "page hash mismatch" }
        val (seen, rows, _) = session.snapshot()
        check(seen.size < 1_000) { "too many pages" }
        val known = rows.map { it.placeId }.toSet()
        val fresh: List<String> = page.items.map { item: ChargingPlace -> item.id }
        check(fresh.distinct().size == fresh.size) { "duplicate place in page" }
        check(fresh.none { it in known }) { "page overlaps committed pages" }
        val additions = page.items.map { toStored(it, session) }
        val estimated = estimateBytes(rows + additions)
        check(rows.size + additions.size <= PLACES_BUDGET) { "snapshot places over budget" }
        check(estimated <= SNAPSHOT_BYTES_BUDGET) { "snapshot bytes over budget" }
        session.append(sha256Hex, page.nextCursor, additions)
    }

    fun commitPack(session: PackSession, tombstonedIds: List<String>, coverage: ChargingCoverage?): CommittedPack {
        val (seen, rows, cursor) = session.snapshot()
        check(seen == session.expectedHashes) { "pack manifest incomplete" }
        check(rows.size.toLong() == session.expectedPlaces) { "pack places mismatch" }
        tombstonedIds.forEach { check(canonicalId.matches(it)) { "tombstone id malformed" } }
        val added = rows.map { it.placeId }.toSet()
        check(tombstonedIds.none { it in added }) { "tombstone overlaps added place" }
        check(tombstonedIds.distinct().size == tombstonedIds.size) { "duplicate tombstone" }
        if (coverage != null) {
            check(coverage.profile == session.meta.profile) { "coverage profile mismatch" }
            check(coverage.sourceRevision == session.meta.sourceRevision) { "coverage revision mismatch" }
            check(coverage.places >= rows.size) { "coverage smaller than pack" }
        }
        val now = System.currentTimeMillis()
        val tombstones = tombstonedIds.map { id ->
            StoredChargingPlace(id, session.newGeneration, session.meta.profile, session.meta.sourceRevision,
                session.meta.origin, session.meta.license, "", "BR", null, null, "UNKNOWN", 0, "[]", true, now)
        }
        val groupCount = rows.sumOf { countGroups(it.groupsJson) }
        val meta = StoredPackMeta(
            key = packKey(session.meta.profile), generation = session.newGeneration,
            profile = session.meta.profile, sourceRevision = session.meta.sourceRevision,
            origin = session.meta.origin, license = session.meta.license,
            schemaVersion = SCHEMA_VERSION, baseGeneration = session.meta.baseGeneration,
            placeCount = (rows.size + tombstones.size).toLong(), groupCount = groupCount,
            cursor = cursor, state = "COMPLETE", savedAtMillis = now,
        )
        check(cursor == null) { "pack cursor not drained" }
        return CommittedPack(meta, rows + tombstones)
    }

    private fun toStored(place: ChargingPlace, session: PackSession): StoredChargingPlace {
        val groups = JSONArray()
        place.groups.forEach { group ->
            groups.put(JSONObject()
                .put("id", group.id)
                .put("entity_key", group.entityKey)
                .put("external_id", group.externalId)
                .put("connector_code", group.connector.name)
                .put("raw_connector_code", group.rawConnectorCode)
                .put("current", group.current.name)
                .put("power_w", group.powerW)
                .put("reported_quantity", group.reportedQuantity))
        }
        return StoredChargingPlace(
            placeId = place.id, generation = session.newGeneration,
            profile = session.meta.profile, sourceRevision = session.meta.sourceRevision,
            origin = session.meta.origin, license = session.meta.license,
            name = place.name, country = place.country, latitude = place.latitude,
            longitude = place.longitude, publicAccess = place.publicAccess.name,
            reportedPoints = place.reportedPoints, groupsJson = groups.toString(),
            tombstone = false, savedAtMillis = System.currentTimeMillis(),
        )
    }

    private fun countGroups(groupsJson: String): Long = try {
        JSONArray(groupsJson).length().toLong()
    } catch (_: Exception) {
        0
    }

    private fun estimateBytes(rows: List<StoredChargingPlace>): Long =
        rows.sumOf { (it.name.length + it.groupsJson.length + 512).toLong() }

    private fun sha256Hex(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
}
