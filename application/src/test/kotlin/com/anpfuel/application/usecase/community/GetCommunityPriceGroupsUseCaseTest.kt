package com.anpfuel.application.usecase.community

import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.BackendPriceGroups
import com.anpfuel.domain.repository.BackendPriceCacheRepository
import com.anpfuel.domain.repository.BackendPriceHttpGateway
import com.anpfuel.domain.valueobject.FuelProduct
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import java.io.IOException
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertInstanceOf
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class GetCommunityPriceGroupsUseCaseTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private val flag: CommunityReadsFlagProvider = mockk()
    private val http: BackendPriceHttpGateway = mockk()
    private val cache: BackendPriceCacheRepository = mockk(relaxed = true)

    private lateinit var useCase: GetCommunityPriceGroupsUseCase

    private fun freshGroups() = BackendPriceGroups.create(
        stationId = stationId,
        fuelFilterWire = null,
        groups = emptyList(),
        fetchedAtMillis = 1_000_000L,
        expiresAtMillis = 1_060_000L,
    )

    @BeforeEach
    fun setUp() {
        useCase = GetCommunityPriceGroupsUseCase(
            flagProvider = flag,
            httpGateway = http,
            cache = cache,
            nowMillis = { 1_000_001L },
        )
    }

    @Test
    fun `disabled flag never touches network or cache`() = runTest {
        every { flag.isEnabled() } returns false

        val outcome = useCase(stationId)

        assertInstanceOf(CommunityPriceGroupsOutcome.Disabled::class.java, outcome)
        coVerify(exactly = 0) { http.fetchGroups(any(), any()) }
        coVerify(exactly = 0) { cache.load(any(), any()) }
    }

    @Test
    fun `backend success saves fresh groups`() = runTest {
        every { flag.isEnabled() } returns true
        coEvery { http.fetchGroups(stationId, null) } returns freshGroups()

        val outcome = useCase(stationId)

        assertInstanceOf(CommunityPriceGroupsOutcome.Fresh::class.java, outcome)
        coVerify { cache.save(any()) }
    }

    @Test
    fun `backend down with no cache is unavailable`() = runTest {
        every { flag.isEnabled() } returns true
        coEvery { http.fetchGroups(stationId, null) } throws IOException("down")
        coEvery { cache.load(stationId, null) } returns null

        val outcome = useCase(stationId)

        assertInstanceOf(CommunityPriceGroupsOutcome.Unavailable::class.java, outcome)
    }

    @Test
    fun `backend down with stale cache returns stale explicitly`() = runTest {
        val stale = BackendPriceGroups.create(
            stationId = stationId,
            fuelFilterWire = null,
            groups = emptyList(),
            fetchedAtMillis = 1_000_000L,
            expiresAtMillis = 1_060_000L,
        )
        val staleUseCase = GetCommunityPriceGroupsUseCase(
            flagProvider = flag,
            httpGateway = http,
            cache = cache,
            nowMillis = { 2_000_000L },
        )
        every { flag.isEnabled() } returns true
        coEvery { http.fetchGroups(stationId, null) } throws IOException("outage")
        coEvery { cache.load(stationId, null) } returns stale

        val outcome = staleUseCase(stationId)

        assertInstanceOf(CommunityPriceGroupsOutcome.StaleCache::class.java, outcome)
        val staleCache = outcome as CommunityPriceGroupsOutcome.StaleCache
        assertTrue(staleCache.stale)
        assertEquals(stationId, staleCache.groups.stationId)
    }

    @Test
    fun `blank station id fails fast`() = runTest {
        every { flag.isEnabled() } returns true

        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking { useCase("   ", FuelProduct.ETHANOL) }
        }
    }
}
