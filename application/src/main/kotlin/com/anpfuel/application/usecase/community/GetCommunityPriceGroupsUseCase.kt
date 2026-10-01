package com.anpfuel.application.usecase.community

import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.BackendPriceGroups
import com.anpfuel.domain.repository.BackendPriceCacheRepository
import com.anpfuel.domain.repository.BackendPriceHttpGateway
import com.anpfuel.domain.rule.BackendPriceCacheRule
import com.anpfuel.domain.valueobject.FuelProduct

/**
 * P10-T02 — Backend community/official reads with explicit offline fallback.
 *
 * Flag disabled: [Outcome.Disabled], caller keeps the local ANP path.
 * Flag enabled + backend ok: [Outcome.Fresh] (saved to cache).
 * Backend down + cached entry: [Outcome.StaleCache] (stale flag explicit,
 * never presented as fresh). Backend down + no cache: [Outcome.Unavailable].
 */
sealed interface CommunityPriceGroupsOutcome {
    data object Disabled : CommunityPriceGroupsOutcome
    data class Fresh(val groups: BackendPriceGroups) : CommunityPriceGroupsOutcome
    data class StaleCache(
        val groups: BackendPriceGroups,
        val cause: Throwable,
        val stale: Boolean,
    ) : CommunityPriceGroupsOutcome
    data class Unavailable(val cause: Throwable) : CommunityPriceGroupsOutcome
}

class GetCommunityPriceGroupsUseCase(
    private val flagProvider: CommunityReadsFlagProvider,
    private val httpGateway: BackendPriceHttpGateway,
    private val cache: BackendPriceCacheRepository,
    private val nowMillis: () -> Long = { System.currentTimeMillis() },
) {
    suspend operator fun invoke(
        stationId: String,
        fuelProduct: FuelProduct? = null,
    ): CommunityPriceGroupsOutcome {
        if (!flagProvider.isEnabled()) {
            return CommunityPriceGroupsOutcome.Disabled
        }
        if (stationId.isBlank()) {
            throw DomainException("station_id is blank")
        }
        try {
            val fresh = httpGateway.fetchGroups(stationId, fuelProduct)
            cache.save(fresh)
            return CommunityPriceGroupsOutcome.Fresh(fresh)
        } catch (error: Exception) {
            val cached = try {
                cache.load(stationId, fuelProduct)
            } catch (_: Exception) {
                null
            }
            if (cached != null) {
                val stale = BackendPriceCacheRule.isStale(nowMillis(), cached.expiresAtMillis)
                return CommunityPriceGroupsOutcome.StaleCache(cached, error, stale)
            }
            return CommunityPriceGroupsOutcome.Unavailable(error)
        }
    }
}
