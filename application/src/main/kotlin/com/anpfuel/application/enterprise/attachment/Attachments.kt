package com.anpfuel.application.enterprise.attachment

import com.anpfuel.application.enterprise.*
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAccountLease
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure
import com.anpfuel.application.enterprise.queue.EnterpriseQueueSource
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.attachment.*
import com.anpfuel.domain.enterprise.sync.EnterpriseQueueEntry
import com.anpfuel.domain.enterprise.sync.QueueRetryPolicy
import com.anpfuel.domain.enterprise.sync.QueueRetryState
import java.time.Instant

/*
 * EMP-OFFL-005 — the offline attachment queue (B-BR-EMP-415..420, EDR-267..270). Operations are delivered by the
 * OFFL-003 queue; this service only declares, links, sends and verifies their attachments, each from its own
 * server answers.
 */

/** One protected snapshot: the index and the retained queue entries of the operations it names. */
class EnterpriseAttachmentSnapshot(val book: AttachmentBook?, owners: Map<String, EnterpriseQueueEntry>) {
    private val values = owners.toMap()
    val owners: Map<String, EnterpriseQueueEntry> get() = values.toMap()
    override fun toString(): String = "EnterpriseAttachmentSnapshot(redacted)"
}

/** The protected attachment port of the creation partition (EDR-267). */
interface EnterpriseAttachmentStore {
    /** The index entry and its sealed bytes in one protected write; the owner must be queued in the same snapshot. */
    suspend fun capture(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment, bytes: ByteArray)
    suspend fun read(attempt: EnterpriseQueueAttempt): EnterpriseAttachmentSnapshot
    /** The sealed bytes, or null when they are absent. */
    suspend fun bytes(attempt: EnterpriseQueueAttempt, documentId: String): ByteArray?
    /** Compare-and-set; a `STORED` next state removes the sealed bytes in the same write. */
    suspend fun transition(attempt: EnterpriseQueueAttempt, expected: EnterpriseAttachment, next: EnterpriseAttachment): Boolean
}

/** A session answer: opened (with the bound response) or finally refused with a closed code. */
sealed interface AttachmentSessionResult {
    class Opened(val response: AttachmentSessionResponse) : AttachmentSessionResult {
        override fun toString(): String = "AttachmentSessionOpened(redacted)"
    }
    class Refused(val refusal: AttachmentRefusal) : AttachmentSessionResult {
        override fun toString(): String = "AttachmentSessionRefused(redacted)"
    }
}

/** An ingress answer: the document's storage after a 200, or a refused transfer to retry. */
sealed interface AttachmentTransferResult {
    class Settled(val storage: AttachmentStorage, val revision: Long) : AttachmentTransferResult {
        override fun toString(): String = "AttachmentTransferSettled(redacted)"
    }
    class Rejected(val retryAfterMillis: Long? = null) : AttachmentTransferResult {
        override fun toString(): String = "AttachmentTransferRejected(redacted)"
    }
}

/**
 * The signed session route and the capability-bound ingress. Retryable transport failures throw
 * [com.anpfuel.application.enterprise.queue.EnterpriseQueueDeliveryFailure].
 */
interface EnterpriseAttachmentTransport {
    suspend fun open(attempt: EnterpriseQueueAttempt, attachment: EnterpriseAttachment): AttachmentSessionResult
    suspend fun upload(attempt: EnterpriseQueueAttempt, documentId: String, grant: AttachmentUploadGrant, bytes: ByteArray): AttachmentTransferResult
}

/** Aggregate counts of one delivery run; nothing private leaves it. */
class EnterpriseAttachmentDeliveryResult(val attempted: Int = 0, val stored: Int = 0, val refused: Int = 0, val missing: Int = 0,
    val pending: Int = 0, val nextAttemptMillis: Long? = null) {
    override fun toString(): String = "EnterpriseAttachmentDeliveryResult(redacted)"
}

