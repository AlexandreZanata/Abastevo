package com.anpfuel.domain.premium

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * PRM09-T01 wire compatibility, cursor durability and owner isolation.
 */
class PremiumSyncTest {

    @Test
    fun `unknown wire values refuse instead of defaulting`() {
        assertThrows(PremiumValidationException::class.java) { CloudDataset.parse("devices") }
        assertThrows(PremiumValidationException::class.java) { LedgerDataset.parse("report") }
        assertThrows(PremiumValidationException::class.java) { ExpenseCategory.parse("fuel") }
        assertThrows(PremiumValidationException::class.java) { PremiumFuel.parse("BATTERY") }
        assertThrows(PremiumValidationException::class.java) { PremiumUnit.parse("GAL") }
        assertEquals(CloudDataset.VEHICLES, CloudDataset.parse("vehicles"))
        assertEquals(LedgerDataset.REFUEL, LedgerDataset.parse("refuel"))
        assertEquals(ExpenseCategory.MAINTENANCE, ExpenseCategory.parse("maintenance"))
    }

    @Test
    fun `cursors return verbatim and corrupted forces resnapshot`() {
        val start = PremiumCursor.start()
        assertTrue(start.isEmpty)

        val opaque = PremiumCursor.parse("hmac-opaque-123")
        assertEquals("hmac-opaque-123", opaque.raw)
        assertFalse(opaque.isEmpty)

        assertThrows(PremiumValidationException::class.java) { PremiumCursor.parse(null) }
        assertThrows(PremiumValidationException::class.java) { PremiumCursor.parse("a\nb") }
        assertThrows(PremiumValidationException::class.java) {
            PremiumCursor.parse("x".repeat(PremiumBounds.MAX_CURSOR_CHARS + 1))
        }
    }

    @Test
    fun `owner scope partitions cursors and commands`() {
        val a = PremiumOwnerScope("acc-a", "origin-a")
        val b = PremiumOwnerScope("acc-a", "origin-b")
        assertFalse(a.cursorKey("vehicles") == b.cursorKey("vehicles"))

        val command = PremiumCommand(
            operationId = "op-1",
            scope = a,
            dataset = "vehicles",
            recordId = "rec-1",
            expectedRevision = 0,
        )
        assertEquals("acc-a", command.scope.accountId)

        assertThrows(IllegalArgumentException::class.java) { PremiumOwnerScope("", "o") }
        assertThrows(PremiumValidationException::class.java) {
            command.copy(expectedRevision = -1)
        }
        assertThrows(PremiumValidationException::class.java) {
            command.copy(payloadJson = "x".repeat(8 * 1024 + 1))
        }
    }

    @Test
    fun `conflicts require explicit fresh-revision resolution`() {
        val conflict = PremiumConflict(
            operationId = "op-1",
            dataset = "vehicles",
            recordId = "rec-1",
            expectedRevision = 1,
            currentRevision = 2,
            currentDeleted = false,
            erased = false,
        )
        assertTrue(conflict.requiresExplicitResolution())
        assertEquals("op-1#retry", conflict.resolutionOperationId())
    }

    @Test
    fun `lapsed capability denies paid writes but keeps free exits`() {
        assertTrue(PremiumCapability.ACTIVE.canWritePaid())
        assertFalse(PremiumCapability.LAPSED.canWritePaid())
        assertFalse(PremiumCapability.UNKNOWN.canWritePaid())
    }
}
