package com.anpfuel.data.repository

import com.anpfuel.data.remote.DirectoryStationHttpClient
import com.anpfuel.data.remote.DirectoryStationJsonCodec
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.repository.ServerStationGateway
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton

/**
 * P35-T01 Directory network gateway (network-only) and last-known
 * memory cache (failed-refresh recovery).
 *
 * The gateway decodes strict server shapes; transport/parse failures
 * throw [IOException]. The cache never invents stations and never
 * rewrites legacy CNPJ rows; it only replays the last good server page
 * or detail. No Room migration in this slice.
 */
@Singleton
class DirectoryStationGatewayImpl @Inject constructor(
    private val httpClient: DirectoryStationHttpClient,
) : ServerStationGateway {

    override suspend fun list(limit: Int, cursor: String?): ServerStationPage {
        val raw = try {
            httpClient.list(limit, cursor)
        } catch (error: IOException) {
            throw error
        } catch (error: Exception) {
            throw IOException("directory list failed", error)
        }
        return DirectoryStationJsonCodec.decodeList(raw)
    }

    override suspend fun nearby(
        lat: Double,
        lon: Double,
        radiusMeters: Int,
        limit: Int,
    ): List<NearbyServerStation> {
        val raw = try {
            httpClient.nearby(lat, lon, radiusMeters, limit)
        } catch (error: IOException) {
            throw error
        } catch (error: Exception) {
            throw IOException("directory nearby failed", error)
        }
        return DirectoryStationJsonCodec.decodeNearby(raw)
    }

    override suspend fun byCnpj(cnpj: String): ServerStation? =
        httpClient.byCnpj(cnpj)?.let(DirectoryStationJsonCodec::decodeStation)

    override suspend fun detail(stationId: String): ServerStation {
        val raw = try {
            httpClient.detail(stationId)
        } catch (error: IOException) {
            throw error
        } catch (error: Exception) {
            throw IOException("directory detail failed", error)
        }
        return DirectoryStationJsonCodec.decodeStation(raw)
    }
}
