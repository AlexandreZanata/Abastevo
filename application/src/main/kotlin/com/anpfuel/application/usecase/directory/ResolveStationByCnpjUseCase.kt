package com.anpfuel.application.usecase.directory

import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
import kotlinx.coroutines.CancellationException

/** Exact public identity binding. Not-found is authoritative; outages alone replay cache. */
class ResolveStationByCnpjUseCase(
    private val flags: CommunityReadsFlagProvider,
    private val gateway: ServerStationGateway,
    private val cache: ServerStationCache,
) {
    suspend operator fun invoke(cnpj: String): ServerStationDetailOutcome {
        if (!flags.isEnabled()) return ServerStationDetailOutcome.Disabled
        if (!cnpj.matches(Regex("[A-Z0-9]{12}[0-9]{2}"))) {
            return ServerStationDetailOutcome.Unavailable(IllegalArgumentException("invalid CNPJ shape"))
        }
        val station = try {
            gateway.byCnpj(cnpj)
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (error: Exception) {
            val cached = try { cache.loadPage()?.items?.filter { it.cnpjNormalized == cnpj }?.singleOrNull() }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (_: Exception) { null }
            return if (cached == null) ServerStationDetailOutcome.Unavailable(error)
            else ServerStationDetailOutcome.StaleCache(cached, error)
        } ?: return ServerStationDetailOutcome.Unavailable(IllegalArgumentException("station not found"))
        if (station.cnpjNormalized != cnpj) {
            return ServerStationDetailOutcome.Unavailable(IllegalArgumentException("station identity mismatch"))
        }
        cache.saveDetail(station)
        return ServerStationDetailOutcome.Fresh(station)
    }
}
