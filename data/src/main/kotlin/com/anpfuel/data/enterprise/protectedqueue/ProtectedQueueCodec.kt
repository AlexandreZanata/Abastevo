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

/** Versioned framing inside authenticated ciphertext; opaque legacy OPERATION bytes never enter here. */
internal object ProtectedQueueCodec {
    private const val MAGIC = 0x51554555 // QUEU
    private const val VERSION = 1
    private const val MAX_PART = 8192
    private const val MAX_FRAME = 2 * MAX_PART + 2048

    fun encode(entry: EnterpriseQueueEntry): ByteArray {
        val buffer = PlainBuffer(MAX_FRAME)
        try {
            DataOutputStream(buffer).use { out ->
                out.writeInt(MAGIC); out.writeInt(VERSION)
                val operation = entry.operation.canonicalBytes
                try { require(operation.size in 1..MAX_PART); out.writeInt(operation.size); out.write(operation) }
                finally { operation.fill(0) }
                val acknowledgement = entry.acknowledgement?.canonicalBytes
                try {
                    out.writeInt(acknowledgement?.size ?: -1)
                    if (acknowledgement != null) { require(acknowledgement.size in 1..MAX_PART); out.write(acknowledgement) }
                } finally { acknowledgement?.fill(0) }
                out.writeLong(entry.retry.attempts)
                val clock = entry.retry.clock
                out.writeBoolean(clock != null)
                if (clock != null) { out.writeUTF(clock.boot); out.writeLong(clock.elapsedMillis); out.writeLong(clock.wallMillis) }
                out.writeLong(entry.retry.delayMillis)
            }
            require(buffer.length <= MAX_FRAME)
            return buffer.toByteArray()
        } finally { buffer.wipe() }
    }

    fun decode(bytes: ByteArray, snapshot: VaultSnapshot, operationId: String): EnterpriseQueueEntry {
        if (bytes.size !in 1..MAX_FRAME) throw EnterprisePersistenceUnavailable()
        DataInputStream(ByteArrayInputStream(bytes)).use { input ->
            // Unknown framing is preserved in the original generation, never repaired or erased.
            if (input.readInt() != MAGIC || input.readInt() != VERSION) throw EnterprisePersistenceUnavailable()
            val operationBytes = part(input, input.readInt())
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
                val raw = part(input, acknowledgementSize)
                try {
                    OperationAcknowledgement.parse(raw, operation).also {
                        if (!it.isFinal || !raw.contentEquals(it.canonicalBytes)) throw EnterprisePersistenceUnavailable()
                    }
                } finally { raw.fill(0) }
            }
            val attempts = input.readLong()
            val clock = when (input.readUnsignedByte()) {
                0 -> null
                1 -> EnterpriseDeviceClock(input.readUTF(), input.readLong(), input.readLong())
                else -> throw EnterprisePersistenceUnavailable()
            }
            val retry = QueueRetryState(attempts, clock, input.readLong())
            if (input.available() != 0) throw EnterprisePersistenceUnavailable()
            return EnterpriseQueueEntry(operation, retry, acknowledgement)
        }
    }

    fun checkBinding(operation: EnterpriseOperation, snapshot: VaultSnapshot) {
        val bound = snapshot.bound ?: throw EnterpriseLocalDenied()
        val p = snapshot.partition
        if (operation.binding.ownerAccountId != p.ownerAccount || operation.binding.apiOrigin != p.origin ||
            operation.binding.organizationId != p.organizationId || operation.membershipId != bound.membershipId ||
            operation.enrollmentId != bound.enrollmentId) throw EnterpriseLocalDenied()
    }

    private fun part(input: DataInputStream, size: Int): ByteArray {
        if (size !in 1..MAX_PART || size > input.available()) throw EnterprisePersistenceUnavailable()
        return ByteArray(size).also(input::readFully)
    }
}
