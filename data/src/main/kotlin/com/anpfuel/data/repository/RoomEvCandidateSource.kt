package com.anpfuel.data.repository

import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.ev.EvAccess
import com.anpfuel.domain.ev.EvCandidateQuery
import com.anpfuel.domain.ev.EvCandidateResult
import com.anpfuel.domain.ev.EvCandidateSource
import com.anpfuel.domain.ev.EvChargingCandidate
import com.anpfuel.domain.ev.EvCoverage
import com.anpfuel.domain.ev.EvHours
import com.anpfuel.domain.ev.EvProvenance
import com.anpfuel.domain.ev.EvRouteGeometry
import com.anpfuel.domain.ev.EvTariff
import com.anpfuel.domain.map.saved.GeoPoint
import javax.inject.Inject
import javax.inject.Singleton

/**
 * MAP-27 offline candidate source over the admitted charging snapshot.
 *
 * Reads only the active Room generation, bounded by the route box and a row limit. The
 * snapshot has no hours, tariff or operator fields; those stay UNKNOWN and no date is
 * invented from the download time. A missing generation is MISSING, never an empty success.
 */
@Singleton
class RoomEvCandidateSource @Inject constructor(
    private val repository: ChargingDiscoveryRepository,
) : EvCandidateSource {

    override suspend fun candidates(query: EvCandidateQuery): EvCandidateResult {
        val meta = repository.generationMeta(ChargingDiscoveryRepository.OCM_PROFILE)
            ?: return EvCandidateResult(emptyList(), EvCoverage.MISSING)
        val bounds = EvRouteGeometry.boundingBox(query.route, query.corridorRadiusMeters)
        val rows = repository.corridor(
            profile = ChargingDiscoveryRepository.OCM_PROFILE,
            generation = meta.generation,
            minLatitude = bounds.minLatitude,
            maxLatitude = bounds.maxLatitude,
            minLongitude = bounds.minLongitude,
            maxLongitude = bounds.maxLongitude,
            limit = query.limit,
        )
        val candidates = rows.flatMap { row ->
            EvCandidateMapper.from(row.place, query.connector, row.sourceRevision)
        }
        return EvCandidateResult(
            candidates = candidates,
            coverage = EvCoverage.PRESENT,
            generation = meta.generation,
            sourceRevision = meta.sourceRevision,
            savedAtMillis = meta.savedAtMillis,
        )
    }
}

/** Pure snapshot -> MAP-27 candidate mapping; malformed external metadata is skipped, not guessed. */
object EvCandidateMapper {
    fun from(
        place: ChargingPlace,
        connector: com.anpfuel.domain.charging.ChargingConnector,
        sourceRevision: String,
    ): List<EvChargingCandidate> {
        val latitude = place.latitude ?: return emptyList()
        val longitude = place.longitude ?: return emptyList()
        val point = runCatching { GeoPoint.parse(latitude, longitude) }.getOrNull() ?: return emptyList()
        val access = when (place.publicAccess) {
            ChargingAccess.PUBLIC -> EvAccess.PUBLIC
            ChargingAccess.UNKNOWN -> EvAccess.UNKNOWN
        }
        return place.groups.mapNotNull { group ->
            if (group.connector != connector) return@mapNotNull null
            runCatching {
                EvChargingCandidate(
                    candidateId = group.id,
                    siteId = place.id,
                    siteName = place.name,
                    equipmentId = group.entityKey.ifBlank { group.id },
                    connectorId = group.id,
                    connector = group.connector,
                    current = group.current,
                    powerW = group.powerW,
                    access = access,
                    hours = EvHours.Unknown,
                    tariff = EvTariff.Unknown,
                    provenance = EvProvenance.STATIC_REFERENCE,
                    sourceRevision = sourceRevision,
                    observedAt = null,
                    point = point,
                )
            }.getOrNull()
        }
    }
}
