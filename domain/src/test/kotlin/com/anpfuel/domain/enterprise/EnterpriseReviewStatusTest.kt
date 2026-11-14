package com.anpfuel.domain.enterprise

import com.anpfuel.domain.enterprise.sync.*
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseReviewStatusTest {
    private fun id(n: Int) = "7e000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private fun operation(files: Boolean = false): EnterpriseOperation {
        val attachment = if (files) """{"id":"${id(9)}","sha256":"${"a".repeat(64)}","bytes":10,"mime":"image/png"}""" else ""
        return EnterpriseOperation.seal("""{"schema_version":1,"operation_id":"${id(10)}","owner_account_id":"${id(1)}","api_origin":"https://review.example.invalid","org_id":"${id(2)}","membership_id":"${id(3)}","enrollment_id":"${id(4)}","type":"validation.review","operation_schema":1,"entity":{"type":"record","id":"${id(8)}","expected_revision":null},"occurred_at":"2026-11-14T00:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[$attachment],"payload":{"private":"synthetic-private-fact"}}""".toByteArray(), SyncBinding.of(id(1), "https://review.example.invalid", id(2)))
    }
    private fun ack(op: EnterpriseOperation, result: String = "ACCEPTED_FOR_REVIEW"): OperationAcknowledgement {
        val review = if (result == "ACCEPTED_FOR_REVIEW") "\"${id(6)}\"" else "null"
        val code = if (result == "ACCEPTED_FOR_REVIEW") "\"PERMISSION_CHANGED\"" else "null"
        val revision = if (result == "ACCEPTED") "1" else "null"
        return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result","code":$code,"entity_revision":$revision,"received_at":"2026-11-14T00:01:00Z","review_item_id":$review,"warnings":[]}""".toByteArray(), op)
    }
    private fun resolution(op: EnterpriseOperation, outcome: String = "ACCEPTED", review: String = id(6), at: String = "2026-11-14T00:02:00Z") =
        ReviewResolution.create(ReviewResolutionSnapshot(op.operationId, op.payloadDigest, review, id(7), 2, outcome,
            "Synthetic manager decision", at, if (outcome == "ACCEPTED") 1 else null))
    @Test fun acceptedResolutionChangesEffectiveStatusWithoutReplacingOriginalReceipt() {
        val op = operation(); val receipt = ack(op); val original = receipt.canonicalBytes
        val entry = EnterpriseQueueEntry(op, acknowledgement = receipt, resolution = resolution(op))
        val item = EnterpriseSyncProjection.item(entry)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, item.status)
        assertFalse(item.pending)
        assertEquals(SyncResult.ACCEPTED_FOR_REVIEW, item.receipt!!.result)
        assertEquals(Instant.parse("2026-11-14T00:01:00Z"), item.receipt!!.receivedAt)
        assertEquals("Synthetic manager decision", item.resolution!!.reason)
        assertArrayEquals(original, entry.acknowledgement!!.canonicalBytes)
        assertFalse(item.toString().contains("synthetic-private-fact"))
        assertEquals(2, item.let { EnterpriseSyncProjection.history(it) }.count { it.source == EnterpriseSyncTimeSource.SERVER })
    }
    @Test fun refusedDecisionCanBeHiddenWithoutAttachmentResurrectionAndAcceptedFilesStayPending() {
        val op = operation(true); val receipt = ack(op)
        val refused = EnterpriseSyncProjection.item(EnterpriseQueueEntry(op, acknowledgement = receipt,
            dismissedRejected = true, resolution = resolution(op, "REFUSED")))
        assertEquals(EnterpriseSyncStatus.REJECTED, refused.status); assertFalse(refused.pending)
        val totals = EnterpriseSyncProjection.totals(listOf(refused), EnterpriseQueueStats(0, 0, 0, 1, 2048))
        assertEquals(0, totals.pendingCount); assertEquals(1, totals.rejectedCount); assertEquals(1, totals.hiddenRejectedCount)
        assertEquals(0, totals.unconfirmedAttachmentCount)
        val accepted = EnterpriseSyncProjection.item(EnterpriseQueueEntry(op, acknowledgement = receipt, resolution = resolution(op)))
        assertEquals(EnterpriseSyncStatus.ACCEPTED, accepted.status); assertTrue(accepted.pending)
    }
    @Test fun resolutionRequiresExactOriginalReviewIntentAndNondecreasingServerTime() {
        val op = operation(); val receipt = ack(op)
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(op, acknowledgement = receipt, resolution = resolution(op, review = id(99))) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(op, resolution = resolution(op)) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(op, acknowledgement = ack(op, "ACCEPTED"), resolution = resolution(op)) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(op, acknowledgement = receipt, resolution = resolution(op, at = "2026-11-14T00:00:59Z")) }
    }
}
