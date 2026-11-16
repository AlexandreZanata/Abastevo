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
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

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

    /** A fresh private camera target; only after the camera permission was granted. */
    suspend fun cameraTarget(): String = originals.cameraTarget()

    /** The camera returned: read and delete its private file, then inspect the bytes. */
    suspend fun cameraResult(reference: String, taken: Boolean) {
        if (!taken) {
            try { originals.discard(reference) } finally { mutable.value = mutable.value.copy(cancelled = true, cameraDenied = false, refusal = null) }
            return
        }
        // The private camera file is the only plaintext moment: it is deleted whatever the read gives (EDR-291).
        val bytes = try { read(reference) } finally { originals.discard(reference) }
        accept(bytes, ReceiptSource.CAMERA)
    }

    /** The photo picker returned a reference, or null when nothing was chosen; the picker's file is never deleted. */
    suspend fun imported(reference: String?) {
        if (reference == null) { mutable.value = mutable.value.copy(cancelled = true, cameraDenied = false, refusal = null); return }
        accept(read(reference), ReceiptSource.IMPORT)
    }

    fun cameraDenied() { mutable.value = mutable.value.copy(cameraDenied = true, cancelled = false, refusal = null) }

    /** Drops the unsaved photo and every message. */
    fun reset() { mutable.value = ReceiptCaptureState() }

    private suspend fun read(reference: String): ByteArray? {
        mutable.value = mutable.value.copy(reading = true)
        return try { originals.read(reference) }
        catch (cancelled: CancellationException) { throw cancelled }
        // Revoked access or a vanished file reads as missing, never as a crash or a saved photo.
        catch (_: Exception) { null }
        finally { mutable.value = mutable.value.copy(reading = false) }
    }

    private suspend fun accept(bytes: ByteArray?, source: ReceiptSource) {
        try {
            when (val inspection = ReceiptIntake.inspect(bytes, source)) {
                // A refused selection never replaces the photo already shown (B-BR-EMP-451).
                is ReceiptInspection.Refused -> mutable.value = mutable.value.copy(refusal = inspection.refusal, cameraDenied = false, cancelled = false)
                is ReceiptInspection.Accepted -> {
                    val sample = try { originals.sample(inspection.original.bytes()) }
                    catch (cancelled: CancellationException) { throw cancelled }
                    catch (_: Exception) { null }
                    mutable.value = ReceiptCaptureState(draft = ReceiptDraft(inspection.original, ReceiptLegibility.assess(sample)))
                }
            }
        } finally { bytes?.fill(0) }
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
