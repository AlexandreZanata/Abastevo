package com.anpfuel.application.usecase.directory

import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ResolveStationByCnpjUseCaseTest {
    private val cnpj = "04218406000104"
    private val station = ServerStation.create("d6c74c23-63db-4c24-a2e5-408cb23bad26", "Central",
        StationLocationQuality.UNKNOWN, null, null, cnpj, null, null, null)
    private val gateway = mockk<ServerStationGateway>()
    private val cache = mockk<ServerStationCache>(relaxed = true)
    private fun useCase(enabled: Boolean = true) = ResolveStationByCnpjUseCase(
        object : CommunityReadsFlagProvider { override fun isEnabled() = enabled }, gateway, cache)

    @Test fun exactIdentifierResolvesAndCachesCanonicalStation() = runTest {
        coEvery { gateway.byCnpj(cnpj) } returns station
        assertEquals(ServerStationDetailOutcome.Fresh(station), useCase()(cnpj))
        coVerify { cache.saveDetail(station) }
    }
    @Test fun malformedAndDisabledPerformNoLookup() = runTest {
        assertTrue(useCase()("123") is ServerStationDetailOutcome.Unavailable)
        assertEquals(ServerStationDetailOutcome.Disabled, useCase(false)(cnpj))
        coVerify(exactly = 0) { gateway.byCnpj(any()) }
    }
    @Test fun unknownIdentifierNeverReplaysStaleAssociation() = runTest {
        coEvery { gateway.byCnpj(cnpj) } returns null
        assertTrue(useCase()(cnpj) is ServerStationDetailOutcome.Unavailable)
        coVerify(exactly = 0) { cache.loadPage() }
    }
    @Test fun wrongIdentifierFailsClosedWithoutCacheOrSocialTarget() = runTest {
        coEvery { gateway.byCnpj("11222333000181") } returns station
        assertTrue(useCase()("11222333000181") is ServerStationDetailOutcome.Unavailable)
        coVerify(exactly = 0) { cache.saveDetail(any()) }
    }
    @Test fun outageCanReplayOnlyExactUniqueCachedIdentity() = runTest {
        coEvery { gateway.byCnpj(cnpj) } throws IOException()
        coEvery { cache.loadPage() } returns com.anpfuel.domain.discovery.ServerStationPage(listOf(station), null)
        assertTrue(useCase()(cnpj) is ServerStationDetailOutcome.StaleCache)
    }
    @Test fun outageWithAmbiguousCacheNeverChoosesAnArbitraryStation() = runTest {
        coEvery { gateway.byCnpj(cnpj) } throws IOException()
        coEvery { cache.loadPage() } returns com.anpfuel.domain.discovery.ServerStationPage(listOf(station, station), null)
        assertTrue(useCase()(cnpj) is ServerStationDetailOutcome.Unavailable)
    }
    @Test fun brokenCacheLeavesRecoverableUnavailableState() = runTest {
        coEvery { gateway.byCnpj(cnpj) } throws IOException()
        coEvery { cache.loadPage() } throws IOException()
        assertTrue(useCase()(cnpj) is ServerStationDetailOutcome.Unavailable)
    }

}
