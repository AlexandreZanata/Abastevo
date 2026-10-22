package com.anpfuel.domain.premium

import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class LedgerDraftTest {

    private val at = Instant.parse("2026-03-15T10:30:00Z")
    private val vid = "22222222-2222-4222-8222-222222222222"
    private val id = "11111111-1111-4111-8111-111111111111"

    @Test
    fun `refuel body is exact and canonical`() {
        val draft = RefuelDraft(
            id = id, vehicleId = vid, occurredAt = at, timezone = "America/Sao_Paulo",
            fuel = PremiumFuel.ETHANOL, quantityMilli = 20000, totalCentavos = 12400,
            unitPriceMilli = 620000, odometerMetres = 100000, fill = LedgerFill.FULL,
            stationId = null, note = "posto",
        )
        assertEquals(
            """{"id":"$id","vehicle_id":"$vid","occurred_at":"2026-03-15T10:30:00Z","timezone":"America/Sao_Paulo","fuel":"ETHANOL","unit":"L","quantity_milli":20000,"total_centavos":12400,"unit_price_milli":620000,"odometer_metres":100000,"fill":"full","note":"posto"}""",
            draft.bodyJson(),
        )
    }

    @Test
    fun `expense and charge bodies omit absent measurements`() {
        val expense = ExpenseDraft(
            id = id, vehicleId = vid, occurredAt = at, timezone = "UTC",
            amountCentavos = 5000, category = ExpenseCategory.TOLLS,
            odometerMetres = null, note = null,
        )
        assertEquals(
            """{"id":"$id","vehicle_id":"$vid","occurred_at":"2026-03-15T10:30:00Z","timezone":"UTC","amount_centavos":5000,"category":"tolls"}""",
            expense.bodyJson(),
        )
        val charge = ChargeDraft(
            id = id, vehicleId = vid, occurredAt = at, timezone = "America/Sao_Paulo",
            energyWh = 15000, totalCentavos = 1275, socStart = 20, socEnd = 80,
            odometerMetres = 50000, feesCentavos = null, tariffId = null,
            tariffRevision = null, note = null,
        )
        assertEquals(
            """{"id":"$id","vehicle_id":"$vid","occurred_at":"2026-03-15T10:30:00Z","timezone":"America/Sao_Paulo","energy_wh":15000,"total_centavos":1275,"soc_start_percent":20,"soc_end_percent":80,"odometer_metres":50000}""",
            charge.bodyJson(),
        )
    }

    @Test
    fun `validation refuses out-of-bounds and mismatched shapes`() {
        // CNG is a valid ledger fuel (cubic metres); fuel/unit mismatch
        // is unrepresentable because the unit derives from the fuel.
        assertThrows(PremiumValidationException::class.java) {
            RefuelDraft(id, vid, at, "America/Sao_Paulo", PremiumFuel.ETHANOL, 0, 12400, null, null, LedgerFill.FULL, null, null)
        }
        assertThrows(PremiumValidationException::class.java) {
            ExpenseDraft(id, vid, at, "Mars/Olympus", 5000, ExpenseCategory.TOLLS, null, null)
        }
        assertThrows(PremiumValidationException::class.java) {
            // Backend parity: a closing SOC below the opening SOC is
            // admitted at write (reports mark it inconsistent); only
            // out-of-range values refuse here.
            ChargeDraft(id, vid, at, "UTC", 15000, 1275, 90, 101, null, null, null, null, null)
        }
        assertThrows(PremiumValidationException::class.java) {
            ChargeDraft(id, vid, at, "UTC", 15000, 1275, null, null, null, null, "t1", null, null)
        }
        assertThrows(PremiumValidationException::class.java) {
            RefuelDraft(id, vid, at, "UTC", PremiumFuel.ETHANOL, 20000, 12400, null, null, LedgerFill.FULL, null, "bad\nnote\u0000")
        }
        assertThrows(PremiumValidationException::class.java) {
            ChargeDraft("not-a-uuid", vid, at, "UTC", 15000, 1275, null, null, null, null, null, null, null)
        }
    }

    @Test
    fun `command ids are stable per content and diverge per revision`() {
        val scope = PremiumOwnerScope("acct-1", "https://api.example.test")
        val expense = ExpenseDraft(id, vid, at, "UTC", 5000, ExpenseCategory.TOLLS, null, null)
        val first = LedgerCommandFactory.toCommand(scope, expense, 0)
        val same = LedgerCommandFactory.toCommand(scope, expense, 0)
        assertEquals(first.operationId, same.operationId)
        assertEquals("expense", first.dataset)
        assertTrue(first.payloadJson.contains("\"amount_centavos\":5000"))
        val corrected = LedgerCommandFactory.toCommand(scope, expense, 1)
        assertTrue(corrected.operationId != first.operationId)
        assertEquals(1L, corrected.expectedRevision)
    }
}
