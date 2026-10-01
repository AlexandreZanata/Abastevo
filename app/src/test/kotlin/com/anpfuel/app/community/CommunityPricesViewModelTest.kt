package com.anpfuel.app.community

import com.anpfuel.application.port.CommunityReadsFlagProvider
import com.anpfuel.application.usecase.community.CommunityPriceGroupsOutcome
import com.anpfuel.application.usecase.community.GetCommunityPriceGroupsUseCase
import com.anpfuel.domain.model.BackendPriceGroups
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/**
 * P10-T06: flag OFF never touches network/cache; official browsing keeps
 * working; stale/unavailable stay explicit.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class CommunityPricesViewModelTest {

    private val dispatcher = StandardTestDispatcher()
    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"

    private val useCase = mockk<GetCommunityPriceGroupsUseCase>()
    private var enabled = false
    private val flags = object : CommunityReadsFlagProvider {
        override fun isEnabled(): Boolean = enabled
    }

    private fun groups() = BackendPriceGroups.create(
        stationId = stationId,
        fuelFilterWire = null,
        groups = emptyList(),
        fetchedAtMillis = 1_000_000L,
        expiresAtMillis = 1_060_000L,
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `disabled flag never calls use case`() = runTest {
        enabled = false
        val vm = CommunityPricesViewModel(useCase, flags)

        vm.load(stationId)
        advanceUntilIdle()

        assertTrue(vm.state.value is CommunityPricesUiState.Disabled)
        coVerify(exactly = 0) { useCase.invoke(any(), any()) }
    }

    @Test
    fun `fresh groups resolve to content`() = runTest {
        enabled = true
        coEvery { useCase.invoke(stationId, null) } returns
            CommunityPriceGroupsOutcome.Fresh(groups())
        val vm = CommunityPricesViewModel(useCase, flags)

        vm.load(stationId)
        advanceUntilIdle()

        assertTrue(vm.state.value is CommunityPricesUiState.Content)
    }

    @Test
    fun `backend down with cache resolves to stale`() = runTest {
        enabled = true
        coEvery { useCase.invoke(stationId, null) } returns
            CommunityPriceGroupsOutcome.StaleCache(groups(), IOException("down"), true)
        val vm = CommunityPricesViewModel(useCase, flags)

        vm.load(stationId)
        advanceUntilIdle()

        assertTrue(vm.state.value is CommunityPricesUiState.Stale)
    }

    @Test
    fun `backend down without cache resolves to unavailable`() = runTest {
        enabled = true
        coEvery { useCase.invoke(stationId, null) } returns
            CommunityPriceGroupsOutcome.Unavailable(IOException("down"))
        val vm = CommunityPricesViewModel(useCase, flags)

        vm.load(stationId)
        advanceUntilIdle()

        assertTrue(vm.state.value is CommunityPricesUiState.Unavailable)
    }
}