class EnterpriseAttachmentService(
    private val source: EnterpriseQueueSource,
    private val store: EnterpriseAttachmentStore,
    private val transport: EnterpriseAttachmentTransport,
    private val clock: () -> EnterpriseDeviceClock,
    private val wallClock: () -> Instant,
    private val entropy: (Long) -> Long,
    private val newId: () -> String,
    private val enabled: Boolean,
    private val retryPolicy: QueueRetryPolicy = QueueRetryPolicy(),
) {
    private suspend fun attempt(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseQueueAttempt {
        account.assertCurrent()
        if (!account.identity.owns(partition)) throw EnterpriseLocalDenied()
        val attempt = source.attempt(account, partition)
        if (attempt.account !== account || attempt.partition != partition) throw EnterpriseLocalDenied()
        attempt.assertCurrent()
        return attempt
    }

    /** Captures a synthetic JPEG for an operation already saved in the partition (B-BR-EMP-415). */
    suspend fun capture(partition: EnterprisePartition, ownerOperationId: String, target: AttachmentTarget, jpeg: ByteArray): EnterpriseAttachment {
        if (!enabled) throw AttachmentLocalException(AttachmentLocalFailure.DISABLED)
        EnterprisePartition.requireId(ownerOperationId)
        val bytes = jpeg.copyOf()
        try {
            val sha256 = AttachmentPolicy.checkJpeg(bytes)
            val ids = List(3) { EnterprisePartition.requireId(newId()) }
            val item = EnterpriseAttachment(ids[0], ids[1], ids[2], ownerOperationId, target, sha256, bytes.size.toLong(), wallClock())
            val attempt = attempt(source.account(), partition)
            store.capture(attempt, item, bytes)
            attempt.assertCurrent()
            return item
        } finally { bytes.fill(0) }
    }

    /** Every attachment of the partition with its own label. */
    suspend fun attachments(partition: EnterprisePartition): List<AttachmentView> {
        val snapshot = store.read(attempt(source.account(), partition))
        val book = snapshot.book ?: return emptyList()
        return book.items.map { AttachmentPresentation.view(it, snapshot.owners[it.ownerOperationId]?.acknowledgement) }
    }

    private class Run {
        var attempted = 0; var stored = 0; var refused = 0; var missing = 0; var pending = 0
        var next: Long? = null
        fun due(delay: Long) { next = minOf(next ?: delay, delay) }
    }

    /** One delivery pass over the due attachments of the partition (EDR-268). */
    suspend fun deliver(account: EnterpriseQueueAccountLease, partition: EnterprisePartition): EnterpriseAttachmentDeliveryResult {
        if (!enabled) return EnterpriseAttachmentDeliveryResult()
        val attempt = attempt(account, partition)
        val snapshot = store.read(attempt)
        val book = snapshot.book ?: return EnterpriseAttachmentDeliveryResult()
        val run = Run()
        for (item in book.items) {
            currentCoroutineContext().ensureActive()
            attempt.assertCurrent()
            if (item.settled) continue
            val now = clock()
            if (!item.retry.isDue(now)) { run.pending++; run.due(remaining(item.retry, now)); continue }
            val gate = AttachmentDelivery.ownerGate(snapshot.owners[item.ownerOperationId]?.acknowledgement)
            if (gate == AttachmentOwnerGate.WAIT) { run.pending++; continue }
            run.attempted++
            if (gate == AttachmentOwnerGate.REFUSE) {
                record(attempt, item, item.copy(state = AttachmentState.REFUSED, refusal = AttachmentRefusal.OWNER_REJECTED), run)
                continue
            }
            deliverOne(attempt, item, run)
        }
        attempt.assertCurrent()
        return EnterpriseAttachmentDeliveryResult(run.attempted, run.stored, run.refused, run.missing, run.pending, run.next)
    }

    private suspend fun deliverOne(attempt: EnterpriseQueueAttempt, item: EnterpriseAttachment, run: Run) {
        val sealed = store.bytes(attempt, item.documentId)
        try {
            // A sealed entry that is missing or no longer matches its declaration is never sent (B-BR-EMP-419).
            if (sealed == null || sealed.size.toLong() != item.bytes || AttachmentPolicy.sha256(sealed) != item.sha256) {
                record(attempt, item, item.copy(state = AttachmentState.FILE_MISSING), run)
                return
            }
            val opened = try { transport.open(attempt, item) }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (failure: EnterpriseQueueDeliveryFailure) { retry(attempt, item, failure.retryAfterMillis, run); return }
            catch (_: Exception) { retry(attempt, item, null, run); return }
            attempt.assertCurrent()
            val response = when (opened) {
                is AttachmentSessionResult.Refused -> {
                    record(attempt, item, item.copy(state = AttachmentState.REFUSED, refusal = opened.refusal), run); return
                }
                is AttachmentSessionResult.Opened -> opened.response
            }
            if (response.documentId != item.documentId) { retry(attempt, item, null, run); return }
            val settled = AttachmentDelivery.settle(item, response.storage, response.storageRevision)
            val grant = response.upload
            if (response.storage != AttachmentStorage.UPLOADING || grant == null) {
                if (settled.state == AttachmentState.DECLARED || settled.state == AttachmentState.RECEIVED) retry(attempt, item, null, run, settled)
                else record(attempt, item, settled, run)
                return
            }
            // The declaration and link exist; record it before any byte leaves the device.
            if (!store.transition(attempt, item, settled)) { run.pending++; return }
            val transfer = try { transport.upload(attempt, item.documentId, grant, sealed) }
            catch (cancelled: CancellationException) { throw cancelled }
            catch (denied: EnterpriseLocalDenied) { throw denied }
            catch (failure: EnterpriseQueueDeliveryFailure) { retry(attempt, settled, failure.retryAfterMillis, run); return }
            catch (_: Exception) { retry(attempt, settled, null, run); return }
            attempt.assertCurrent()
            when (transfer) {
                is AttachmentTransferResult.Rejected -> retry(attempt, settled, transfer.retryAfterMillis, run)
                is AttachmentTransferResult.Settled -> {
                    val next = AttachmentDelivery.settle(settled, transfer.storage, transfer.revision)
                    if (next.state == AttachmentState.DECLARED || next.state == AttachmentState.RECEIVED) retry(attempt, settled, null, run, next)
                    else record(attempt, settled, next, run)
                }
            }
        } finally { sealed?.fill(0) }
    }

    private fun remaining(retry: QueueRetryState, now: EnterpriseDeviceClock): Long {
        val at = retry.clock ?: return 0
        if (at.boot != now.boot || now.elapsedMillis < at.elapsedMillis) return retry.delayMillis
        return maxOf(0, retry.delayMillis - (now.elapsedMillis - at.elapsedMillis))
    }

    /** Keeps the attachment pending with the OFFL-003 backoff; [next] carries a server answer to keep. */
    private suspend fun retry(attempt: EnterpriseQueueAttempt, expected: EnterpriseAttachment, floor: Long?, run: Run,
        next: EnterpriseAttachment = expected) {
        val attempts = retryPolicy.nextAttempts(expected.retry.attempts)
        val delay = retryPolicy.delayMillis(attempts, entropy, floor)
        run.pending++
        run.due(delay)
        try { store.transition(attempt, expected, next.copy(retry = QueueRetryState(attempts, clock(), delay))) }
        catch (cancelled: CancellationException) { throw cancelled }
        catch (denied: EnterpriseLocalDenied) { throw denied }
        catch (_: Exception) {
            // The committed state stays; a failed retry write never becomes progress.
        }
    }

    private suspend fun record(attempt: EnterpriseQueueAttempt, expected: EnterpriseAttachment, next: EnterpriseAttachment, run: Run) {
        if (!store.transition(attempt, expected, next)) { run.pending++; return }
        when (next.state) {
            AttachmentState.STORED -> run.stored++
            AttachmentState.REFUSED -> run.refused++
            AttachmentState.FILE_MISSING -> run.missing++
            else -> run.pending++
        }
    }

    override fun toString(): String = "EnterpriseAttachmentService(redacted)"
}
