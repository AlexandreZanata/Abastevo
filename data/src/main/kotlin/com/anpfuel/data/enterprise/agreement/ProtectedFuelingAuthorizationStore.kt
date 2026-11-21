package com.anpfuel.data.enterprise.agreement

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.agreement.EnterpriseFuelingAuthorizationProducers
import com.anpfuel.application.enterprise.agreement.FieldVoucherNotice
import com.anpfuel.application.enterprise.queue.EnterpriseQueueAttempt
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.data.enterprise.VaultEntry
import com.anpfuel.data.enterprise.VaultKind
import com.anpfuel.data.enterprise.VaultQueueChange
import java.io.ByteArrayInputStream
import java.io.DataInputStream

/**
 * Protected local storage for the driver's own Phase F7 fueling
 * authorization notices inside the encrypted enterprise vault (no
 * Room/plaintext storage). The one-time voucher code is never persisted;
 * only the field-safe notice metadata is.
 */
class ProtectedFuelingAuthorizationStore(
    private val persistence: ProtectedEnterprisePersistence,
) {
    suspend fun read(attempt: EnterpriseQueueAttempt): List<FieldVoucherNotice>? =
        persistence.queueAccess(attempt) { snapshot ->
            val stored = snapshot.entries[NOTICE_KEY] ?: return@queueAccess VaultQueueChange(null)
            val notices = try {
                decodeNotices(stored.bytes)
            } catch (_: Exception) {
                throw EnterprisePersistenceUnavailable()
            }
            VaultQueueChange(notices)
        }

    suspend fun save(attempt: EnterpriseQueueAttempt, notices: List<FieldVoucherNotice>) {
        persistence.queueAccess(attempt) { snapshot ->
            val bytes = encodeNotices(notices)
            val unchanged = snapshot.entries[NOTICE_KEY]?.bytes?.contentEquals(bytes) == true
            if (unchanged) {
                VaultQueueChange(Unit)
            } else {
                snapshot.entries[NOTICE_KEY] = VaultEntry(
                    kind = VaultKind.RECORD,
                    id = EnterpriseFuelingAuthorizationProducers.VOUCHER_NOTICE_RECORD_ID,
                    target = "",
                    bytes = bytes,
                )
                VaultQueueChange(Unit, changed = true)
            }
        }
    }

    override fun toString(): String = "ProtectedFuelingAuthorizationStore(redacted)"

    private companion object {
        const val FORMAT_VERSION = 1
        const val NOTICE_KEY = "RECORD:" + EnterpriseFuelingAuthorizationProducers.VOUCHER_NOTICE_RECORD_ID
    }
}

private fun encodeNotices(notices: List<FieldVoucherNotice>): ByteArray {
    val out = java.io.ByteArrayOutputStream()
    java.io.DataOutputStream(out).use { stream ->
        stream.writeInt(1)
        stream.writeInt(notices.size)
        for (notice in notices) {
            stream.writeUTF(notice.authorizationId)
            stream.writeUTF(notice.product)
            stream.writeUTF(notice.unit)
            stream.writeLong(notice.maxQuantityMilli)
            stream.writeLong(notice.maxCentavos)
            stream.writeLong(notice.validUntilEpochSeconds)
            stream.writeUTF(notice.storedState)
        }
    }
    return out.toByteArray()
}

private fun decodeNotices(bytes: ByteArray): List<FieldVoucherNotice> {
    val input = DataInputStream(ByteArrayInputStream(bytes))
    val format = input.readInt()
    if (format != 1) throw IllegalArgumentException("unsupported fueling authorization notice format")
    val count = input.readInt()
    if (count < 0 || count > 100) throw IllegalArgumentException("fueling authorization notice count invalid")
    val out = mutableListOf<FieldVoucherNotice>()
    repeat(count) {
        val notice = FieldVoucherNotice(
            authorizationId = input.readUTF(),
            product = input.readUTF(),
            unit = input.readUTF(),
            maxQuantityMilli = input.readLong(),
            maxCentavos = input.readLong(),
            validUntilEpochSeconds = input.readLong(),
            storedState = input.readUTF(),
        )
        out.add(notice)
    }
    return out
}
