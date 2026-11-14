package com.anpfuel.domain.enterprise.sync

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import java.security.MessageDigest
import java.time.Instant

enum class EnterpriseSyncMode { PENDING, HISTORY }
enum class EnterpriseSyncStatus { SAVED_ON_DEVICE, SENDING, SENT, ACCEPTED, NEEDS_REVIEW, REJECTED }

data class EnterpriseOperationIdentity(val operationId: String, val payloadDigest: String) {
    init { EnterprisePartition.requireId(operationId); digest(payloadDigest) }
    override fun toString(): String = "EnterpriseOperationIdentity(redacted)"
}

/** A sanitized receipt; original canonical bytes remain in the protected store. */
class EnterpriseSyncReceipt(val result: SyncResult, val code: String?, val receivedAt: Instant,
    warnings: List<String>, val entityRevision: Long?, val receiptDigest: String) {
    private val values = warnings.toList()
    init {
        digest(receiptDigest)
        require(EnterpriseTime.parseInstant(EnterpriseTime.formatInstant(receivedAt)) == receivedAt)
        require(receivedAt != Instant.parse("0001-01-01T00:00:00Z") &&
            (entityRevision == null || entityRevision > 0) && values.size <= 1 && values.all { it == "CLOCK_SKEW" })
        require(when (result) {
            SyncResult.ACCEPTED -> code == null && entityRevision != null
            SyncResult.ACCEPTED_FOR_REVIEW -> code in reviewCodes
            SyncResult.REJECTED -> code in rejectionCodes && entityRevision == null
            SyncResult.RETRY -> false
        })
    }
    val warnings: List<String> get() = values.toList()
    override fun toString(): String = "EnterpriseSyncReceipt(redacted)"
    companion object {
        private val reviewCodes = setOf("ORPHAN_REFERENCE", "ACCESS_ENDED_BEFORE_RECEIPT", "PERMISSION_CHANGED", "ASSET_UNAVAILABLE", "ASSET_BUSY", "TRIP_ALREADY_CLOSED", "PERIOD_LOCKED", "STALE_REVISION", "ORG_SUSPENDED_GRACE", "PLAN_CHANGED")
        private val rejectionCodes = setOf("OPERATION_ID_REUSED", "MALFORMED", "DEVICE_REVOKED", "MEMBERSHIP_ENDED", "INTAKE_WINDOW_CLOSED", "FORBIDDEN", "ORG_SUSPENDED")
        fun from(acknowledgement: OperationAcknowledgement): EnterpriseSyncReceipt {
            require(acknowledgement.isFinal)
            val hash = "sha256:" + MessageDigest.getInstance("SHA-256").digest(acknowledgement.canonicalBytes)
                .joinToString("") { "%02x".format(it.toInt() and 255) }
            return EnterpriseSyncReceipt(acknowledgement.result, acknowledgement.code, requireNotNull(acknowledgement.receivedAt),
                acknowledgement.warnings, acknowledgement.entityRevision, hash)
        }
    }
}

class EnterpriseSyncItem(val identity: EnterpriseOperationIdentity, val type: String, val operationSchema: Long,
    val declared: TimeDeclaration, val retry: QueueRetryState, val receipt: EnterpriseSyncReceipt?,
    val attachmentCount: Int, val dismissedRejected: Boolean = false, val resolution: EnterpriseSyncResolution? = null) {
    init {
        val segments = type.split('.')
        require(type.length <= 8192 && segments.size >= 2 && segments.all(::segmented) && operationSchema > 0 && attachmentCount >= 0)
        if (resolution != null) {
            require(receipt?.result == SyncResult.ACCEPTED_FOR_REVIEW && resolution.identity == identity &&
                !resolution.resolvedAt.isBefore(receipt.receivedAt))
        }
        require(!dismissedRejected || receipt?.result == SyncResult.REJECTED ||
            resolution?.outcome == EnterpriseReviewOutcome.REFUSED)
    }
    val status: EnterpriseSyncStatus get() = EnterpriseSyncProjection.status(this)
    val pending: Boolean get() = EnterpriseSyncProjection.pending(this)
    override fun toString(): String = "EnterpriseSyncItem(redacted)"
}

enum class EnterpriseReviewOutcome { ACCEPTED, REFUSED }

