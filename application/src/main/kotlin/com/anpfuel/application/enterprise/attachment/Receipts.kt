package com.anpfuel.application.enterprise.attachment

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.attachment.AttachmentLocalException
import com.anpfuel.domain.enterprise.attachment.AttachmentLocalFailure
import com.anpfuel.domain.enterprise.attachment.AttachmentTarget
import com.anpfuel.domain.enterprise.attachment.AttachmentView
import com.anpfuel.domain.enterprise.attachment.EnterpriseAttachment
import com.anpfuel.domain.enterprise.attachment.LegibilityHint
import com.anpfuel.domain.enterprise.attachment.LumaSample
import com.anpfuel.domain.enterprise.attachment.ReceiptInspection
import com.anpfuel.domain.enterprise.attachment.ReceiptIntake
import com.anpfuel.domain.enterprise.attachment.ReceiptLegibility
import com.anpfuel.domain.enterprise.attachment.ReceiptOriginal
import com.anpfuel.domain.enterprise.attachment.ReceiptRefusal
import com.anpfuel.domain.enterprise.attachment.ReceiptSource
import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.withContext

/*
 * EMP-DOC-004 — capture and import (B-BR-EMP-450..454, EDR-291..293). The platform reads the selected file once;
 * the controller keeps the unsaved photo in memory only; the record's own flow seals it through EMP-OFFL-005.
 */

/** The platform side of a capture: a private camera file, bounded reads and a decoded sample (EDR-291). */
interface ReceiptOriginals {
    /** Deletes leftover camera files, then creates a fresh private camera target (a content reference). */
    suspend fun cameraTarget(): String
    /** At most [com.anpfuel.domain.enterprise.attachment.ReceiptIntake.READ_LIMIT] bytes, or null when nothing is readable. */
    suspend fun read(reference: String): ByteArray?
    /** Deletes a camera target; references the app did not create are left alone. */
    suspend fun discard(reference: String)
    /** A downscaled luminance sample, or null when the bytes do not decode (EDR-292). */
    suspend fun sample(bytes: ByteArray): LumaSample?
}

/** The unsaved photo of one capture: memory only, gone with the process (B-BR-EMP-453). */
class ReceiptDraft(val original: ReceiptOriginal, val hint: LegibilityHint) {
    override fun toString(): String = "ReceiptDraft(redacted)"
}

/** What one capture sheet shows; nothing in it is persisted. */
data class ReceiptCaptureState(
    val draft: ReceiptDraft? = null,
    /** The latest selection was refused; a previous unsaved photo stays. */
    val refusal: ReceiptRefusal? = null,
    val cameraDenied: Boolean = false,
    val cancelled: Boolean = false,
    val reading: Boolean = false,
) {
    override fun toString(): String = "ReceiptCaptureState(redacted)"
}

/** One capture sheet's state holder; the record flow saves its draft (EDR-293). */
class ReceiptCaptureController(private val originals: ReceiptOriginals) {
    private val mutable = MutableStateFlow(ReceiptCaptureState())
    val state: StateFlow<ReceiptCaptureState> = mutable.asStateFlow()
    private val lock = Any()
    private var currentRequest: String? = null

    /** Bind a picker gesture before asking Android for permission or launching an activity. */
    fun beginSelection(): String = synchronized(lock) {
        val request = UUID.randomUUID().toString()
        currentRequest = request
        mutable.value = mutable.value.copy(reading = false)
        request
    }

    private fun isCurrent(request: String): Boolean = synchronized(lock) { currentRequest == request }

    /** Create and clean up a private target even when its preparing coroutine loses ownership. */
    suspend fun cameraTarget(request: String): String? {
        if (!isCurrent(request)) return null
        val target = withContext(NonCancellable) { originals.cameraTarget() }
        if (isCurrent(request) && currentCoroutineContext().isActive) return target
        discardCamera(target)
        return null
    }

    suspend fun cameraResult(request: String, reference: String, taken: Boolean) {
        if (taken) capture(request, reference, ReceiptSource.CAMERA)
        else {
            discardCamera(reference)
            showCancellation(request)
        }
    }

    suspend fun imported(request: String, reference: String?) {
        if (reference == null) showCancellation(request)
        else capture(request, reference, ReceiptSource.IMPORT)
    }

    fun cameraDenied(request: String) {
        update(request) { it.copy(cameraDenied = true, cancelled = false, refusal = null) }
    }

    /** Invalidate pending callbacks and remove the unsaved photo and messages together. */
    fun reset() {
        synchronized(lock) {
            currentRequest = null
            mutable.value = ReceiptCaptureState()
        }
    }

    private fun update(request: String, change: (ReceiptCaptureState) -> ReceiptCaptureState): Boolean = synchronized(lock) {
        if (currentRequest != request) return@synchronized false
        mutable.value = change(mutable.value)
        true
    }

    private fun showCancellation(request: String) {
        update(request) { it.copy(cancelled = true, cameraDenied = false, refusal = null) }
    }

    private suspend fun discardCamera(reference: String) = withContext(NonCancellable) {
        originals.discard(reference)
    }

    private suspend fun capture(request: String, reference: String, source: ReceiptSource) {
        val reading = update(request) { it.copy(reading = true) }
        var bytes: ByteArray? = null
        try {
            try {
                if (reading) bytes = originals.read(reference)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                // A vanished or unreadable file follows the existing missing-file path.
            } finally {
                if (source == ReceiptSource.CAMERA) discardCamera(reference)
            }
            if (!reading || !isCurrent(request)) return
            when (val inspection = ReceiptIntake.inspect(bytes, source)) {
                is ReceiptInspection.Refused -> update(request) {
                    it.copy(refusal = inspection.refusal, cameraDenied = false, cancelled = false)
                }
                is ReceiptInspection.Accepted -> {
                    val sampleBytes = inspection.original.bytes()
                    val sample = try { originals.sample(sampleBytes) }
                    catch (cancelled: CancellationException) { throw cancelled }
                    catch (_: Exception) { null }
                    finally { sampleBytes.fill(0) }
                    val draft = ReceiptDraft(inspection.original, ReceiptLegibility.assess(sample))
                    update(request) { ReceiptCaptureState(draft = draft) }
                }
            }
        } finally {
            bytes?.fill(0)
            if (reading) update(request) { it.copy(reading = false) }
        }
    }
    override fun toString(): String = "ReceiptCaptureController(redacted)"
}

/** A local refusal of a save with its closed reason; nothing was saved (B-BR-EMP-453). */
class ReceiptSaveRefused(val failure: AttachmentLocalFailure) : IllegalStateException("receipt not saved")

/** The thin EMP-DOC-004 face of the EMP-OFFL-005 attachment queue. */
class EnterpriseReceipts(private val attachments: EnterpriseAttachmentService, val enabled: Boolean) {
    /** Seals [draft] beside the already queued [ownerOperationId] of [partition]. */
    suspend fun save(partition: EnterprisePartition, ownerOperationId: String, target: AttachmentTarget, draft: ReceiptDraft): EnterpriseAttachment {
        val bytes = draft.original.bytes()
        try { return attachments.capture(partition, ownerOperationId, target, bytes) }
        catch (local: AttachmentLocalException) { throw ReceiptSaveRefused(local.failure) }
        finally { bytes.fill(0) }
    }

    /** Every attachment of the partition, grouped by its owning operation, in capture order. */
    suspend fun byOwner(partition: EnterprisePartition): Map<String, List<AttachmentView>> =
        if (!enabled) emptyMap() else attachments.attachments(partition).groupBy { it.attachment.ownerOperationId }
    override fun toString(): String = "EnterpriseReceipts(redacted)"
}
