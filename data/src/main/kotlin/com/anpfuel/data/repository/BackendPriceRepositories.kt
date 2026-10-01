package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.BackendPriceCacheDao
import com.anpfuel.data.local.entity.BackendPriceCacheEntity
import com.anpfuel.data.mapper.WireFuelMapper
import com.anpfuel.data.remote.BackendPriceGroupsJsonCodec
import com.anpfuel.data.remote.BackendStationPriceHttpClient
import com.anpfuel.domain.model.BackendPriceGroups
import com.anpfuel.domain.repository.BackendPriceCacheRepository
import com.anpfuel.domain.repository.BackendPriceHttpGateway
import com.anpfuel.domain.rule.BackendPriceCacheRule
import com.anpfuel.domain.valueobject.FuelProduct
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P10-T02 data adapters: network-only HTTP gateway plus Room-backed
 * source/version/expiry cache. ANP tables are never touched here.
 */
@Singleton
class BackendPriceHttpGatewayImpl @Inject constructor(
    private val httpClient: BackendStationPriceHttpClient,
) : BackendPriceHttpGateway {

    override suspend fun fetchGroups(
        stationId: String,
        fuelProduct: FuelProduct?,
    ): BackendPriceGroups {
        val wire = fuelProduct?.let(WireFuelMapper::toWire)
        val raw = try {
            httpClient.fetch(stationId, wire)
        } catch (error: IOException) {
            throw error
        }
        val expiresAt = BackendPriceCacheRule.expiryFor(raw.fetchedAtMillis)
        return BackendPriceGroupsJsonCodec.decode(
            payload = raw.body,
            stationId = stationId,
            fuelFilterWire = wire,
            fetchedAtMillis = raw.fetchedAtMillis,
            expiresAtMillis = expiresAt,
        )
    }
}

@Singleton
class BackendPriceCacheRepositoryImpl @Inject constructor(
    private val dao: BackendPriceCacheDao,
) : BackendPriceCacheRepository {

    override suspend fun load(
        stationId: String,
        fuelProduct: FuelProduct?,
    ): BackendPriceGroups? {
        val wire = fuelProduct?.let(WireFuelMapper::toWire)
        val entity = dao.findByKey(BackendPriceCacheEntity.keyFor(stationId, wire))
            ?: return null
        return BackendPriceGroupsJsonCodec.decode(
            payload = entity.payloadJson,
            stationId = entity.stationId,
            fuelFilterWire = entity.fuelFilter.takeIf { it != BackendPriceCacheEntity.ALL_FUELS },
            fetchedAtMillis = entity.fetchedAtMillis,
            expiresAtMillis = entity.expiresAtMillis,
        )
    }

    override suspend fun save(groups: BackendPriceGroups) {
        val wire = groups.fuelFilterWire
        dao.upsert(
            BackendPriceCacheEntity(
                key = BackendPriceCacheEntity.keyFor(groups.stationId, wire),
                stationId = groups.stationId,
                fuelFilter = wire ?: BackendPriceCacheEntity.ALL_FUELS,
                payloadJson = BackendPriceGroupsJsonCodec.encode(groups),
                source = groups.source,
                version = groups.version,
                fetchedAtMillis = groups.fetchedAtMillis,
                expiresAtMillis = groups.expiresAtMillis,
            ),
        )
    }

    override suspend fun clear() {
        dao.clear()
    }
}
