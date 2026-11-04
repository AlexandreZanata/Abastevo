package com.anpfuel.data.repository

import com.anpfuel.data.local.charging.NATIONAL_CHARGING_PROFILE
import com.anpfuel.data.local.charging.MATO_GROSSO_PILOT_PROFILE
import com.anpfuel.data.local.charging.SORRISO_PILOT_PROFILE
import com.anpfuel.data.local.charging.SORRISO_SUPPLEMENT_PROFILE
import com.anpfuel.data.local.charging.ChargingSnapshotImport
import com.anpfuel.data.local.charging.ChargingSnapshotMapper
import com.anpfuel.data.local.charging.StoredPackMeta
import com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity
import com.anpfuel.data.local.dao.ChargingSnapshotDao
import com.anpfuel.data.remote.ApiEnvironment
import com.anpfuel.data.remote.charging.ChargingSnapshotHttpClient
import com.anpfuel.data.remote.charging.ChargingSnapshotUnavailable
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingCatalogOffer
import com.anpfuel.domain.charging.ChargingCompatibility
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingPackMeta
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingPlacePage
import com.anpfuel.domain.charging.Compatibility
import com.anpfuel.domain.discovery.StationPageIdentity
import java.io.IOException
import java.time.Instant
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONObject

class ChargingDiscoveryUnavailable(val code: String) : IllegalStateException(code)

/** One cached public place with its compatibility verdict. Availability
 * is always CACHED_UNKNOWN: an offline snapshot never claims live
 * AVAILABLE or a guaranteed fast charge. */
data class DiscoveryRow(
    val place: ChargingPlace,
    val verdict: Compatibility,
    val profile: String,
    val sourceRevision: String,
    val generation: Long,
    val savedAtMillis: Long,
)

data class ChargingExplorePage(val rows: List<DiscoveryRow>, val nextOffset: Int?)

data class PlaceDetail(
    val place: ChargingPlace,
    val profile: String,
    val sourceRevision: String,
    val generation: Long,
    val savedAtMillis: Long,
)

/** MAP-27 bounded corridor row: identity and provenance travel with the place; no live claim. */
data class CorridorRow(
    val place: ChargingPlace,
    val profile: String,
    val sourceRevision: String,
    val generation: Long,
    val savedAtMillis: Long,
)

/**
 * EE-21 compatible discovery over EE-18 snapshot generations.
 *
 * Refresh pulls bounded EE-09 pages, validates them through the pure
 * pack planner and commits one atomic Room generation; any failure
 * leaves the previous generation intact. Reads serve Room only (lazy
 * windows, resumable cursors), so offline listings stay fully usable
 * without ever asserting live state. The legacy OCM and the verified
 * national profile sync here. PBE rights remain pending.
 */
