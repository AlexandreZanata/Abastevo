package com.anpfuel.app.ui.ledger

import com.anpfuel.application.port.EstimatedEfficiencyStore
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.ledger.*
import com.anpfuel.application.usecase.vehicle.GetVehicleUseCase
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.*
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.PremiumCommandStore
import com.anpfuel.domain.repository.StoredLedgerDraft
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.*
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import java.util.Locale
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.*
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

/** Actual form -> draft -> actual monthly rules, not a duplicate arithmetic implementation. */
@OptIn(ExperimentalCoroutinesApi::class)
class DriverFoundationRegressionTest {
    private val dispatcher = StandardTestDispatcher()
    private val id = DomainId.from("22222222-2222-4222-8222-222222222222")
    private val car = Vehicle.create(displayName = "Gol", tankCapacity = TankCapacity.of(50.0), fuelProduct = FuelProduct.GASOLINE_REGULAR, priceSource = VehiclePriceSource.cheapest(), id = id)
    private val rows = linkedMapOf<String, StoredLedgerDraft>()
    private val list = mockk<ListLedgerEntriesUseCase>()
    private val get = mockk<GetLedgerEntryDraftUseCase>()
    private val save = mockk<SaveLedgerDraftUseCase>()
    private val efficiency = mockk<EstimatedEfficiencyUseCase>()
    private val getVehicle = mockk<GetVehicleUseCase>()
    private lateinit var vm: LedgerEntriesViewModel

    @BeforeEach fun prepare() {
        Dispatchers.setMain(dispatcher)
        coEvery { getVehicle.invoke(id) } returns car
        coEvery { list.invoke(id) } returns emptyList()
        every { efficiency.current(any()) } returns null
        coEvery { save.invoke(any(), any(), any(), any()) } answers {
            val draft = firstArg<LedgerDraft>()
            StoredLedgerDraft(draft, secondArg(), rows.size.toLong(), odometerEvidence = arg(3)).also { rows[draft.recordId] = it }
        }
        coEvery { get.invoke(any(), any()) } answers { rows[secondArg<String>()] }
        vm = LedgerEntriesViewModel(list, get, save, mockk(), mockk(), mockk(), mockk(), efficiency, getVehicle)
    }
    @AfterEach fun cleanup() { Dispatchers.resetMain() }

    private fun enter(km: String, date: String, litres: String = "40", paid: String = "260") {
        vm.startAdd(LedgerDataset.REFUEL)
        vm.onDateChanged(date)
        vm.onTimeChanged("12:00")
        vm.onQuantityChanged(litres)
        vm.onTotalChanged(paid)
        vm.onOdometerChanged(km)
        vm.saveDraft()
    }

    @Test fun `km-labelled form stores metres and produces correct real monthly consumption`() = runTest(dispatcher) {
        vm.load(id, Locale.forLanguageTag("pt-BR")); advanceUntilIdle()
        enter("10000", "2026-03-01"); advanceUntilIdle()
        enter("10500", "2026-03-20"); advanceUntilIdle()
        val fills = rows.values.map { it.draft as RefuelDraft }
        assertEquals(listOf(10000000L, 10500000L), fills.map { it.odometerMetres })
        val drafts = mockk<LedgerDraftStore>()
        coEvery { drafts.list("local", "local-origin") } answers { rows.values.toList() }
        val vehicles = mockk<VehicleRepository>()
        coEvery { vehicles.findById(id) } returns car
        val estimates = mockk<EstimatedEfficiencyStore>()
        every { estimates.get(any()) } returns null
        val commands = mockk<PremiumCommandStore>()
        coEvery { commands.listPending(any(), any()) } returns emptyList()
        val sessions = mockk<AuthSessionStore>()
        every { sessions.load() } returns null
        val actual = GetMonthlyIndicatorsUseCase(drafts, vehicles, estimates, commands, sessions, "local-origin")
            .invoke(id, 2026, 3, "America/Cuiaba").single()
        assertEquals(500000L, actual.measured.distanceMetres)
        assertEquals("25/2", actual.measured.kmPerUnit)
        assertEquals("52", actual.costPerKm)
        assertEquals("fuel-purchase-interval", actual.costPerKmMethod)
    }

    @Test fun `comma kilometre input survives reopening and saving without rescaling twice`() = runTest(dispatcher) {
        vm.load(id, Locale.forLanguageTag("pt-BR")); advanceUntilIdle()
        enter("12345,6", "2026-03-01"); advanceUntilIdle()
        assertNull(vm.uiState.value.form.validationErrorKey)
        val stored = rows.values.single()
        assertEquals(12345600L, (stored.draft as RefuelDraft).odometerMetres)
        val row = LedgerEntryRow(stored.draft.recordId, LedgerDataset.REFUEL, id.value, "Gol",
            (stored.draft as RefuelDraft).occurredAt, "America/Cuiaba", LedgerEntrySummary.Refuel(40000,26000,LedgerFill.FULL),0,null,LedgerEntryState.LOCAL,null)
        vm.startEdit(row); advanceUntilIdle()
        assertEquals("12345,6", vm.uiState.value.form.odometerInput)
        vm.saveDraft(); advanceUntilIdle()
        assertEquals(1, rows.size)
        assertEquals(12345600L, (rows.values.single().draft as RefuelDraft).odometerMetres)
    }
    @Test fun `decimal inputs cannot wrap Long into a plausible small amount`() {
        assertNull(vm.parseCentavos("184467440737095516.16"))
        assertNull(vm.parseMilli("18446744073709551.617"))
        assertNull(vm.parseRateMilli("18446744073709551.617"))
    }

}
