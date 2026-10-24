package com.anpfuel.data.mapper

import com.anpfuel.domain.premium.ChargeDraft
import com.anpfuel.domain.premium.ExpenseCategory
import com.anpfuel.domain.premium.ExpenseDraft
import com.anpfuel.domain.premium.LedgerDataset
import com.anpfuel.domain.premium.LedgerFill
import com.anpfuel.domain.premium.PremiumFuel
import com.anpfuel.domain.premium.PremiumValidationException
import com.anpfuel.domain.premium.RefuelDraft
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class LedgerDraftCodecTest {

    private val at = Instant.parse("2026-03-15T10:30:00Z")
    private val vid = "22222222-2222-4222-8222-222222222222"
    private val id = "11111111-1111-4111-8111-111111111111"

    @Test
    fun `bodies round-trip through storage`() {
        val refuel = RefuelDraft(id, vid, at, "America/Sao_Paulo", PremiumFuel.GASOLINE_REGULAR, 20000, 12400, 620000, 100000, LedgerFill.FULL, null, "posto")
        assertEquals(refuel, LedgerDraftCodec.decode(LedgerDataset.REFUEL, refuel.bodyJson()))
        val expense = ExpenseDraft(id, vid, at, "UTC", 5000, ExpenseCategory.TOLLS, null, null)
        assertEquals(expense, LedgerDraftCodec.decode(LedgerDataset.EXPENSE, expense.bodyJson()))
        val charge = ChargeDraft(id, vid, at, "America/Sao_Paulo", 15000, 1275, 20, 80, 50000, null, null, null, null)
        assertEquals(charge, LedgerDraftCodec.decode(LedgerDataset.CHARGE, charge.bodyJson()))
    }

    @Test
    fun `corrupt drifted and foreign bodies refuse`() {
        assertThrows(PremiumValidationException::class.java) {
            LedgerDraftCodec.decode(LedgerDataset.EXPENSE, """{"id":"$id"}""")
        }
        assertThrows(PremiumValidationException::class.java) {
            LedgerDraftCodec.decode(LedgerDataset.REFUEL, """{"id":"not-a-uuid"}""")
        }
        val expense = ExpenseDraft(id, vid, at, "UTC", 5000, ExpenseCategory.TOLLS, null, null)
        assertThrows(PremiumValidationException::class.java) {
            LedgerDraftCodec.decode(LedgerDataset.REFUEL, expense.bodyJson())
        }
    }
}
