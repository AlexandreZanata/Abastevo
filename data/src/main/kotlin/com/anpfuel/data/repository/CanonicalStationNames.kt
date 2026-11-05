package com.anpfuel.data.repository

import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
import com.anpfuel.domain.valueobject.BrazilianState
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withTimeoutOrNull

/** Current public names overlay historical price rows; prices and their IDs stay intact. */
@Singleton
class CanonicalStationNames @Inject constructor(
    private val gateway: ServerStationGateway,
    private val cache: ServerStationCache,
    private val municipalities: MunicipalityCatalogRepository,
) {
    private val mutex = Mutex()
    private val attemptedAt = mutableMapOf<String, Long>()

    suspend fun enrich(rows: List<StationPrice>, state: BrazilianState, city: String): List<StationPrice> {
        if (rows.isEmpty()) return rows
        val names = try {
            val code = municipalities.findCatalogEntry(state, city)?.ibgeCode ?: return rows
            names(code).filter { it.state == state.abbreviation && it.municipalityCode == code }
                .filter { !it.cnpjNormalized.isNullOrBlank() }.groupBy { it.cnpjNormalized }
                .filterValues { it.size == 1 }.mapValues { it.value.single() }
        } catch (cancelled: CancellationException) { throw cancelled
        } catch (_: Exception) { return rows }
        return rows.map { price ->
            val identity = names[price.station.cnpj.digits] ?: return@map price
            val previous = price.station
            val station = RetailStation.create(previous.cnpj, identity.legalName, identity.displayName,
                identity.addressLine.ifBlank { previous.address }, previous.municipality, previous.state, previous.brand)
            StationPrice.create(price.priceSurveyId, price.surveyWeek, station, price.fuelProduct,
                price.price, price.collectedAt, price.id)
        }
    }

    private suspend fun names(code: String): List<ServerStation> = mutex.withLock {
        val scope = "station-names:$code"
        val cached = cache.loadScopedPage(scope)?.items.orEmpty()
        val now = System.currentTimeMillis()
        if (now - (attemptedAt[code] ?: 0L) < 300_000L) return@withLock cached
        attemptedAt[code] = now
        val fresh = withTimeoutOrNull(6_000L) {
            try {
                val rows = mutableListOf<ServerStation>()
                val cursors = mutableSetOf<String>()
                var cursor: String? = null
                var pages = 0
                do {
                    val page = gateway.listInMunicipality(code, 100, cursor)
                    rows += page.items
                    pages++
                    cursor = page.nextCursor
                    check(cursor == null || (pages < 25 && cursors.add(cursor)))
                } while (cursor != null)
                check(rows.distinctBy { it.stationId }.size == rows.size)
                cache.saveScopedPage(scope, ServerStationPage(rows, null))
                rows
            } catch (cancelled: CancellationException) { throw cancelled
            } catch (_: Exception) { null }
        }
        fresh ?: cached
    }
}
