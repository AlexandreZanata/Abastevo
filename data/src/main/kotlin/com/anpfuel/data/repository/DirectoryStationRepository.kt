package com.anpfuel.data.repository

import com.anpfuel.data.remote.DirectoryStationHttpClient
import com.anpfuel.data.remote.DirectoryStationJsonCodec
import com.anpfuel.domain.discovery.NearbyServerStation
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.repository.ServerStationGateway
import java.io.IOException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
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

    override suspend fun list(limit: Int, cursor: String?): ServerStationPage = withContext(Dispatchers.IO) {
        val raw = try {
            httpClient.awaitList(limit, cursor)
        } catch (error: IOException) {
            throw error
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            throw IOException("directory list failed", error)
        }
        DirectoryStationJsonCodec.decodeList(raw)
    }

    override suspend fun listInMunicipality(municipalityCode: String, limit: Int, cursor: String?): ServerStationPage =
        withContext(Dispatchers.IO) {
            DirectoryStationJsonCodec.decodeList(httpClient.awaitCityList(municipalityCode, limit, cursor))
        }

    override suspend fun search(municipalityCode: String, query: String, limit: Int): ServerStationPage =
        withContext(Dispatchers.IO) { DirectoryStationJsonCodec.decodeList(httpClient.search(municipalityCode, query, limit)) }

    override suspend fun nearby(
        lat: Double,
        lon: Double,
        radiusMeters: Int,
        limit: Int,
    ): List<NearbyServerStation> = withContext(Dispatchers.IO) {
        val raw = try {
            httpClient.nearby(lat, lon, radiusMeters, limit)
        } catch (error: IOException) {
            throw error
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            throw IOException("directory nearby failed", error)
        }
        DirectoryStationJsonCodec.decodeNearby(raw)
    }

    override suspend fun byCnpj(cnpj: String): ServerStation? =
        withContext(Dispatchers.IO) { httpClient.byCnpj(cnpj)?.let(DirectoryStationJsonCodec::decodeStation) }

    override suspend fun detail(stationId: String): ServerStation = withContext(Dispatchers.IO) {
        val raw = try {
            httpClient.awaitDetail(stationId)
        } catch (error: IOException) {
            throw error
        } catch (error: CancellationException) {
            throw error
        } catch (error: Exception) {
            throw IOException("directory detail failed", error)
        }
        DirectoryStationJsonCodec.decodeStation(raw)
    }
}
