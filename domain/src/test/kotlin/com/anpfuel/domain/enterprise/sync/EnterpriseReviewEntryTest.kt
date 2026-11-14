package com.anpfuel.domain.enterprise.sync

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

/** Separate review retry/dismissal invariants of a retained queue entry. */
class EnterpriseReviewEntryTest {
    private fun id(n: Int) = "7f000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val op = EnterpriseOperation.seal("""{"schema_version":1,"operation_id":"${id(10)}","owner_account_id":"${id(1)}","api_origin":"https://entry.example.invalid","org_id":"${id(2)}","membership_id":"${id(3)}","enrollment_id":"${id(4)}","type":"validation.review","operation_schema":1,"entity":{"type":"record","id":"${id(8)}","expected_revision":null},"occurred_at":"2026-11-14T00:00:00Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[],"payload":{"value":"synthetic"}}""".toByteArray(), SyncBinding.of(id(1), "https://entry.example.invalid", id(2)))
    private fun ack(result: String): OperationAcknowledgement {
        val review = if (result == "ACCEPTED_FOR_REVIEW") "\"${id(6)}\"" else "null"
        val code = when (result) { "ACCEPTED_FOR_REVIEW" -> "\"STALE_REVISION\""; "REJECTED" -> "\"FORBIDDEN\""; else -> "null" }
        return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${op.operationId}","payload_digest":"${op.payloadDigest}","result":"$result","code":$code,"entity_revision":${if (result == "ACCEPTED") "1" else "null"},"received_at":"2026-11-14T00:01:00Z","review_item_id":$review,"warnings":[]}""".toByteArray(), op)
    }
    private fun decided(outcome: String) = ReviewResolution.create(ReviewResolutionSnapshot(op.operationId, op.payloadDigest, id(6), id(7), 2,
        outcome, if (outcome == "REFUSED") "Synthetic" else "", "2026-11-14T00:01:00Z", if (outcome == "ACCEPTED") 1 else null))
    private val retry = QueueRetryState(1, EnterpriseDeviceClock("boot", 1, 1), 30_000)

    @Test fun reviewRetryExistsOnlyForAnUnresolvedOriginalReview() {
        assertTrue(EnterpriseQueueEntry(op, acknowledgement = ack("ACCEPTED_FOR_REVIEW"), reviewRetry = retry).unresolvedReview)
        for (bad in listOf<() -> Unit>(
            { EnterpriseQueueEntry(op, reviewRetry = retry) },
            { EnterpriseQueueEntry(op, acknowledgement = ack("ACCEPTED"), reviewRetry = retry) },
            { EnterpriseQueueEntry(op, acknowledgement = ack("REJECTED"), reviewRetry = retry) },
            { EnterpriseQueueEntry(op, acknowledgement = ack("ACCEPTED_FOR_REVIEW"), resolution = decided("ACCEPTED"), reviewRetry = retry) },
        )) assertThrows(IllegalArgumentException::class.java) { bad() }
        assertTrue(QueueRetryState().initial); assertFalse(retry.initial)
        assertTrue(retry.sameAs(QueueRetryState(1, EnterpriseDeviceClock("boot", 1, 1), 30_000)))
    }

    @Test fun onlyRefusedEffectiveOutcomesCanBeHiddenAndEqualServerInstantIsAllowed() {
        val review = ack("ACCEPTED_FOR_REVIEW")
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(op, acknowledgement = review, dismissedRejected = true) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(op, acknowledgement = review, dismissedRejected = true, resolution = decided("ACCEPTED")) }
        val refused = EnterpriseQueueEntry(op, acknowledgement = review, dismissedRejected = true, resolution = decided("REFUSED"))
        assertTrue(refused.refusedOutcome); assertFalse(refused.unresolvedReview)
        assertTrue(EnterpriseQueueEntry(op, acknowledgement = ack("REJECTED"), dismissedRejected = true).refusedOutcome)
        assertFalse(EnterpriseQueueEntry(op, acknowledgement = review, resolution = decided("ACCEPTED")).refusedOutcome)
    }
}
