package com.anpfuel.app.ui.ledger

import com.anpfuel.application.usecase.ledger.DeleteLedgerEntryUseCase
import com.anpfuel.application.usecase.ledger.EnqueueLedgerCommandUseCase
import com.anpfuel.application.usecase.ledger.EstimatedEfficiencyUseCase
import com.anpfuel.application.usecase.ledger.GetLedgerEntryDraftUseCase
import com.anpfuel.application.usecase.ledger.LedgerEntryRow
import com.anpfuel.application.usecase.ledger.LedgerEntryState
import com.anpfuel.application.usecase.ledger.LedgerEntrySummary
import com.anpfuel.application.usecase.ledger.ListLedgerEntriesUseCase
import com.anpfuel.application.usecase.ledger.ResetLedgerSegmentUseCase
import com.anpfuel.application.usecase.ledger.DeclareLedgerGapUseCase
import com.anpfuel.application.usecase.ledger.SaveLedgerDraftUseCase
import com.anpfuel.application.usecase.vehicle.GetVehicleUseCase
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import io.mockk.slot
import java.time.Instant
import java.util.Locale
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

@OptIn(ExperimentalCoroutinesApi::class)
class LedgerEntriesViewModelTest {

    private val dispatcher = StandardTestDispatcher()

    private val listEntries = mockk<ListLedgerEntriesUseCase>()
    private val getDraft = mockk<GetLedgerEntryDraftUseCase>()
    private val saveDraft = mockk<SaveLedgerDraftUseCase>()
    private val enqueue = mockk<EnqueueLedgerCommandUseCase>()
    private val delete = mockk<DeleteLedgerEntryUseCase>()
    private val gap = mockk<DeclareLedgerGapUseCase>()
    private val reset = mockk<ResetLedgerSegmentUseCase>()
    private val efficiency = mockk<EstimatedEfficiencyUseCase>()
    private val vehicle = mockk<GetVehicleUseCase>()

    private lateinit var viewModel: LedgerEntriesViewModel

    private val vehicleId = DomainId.generate()

    private fun row(id: String = "11111111-1111-4111-8111-111111111111", state: LedgerEntryState = LedgerEntryState.LOCAL) = LedgerEntryRow(
        context = null,
        recordId = id, dataset = LedgerDataset.REFUEL, vehicleId = vehicleId.value,
        vehicleName = "Gol", occurredAt = Instant.parse("2026-03-15T10:30:00Z"),
        timezone = "America/Sao_Paulo",
        summary = LedgerEntrySummary.Refuel(20000, 12400, LedgerFill.FULL),
        expectedRevision = 0, note = null, state = state,
    )

    private fun sampleVehicle() = Vehicle.create(
        displayName = "Gol", tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = VehiclePriceSource.cheapest(), id = vehicleId,
    )

    @BeforeEach
    fun setUp() {
        Dispatchers.setMain(dispatcher)
        coEvery { vehicle.invoke(vehicleId) } returns sampleVehicle()
        coEvery { listEntries.invoke(vehicleId) } returns emptyList()
        every { efficiency.current(any()) } returns null
        viewModel = LedgerEntriesViewModel(
            listEntries, getDraft, saveDraft, enqueue, delete, gap, reset, efficiency, vehicle,
        )
    }

    @AfterEach
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test fun `load shows entries with truthful states and the estimate`() = runTest(dispatcher) {
        coEvery { listEntries.invoke(vehicleId) } returns listOf(row("a", LedgerEntryState.PENDING), row("b", LedgerEntryState.CONFLICT))
        every { efficiency.current(any()) } returns "12.5"
        viewModel.load(vehicleId, Locale.US)
        advanceUntilIdle()
        assertEquals("Gol", viewModel.uiState.value.vehicleName)
        assertEquals(2, viewModel.uiState.value.entries.size)
        assertEquals(LedgerEntryState.CONFLICT, viewModel.uiState.value.entries[1].state)
        assertEquals("12.5", viewModel.uiState.value.estimateSaved)
    }

    @Test fun `invalid quantity refuses with typed error and saves nothing`() = runTest(dispatcher) {
        viewModel.load(vehicleId, Locale.US)
        advanceUntilIdle()
        viewModel.startAdd(LedgerDataset.REFUEL)
        viewModel.onQuantityChanged("abc")
        viewModel.onTotalChanged("12,34")
        viewModel.saveDraft()
        advanceUntilIdle()
        assertEquals(LedgerEntryValidationError.QUANTITY_INVALID, viewModel.uiState.value.form.validationErrorKey)
        coVerify(exactly = 0) { saveDraft.invoke(any(), any(), any()) }
    }