@Singleton
class ChargingDiscoveryRepository @Inject constructor(
    private val client: ChargingSnapshotHttpClient,
    private val dao: ChargingSnapshotDao,
    @javax.inject.Named("apiOrigin") private val environment: ApiEnvironment,
) {
    private val origin: String = environment.origin
    private val pilotRefresh = Mutex()
    private val pilotRetryAfterMillis = mutableMapOf<String, Long>()

    companion object {
        const val OCM_PROFILE = "ocm-br-20261021-v1"
        const val OCM_LICENSE = "CC-BY-4.0 (provider metadata, attribution required)"
        const val NATIONAL_PROFILE = NATIONAL_CHARGING_PROFILE
        private const val NATIONAL_LICENSE = "ODbL-1.0 and source-specific OCM provider terms"
        private const val PILOT_SOURCE = "Abastevo staging pilot; independently reviewed public site facts"
        private const val MT_PILOT_SOURCE = "ODbL-1.0; OpenStreetMap and Open Charge Map source attribution in pilot manifest"
        private const val SORRISO_SUPPLEMENT_SOURCE = "ODbL-1.0; OpenStreetMap and public charging evidence in the Sorriso manifest"
        private const val PILOT_BOOTSTRAP_REVISION = "2026-11-04"
        private val PILOT_BOOTSTRAP_IDS = listOf(
            "67c3d1f0-5898-54f0-bca3-689e9b48b099",
            "451a0ea1-2a8c-5ed3-98ae-a6d11753dbdb",
        )
        const val PAGE_SIZE = 100
    }

    private val mtPilotIDs by lazy { pilotManifestIDs(MATO_GROSSO_PILOT_PROFILE, "mt-top10-pilot-v1.json") }
    private val sorrisoSupplementIDs by lazy {
        pilotManifestIDs(SORRISO_SUPPLEMENT_PROFILE, "sorriso-supplement-v1.json")
    }

    private fun pilotManifestIDs(profile: String, filename: String): List<String> {
        val resource = checkNotNull(javaClass.getResourceAsStream("/charging/$filename"))
        val manifest = JSONObject(resource.bufferedReader().use { it.readText() })
        check(manifest.getString("profile") == profile &&
            manifest.getString("revision") == PILOT_BOOTSTRAP_REVISION)
        val rows = manifest.getJSONArray("places")
        check(rows.length() in 1..PAGE_SIZE)
        return (0 until rows.length()).map { rows.getJSONObject(it).getString("id") }.also { ids ->
            check(ids.distinct().size == ids.size && ids.none { it in PILOT_BOOTSTRAP_IDS })
        }
    }

    suspend fun sorrisoPilotForMap(): StoredPackMeta? =
        pilotForMap(SORRISO_PILOT_PROFILE, PILOT_SOURCE)

    suspend fun matoGrossoPilotForMap(): StoredPackMeta? =
        pilotForMap(MATO_GROSSO_PILOT_PROFILE, MT_PILOT_SOURCE)

    suspend fun sorrisoSupplementForMap(): StoredPackMeta? =
        pilotForMap(SORRISO_SUPPLEMENT_PROFILE, SORRISO_SUPPLEMENT_SOURCE)

    private suspend fun pilotForMap(profile: String, source: String): StoredPackMeta? {
        if (environment != ApiEnvironment.STAGING) return null
        return pilotRefresh.withLock {
            val cached = generationMeta(profile)?.takeIf { it.placeCount > 0 }
            val now = Instant.now().toEpochMilli()
            if (now < (pilotRetryAfterMillis[profile] ?: 0L) ||
                (cached != null && now - cached.savedAtMillis < 900_000)) {
                return@withLock cached
            }
            pilotRetryAfterMillis[profile] = now + 60_000
            try {
                refresh(profile, licenseNote = source, nowMillis = now)
            } catch (cancelled: CancellationException) {
                pilotRetryAfterMillis.remove(profile)
                throw cancelled
            } catch (_: IOException) {
                cached
            } catch (_: IllegalStateException) {
                cached
            }
        }
    }

    suspend fun refresh(
        profile: String,
        revision: String? = null,
        baseGeneration: Long? = null,
        licenseNote: String = OCM_LICENSE,
        nowMillis: Long = Instant.now().toEpochMilli(),
    ): StoredPackMeta {
        val pilot = profile == SORRISO_PILOT_PROFILE || profile == MATO_GROSSO_PILOT_PROFILE ||
            profile == SORRISO_SUPPLEMENT_PROFILE
        if (profile != OCM_PROFILE && profile != NATIONAL_CHARGING_PROFILE &&
            !(pilot && environment == ApiEnvironment.STAGING)) {
            throw ChargingDiscoveryUnavailable("pbe-rights-pending")
        }
        val reviewedIDs = when (profile) {
            MATO_GROSSO_PILOT_PROFILE -> mtPilotIDs
            SORRISO_SUPPLEMENT_PROFILE -> sorrisoSupplementIDs
            else -> null
        }
        val stored = dao.findMeta(key(profile))?.generation ?: 0L
        if (baseGeneration != null && baseGeneration != stored) {
            throw IllegalStateException("pack base $baseGeneration != stored $stored")
        }
        val coverage = if (profile == NATIONAL_CHARGING_PROFILE) client.coverage(profile) else null
        if (coverage != null) {
            val expectedOffer = ChargingCatalogOffer.forRevision(coverage.sourceRevision)
            check(coverage.profile == profile && expectedOffer != null && coverage.offer == expectedOffer &&
                coverage.places in 1..ChargingSnapshotImport.PLACES_BUDGET.toLong() &&
                (revision == null || revision == coverage.sourceRevision)) { "national coverage refused" }
        }
        val requestedRevision = coverage?.sourceRevision ?: revision
        val current = stored
        val meta = ChargingPackMeta(
            profile = profile, sourceRevision = requestedRevision ?: if (pilot) {
                PILOT_BOOTSTRAP_REVISION
            } else currentRevision(profile, current),
            origin = origin, license = if (coverage == null) licenseNote else NATIONAL_LICENSE, schemaVersion = 1,
            baseGeneration = current, generatedAt = Instant.ofEpochMilli(nowMillis).toString(),
        )
        val session = ChargingSnapshotImport.beginLivePack(meta, origin, current)
        var cursor: String? = null
        do {
            val page = try {
                client.places(profile, requestedRevision, PAGE_SIZE, cursor)
            } catch (unavailable: ChargingSnapshotUnavailable) {
                if (!pilot || requestedRevision != null || cursor != null ||
                    unavailable.httpStatus != 400) throw unavailable
                bootstrapPilot(profile, reviewedIDs ?: PILOT_BOOTSTRAP_IDS)
            }
            // Live packs carry no pre-published manifest: the hash is an
            // audit trail over the decoded page, while TLS plus strict
            // decode remain the integrity boundary.
            val raw = page.toString().toByteArray()
            ChargingSnapshotImport.addPage(session, raw, page, shaHex(raw))
            cursor = page.nextCursor
        } while (cursor != null)
        val committed = ChargingSnapshotImport.commitPack(session, emptyList(), coverage)
        if (reviewedIDs != null) {
            check(committed.places.map { it.placeId }.toSet() == reviewedIDs.toSet()) { "Mato Grosso pilot census refused" }
        }
        if (coverage != null) {
            check(committed.meta.placeCount == coverage.places && committed.meta.groupCount == coverage.groups) {
                "national snapshot counts differ from coverage"
            }
            val currentCoverage = client.coverage(profile)
            check(currentCoverage.copy(generatedAt = coverage.generatedAt) == coverage) {
                "national generation changed during refresh"
            }
        }
        dao.replaceGeneration(
            ChargingSnapshotMapper.toMetaEntity(committed.meta, key(profile)),
            committed.places.map { ChargingSnapshotMapper.toEntity(it, nowMillis) },
        )
        return committed.meta
    }

    /** Compatibility with the staging API until it serves the pilot page. */
    private suspend fun bootstrapPilot(profile: String, ids: List<String>): ChargingPlacePage {
        val coverage = client.coverage(profile)
        check(coverage.profile == profile && coverage.sourceRevision == PILOT_BOOTSTRAP_REVISION &&
            coverage.places == ids.size.toLong() && coverage.groups == 0L) {
            "staging pilot bootstrap edition refused"
        }
        val places = coroutineScope {
            ids.chunked(6).flatMap { batch ->
                batch.map { id ->
                    async { client.placeDetail(id).also { check(it.id == id) { "staging pilot place identity changed" } } }
                }.awaitAll()
            }
        }
        return ChargingPlacePage(places, null, coverage.generatedAt)
    }

    suspend fun list(
        profile: String,
        generation: Long?,
        connector: ChargingConnector?,
        current: ChargingCurrent?,
        access: ChargingAccess?,
        limit: Int,
        offset: Int,
    ): List<DiscoveryRow> {
        val gen = generation ?: dao.findMeta(key(profile))?.generation ?: return emptyList()
        val meta = dao.findMeta(key(profile))?.takeIf { it.generation == gen } ?: return emptyList()
        return dao.readPlaces(profile, gen, limit.coerceIn(1, 100), offset.coerceAtLeast(0))
            .mapNotNull { entity ->
                val place = runCatching { ChargingSnapshotMapper.toDomain(entity) }.getOrNull() ?: return@mapNotNull null
                if (!ChargingCompatibility.matches(place, connector, current, access)) return@mapNotNull null
                DiscoveryRow(
                    place = place,
                    verdict = ChargingCompatibility.assess(connector, place.groups),
                    profile = profile,
                    sourceRevision = meta.sourceRevision,
                    generation = gen,
                    savedAtMillis = meta.savedAtMillis,
                )
            }
    }

    /** Continue at the next matching row, even across sparse raw windows. */
    suspend fun explorePage(
        profile: String,
        generation: Long,
        connector: ChargingConnector?,
        current: ChargingCurrent?,
        access: ChargingAccess?,
        minPowerW: Long?,
        limit: Int,
        offset: Int,
    ): ChargingExplorePage {
        val meta = dao.findMeta(key(profile))?.takeIf { it.generation == generation }
            ?: return ChargingExplorePage(emptyList(), null)
        val size = limit.coerceIn(1, PAGE_SIZE)
        val matches = ArrayList<DiscoveryRow>(size)
        var rawOffset = offset.coerceAtLeast(0)
        while (true) {
            val window = dao.readPlaces(profile, generation, PAGE_SIZE, rawOffset)
            for ((index, entity) in window.withIndex()) {
                val place = runCatching { ChargingSnapshotMapper.toDomain(entity) }.getOrNull() ?: continue
                if (!ChargingCompatibility.matches(place, connector, current, access, minPowerW)) continue
                if (matches.size == size) return ChargingExplorePage(matches, rawOffset + index)
                matches += DiscoveryRow(
                    place, ChargingCompatibility.assess(connector, place.groups), profile,
                    meta.sourceRevision, generation, meta.savedAtMillis,
                )
            }
            if (window.size < PAGE_SIZE) return ChargingExplorePage(matches, null)
            rawOffset += window.size
        }
    }

    suspend fun linkedToStation(stationId: String): List<PlaceDetail> {
        val canonical = StationPageIdentity.parse(stationId) as? StationPageIdentity.Canonical ?: return emptyList()
        return dao.readLinkedPlaces(key(NATIONAL_PROFILE), canonical.stationId).mapNotNull { row ->
            val place = runCatching { ChargingSnapshotMapper.toDomain(row) }.getOrNull() ?: return@mapNotNull null
            PlaceDetail(place, row.profile, row.sourceRevision, row.generation, row.savedAtMillis)
        }
    }

    suspend fun detail(placeId: String, profile: String, generation: Long?): PlaceDetail? {
        val gen = generation ?: dao.findMeta(key(profile))?.generation ?: return null
        val meta = dao.findMeta(key(profile))?.takeIf { it.generation == gen } ?: return null
        val entity = dao.findPlace(placeId, profile, gen) ?: return null
        val place = runCatching { ChargingSnapshotMapper.toDomain(entity) }.getOrNull() ?: return null
        return PlaceDetail(place, profile, meta.sourceRevision, gen, meta.savedAtMillis)
    }

    suspend fun corridor(
        profile: String,
        generation: Long,
        minLatitude: Double,
        maxLatitude: Double,
        minLongitude: Double,
        maxLongitude: Double,
        limit: Int,
    ): List<CorridorRow> {
        val meta = dao.findMeta(key(profile))?.takeIf { it.generation == generation } ?: return emptyList()
        return dao.readPlacesInBounds(
            profile, generation, minLatitude, maxLatitude, minLongitude, maxLongitude, limit.coerceIn(1, 500),
        ).mapNotNull { entity ->
            val place = runCatching { ChargingSnapshotMapper.toDomain(entity) }.getOrNull() ?: return@mapNotNull null
            CorridorRow(place, profile, meta.sourceRevision, generation, meta.savedAtMillis)
        }
    }

    suspend fun generationMeta(profile: String): StoredPackMeta? {
        val meta = dao.findMeta(key(profile)) ?: return null
        return StoredPackMeta(
            key = meta.key, generation = meta.generation, profile = meta.profile,
            sourceRevision = meta.sourceRevision, origin = meta.origin, license = meta.license,
            schemaVersion = meta.schemaVersion, baseGeneration = meta.baseGeneration,
            placeCount = meta.placeCount, groupCount = meta.groupCount, cursor = meta.cursor,
            state = meta.state, savedAtMillis = meta.savedAtMillis,
        )
    }

    private suspend fun currentRevision(profile: String, generation: Long): String {
        val meta = dao.findMeta(key(profile))
        if (meta != null && meta.generation == generation) return meta.sourceRevision
        return "unknown"
    }

    private fun key(profile: String) = ChargingSnapshotMetaEntity.packKey(profile)

    private fun shaHex(bytes: ByteArray): String {
        val digest = java.security.MessageDigest.getInstance("SHA-256").digest(bytes)
        return digest.joinToString("") { "%02x".format(it) }
    }
}
