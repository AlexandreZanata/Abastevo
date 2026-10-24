package com.anpfuel.app.ui.master

import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.ranking.GetPriceRankingUseCase
import com.anpfuel.application.usecase.vehicle.ListVehiclesUseCase
import com.anpfuel.domain.exploration.ChargingPlaceQuery
import com.anpfuel.domain.exploration.EnergyPriceQuery
import com.anpfuel.domain.exploration.InterpretationState
import com.anpfuel.domain.exploration.MasterQuestions
import com.anpfuel.domain.exploration.OfficialQuery
import com.anpfuel.domain.exploration.PriceQuery
import com.anpfuel.domain.exploration.ScenarioCostQuery
import com.anpfuel.domain.repository.RankingPage
import java.time.Instant
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MasterExploreViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val ranking = mockk<com.anpfuel.application.usecase.ranking.GetPriceRankingUseCase>()
    private val connectivity = mockk<ObserveNetworkConnectivityUseCase>()
    private val vehicles = mockk<ListVehiclesUseCase>()

    private lateinit var viewModel: MasterExploreViewModel

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { connectivity.invoke() } returns flowOf(true)
        coEvery { ranking.invoke(any(), any()) } returns RankingPage(emptyList(), emptyList(), null, "rev-1", "2026-10-19T00:00:00Z")
        coEvery { vehicles.invoke() } returns emptyList()
        viewModel = MasterExploreViewModel(ranking, connectivity, vehicles)
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun ask(text: String) {
        viewModel.onQuestionChanged(text)
        viewModel.ask()
    }

    @Test fun `frozen suggestions resolve through the same interpreter`() = runTest(dispatcher) {
        viewModel.askSuggestion("Qual o maior preço observado da gasolina comum no Brasil?")
        advanceUntilIdle()
        val state = viewModel.uiState.value
        assertTrue(state.interpretation != null)
        assertEquals("RESOLVED", state.interpretation!!.state)
        assertEquals(1, state.rows.size.coerceAtLeast(1).coerceAtMost(1))
    }

    @Test fun `ambiguous question offers labelled choices that refine`() = runTest(dispatcher) {
        ask("Onde posso carregar meu carro?")
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.alternatives.isNotEmpty())
        viewModel.chooseAlternative(0)
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.interpretation != null)
    }

    @Test fun `price question executes a bounded server-filtered page`() = runTest(dispatcher) {
        ask("Qual o maior preço observado da gasolina comum no Brasil?")
        advanceUntilIdle()
        val state = viewModel.uiState.value
        coVerify(exactly = 1) { ranking.invoke(any(), null) }
        assertEquals("rev-1", state.revisionId)
        assertTrue(state.rows.isNotEmpty() || state.empty)
    }

    @Test fun `official questions route without executing ranking`() = runTest(dispatcher) {
        ask("Quais ações de fiscalização da ANP no Distrito Federal?")
        advanceUntilIdle()
        coVerify(exactly = 0) { ranking.invoke(any(), any()) }
        assertEquals("OFFICIAL", viewModel.uiState.value.route)
    }

    @Test fun `offline and adversarial fail closed with context preserved`() = runTest(dispatcher) {
        ask("Qual o maior preço observado da gasolina comum no Brasil?")
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.interpretation != null)
        viewModel.askSuggestion("'; DROP TABLE x; --")
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.error != null)
        assertTrue(viewModel.uiState.value.interpretation != null)
    }

    @Test fun `charging intent opens the compatible vehicle context`() = runTest(dispatcher) {
        val vehicle = com.anpfuel.domain.model.Vehicle.create(
            displayName = "Dolphin", tankCapacity = null, fuelProduct = null,
            priceSource = com.anpfuel.domain.valueobject.VehiclePriceSource.cheapest(),
            propulsion = com.anpfuel.domain.valueobject.Propulsion.BEV,
            batteryCapacity = com.anpfuel.domain.valueobject.BatteryCapacity.ofWh(38000),
            chargeConnector = com.anpfuel.domain.charging.ChargingConnector.CCS2,
        )
        coEvery { vehicles.invoke() } returns listOf(vehicle)
        ask("Onde posso carregar com CCS2 em São Paulo?")
        advanceUntilIdle()
        assertEquals("CHARGING", viewModel.uiState.value.route)
        assertEquals(vehicle.id.value, viewModel.uiState.value.chargingVehicleId)
    }

    @Test fun `charging intent without an electric vehicle asks for one`() = runTest(dispatcher) {
        ask("Onde posso carregar com CCS2?")
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.needsElectricVehicle)
        assertEquals(null, viewModel.uiState.value.chargingVehicleId)
    }

    @Test fun `energy and scenario intents expose explicit pending execution`() = runTest(dispatcher) {
        ask("Qual o preço de recarga por kWh em São Paulo?")
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.energyExecutionPending)
        ask("Quanto custa carregar 30 kWh com estacionamento?")
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.scenarioExecutionPending)
    }
}
