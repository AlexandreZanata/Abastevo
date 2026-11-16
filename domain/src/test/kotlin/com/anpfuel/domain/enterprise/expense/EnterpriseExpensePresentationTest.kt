package com.anpfuel.domain.enterprise.expense

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fueling.FuelingEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.EnterpriseSyncStatus
import com.anpfuel.domain.enterprise.sync.OperationAcknowledgement
import com.anpfuel.domain.enterprise.sync.QueueRetryState
import com.anpfuel.domain.enterprise.sync.SyncBinding
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** EMP-EXP-004 (B-BR-EMP-518/519/521, EDR-336/337): exact amounts and a presentation that only receipts move. */
class EnterpriseExpensePresentationTest {
    private fun id(n: Int) = "7b0e2a54-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val binding = SyncBinding.of(id(9601), "https://api.example.invalid", id(9602))
    private fun expense(op: Int, expenseId: Int, at: String, paid: String = "1250", company: String = "1250", category: String = "TOLL"): EnterpriseOperation =
        ExpenseOperations.submit(ExpenseEnvelope(binding, id(9603), id(9604), id(op), TimeDeclaration(EnterpriseTime.parseInstant(at), "UTC", 0)),
            id(expenseId), id(9621), category, paid, company, "EMPLOYEE", null)
    private fun ack(op: EnterpriseOperation, result: String, code: String? = null) = OperationAcknowledgement.parse(("""{"schema_version":1,""" +
        """"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},""" +
        """"entity_revision":${if (result == "ACCEPTED") "1" else "null"},"received_at":${if (result == "RETRY") "null" else "\"2026-11-15T18:00:00Z\""},""" +
        """"review_item_id":${if (result == "ACCEPTED_FOR_REVIEW") "\"${id(8888)}\"" else "null"},"warnings":[]}""").toByteArray(), op)

    @Test fun `typed reais convert to exact centavos or not at all`() {
        assertEquals("1250", FuelingEntry.money("12,50"))
        assertEquals("800", FuelingEntry.money("8"))
        assertEquals("5", FuelingEntry.money("0,05"))
        for (bad in listOf("12,345", "1.234,56", "R$ 10", "10,", "-1", "1e2", "1 000", "")) assertNull(FuelingEntry.money(bad), bad)
    }

    @Test fun `each expense shows only what its own receipt proves`() {
        val saved = expense(9701, 9711, "2026-11-15T10:00:00Z")
        val accepted = expense(9702, 9712, "2026-11-15T12:00:00Z", paid = "800", company = "0", category = "PARKING")
        val held = expense(9703, 9713, "2026-11-15T11:00:00Z")
        val refused = expense(9704, 9714, "2026-11-15T09:00:00Z")
        val views = ExpensePresentation.views(listOf(
            EnterpriseQueueEntry(saved), EnterpriseQueueEntry(accepted, acknowledgement = ack(accepted, "ACCEPTED")),
            EnterpriseQueueEntry(held, retry = QueueRetryState(2, EnterpriseDeviceClock("boot-1", 5_000, 1_763_208_000_000L), 30_000)),
            EnterpriseQueueEntry(refused, acknowledgement = ack(refused, "REJECTED", "MALFORMED"))))
        assertEquals(listOf(id(9712), id(9713), id(9711), id(9714)), views.map { it.fact.expenseId })
        val byId = views.associateBy { it.fact.expenseId }
        assertEquals(EnterpriseSyncStatus.ACCEPTED, byId.getValue(id(9712)).sync)
        assertEquals(ExpenseReviewLine.AWAITING_APPROVAL, byId.getValue(id(9712)).review)
        assertEquals(ExpenseReviewLine.NOT_SENT, byId.getValue(id(9711)).review)
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, byId.getValue(id(9711)).sync)
        // A held expense is never shown accepted.
        assertNotEquals(EnterpriseSyncStatus.ACCEPTED, byId.getValue(id(9713)).sync)
        assertEquals(ExpenseReviewLine.NOT_SENT, byId.getValue(id(9713)).review)
        assertEquals(ExpenseReviewLine.REFUSED, byId.getValue(id(9714)).review)
        assertEquals("MALFORMED", byId.getValue(id(9714)).code)
        // The entered amounts are the operation's own, never recomputed.
        assertEquals(listOf(800L, 0L, "PARKING"), listOf(byId.getValue(id(9712)).fact.paidCentavos, byId.getValue(id(9712)).fact.companyCentavos,
            byId.getValue(id(9712)).fact.category))
    }

    @Test fun `entries of other operations are ignored and a foreign shape is refused`() {
        val other = EnterpriseOperation.seal(("""{"schema_version":1,"operation_id":"${id(1)}","owner_account_id":"${id(9601)}","api_origin":"https://api.example.invalid","org_id":"${id(9602)}","membership_id":"${id(9603)}","enrollment_id":"${id(9604)}","type":"validation.record","operation_schema":1,"entity":{"type":"record","id":"${id(2)}","expected_revision":null},"occurred_at":"2026-11-15T12:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"SYNTHETIC"}}""").toByteArray(), binding)
        assertTrue(ExpensePresentation.views(listOf(EnterpriseQueueEntry(other))).isEmpty())
        assertEquals("EnterpriseExpenseView(redacted)", ExpensePresentation.view(EnterpriseQueueEntry(expense(9705, 9715, "2026-11-15T12:00:00Z"))).toString())
    }
}