/** Minimal decision projection; canonical bytes, review/resolution IDs and reviewer proof never enter UI models. */
class EnterpriseSyncResolution private constructor(val identity: EnterpriseOperationIdentity, val outcome: EnterpriseReviewOutcome,
    val reason: String, val resolvedAt: Instant, val entityRevision: Long?, val resolutionDigest: String) {
    override fun toString(): String = "EnterpriseSyncResolution(redacted)"
    companion object {
        fun from(value: ReviewResolution): EnterpriseSyncResolution {
            val decided = value.snapshot()
            val outcome = when (decided.outcome) {
                ReviewResolution.ACCEPTED -> EnterpriseReviewOutcome.ACCEPTED
                ReviewResolution.REFUSED -> EnterpriseReviewOutcome.REFUSED
                else -> throw IllegalArgumentException("Unknown review outcome")
            }
            val hash = "sha256:" + MessageDigest.getInstance("SHA-256").digest(value.canonicalBytes)
                .joinToString("") { "%02x".format(it.toInt() and 255) }
            return EnterpriseSyncResolution(EnterpriseOperationIdentity(decided.operationId, decided.payloadDigest), outcome,
                decided.reason, EnterpriseTime.parseInstant(decided.resolvedAt), decided.entityRevision, hash)
        }
    }
}

/** Local file evidence is not a document upload lifecycle state. */
enum class EnterpriseAttachmentEvidence { NOT_INSPECTED, MISSING, MATCHING, MISMATCH, UNAVAILABLE }
class EnterpriseSyncAttachment(val id: String, val declaredBytes: Long, val sha256: String, val mime: String,
    val localEvidence: EnterpriseAttachmentEvidence) {
    init {
        EnterprisePartition.requireId(id); hash(sha256)
        require(declaredBytes > 0 && mime.length <= 8192 && Regex("[a-z0-9][a-z0-9!#$&^_.+\\-]*/[a-z0-9][a-z0-9!#$&^_.+\\-]*").matches(mime))
    }
    val uploadConfirmed: Boolean get() = false
    override fun toString(): String = "EnterpriseSyncAttachment(redacted)"
}

enum class EnterpriseSyncHistoryKind { DURABLE_SAVE, RETRY_OBSERVATION, SERVER_RECEIPT, REVIEW_RESOLUTION }
enum class EnterpriseSyncTimeSource { UNKNOWN, DEVICE, SERVER }
class EnterpriseSyncHistoryEvent(val kind: EnterpriseSyncHistoryKind, val time: Instant?, val source: EnterpriseSyncTimeSource) {
    override fun toString(): String = "EnterpriseSyncHistoryEvent(redacted)"
}

class EnterpriseSyncDetail(val item: EnterpriseSyncItem, attachments: List<EnterpriseSyncAttachment>) {
    private val values = attachments.toList()
    init { require(values.size == item.attachmentCount && values.map { it.id }.distinct().size == values.size) }
    val attachments: List<EnterpriseSyncAttachment> get() = values.toList()
    val history: List<EnterpriseSyncHistoryEvent> get() = EnterpriseSyncProjection.history(item)
    override fun toString(): String = "EnterpriseSyncDetail(redacted)"
}

class EnterpriseSyncTotals(val totalCount: Int, val pendingCount: Int, val savedCount: Int, val reviewCount: Int,
    val rejectedCount: Int, val acceptedCount: Int, val hiddenRejectedCount: Int,
    val declaredAttachmentCount: Long, val unconfirmedAttachmentCount: Long, val storage: EnterpriseQueueStats,
    val oldestPendingDeclaredAt: Instant?, val latestConfirmedReceiptAt: Instant?) {
    init {
        require(totalCount >= 0 && pendingCount in 0..totalCount && savedCount >= 0 && reviewCount >= 0 &&
            rejectedCount >= 0 && acceptedCount >= 0 && hiddenRejectedCount in 0..rejectedCount &&
            savedCount.toLong() + reviewCount + rejectedCount + acceptedCount == totalCount.toLong() &&
            declaredAttachmentCount >= 0 && unconfirmedAttachmentCount in 0..declaredAttachmentCount)
    }
    val visibleRejectedCount: Int get() = rejectedCount - hiddenRejectedCount
    override fun toString(): String = "EnterpriseSyncTotals(redacted)"
}

