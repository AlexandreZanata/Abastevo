package com.anpfuel.domain.enterprise

import com.anpfuel.domain.enterprise.sync.*
import java.security.MessageDigest
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseSyncStatusTest {
    private fun id(n: Int) = "7d000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private fun operation(n: Int, attachments: Int = 0): EnterpriseOperation {
        val files = (0 until attachments).joinToString { """{"id":"${id(n * 100 + it)}","sha256":"${"0".repeat(64)}","bytes":10,"mime":"image/png"}""" }
        val raw = """{"schema_version":1,"operation_id":"${id(n)}","owner_account_id":"${id(1)}","api_origin":"https://status.example.invalid","org_id":"${id(2)}","membership_id":"${id(3)}","enrollment_id":"${id(4)}","type":"validation.status","operation_schema":1,"entity":{"type":"record","id":"${id(8)}","expected_revision":null},"occurred_at":"2026-11-13T00:00:${n.toString().padStart(2, '0')}Z","entry_zone":"UTC","device_offset_seconds":0,"depends_on":[],"soft_references":[],"attachments":[$files],"payload":{"private":"SYNTHETIC-STATUS-PAYLOAD-MUST-NOT-REACH-UI"}}"""
        return EnterpriseOperation.seal(raw.toByteArray(), SyncBinding.of(id(1), "https://status.example.invalid", id(2)))
    }
    private fun acknowledgement(operation: EnterpriseOperation, result: String = "ACCEPTED", code: String? = null): OperationAcknowledgement {
        val revision = if (result == "ACCEPTED") "1" else "null"
        val review = if (result == "ACCEPTED_FOR_REVIEW") "\"${id(6)}\"" else "null"
        val receipt = if (result == "RETRY") "null" else "\"2026-11-13T00:01:00Z\""
        return OperationAcknowledgement.parse("""{"schema_version":1,"operation_id":"${operation.operationId}","payload_digest":"${operation.payloadDigest}","result":"$result","code":${code?.let { "\"$it\"" } ?: "null"},"entity_revision":$revision,"received_at":$receipt,"review_item_id":$review,"warnings":[]}""".toByteArray(), operation)
    }
    private fun item(n: Int, result: String? = null, code: String? = null, attachments: Int = 0, hidden: Boolean = false): EnterpriseSyncItem {
        val operation = operation(n, attachments)
        return EnterpriseSyncProjection.item(EnterpriseQueueEntry(operation, acknowledgement = result?.let { acknowledgement(operation, it, code) }, dismissedRejected = hidden))
    }

    @Test fun finalReceiptAndOnlyActualMatchingActivityDetermineCanonicalStatus() {
        val saved = item(10)
        assertEquals(EnterpriseSyncStatus.SAVED_ON_DEVICE, saved.status)
        assertEquals(EnterpriseSyncStatus.SENDING, EnterpriseSyncProjection.status(saved, true))
        val accepted = item(11, "ACCEPTED")
        assertEquals(EnterpriseSyncStatus.ACCEPTED, accepted.status)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, EnterpriseSyncProjection.status(accepted, true))
        val reviewCodes = listOf("ORPHAN_REFERENCE", "ACCESS_ENDED_BEFORE_RECEIPT", "PERMISSION_CHANGED", "ASSET_UNAVAILABLE", "ASSET_BUSY", "TRIP_ALREADY_CLOSED", "PERIOD_LOCKED", "STALE_REVISION", "ORG_SUSPENDED_GRACE", "PLAN_CHANGED")
        for (code in reviewCodes) {
            val value = item(12, "ACCEPTED_FOR_REVIEW", code)
            assertEquals(EnterpriseSyncStatus.NEEDS_REVIEW, value.status)
            assertEquals(code, value.receipt!!.code)
            assertEquals(EnterpriseSyncStatus.NEEDS_REVIEW, EnterpriseSyncProjection.status(value, true))
        }
        for (code in listOf("OPERATION_ID_REUSED", "MALFORMED", "DEVICE_REVOKED", "MEMBERSHIP_ENDED", "INTAKE_WINDOW_CLOSED", "FORBIDDEN", "ORG_SUSPENDED")) {
            val value = item(13, "REJECTED", code)
            assertEquals(EnterpriseSyncStatus.REJECTED, value.status); assertEquals(code, value.receipt!!.code)
        }
    }

    @Test fun receiptFingerprintIsActualImmutableCanonicalEvidenceAndCannotAdoptRetryOrMissingReason() {
        val operation = operation(10); val ack = acknowledgement(operation)
        val expected = "sha256:" + MessageDigest.getInstance("SHA-256").digest(ack.canonicalBytes).joinToString("") { "%02x".format(it.toInt() and 255) }
        val receipt = EnterpriseSyncReceipt.from(ack)
        assertEquals(expected, receipt.receiptDigest); assertEquals(ack.result, receipt.result)
        assertEquals(ack.receivedAt, receipt.receivedAt); assertEquals(ack.entityRevision, receipt.entityRevision)
        assertThrows(IllegalArgumentException::class.java) { EnterpriseSyncReceipt.from(acknowledgement(operation, "RETRY", "UNAVAILABLE")) }
        fun invalid(result: SyncResult = SyncResult.ACCEPTED, code: String? = null, revision: Long? = 1,
            time: Instant = Instant.parse("2026-11-13T00:01:00Z"), digest: String = expected) =
            EnterpriseSyncReceipt(result, code, time, emptyList(), revision, digest)
        assertThrows(IllegalArgumentException::class.java) { invalid(revision = 0) }
        assertThrows(IllegalArgumentException::class.java) { invalid(code = "FORBIDDEN") }
        assertThrows(IllegalArgumentException::class.java) { invalid(SyncResult.REJECTED, null, null) }
        assertThrows(IllegalArgumentException::class.java) { invalid(SyncResult.ACCEPTED_FOR_REVIEW, "UNKNOWN", null) }
        assertThrows(IllegalArgumentException::class.java) { invalid(time = Instant.parse("0001-01-01T00:00:00Z")) }
        assertThrows(IllegalArgumentException::class.java) { invalid(digest = "not-a-digest") }
    }

    @Test fun pendingUnionCountsOperationsOnceAndNeverResurrectsHiddenRefusalThroughAttachments() {
        val items = listOf(item(10, attachments = 2), item(11, "ACCEPTED_FOR_REVIEW", "ORPHAN_REFERENCE", 1),
            item(12, "REJECTED", "MALFORMED"), item(13, "REJECTED", "FORBIDDEN", 2, true),
            item(14, "ACCEPTED"), item(15, "ACCEPTED", attachments = 3))
        val stats = EnterpriseQueueStats(1, 100, 2, 10, 27_000_000)
        val totals = EnterpriseSyncProjection.totals(items, stats)
        assertEquals(6, totals.totalCount); assertEquals(4, totals.pendingCount)
        assertEquals(1, totals.savedCount); assertEquals(1, totals.reviewCount)
        assertEquals(2, totals.rejectedCount); assertEquals(1, totals.hiddenRejectedCount); assertEquals(1, totals.visibleRejectedCount)
        assertEquals(2, totals.acceptedCount); assertEquals(8L, totals.declaredAttachmentCount); assertEquals(6L, totals.unconfirmedAttachmentCount)
        assertSame(stats, totals.storage); assertTrue(totals.storage.pressure); assertEquals(2, totals.storage.legacyCount)
        assertEquals(items[0].declared.occurredAt, totals.oldestPendingDeclaredAt)
        assertEquals(Instant.parse("2026-11-13T00:01:00Z"), totals.latestConfirmedReceiptAt)
        assertFalse(items[3].pending); assertFalse(items[4].pending); assertTrue(items[5].pending)
    }

    @Test fun historyPreservesUnknownSaveTimeAndLabelsRebasedClockOnlyAsRetainedObservation() {
        val operation = operation(10)
        val clock = EnterpriseDeviceClock("new-boot", 1234, 1700000000000)
        val saved = EnterpriseSyncProjection.item(EnterpriseQueueEntry(operation, QueueRetryState(4, clock, 120000)))
        val history = EnterpriseSyncProjection.history(saved)
        assertEquals(listOf(EnterpriseSyncHistoryKind.DURABLE_SAVE, EnterpriseSyncHistoryKind.RETRY_OBSERVATION), history.map { it.kind })
        assertNull(history[0].time); assertEquals(EnterpriseSyncTimeSource.UNKNOWN, history[0].source)
        assertEquals(Instant.ofEpochMilli(clock.wallMillis), history[1].time); assertEquals(EnterpriseSyncTimeSource.DEVICE, history[1].source)
        assertFalse(history.any { it.time == operation.declaration.occurredAt })
        val accepted = item(11, "ACCEPTED")
        val confirmed = EnterpriseSyncProjection.history(accepted)
        assertEquals(EnterpriseSyncHistoryKind.SERVER_RECEIPT, confirmed.last().kind)
        assertEquals(accepted.receipt!!.receivedAt, confirmed.last().time)
        assertEquals(EnterpriseSyncTimeSource.SERVER, confirmed.last().source)
    }

    @Test fun exactOwningHeadIsRequiredAndNoLocalAttachmentEvidenceClaimsUpload() {
        val accepted = item(10, "ACCEPTED", attachments = 1)
        assertEquals(EnterpriseSyncStatus.ACCEPTED, EnterpriseSyncProjection.recordStatus(accepted.identity, accepted))
        assertNull(EnterpriseSyncProjection.recordStatus(EnterpriseOperationIdentity(id(99), accepted.identity.payloadDigest), accepted))
        assertNull(EnterpriseSyncProjection.recordStatus(EnterpriseOperationIdentity(accepted.identity.operationId, "sha256:" + "1".repeat(64)), accepted))
        for (evidence in EnterpriseAttachmentEvidence.entries) {
            val attachment = EnterpriseSyncAttachment(id(100), 10, "0".repeat(64), "image/png", evidence)
            assertFalse(attachment.uploadConfirmed)
            val detail = EnterpriseSyncDetail(accepted, listOf(attachment))
            assertEquals(evidence, detail.attachments.single().localEvidence)
            assertEquals(1, detail.item.attachmentCount)
            assertFalse(detail.toString().contains(accepted.identity.operationId))
        }
    }

    @Test fun onlyAStoredFinalRefusalMayCarryTheLocalDismissedFlag() {
        val operation = operation(10)
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(operation, dismissedRejected = true) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(operation, acknowledgement = acknowledgement(operation), dismissedRejected = true) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseQueueEntry(operation,
            acknowledgement = acknowledgement(operation, "ACCEPTED_FOR_REVIEW", "ORPHAN_REFERENCE"), dismissedRejected = true) }
        val rejected = EnterpriseQueueEntry(operation, acknowledgement = acknowledgement(operation, "REJECTED", "MALFORMED"), dismissedRejected = true)
        assertTrue(rejected.dismissedRejected); assertFalse(EnterpriseSyncProjection.item(rejected).pending)
    }

    @Test fun publicReceiptRejectsInstantsOutsideFrozenCanonicalServerRange() {
        fun receipt(at: Instant) = EnterpriseSyncReceipt(SyncResult.ACCEPTED, null, at, emptyList(), 1,
            "sha256:" + "1".repeat(64))
        for (at in listOf(Instant.parse("+10000-01-01T00:00:00Z"), Instant.parse("0000-12-31T23:59:59Z"), Instant.MIN, Instant.MAX))
            assertThrows(IllegalArgumentException::class.java) { receipt(at) }
        for (at in listOf(Instant.parse("0001-01-01T00:00:00.000000001Z"), Instant.parse("9999-12-31T23:59:59.999999999Z")))
            assertEquals(at, receipt(at).receivedAt)
    }
}
