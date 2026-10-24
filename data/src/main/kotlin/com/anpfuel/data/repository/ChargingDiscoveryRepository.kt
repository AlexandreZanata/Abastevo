package com.anpfuel.data.repository

import com.anpfuel.data.local.charging.ChargingSnapshotImport
import com.anpfuel.data.local.charging.ChargingSnapshotMapper
import com.anpfuel.data.local.charging.StoredPackMeta
import com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity
import com.anpfuel.data.local.dao.ChargingSnapshotDao
import com.anpfuel.data.remote.charging.ChargingSnapshotHttpClient
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingCompatibility
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingPackMeta
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.Compatibility
import java.time.Instant
import javax.inject.Inject
import javax.inject.Singleton

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

data class PlaceDetail(
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
 * without ever asserting live state. Only the admitted OCM profile
 * syncs here (PBE rights remain pending); hours/operator are not on
 * the wire and are shown as not-reported, never invented.
 */
@Singleton
class ChargingDiscoveryRepository @Inject constructor(
    private val client: ChargingSnapshotHttpClient,
    private val dao: ChargingSnapshotDao,
    @javax.inject.Named("apiOrigin") environment: com.anpfuel.data.remote.ApiEnvironment,
) {
    private val origin: String = environment.origin

    companion object {
        const val OCM_PROFILE = "ocm-br-20261021-v1"
        const val OCM_LICENSE = "CC-BY-4.0 (provider metadata, attribution required)"
        const val PAGE_SIZE = 100
    }

    suspend fun refresh(
        profile: String,
        revision: String? = null,
        baseGeneration: Long? = null,
        licenseNote: String = OCM_LICENSE,
        nowMillis: Long = Instant.now().toEpochMilli(),
    ): StoredPackMeta {
        if (profile != OCM_PROFILE) throw ChargingDiscoveryUnavailable("pbe-rights-pending")
        val stored = dao.findMeta(key(profile))?.generation ?: 0L
        if (baseGeneration != null && baseGeneration != stored) {
            throw IllegalStateException("pack base $baseGeneration != stored $stored")
        }
        val current = stored
        val meta = ChargingPackMeta(
            profile = profile, sourceRevision = revision ?: currentRevision(profile, current),
            origin = origin, license = licenseNote, schemaVersion = 1,
            baseGeneration = current, generatedAt = Instant.ofEpochMilli(nowMillis).toString(),
        )
        val session = ChargingSnapshotImport.beginLivePack(meta, origin, current)
        var cursor: String? = null
        do {
            val page = client.places(profile, revision, PAGE_SIZE, cursor)
            // Live packs carry no pre-published manifest: the hash is an
            // audit trail over the decoded page, while TLS plus strict
            // decode remain the integrity boundary.
            val raw = page.toString().toByteArray()
            ChargingSnapshotImport.addPage(session, raw, page, shaHex(raw))
            cursor = page.nextCursor
        } while (cursor != null)
        val committed = ChargingSnapshotImport.commitPack(session, emptyList(), null)
        dao.replaceGeneration(
            ChargingSnapshotMapper.toMetaEntity(committed.meta, key(profile)),
            committed.places.map { ChargingSnapshotMapper.toEntity(it, nowMillis) },
        )
        return committed.meta
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

    suspend fun detail(placeId: String, profile: String, generation: Long?): PlaceDetail? {
        val gen = generation ?: dao.findMeta(key(profile))?.generation ?: return null
        val meta = dao.findMeta(key(profile))?.takeIf { it.generation == gen } ?: return null
        val entity = dao.findPlace(placeId, profile, gen) ?: return null
        val place = runCatching { ChargingSnapshotMapper.toDomain(entity) }.getOrNull() ?: return null
        return PlaceDetail(place, profile, meta.sourceRevision, gen, meta.savedAtMillis)
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
