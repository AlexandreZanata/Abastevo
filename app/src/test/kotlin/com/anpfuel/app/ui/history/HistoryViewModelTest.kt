package com.anpfuel.app.ui.history

import com.anpfuel.application.usecase.network.ObserveNetworkConnectivityUseCase
import com.anpfuel.application.usecase.price.GetPriceHistoryUseCase
import com.anpfuel.application.usecase.price.PriceHistoryOutcome
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import java.util.Locale
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class HistoryViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val getPriceHistoryUseCase = mockk<GetPriceHistoryUseCase>()
    private val observeNetworkConnectivityUseCase = mockk<ObserveNetworkConnectivityUseCase>()

    private val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    private lateinit var viewModel: HistoryViewModel

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        every { observeNetworkConnectivityUseCase.invoke() } returns flowOf(true)
        viewModel = HistoryViewModel(
            getPriceHistoryUseCase = getPriceHistoryUseCase,
            observeNetworkConnectivityUseCase = observeNetworkConnectivityUseCase,
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    private fun average(product: FuelProduct, value: String) = AveragePrice.create(
        priceSurveyId = DomainId.generate(),
        surveyWeek = week,
        state = BrazilianState.PARANA,
        municipality = "Curitiba",
        fuelProduct = product,
        average = PriceAmount.of(value),
    )

    @Test
    fun `loads entries for the selected fuel`() = runTest(dispatcher) {
        coEvery { getPriceHistoryUseCase(fuelProduct = FuelProduct.GASOLINE_REGULAR) } returns
            PriceHistoryOutcome.Success(
                BrazilianState.PARANA,
                "Curitiba",
                FuelProduct.GASOLINE_REGULAR,
                listOf(average(FuelProduct.GASOLINE_REGULAR, "5.19")),
            )

        viewModel.load(Locale.US)
        advanceUntilIdle()

        assertEquals(1, viewModel.uiState.value.entries.size)
        assertEquals(FuelProduct.GASOLINE_REGULAR, viewModel.uiState.value.selectedFuelProduct)
    }

    @Test
    fun `fuel switch supersedes the pending load`() = runTest(dispatcher) {
        val release = kotlinx.coroutines.CompletableDeferred<Unit>()
        coEvery { getPriceHistoryUseCase(fuelProduct = FuelProduct.ETHANOL) } coAnswers {
            kotlinx.coroutines.withContext(kotlinx.coroutines.NonCancellable) { release.await() }
            PriceHistoryOutcome.Success(
                BrazilianState.PARANA,
                "Curitiba",
                FuelProduct.ETHANOL,
                listOf(average(FuelProduct.ETHANOL, "3.49")),
            )
        }
        coEvery { getPriceHistoryUseCase(fuelProduct = FuelProduct.GASOLINE_REGULAR) } returns
            PriceHistoryOutcome.Success(
                BrazilianState.PARANA,
                "Curitiba",
                FuelProduct.GASOLINE_REGULAR,
                listOf(average(FuelProduct.GASOLINE_REGULAR, "5.19")),
            )

        viewModel.onFuelProductSelected(FuelProduct.ETHANOL, Locale.US)
        runCurrent()
        viewModel.onFuelProductSelected(FuelProduct.GASOLINE_REGULAR, Locale.US)
        release.complete(Unit)
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertEquals(FuelProduct.GASOLINE_REGULAR, state.selectedFuelProduct)
        assertEquals(1, state.entries.size)
        assertEquals(
            java.math.BigDecimal("5.19"),
            state.entries.single().averageValue,
        )
    }
}