/** Only retained evidence and a matching live hint can establish a display status. */
object EnterpriseSyncProjection {
    fun item(entry: EnterpriseQueueEntry): EnterpriseSyncItem = EnterpriseSyncItem(
        EnterpriseOperationIdentity(entry.operation.operationId, entry.operation.payloadDigest), entry.operation.type,
        entry.operation.operationSchema, entry.operation.declaration, entry.retry,
        entry.acknowledgement?.let(EnterpriseSyncReceipt::from), entry.operation.attachments.size, entry.dismissedRejected,
        entry.resolution?.let(EnterpriseSyncResolution::from),
    )
    /** A verified separate resolution decides the effective status; the original receipt stays unchanged. */
    fun status(item: EnterpriseSyncItem, sending: Boolean = false): EnterpriseSyncStatus = when (item.receipt?.result) {
        SyncResult.ACCEPTED -> EnterpriseSyncStatus.ACCEPTED
        SyncResult.ACCEPTED_FOR_REVIEW -> when (item.resolution?.outcome) {
            EnterpriseReviewOutcome.ACCEPTED -> EnterpriseSyncStatus.ACCEPTED
            EnterpriseReviewOutcome.REFUSED -> EnterpriseSyncStatus.REJECTED
            null -> EnterpriseSyncStatus.NEEDS_REVIEW
        }
        SyncResult.REJECTED -> EnterpriseSyncStatus.REJECTED
        else -> if (sending) EnterpriseSyncStatus.SENDING else EnterpriseSyncStatus.SAVED_ON_DEVICE
    }
    fun pending(item: EnterpriseSyncItem): Boolean = !item.dismissedRejected &&
        (status(item) != EnterpriseSyncStatus.ACCEPTED || item.attachmentCount > 0)
    fun history(item: EnterpriseSyncItem): List<EnterpriseSyncHistoryEvent> = buildList {
        add(EnterpriseSyncHistoryEvent(EnterpriseSyncHistoryKind.DURABLE_SAVE, null, EnterpriseSyncTimeSource.UNKNOWN))
        if (item.retry.attempts > 0 || item.retry.clock != null) {
            val clock = item.retry.clock
            add(EnterpriseSyncHistoryEvent(EnterpriseSyncHistoryKind.RETRY_OBSERVATION, clock?.let { Instant.ofEpochMilli(it.wallMillis) },
                if (clock == null) EnterpriseSyncTimeSource.UNKNOWN else EnterpriseSyncTimeSource.DEVICE))
        }
        item.receipt?.let { add(EnterpriseSyncHistoryEvent(EnterpriseSyncHistoryKind.SERVER_RECEIPT, it.receivedAt, EnterpriseSyncTimeSource.SERVER)) }
        item.resolution?.let { add(EnterpriseSyncHistoryEvent(EnterpriseSyncHistoryKind.REVIEW_RESOLUTION, it.resolvedAt, EnterpriseSyncTimeSource.SERVER)) }
    }
    fun recordStatus(currentHead: EnterpriseOperationIdentity, item: EnterpriseSyncItem,
        sending: Boolean = false): EnterpriseSyncStatus? = if (currentHead == item.identity) status(item, sending) else null
    fun totals(items: List<EnterpriseSyncItem>, storage: EnterpriseQueueStats): EnterpriseSyncTotals {
        require(items.size <= 10000 && items.map { it.identity.operationId }.distinct().size == items.size)
        val pending = items.filter(::pending)
        val effective = items.map(::status)
        return EnterpriseSyncTotals(items.size, pending.size, items.count { it.receipt == null },
            effective.count { it == EnterpriseSyncStatus.NEEDS_REVIEW }, effective.count { it == EnterpriseSyncStatus.REJECTED },
            effective.count { it == EnterpriseSyncStatus.ACCEPTED }, items.count { it.dismissedRejected },
            items.sumOf { it.attachmentCount.toLong() }, items.filterNot { it.dismissedRejected }.sumOf { it.attachmentCount.toLong() },
            storage, pending.minOfOrNull { it.declared.occurredAt }, items.mapNotNull { it.receipt?.receivedAt }.maxOrNull())
    }
}
