package com.anpfuel.domain.enterprise.fueling

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.QueueRetryState
import com.anpfuel.domain.enterprise.sync.SyncBinding
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** EMP-FUEL-011 (B-BR-EMP-456..458, EDR-295/296): exact entry and a presentation that only receipts move. */
class EnterpriseFuelingPresentationTest {
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val binding = SyncBinding.of(id(9501), "https://api.example.invalid", id(9502))
    private fun fueling(op: Int, fuelingId: Int, at: String, paid: String = "97000"): EnterpriseOperation = FuelingOperations.submit(
        FuelingEnvelope(binding, id(9503), id(9504), id(op), TimeDeclaration(EnterpriseTime.parseInstant(at), "UTC", 0)), id(fuelingId),
        FuelingSubmitInput(id(9721), id(9731), "DIESEL_S10", "L", "150000", paid, null, "6490", null, "FULL", "COMPANY", "FUEL_CARD",
            FuelingReadingInput(id(9751), id(9752), id(op + 50), "ODOMETER", "184320000")))
    private fun ack(op: EnterpriseOperation, result: String, code: String? = null) = OperationAcknowledgement.parse(("""{"schema_version":1,""" +
        """"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},""" +
        """"entity_revision":${if (result == "ACCEPTED") "1" else "null"},"received_at":${if (result == "RETRY") "null" else "\"2026-11-15T18:00:00Z\""},""" +
        """"review_item_id":${if (result == "ACCEPTED_FOR_REVIEW") "\"${id(8888)}\"" else "null"},"warnings":[]}""").toByteArray(), op)

    @Test fun `typed values convert exactly or not at all`() {
        assertEquals("150000", FuelingEntry.quantity("150"))
        assertEquals("37415", FuelingEntry.quantity("37,415"))
        assertEquals("37415", FuelingEntry.quantity(" 37.415 "))
        assertEquals("1", FuelingEntry.quantity("0,001"))
        assertEquals("500", FuelingEntry.quantity(",5"))
        assertEquals("97350", FuelingEntry.money("973,50"))
        assertEquals("97000", FuelingEntry.money("970"))
        assertEquals("0", FuelingEntry.money("0"))
        assertEquals("6490", FuelingEntry.unitPrice("6,49"))
        assertEquals("150000", FuelingEntry.quantity("0150"))
        for (bad in listOf("", " ", "1,0001", "1.2.3", "1,234.5", "1 000", "-5", "+5", "5,", ",", "1e3", "١٢", "abc", "12a")) {
            assertNull(FuelingEntry.quantity(bad), bad)
        }
        assertNull(FuelingEntry.money("1,005"))
        assertNull(FuelingEntry.unitPrice("6,4901"))
        assertNull(FuelingEntry.quantity("1".repeat(25)))
    }

    @Test fun `each fueling shows only what its own receipt proves`() {
        val saved = fueling(9701, 9711, "2026-11-15T10:00:00Z")
        val accepted = fueling(9702, 9712, "2026-11-15T12:00:00Z")
        val held = fueling(9703, 9713, "2026-11-15T11:00:00Z")
        val refused = fueling(9704, 9714, "2026-11-15T09:00:00Z")
        val views = FuelingPresentation.views(listOf(
            EnterpriseQueueEntry(saved), EnterpriseQueueEntry(accepted, acknowledgement = ack(accepted, "ACCEPTED")),
            // A held fueling (EDR-177) is answered RETRY: it stays an unacknowledged entry with its retry state.
            EnterpriseQueueEntry(held, retry = QueueRetryState(2, EnterpriseDeviceClock("boot-1", 5_000, 1_763_208_000_000L), 30_000)),
            EnterpriseQueueEntry(refused, acknowledgement = ack(refused, "REJECTED", "MALFORMED"))))
        assertEquals(listOf(id(9712), id(9713), id(9711), id(9714)), views.map { it.fact.fuelingId })
        val byId = views.associateBy { it.fact.fuelingId }
        assertEquals(EnterpriseSyncStatus.ACCEPTED, byId.getValue(id(9712)).sync)
        assertEquals(FuelingReviewLine.AWAITING_APPROVAL, byId.getValue(id(9712)).review)
        assertEquals(FuelingReviewLine.NOT_SENT, byId.getValue(id(9711)).review)
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, byId.getValue(id(9711)).sync)
        // A held fueling (EDR-177) is never shown accepted.
        assertNotEquals(EnterpriseSyncStatus.ACCEPTED, byId.getValue(id(9713)).sync)
        assertEquals(FuelingReviewLine.NOT_SENT, byId.getValue(id(9713)).review)
        assertEquals(FuelingReviewLine.REFUSED, byId.getValue(id(9714)).review)
        assertEquals("MALFORMED", byId.getValue(id(9714)).code)
        // The entered amounts are the operation's own, never recomputed.
        assertEquals(97000L, byId.getValue(id(9712)).fact.amounts.paid)
        assertEquals(150000L, byId.getValue(id(9712)).fact.quantity.amount)
    }

    @Test fun `entries of other operations are ignored and a foreign shape is refused`() {
        val other = EnterpriseOperation.seal(("""{"schema_version":1,"operation_id":"${id(1)}","owner_account_id":"${id(9501)}","api_origin":"https://api.example.invalid","org_id":"${id(9502)}","membership_id":"${id(9503)}","enrollment_id":"${id(9504)}","type":"validation.record","operation_schema":1,"entity":{"type":"record","id":"${id(2)}","expected_revision":null},"occurred_at":"2026-11-15T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"SYNTHETIC"}}""").toByteArray(), binding)
        assertTrue(FuelingPresentation.views(listOf(EnterpriseQueueEntry(other))).isEmpty())
        val failure = assertThrows(FuelingOperationException::class.java) { FuelingOperations.read(other) }
        assertEquals(FuelingOperationFailure.MALFORMED, failure.failure)
        assertEquals("EnterpriseFuelingFact(redacted)", FuelingOperations.read(fueling(9705, 9715, "2026-11-15T12:00:00Z")).toString())
    }
}
