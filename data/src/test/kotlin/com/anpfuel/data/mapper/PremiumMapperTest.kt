package com.anpfuel.data.mapper

import com.anpfuel.domain.premium.PremiumCommand
import com.anpfuel.domain.premium.PremiumCursor
import com.anpfuel.domain.premium.PremiumOwnerScope
import com.anpfuel.domain.premium.PremiumValidationException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

/**
 * PRM09-T01 mapper: round-trip preserves owner/origin partitions and
 * corrupted cursors refuse instead of silently skipping history.
 */
class PremiumMapperTest {

    private val scope = PremiumOwnerScope("acc-1", "origin-1")

    @Test
    fun `round trip preserves command fields and scope`() {
        val command = PremiumCommand(
            operationId = "op-1",
            scope = scope,
            dataset = "vehicles",
            recordId = "rec-1",
            expectedRevision = 2,
            payloadJson = """{"id":"rec-1"}""",
        )

        val entity = PremiumMapper.toEntity(command)
        assertEquals("acc-1", entity.ownerAccount)
        assertEquals("origin-1", entity.origin)
        assertEquals("vehicles", entity.dataset)

        val restored = PremiumMapper.toDomain(entity)
        assertEquals(command, restored)
    }

    @Test
    fun `cursor round trip preserves verbatim value`() {
        val cursor = PremiumCursor.parse("opaque-hmac-abc")
        val entity = PremiumMapper.cursorToEntity(scope, "vehicles", cursor, 3L, 999L)
        assertEquals("opaque-hmac-abc", entity.cursor)
        assertEquals(3L, entity.epoch)
        assertEquals(cursor, PremiumMapper.cursorToDomain(entity))
    }

    @Test
    fun `corrupted stored cursor refuses`() {
        val entity = PremiumMapper.cursorToEntity(scope, "vehicles", PremiumCursor.start(), 1L, 1L)
            .copy(cursor = "bad\ncursor")
        assertThrows(PremiumValidationException::class.java) {
            PremiumMapper.cursorToDomain(entity)
        }
    }
}
