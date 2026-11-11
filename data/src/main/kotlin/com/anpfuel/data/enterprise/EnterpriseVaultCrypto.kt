package com.anpfuel.data.enterprise

import com.anpfuel.domain.enterprise.EnterpriseDeviceClock
import com.anpfuel.domain.enterprise.EnterpriseOfflineLease
import com.anpfuel.domain.enterprise.EnterprisePartition
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.DataInputStream
import java.io.DataOutputStream
import java.security.MessageDigest
import javax.crypto.Cipher
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

data class EnterpriseVaultKey(val secret: SecretKey, val managedUnlockSeconds: Int?) {
    override fun toString(): String = "EnterpriseVaultKey(redacted)"
}

/** Reads never create a key. Deletion must report failure, never quietly succeed. */
interface EnterpriseVaultKeys {
    fun existing(alias: String): EnterpriseVaultKey?
    fun create(alias: String, managedUnlockSeconds: Int?): EnterpriseVaultKey
    fun delete(alias: String)
    fun aliases(prefix: String): List<String>
}

internal enum class VaultKind { RECORD, OPERATION, ATTACHMENT, CACHE }
internal class VaultEntry(val kind: VaultKind, val id: String, val target: String, val bytes: ByteArray)
internal class VaultSnapshot(
    val partition: EnterprisePartition,
    var lease: EnterpriseOfflineLease,
    val entries: LinkedHashMap<String, VaultEntry> = linkedMapOf(),
)

internal object EnterpriseVaultCodec {
    // Local allocation/decoding budgets, not a product quota. Never evict to fit.
    const val MAX_PLAIN = 32 * 1024 * 1024
    const val MAX_ENTRIES = 10000
    const val MAX_SEALED = MAX_PLAIN + 32
    private val magic = byteArrayOf(65, 86, 48, 49) // AV01, format and AAD version.

    private fun frame(vararg values: String): ByteArray = ByteArrayOutputStream().also { bytes ->
        DataOutputStream(bytes).use { out -> values.forEach(out::writeUTF) }
    }.toByteArray()

    private fun digest(bytes: ByteArray): String = MessageDigest.getInstance("SHA-256")
        .digest(bytes).joinToString("") { "%02x".format(it) }

    fun accountTag(owner: String, origin: String): String = digest(frame(
        "Abastevo.enterprise.account.v1", EnterprisePartition.requireId(owner), EnterprisePartition.canonicalOrigin(origin),
    ))

    fun token(p: EnterprisePartition): String = accountTag(p.ownerAccount, p.origin) + "-" +
        digest(frame("Abastevo.enterprise.partition.v1", p.ownerAccount, p.origin, p.organizationId))

    fun alias(token: String): String = "abastevo_enterprise_v1_" + token
    private fun aad(p: EnterprisePartition): ByteArray =
        magic + frame("Abastevo.enterprise.vault.v1", p.ownerAccount, p.origin, p.organizationId)

    fun seal(snapshot: VaultSnapshot, key: SecretKey): ByteArray {
        val plain = encode(snapshot)
        try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, key) // Provider IV; AndroidKeyStore forbids a caller IV.
            check(cipher.iv.size == 12)
            cipher.updateAAD(aad(snapshot.partition))
            return magic + cipher.iv + cipher.doFinal(plain)
        } finally { plain.fill(0) }
    }

    fun open(bytes: ByteArray, p: EnterprisePartition, key: SecretKey): VaultSnapshot {
        require(bytes.size in 32..MAX_SEALED && bytes.copyOfRange(0, 4).contentEquals(magic))
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, bytes.copyOfRange(4, 16)))
        cipher.updateAAD(aad(p))
        val plain = cipher.doFinal(bytes, 16, bytes.size - 16)
        try { return decode(plain, p) } finally { plain.fill(0) }
    }

    private fun encode(s: VaultSnapshot): ByteArray {
        require(s.entries.size <= MAX_ENTRIES)
        require(s.entries.values.sumOf { it.bytes.size.toLong() + 512 } + 2048 <= MAX_PLAIN)
        return ByteArrayOutputStream().also { bytes ->
            DataOutputStream(bytes).use { out ->
                out.writeInt(1)
                out.writeUTF(s.partition.ownerAccount); out.writeUTF(s.partition.origin); out.writeUTF(s.partition.organizationId)
                val l = s.lease
                out.writeUTF(l.sessionFamily); out.writeLong(l.serverContactMillis)
                out.writeUTF(l.contactClock.boot); out.writeLong(l.contactClock.elapsedMillis); out.writeLong(l.contactClock.wallMillis)
                out.writeLong(l.validityMillis); out.writeInt(l.managedUnlockSeconds ?: 0)
                out.writeInt(s.entries.size)
                s.entries.values.forEach { e ->
                    out.writeInt(e.kind.ordinal); out.writeUTF(e.id); out.writeUTF(e.target)
                    out.writeInt(e.bytes.size); out.write(e.bytes)
                }
            }
        }.toByteArray().also { require(it.size <= MAX_PLAIN) }
    }

    private fun decode(bytes: ByteArray, p: EnterprisePartition): VaultSnapshot {
        require(bytes.size <= MAX_PLAIN)
        DataInputStream(ByteArrayInputStream(bytes)).use { input ->
            require(input.readInt() == 1)
            require(input.readUTF() == p.ownerAccount && input.readUTF() == p.origin && input.readUTF() == p.organizationId)
            val family = input.readUTF(); val server = input.readLong()
            val clock = EnterpriseDeviceClock(input.readUTF(), input.readLong(), input.readLong())
            val validity = input.readLong(); val unlock = input.readInt()
            require(unlock >= 0)
            val lease = EnterpriseOfflineLease(family, server, clock, validity, unlock.takeUnless { it == 0 })
            val snapshot = VaultSnapshot(p, lease)
            val count = input.readInt()
            require(count in 0..MAX_ENTRIES)
            repeat(count) {
                val ordinal = input.readInt()
                require(ordinal in VaultKind.entries.indices)
                val kind = VaultKind.entries[ordinal]
                val id = EnterprisePartition.requireId(input.readUTF())
                val target = input.readUTF()
                require(if (kind == VaultKind.OPERATION) runCatching { EnterprisePartition.requireId(target) }.isSuccess else target.isEmpty())
                val size = input.readInt()
                require(size in 0..MAX_PLAIN && size <= input.available())
                val payload = ByteArray(size).also(input::readFully)
                val entry = VaultEntry(kind, id, target, payload)
                require(snapshot.entries.put(kind.name + ":" + id, entry) == null)
            }
            require(input.available() == 0)
            return snapshot
        }
    }
}
