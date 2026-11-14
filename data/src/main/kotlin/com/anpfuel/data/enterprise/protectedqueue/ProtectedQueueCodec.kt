package com.anpfuel.data.enterprise.protectedqueue

import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.data.enterprise.PlainBuffer
import com.anpfuel.data.enterprise.VaultSnapshot
import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.sync.*
import java.io.ByteArrayInputStream
import java.io.DataInputStream
import java.io.DataOutputStream

/**
 * Versioned framing inside authenticated ciphertext; opaque legacy OPERATION bytes never enter here.
 * V1/V2 frames stay readable. A record is written as V3 only while it carries review-only state
 * (a separate resolution or non-initial review retry); otherwise the minimal V2 frame is kept.
 */
internal object ProtectedQueueCodec {
    private const val MAGIC = 0x51554555 // QUEU
    private const val VERSION = 3
    private const val MAX_PART = 8192
    private const val MAX_FRAME = 2 * MAX_PART + ReviewResolution.MAX_BYTES + 2048
    private const val NOT_DISMISSED = 0
    private const val DISMISSED_REJECTED_RECEIPT = 1
    private const val DISMISSED_REFUSED_RESOLUTION = 2

    fun encode(entry: EnterpriseQueueEntry): ByteArray {
        val buffer = PlainBuffer(MAX_FRAME)
        try {
            if (entry.dismissedRejected && !entry.refusedOutcome) throw EnterprisePersistenceUnavailable()
            val version = if (entry.resolution != null || !entry.reviewRetry.initial) VERSION else 2
            DataOutputStream(buffer).use { out ->
                out.writeInt(MAGIC); out.writeInt(version)
                val operation = entry.operation.canonicalBytes
                try { require(operation.size in 1..MAX_PART); out.writeInt(operation.size); out.write(operation) }
                finally { operation.fill(0) }
                val acknowledgement = entry.acknowledgement?.canonicalBytes
                try {
                    out.writeInt(acknowledgement?.size ?: -1)
                    if (acknowledgement != null) { require(acknowledgement.size in 1..MAX_PART); out.write(acknowledgement) }
                } finally { acknowledgement?.fill(0) }
                writeRetry(out, entry.retry)
                if (version == 2) out.writeBoolean(entry.dismissedRejected)
                else {
                    out.writeByte(when {
                        !entry.dismissedRejected -> NOT_DISMISSED
                        entry.acknowledgement?.result == SyncResult.REJECTED -> DISMISSED_REJECTED_RECEIPT
                        else -> DISMISSED_REFUSED_RESOLUTION
                    })
                    val resolution = entry.resolution?.canonicalBytes
                    try {
                        out.writeInt(resolution?.size ?: -1)
                        if (resolution != null) { require(resolution.size in 1..ReviewResolution.MAX_BYTES); out.write(resolution) }
                    } finally { resolution?.fill(0) }
                    writeRetry(out, entry.reviewRetry)
                }
            }
            require(buffer.length <= MAX_FRAME)
            return buffer.toByteArray()
        } finally { buffer.wipe() }
    }

