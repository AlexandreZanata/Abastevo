package com.anpfuel.data.local.preferences

import com.anpfuel.domain.charging.ChargingCondition
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingObservationDraft
import com.anpfuel.domain.charging.PendingChargingKind
import com.anpfuel.domain.charging.PendingChargingOperation
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ChargingPendingCodecTest {
    private val draft = ChargingObservationDraft(
        placeId = "11111111-1111-4111-8111-111111111111",
        connector = ChargingConnector.CCS2,
        priceMilliBrlPerKwh = 1900,
        condition = ChargingCondition.OFF_PEAK,
        observedDay = "2026-10-22",
        note = "note",
    )

    @Test fun `submit correction and withdraw survive restart exactly`() {
        val operations = listOf(
            PendingChargingOperation("aaaaaaaa-1111-4111-8111-000000000001", PendingChargingKind.SUBMIT, null, null, draft, 1L),
            PendingChargingOperation("aaaaaaaa-1111-4111-8111-000000000002", PendingChargingKind.CORRECT, "bbbbbbbb-1111-4111-8111-000000000001", 3L, draft, 2L, "conflict"),
            PendingChargingOperation("aaaaaaaa-1111-4111-8111-000000000003", PendingChargingKind.WITHDRAW, "bbbbbbbb-1111-4111-8111-000000000002", 4L, null, 3L),
        )
        val decoded = ChargingPendingCodec.decode(ChargingPendingCodec.encode(operations))
        assertEquals(operations.map { it.operationId }, decoded.map { it.operationId })
        assertEquals(operations.map { it.kind }, decoded.map { it.kind })
        assertEquals(draft, decoded.first().draft)
        assertEquals("conflict", decoded[1].lastError)
        assertEquals(4L, decoded[2].expectedRevision)
    }

    @Test fun `corrupt oversized and invalid operation ids fail closed`() {
        assertTrue(runCatching { ChargingPendingCodec.decode("[") }.isFailure)
        assertTrue(runCatching { ChargingPendingCodec.decode("x".repeat(64 * 1024 + 1)) }.isFailure)
        assertTrue(runCatching {
            ChargingPendingCodec.decode("""[{"operation_id":"not-a-uuid","kind":"SUBMIT","created_at":1,"draft":{"connector":"TYPE2","condition":"STANDARD","observed_day":"2026-10-22"}}]""")
        }.isFailure)
    }
}
