package com.anpfuel.application.usecase.ledger

import com.anpfuel.application.port.EstimatedEfficiencyStore
import com.anpfuel.domain.model.Vehicle
import com.anpfuel.domain.premium.ChargeDraft
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.ExpenseDraft
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerDraft
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.premium.PremiumFuel
import com.anpfuel.domain.premium.RefuelDraft
import com.anpfuel.domain.repository.LedgerDraftStore
import com.anpfuel.domain.repository.StoredLedgerDraft
import com.anpfuel.domain.repository.VehicleRepository
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import io.mockk.coEvery
import io.mockk.every
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class MonthlyIndicatorsUseCasesTest {

    private val at1 = Instant.parse("2026-03-05T10:00:00Z")
    private val at2 = Instant.parse("2026-03-20T10:00:00Z")
    private val vid = "22222222-2222-4222-8222-222222222222"
    private val vehicleId = DomainId.from(vid)

    private class FakeDrafts : LedgerDraftStore {
        val rows = mutableListOf<StoredLedgerDraft>()
        var now = 0L
        override suspend fun save(accountId: String, origin: String, draft: LedgerDraft, expectedRevision: Long, nowMillis: Long, context: com.anpfuel.domain.premium.LedgerSessionContext?, odometerEvidence: com.anpfuel.domain.premium.OdometerEvidence) =
            StoredLedgerDraft(draft, expectedRevision, nowMillis, odometerEvidence = com.anpfuel.domain.premium.OdometerEvidence.VERIFIED_METRES).also { rows.add(it) }
        override suspend fun find(accountId: String, origin: String, dataset: LedgerDataset, recordId: String) = rows.firstOrNull { it.draft.recordId == recordId }
        override suspend fun list(accountId: String, origin: String) = rows.toList()
        override suspend fun delete(accountId: String, origin: String, dataset: LedgerDataset, recordId: String) {
            rows.removeIf { it.draft.recordId == recordId }
        }
        override suspend fun clearOwner(accountId: String, origin: String) {
            rows.clear()
        }
    }

    private val drafts = FakeDrafts()
    private val vehicles = mockk<VehicleRepository>()
    private val efficiency = mockk<EstimatedEfficiencyStore>()
    private val commands = mockk<com.anpfuel.domain.repository.PremiumCommandStore>()
    private val sessions = mockk<com.anpfuel.application.portable.AuthSessionStore>()

    private lateinit var useCase: GetMonthlyIndicatorsUseCase

    private fun vehicle() = Vehicle.create(
        displayName = "Gol", tankCapacity = TankCapacity.of(50.0),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        priceSource = VehiclePriceSource.cheapest(), id = vehicleId,
    )

    private fun refuel(id: String, at: Instant, odo: Long?, fill: LedgerFill = LedgerFill.FULL) = RefuelDraft(
        id = id, vehicleId = vid, occurredAt = at, timezone = "America/Sao_Paulo",
        fuel = PremiumFuel.ETHANOL, quantityMilli = 20000, totalCentavos = 12400,
        unitPriceMilli = null, odometerMetres = odo, fill = fill, stationId = null, note = null,
    )

    @BeforeEach
    fun setUp() {
        coEvery { vehicles.findById(any()) } returns vehicle()
        coEvery { vehicles.listAll() } returns listOf(vehicle())
        every { efficiency.get(any()) } returns null
        coEvery { commands.listPending(any(), any()) } returns emptyList()
        coEvery { sessions.load() } returns null
        useCase = GetMonthlyIndicatorsUseCase(drafts, vehicles, efficiency, commands, sessions, origin = "https://api.example.test")
    }

    @Test fun `month totals with measured interval and golden parity`() = runTest {
        val a = "aaaaaaaa-1111-4111-8111-111111111111"
        val b = "bbbbbbbb-2222-4222-8222-222222222222"
        drafts.save("local", "o", refuel(a, at1, 100000), 0, 1)
        drafts.save("local", "o", refuel(b, at2, 200000), 0, 2)
        val got = useCase(vehicleId, 2026, 3, "America/Sao_Paulo").single()
        assertEquals(24800L, got.purchaseCentavos)
        assertEquals("", got.measured.unknown)
        // 100000 m over 20000 mL -> 5 km/L exactly; cost 12400*1000/100000 = 124.
        assertEquals("5", got.measured.kmPerUnit)
        assertEquals("124", got.measured.centavosPerKm)
        assertEquals("fuel-purchase-interval", got.costPerKmMethod)
        // Fuel-only month: bought energy is zero, measured energy is unmeasured.
        assertEquals(0L, got.chargeEnergyWh)
        assertEquals("energy-unmeasured", got.energyMeasured.unknown)
    }

    @Test fun `crossing months timezones and stale correction`() = runTest {
        val id = "aaaaaaaa-1111-4111-8111-111111111111"
        drafts.save("local", "o", refuel(id, Instant.parse("2026-03-01T02:00:00Z"), 100000), 0, 1)
        // 2026-03-01T02:00:00Z is still February in Sao Paulo.
        assertEquals(0, useCase(vehicleId, 2026, 3, "America/Sao_Paulo").single().purchaseCentavos)
        assertEquals(12400L, useCase(vehicleId, 2026, 2, "America/Sao_Paulo").single().purchaseCentavos)
        assertEquals(12400L, useCase(vehicleId, 2026, 3, "UTC").single().purchaseCentavos)
    }

    @Test fun `unequal weights mixed units and phev basis`() = runTest {
        val a = "aaaaaaaa-1111-4111-8111-111111111111"
        val b = "bbbbbbbb-2222-4222-8222-222222222222"
        // Unequal fills: sum-first (150000 m for 50000 mL -> 3), never mean of ratios (35/12).
        drafts.save("local", "o", refuel(a, at1, 100000).copy(quantityMilli = 10000, totalCentavos = 1000), 0, 1)
        drafts.save("local", "o", refuel(b, at2, 200000).copy(quantityMilli = 30000, totalCentavos = 3000), 0, 2)
        drafts.save("local", "o", refuel("dddddddd-4444-4444-8444-444444444444", at2, 250000).copy(quantityMilli = 20000, totalCentavos = 2000), 0, 3)
        val weighted = useCase(vehicleId, 2026, 3, "UTC").single()
        assertEquals("3", weighted.measured.kmPerUnit)
        assertEquals("100/3", weighted.measured.centavosPerKm)
        // Mixed units across fills refuse instead of mixing L and m3.
        val cng = "cccccccc-3333-4333-8333-333333333333"
        drafts.save(
            "local", "o",
            RefuelDraft(cng, vid, at2, "UTC", PremiumFuel.CNG, 20000, 12400, null, 300000, LedgerFill.FULL, null, null),
            0, 3,
        )
        assertEquals("incompatible-unit", useCase(vehicleId, 2026, 3, "UTC").single().measured.unknown)
    }

    @Test fun `csv escapes formulas and compares against empty baseline`() = runTest {
        val rows = useCase(vehicleId, 2026, 3, "UTC")
        val csv = MonthlyIndicatorsCsv.build("=cmd|'/c calc.exe", rows)
        assertTrue(csv.lines()[1].startsWith("\"'=cmd"))
        assertTrue(rows.single().comparisonUnknown.isNotEmpty())
        assertTrue(csv.lines().any { it.endsWith(",energy_unknown,no-records") })
        assertTrue(csv.lines().any { it.endsWith(",charge_fees_centavos,0") })
    }

    @Test fun `estimate fills unknown measured and zero baseline stays unknown`() = runTest {
        every { efficiency.get(any()) } returns "12,5"
        val id = "aaaaaaaa-1111-4111-8111-111111111111"
        drafts.save("local", "o", refuel(id, at1, null), 0, 1)
        val got = useCase(vehicleId, 2026, 3, "UTC").single()
        assertEquals("odometer-missing", got.measured.unknown)
        assertEquals("12.5", got.estimatedKmPerL)
    }

    @Test fun `phev shares one distance with exact combined money`() = runTest {
        val phevId = DomainId.from("33333333-3333-4333-8333-333333333333")
        val phev = Vehicle.create(
            displayName = "GWM", tankCapacity = TankCapacity.of(55.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(), id = phevId,
            propulsion = com.anpfuel.domain.valueobject.Propulsion.PHEV,
            batteryCapacity = com.anpfuel.domain.valueobject.BatteryCapacity.ofWh(20000),
        )
        coEvery { vehicles.findById(phevId) } returns phev
        coEvery { vehicles.listAll() } returns listOf(phev)
        val c = "cccccccc-3333-4333-8333-333333333333"
        drafts.save("local", "o", refuel("aaaaaaaa-1111-4111-8111-111111111111", at1, 100000).copy(vehicleId = phevId.value), 0, 1)
        drafts.save("local", "o", refuel("bbbbbbbb-2222-4222-8222-222222222222", at2, 200000).copy(vehicleId = phevId.value), 0, 2)
        drafts.save(
            "local", "o",
            com.anpfuel.domain.premium.ChargeDraft(
                c, phevId.value, at2, "UTC", 15000, 1275, 20, 80, 200000, null, null, null, null,
            ),
            0, 3,
        )
        val got = useCase(phevId, 2026, 3, "UTC").single()
        // Money exact and counted once; no carrier ratio double-attributes the shared 100000 m.
        assertEquals(24800L, got.purchaseCentavos)
        assertEquals(15000L, got.chargeEnergyWh)
        assertEquals(1275L, got.chargeCostCentavos)
        assertEquals("phev-shared-distance", got.measured.unknown)
        assertEquals("26075", got.purchaseCentavos.plus(got.chargeCostCentavos).toString())
        assertEquals("", got.combinedUnknown)
        assertEquals("combined-purchase-interval", got.combinedMethod)
        assertEquals("1043/4", got.combinedCentavosPerKm)
        // Comparison against the empty previous month stays unknown, never zero-filled.
        assertTrue(got.comparisonUnknown.isNotEmpty())
        assertEquals("no-records", useCase(vehicleId, 2026, 3, "UTC").single().energyMeasured.unknown)
    }

    @Test fun `charge-only month measures exact energy with historical reference`() = runTest {
        val bevId = DomainId.from("55555555-5555-4555-8555-555555555555")
        val bev = Vehicle.create(
            displayName = "Dolphin", tankCapacity = null, fuelProduct = null,
            priceSource = VehiclePriceSource.cheapest(), id = bevId,
            propulsion = com.anpfuel.domain.valueobject.Propulsion.BEV,
            batteryCapacity = com.anpfuel.domain.valueobject.BatteryCapacity.ofWh(38000),
        )
        coEvery { vehicles.findById(bevId) } returns bev
        coEvery { vehicles.listAll() } returns listOf(bev)
        fun charge(id: String, wh: Long, total: Long, odo: Long, at: java.time.Instant) =
            com.anpfuel.domain.premium.ChargeDraft(id, bevId.value, at, "UTC", wh, total, 20, 80, odo, null, null, null, null)
        drafts.save("local", "o", charge("aaaaaaaa-1111-4111-8111-111111111111", 10000, 1000, 100000, at1), 0, 1)
        drafts.save("local", "o", charge("bbbbbbbb-2222-4222-8222-222222222222", 30000, 3000, 250000, at2), 0, 2)
        val got = useCase(bevId, 2026, 3, "UTC").single()
        assertEquals(40000L, got.chargeEnergyWh)
        assertEquals("", got.energyMeasured.unknown)
        assertEquals("800/3", got.energyMeasured.whPerKm)
        // Reference reuses the whole held history; comparison is exact.
        assertEquals("800/3", got.energyReference.whPerKm)
        assertEquals("0", got.energyComparison.changePercent)
    }
    @Test fun `legacy unknown odometer units retain money but cannot certify consumption`() = runTest {
        drafts.rows.add(StoredLedgerDraft(refuel("aaaaaaaa-1111-4111-8111-111111111111",at1,10000),0,1))
        drafts.rows.add(StoredLedgerDraft(refuel("bbbbbbbb-2222-4222-8222-222222222222",at2,10500),0,2))
        val got = useCase(vehicleId,2026,3,"America/Sao_Paulo").single()
        assertEquals(24800L,got.purchaseCentavos)
        assertEquals("odometer-unit-unverified",got.measured.unknown)
        assertEquals("",got.costPerKm)
        assertEquals("odometer-unit-unverified",got.combinedUnknown)
    }
    @Test fun `unverified expense odometer does not block consumption from verified refuel anchors`() = runTest {
        drafts.save("local","o",refuel("aaaaaaaa-1111-4111-8111-111111111111",at1,100000),0,1)
        drafts.save("local","o",refuel("bbbbbbbb-2222-4222-8222-222222222222",at2,200000),0,2)
        val expense = ExpenseDraft("cccccccc-3333-4333-8333-333333333333",vid,at1,"America/Sao_Paulo",5000,ExpenseCategory.MAINTENANCE,150,"legacy note")
        drafts.rows.add(StoredLedgerDraft(expense,0,3))
        val got = useCase(vehicleId,2026,3,"America/Sao_Paulo").single()
        assertEquals("",got.measured.unknown)
        assertEquals("5",got.measured.kmPerUnit)
        assertEquals("124",got.costPerKm)
        assertEquals(5000L,got.expenseCentavos)
        assertEquals(false,got.windowNote.hasUnverifiedOdometer)
    }

    @Test fun `one unverified refuel among verified anchors blocks the derived distance not the money`() = runTest {
        drafts.save("local","o",refuel("aaaaaaaa-1111-4111-8111-111111111111",at1,100000),0,1)
        drafts.rows.add(StoredLedgerDraft(refuel("bbbbbbbb-2222-4222-8222-222222222222",at2,200),0,2))
        val got = useCase(vehicleId,2026,3,"America/Sao_Paulo").single()
        assertEquals(24800L,got.purchaseCentavos)
        assertEquals("odometer-unit-unverified",got.measured.unknown)
        assertEquals(true,got.windowNote.hasUnverifiedOdometer)
        assertEquals(0L,got.windowNote.distanceMetres)
    }

}