    fun decode(bytes: ByteArray, snapshot: VaultSnapshot, operationId: String): EnterpriseQueueEntry {
        if (bytes.size !in 1..MAX_FRAME) throw EnterprisePersistenceUnavailable()
        DataInputStream(ByteArrayInputStream(bytes)).use { input ->
            // Unknown framing is preserved in the original generation, never repaired or erased.
            val magic = input.readInt()
            val version = input.readInt()
            if (magic != MAGIC || version !in 1..VERSION) throw EnterprisePersistenceUnavailable()
            val operationBytes = part(input, input.readInt(), MAX_PART)
            val operation = try {
                val p = snapshot.partition
                EnterpriseOperation.parse(operationBytes, SyncBinding.of(p.ownerAccount, p.origin, p.organizationId)).also {
                    if (!operationBytes.contentEquals(it.canonicalBytes)) throw EnterprisePersistenceUnavailable()
                    if (it.operationId != operationId) throw EnterprisePersistenceUnavailable()
                    checkBinding(it, snapshot)
                }
            } finally { operationBytes.fill(0) }
            val acknowledgementSize = input.readInt()
            val acknowledgement = if (acknowledgementSize == -1) null else {
                val raw = part(input, acknowledgementSize, MAX_PART)
                try {
                    OperationAcknowledgement.parse(raw, operation).also {
                        if (!it.isFinal || !raw.contentEquals(it.canonicalBytes)) throw EnterprisePersistenceUnavailable()
                    }
                } finally { raw.fill(0) }
            }
            val retry = readRetry(input)
            val dismissedKind = when (version) {
                1 -> NOT_DISMISSED
                2 -> when (input.readUnsignedByte()) {
                    0 -> NOT_DISMISSED
                    1 -> DISMISSED_REJECTED_RECEIPT
                    else -> throw EnterprisePersistenceUnavailable()
                }
                else -> input.readUnsignedByte()
            }
            var resolution: ReviewResolution? = null
            var reviewRetry = QueueRetryState()
            if (version == VERSION) {
                val resolutionSize = input.readInt()
                if (resolutionSize != -1) {
                    val raw = part(input, resolutionSize, ReviewResolution.MAX_BYTES)
                    try {
                        resolution = try { ReviewResolution.parse(raw) } catch (_: SyncProtocolException) { throw EnterprisePersistenceUnavailable() }
                        if (!raw.contentEquals(resolution.canonicalBytes)) throw EnterprisePersistenceUnavailable()
                    } finally { raw.fill(0) }
                }
                reviewRetry = readRetry(input)
            }
            // Dismissal must name the effective refused outcome actually retained in this frame.
            when (dismissedKind) {
                NOT_DISMISSED -> Unit
                DISMISSED_REJECTED_RECEIPT -> if (acknowledgement?.result != SyncResult.REJECTED) throw EnterprisePersistenceUnavailable()
                DISMISSED_REFUSED_RESOLUTION -> if (acknowledgement?.result != SyncResult.ACCEPTED_FOR_REVIEW ||
                    resolution?.snapshot()?.outcome != ReviewResolution.REFUSED) throw EnterprisePersistenceUnavailable()
                else -> throw EnterprisePersistenceUnavailable()
            }
            // A V3 frame without review-only state is never produced; refuse it rather than adopting ambiguous framing.
            if (version == VERSION && resolution == null && reviewRetry.initial) throw EnterprisePersistenceUnavailable()
            if (input.available() != 0) throw EnterprisePersistenceUnavailable()
            return try {
                EnterpriseQueueEntry(operation, retry, acknowledgement, dismissedKind != NOT_DISMISSED, resolution, reviewRetry)
            } catch (_: IllegalArgumentException) { throw EnterprisePersistenceUnavailable() }
        }
    }

    fun checkBinding(operation: EnterpriseOperation, snapshot: VaultSnapshot) {
        val bound = snapshot.bound ?: throw EnterpriseLocalDenied()
        val p = snapshot.partition
        if (operation.binding.ownerAccountId != p.ownerAccount || operation.binding.apiOrigin != p.origin ||
            operation.binding.organizationId != p.organizationId || operation.membershipId != bound.membershipId ||
            operation.enrollmentId != bound.enrollmentId) throw EnterpriseLocalDenied()
    }

    private fun writeRetry(out: DataOutputStream, retry: QueueRetryState) {
        out.writeLong(retry.attempts)
        val clock = retry.clock
        out.writeBoolean(clock != null)
        if (clock != null) { out.writeUTF(clock.boot); out.writeLong(clock.elapsedMillis); out.writeLong(clock.wallMillis) }
        out.writeLong(retry.delayMillis)
    }

    private fun readRetry(input: DataInputStream): QueueRetryState {
        val attempts = input.readLong()
        val clock = when (input.readUnsignedByte()) {
            0 -> null
            1 -> EnterpriseDeviceClock(input.readUTF(), input.readLong(), input.readLong())
            else -> throw EnterprisePersistenceUnavailable()
        }
        return try { QueueRetryState(attempts, clock, input.readLong()) }
        catch (_: IllegalArgumentException) { throw EnterprisePersistenceUnavailable() }
    }

    private fun part(input: DataInputStream, size: Int, maximum: Int): ByteArray {
        if (size !in 1..maximum || size > input.available()) throw EnterprisePersistenceUnavailable()
        return ByteArray(size).also(input::readFully)
    }
}