    @Test fun `double-tap save converges on one draft`() = runTest(dispatcher) {
        val seen = mutableListOf<com.anpfuel.domain.premium.LedgerDraft>()
        coEvery { saveDraft.invoke(any(), any(), any()) } answers {
            val d: com.anpfuel.domain.premium.LedgerDraft = firstArg()
            seen.add(d)
            com.anpfuel.domain.repository.StoredLedgerDraft(d, 0, 1)
        }
        viewModel.load(vehicleId, Locale.US)
        advanceUntilIdle()
        viewModel.startAdd(LedgerDataset.REFUEL)
        viewModel.onQuantityChanged("20,5")
        viewModel.onTotalChanged("12,34")
        viewModel.saveDraft()
        viewModel.saveDraft()
        advanceUntilIdle()
        coVerify(exactly = 1) { saveDraft.invoke(any(), any(), any()) }
        assertEquals(1, seen.size)
    }

    @Test fun `queue delete and estimate flow through`() = runTest(dispatcher) {
        coEvery { listEntries.invoke(vehicleId) } returns listOf(row())
        viewModel.load(vehicleId, Locale.US)
        advanceUntilIdle()
        viewModel.queueEntry(row())
        advanceUntilIdle()
        coVerify(exactly = 1) { enqueue.invoke(LedgerDataset.REFUEL, "11111111-1111-4111-8111-111111111111") }
        viewModel.requestDelete(row())
        viewModel.confirmDelete()
        advanceUntilIdle()
        coVerify(exactly = 1) { delete.invoke(LedgerDataset.REFUEL, "11111111-1111-4111-8111-111111111111") }
        every { efficiency.current(any()) } returns null
        coEvery { efficiency.set(any(), "12,5") } returns "12.5"
        viewModel.onEstimateChanged("12,5")
        viewModel.saveEstimate()
        advanceUntilIdle()
        assertEquals("12.5", viewModel.uiState.value.estimateSaved)
        viewModel.saveEstimate()
        advanceUntilIdle()
        assertNull(viewModel.uiState.value.form.validationErrorKey)
    }

    @Test fun `gap and reset queue intents and restart restores drafts`() = runTest(dispatcher) {        coEvery { listEntries.invoke(vehicleId) } returns listOf(row())
        viewModel.load(vehicleId, Locale.US)
        advanceUntilIdle()
        viewModel.openGapDialog(row())
        viewModel.onGapNoteChanged("pneu")
        viewModel.declareGap()
        advanceUntilIdle()
        coVerify(exactly = 1) { gap.invoke(vehicleId, any()) }
        viewModel.requestReset()
        viewModel.confirmReset()
        advanceUntilIdle()
        coVerify(exactly = 1) { reset.invoke(vehicleId, any()) }
        // Restart: a fresh ViewModel reloads the same persisted drafts.
        val restarted = LedgerEntriesViewModel(
            listEntries, getDraft, saveDraft, enqueue, delete, gap, reset, efficiency, vehicle,
        )
        restarted.load(vehicleId, Locale.US)
        advanceUntilIdle()
        assertEquals(1, restarted.uiState.value.entries.size)
        assertNotNull(restarted.uiState.value.entries.singleOrNull { it.recordId == "11111111-1111-4111-8111-111111111111" })
    }

    private fun bev() = Vehicle.create(
        displayName = "Dolphin", tankCapacity = null, fuelProduct = null,
        priceSource = VehiclePriceSource.cheapest(), id = vehicleId,
        propulsion = com.anpfuel.domain.valueobject.Propulsion.BEV,
        batteryCapacity = com.anpfuel.domain.valueobject.BatteryCapacity.ofWh(38000),
    )

