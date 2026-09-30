package com.anpfuel.data.local.outbox

import com.anpfuel.application.portable.OutboxCommand
import com.anpfuel.application.portable.OutboxState
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/**
 * P12-T03 native boundary: the Room adapter persists the portable outbox
 * record shape field-for-field. No entity or migration changes in this task;
 * this mapper freezes the column contract the future entity must store.
 */
class OutboxCommandMapperTest {

    @Test
    fun mapsCommandToStableColumnFields() {
        val fields = OutboxCommandMapper.toEntityFields(
            OutboxCommand(
                commandId = "cmd-1",
                kind = "observation.submit",
                payload = "{\"price\":5999}",
                revision = 2,
                state = OutboxState.IN_FLIGHT,
                attempts = 1,
                nextEligibleTick = 1000L,
                nonce = "nonce-3",
            ),
        )
        assertEquals("cmd-1", fields["command_id"])
        assertEquals("observation.submit", fields["kind"])
        assertEquals("{\"price\":5999}", fields["payload"])
        assertEquals("2", fields["revision"])
        assertEquals("IN_FLIGHT", fields["state"])
        assertEquals("1", fields["attempts"])
        assertEquals("1000", fields["next_eligible_tick"])
        assertEquals("nonce-3", fields["nonce"])
    }

    @Test
    fun roundTripPreservesCommand() {
        val command = OutboxCommand("cmd-7", "k", "p", revision = 4, attempts = 3)
        assertEquals(command, OutboxCommandMapper.fromEntityFields(OutboxCommandMapper.toEntityFields(command)))
    }
}