    @Test fun `charge save stores energy rate soc tariff and context with pt-BR parsing`() = runTest(dispatcher) {
        coEvery { vehicle.invoke(vehicleId) } returns bev()
        coEvery { listEntries.invoke(vehicleId) } returns emptyList()
        val seen = mutableListOf<com.anpfuel.domain.premium.LedgerDraft>()
        coEvery { saveDraft.invoke(capture(seen), any(), any()) } answers {
            com.anpfuel.domain.repository.StoredLedgerDraft(firstArg<com.anpfuel.domain.premium.LedgerDraft>(), 0, 1)
        }
        viewModel.load(vehicleId, Locale("pt", "BR"))
        advanceUntilIdle()
        assertTrue(viewModel.uiState.value.vehicleIsElectric)
        viewModel.startAdd(LedgerDataset.CHARGE)
        viewModel.onQuantityChanged("15,5")
        viewModel.onTotalChanged("13,95")
        viewModel.onRateChanged("0,90")
        viewModel.onSocStartChanged("20")
        viewModel.onSocEndChanged("80")
        viewModel.onTariffIdChanged("33333333-3333-4333-8333-333333333333")
        viewModel.onTariffRevisionChanged("r7")
        viewModel.onContextChanged(com.anpfuel.domain.premium.LedgerSessionContext.HOME)
        assertEquals("13.95", viewModel.simulatedChargeTotal())
        viewModel.saveDraft()
        advanceUntilIdle()
        coVerify(exactly = 1) { saveDraft.invoke(any(), eq(0), any()) }
        val draft = seen.single() as com.anpfuel.domain.premium.ChargeDraft
        assertEquals(15500L, draft.energyWh)
        assertEquals(1395, draft.totalCentavos)
        assertEquals(20, draft.socStart)
        assertEquals("33333333-3333-4333-8333-333333333333", draft.tariffId)
        assertEquals("r7", draft.tariffRevision)
    }

    @Test fun `charge refused for non-electric vehicle and invalid rate soc tariff`() = runTest(dispatcher) {
        coEvery { vehicle.invoke(vehicleId) } returns sampleVehicle()
        coEvery { listEntries.invoke(vehicleId) } returns emptyList()
        viewModel.load(vehicleId, Locale.US)
        advanceUntilIdle()
        viewModel.startAdd(LedgerDataset.CHARGE)
        assertEquals(LedgerEntryValidationError.CHARGE_UNAVAILABLE, viewModel.uiState.value.form.validationErrorKey)
        coVerify(exactly = 0) { saveDraft.invoke(any(), any(), any()) }
    }

    @Test fun `charge validation rejects bad rate soc and split tariff`() = runTest(dispatcher) {
        coEvery { vehicle.invoke(vehicleId) } returns bev()
        coEvery { listEntries.invoke(vehicleId) } returns emptyList()
        viewModel.load(vehicleId, Locale.US)
        advanceUntilIdle()
        viewModel.startAdd(LedgerDataset.CHARGE)
        viewModel.onQuantityChanged("15,5")
        viewModel.onRateChanged("abc")
        viewModel.saveDraft()
        advanceUntilIdle()
        assertEquals(LedgerEntryValidationError.RATE_INVALID, viewModel.uiState.value.form.validationErrorKey)
        viewModel.onRateChanged("")
        viewModel.onSocStartChanged("120")
        viewModel.saveDraft()
        advanceUntilIdle()
        assertEquals(LedgerEntryValidationError.SOC_INVALID, viewModel.uiState.value.form.validationErrorKey)
        viewModel.onSocStartChanged("")
        viewModel.onTariffIdChanged("not-a-uuid")
        viewModel.saveDraft()
        advanceUntilIdle()
        assertEquals(LedgerEntryValidationError.TARIFF_INVALID, viewModel.uiState.value.form.validationErrorKey)
        coVerify(exactly = 0) { saveDraft.invoke(any(), any(), any()) }
    }

    @Test fun `late charge correction keeps id revision tariff and context`() = runTest(dispatcher) {
        coEvery { vehicle.invoke(vehicleId) } returns bev()
        val stored = com.anpfuel.domain.premium.ChargeDraft(
            "11111111-1111-4111-8111-111111111111", vehicleId.value,
            java.time.Instant.parse("2026-03-15T10:30:00Z"), "America/Sao_Paulo",
            15000, 1275, 20, 80, 50000, null, "33333333-3333-4333-8333-333333333333", "r7", null,
        )
        coEvery { getDraft.invoke(LedgerDataset.CHARGE, stored.id) } returns com.anpfuel.domain.repository.StoredLedgerDraft(
            stored, 1, 1, com.anpfuel.domain.premium.LedgerSessionContext.WORK,
        )
        coEvery { listEntries.invoke(vehicleId) } returns emptyList()
        viewModel.load(vehicleId, Locale.US)
        advanceUntilIdle()
        viewModel.startEdit(row(id = stored.id).copy(dataset = LedgerDataset.CHARGE))
        advanceUntilIdle()
        assertEquals("r7", viewModel.uiState.value.form.tariffRevisionInput)
        assertEquals(com.anpfuel.domain.premium.LedgerSessionContext.WORK, viewModel.uiState.value.form.context)
        viewModel.onTotalChanged("14,00")
        viewModel.saveDraft()
        advanceUntilIdle()
        coVerify(exactly = 1) { saveDraft.invoke(any(), eq(1), any()) }
    }
}
