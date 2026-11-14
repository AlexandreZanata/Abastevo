package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseVaultUnrecoverable
import com.anpfuel.domain.enterprise.*
import java.io.*
import javax.crypto.Cipher
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

internal data class VaultMaintenance(val partition: EnterprisePartition, val lease: EnterpriseBoundOfflineLease) {
    override fun toString() = "VaultMaintenance(redacted)"
}

/** Alias contents are opaque hashes; only the trusted provider's head selects committed files. */
internal object EnterpriseVaultGenerationNames {
    const val PREFIX = "abastevo_enterprise_v2_"
    fun prefix(token: String) = PREFIX + token + "_"
    fun data(token: String, generation: Long) = prefix(token) + digits(generation) + "_d"
    fun maintenance(token: String, generation: Long) = prefix(token) + digits(generation) + "_m"
    fun head(token: String, generation: Long) = prefix(token) + digits(generation) + "_h"
    fun denied(token: String) = PREFIX + "denied_" + token
    fun retained(token: String) = PREFIX + "lock_" + token
    private fun digits(value: Long): String { require(value > 0); return value.toString().padStart(19, '0') }
    private val generation = Regex("([0-9]{19})_[dmh]")
    private val token = Regex("[0-9a-f]{64}-[0-9a-f]{64}")
    fun generation(alias: String, token: String): Long? =
        alias.takeIf { it.startsWith(prefix(token)) }?.removePrefix(prefix(token))?.let {
            generation.matchEntire(it)?.groupValues?.get(1)?.toLongOrNull()?.takeIf { n -> n > 0 }
        }
    fun token(alias: String): String? {
        if (!alias.startsWith(PREFIX)) return null
        val rest = alias.removePrefix(PREFIX).removePrefix("denied_").removePrefix("lock_")
        return rest.take(129).takeIf { token.matches(it) }
    }
}

/** No payload key or user unlock is required to enforce expiry and retry an owned cleanup. */
private object VaultMaintenanceCodec {
    private const val MAX = 16384
    private val magic = byteArrayOf(65, 77, 48, 50)
    private fun aad(token: String, generation: Long) = ("Abastevo.enterprise.maintenance.v2:" + token + ":" + generation).toByteArray(Charsets.US_ASCII)
    fun seal(value: VaultMaintenance, token: String, generation: Long, key: SecretKey): ByteArray {
        val buffer = PlainBuffer(MAX)
        DataOutputStream(buffer).use { out ->
            out.writeInt(2)
            out.writeLong(generation)
            out.writeUTF(value.partition.ownerAccount); out.writeUTF(value.partition.origin); out.writeUTF(value.partition.organizationId)
            out.writeUTF(value.lease.membershipId); out.writeUTF(value.lease.enrollmentId); out.writeLong(value.lease.enrollmentRevision)
            val time = value.lease.time
            out.writeUTF(time.sessionFamily); out.writeLong(time.serverContactMillis)
            out.writeUTF(time.contactClock.boot); out.writeLong(time.contactClock.elapsedMillis); out.writeLong(time.contactClock.wallMillis)
            out.writeLong(time.validityMillis); out.writeInt(time.managedUnlockSeconds ?: 0)
        }
        try {
            require(buffer.length <= MAX)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding"); cipher.init(Cipher.ENCRYPT_MODE, key)
            check(cipher.iv.size == 12); cipher.updateAAD(aad(token, generation))
            return magic + cipher.iv + cipher.doFinal(buffer.bytes, 0, buffer.length)
        } finally { buffer.wipe() }
    }
    fun open(bytes: ByteArray, token: String, generation: Long, key: SecretKey): VaultMaintenance {
        require(bytes.size in 32..MAX + 32 && bytes.copyOfRange(0, 4).contentEquals(magic))
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, bytes.copyOfRange(4, 16)))
        cipher.updateAAD(aad(token, generation))
        val plain = cipher.doFinal(bytes, 16, bytes.size - 16)
        try {
            DataInputStream(ByteArrayInputStream(plain)).use { input ->
                require(input.readInt() == 2 && input.readLong() == generation)
                val p = EnterprisePartition.of(input.readUTF(), input.readUTF(), input.readUTF())
                require(EnterpriseVaultCodec.token(p) == token)
                val member = input.readUTF(); val enrollment = input.readUTF(); val revision = input.readLong()
                val family = input.readUTF(); val server = input.readLong()
                val clock = EnterpriseDeviceClock(input.readUTF(), input.readLong(), input.readLong())
                val validity = input.readLong(); val unlock = input.readInt(); require(unlock >= 0)
                val result = VaultMaintenance(p, EnterpriseBoundOfflineLease(member, enrollment, revision,
                    EnterpriseOfflineLease(family, server, clock, validity, unlock.takeUnless { it == 0 })))
                require(input.available() == 0)
                return result
            }
        } finally { plain.fill(0) }
    }
}

/** All calls are made inside the account OS lock; trusted head markers are never file-backed. */
internal class EnterpriseVaultGenerations(private val files: EnterpriseVaultFiles, private val keys: EnterpriseVaultKeys) {
    private fun aliases(token: String): List<String> = keys.aliases(EnterpriseVaultGenerationNames.prefix(token)).also {
        // Allocation/preparation recovery bound, not an eviction quota.
        check(it.size <= 768)
    }
    fun head(token: String): Long? = aliases(token).filter { it.endsWith("_h") }
        .mapNotNull { EnterpriseVaultGenerationNames.generation(it, token) }.maxOrNull()

    fun descriptor(token: String): VaultMaintenance? {
        val generation = head(token)
        if (generation == null) {
            // A fixed-key legacy snapshot cannot acquire new membership/enrollment provenance.
            if (reading { files.read(token) } != null) throw EnterpriseVaultUnrecoverable()
            // Missing head cannot distinguish an unacknowledged first preparation
            // from lost trusted metadata for previously acknowledged work.
            if (files.generations(token).isNotEmpty() || aliases(token).isNotEmpty()) throw EnterpriseVaultUnrecoverable()
            return null
        }
        val key = keys.existing(EnterpriseVaultGenerationNames.maintenance(token, generation)) ?: throw EnterpriseVaultUnrecoverable()
        if (key.managedUnlockSeconds != null) throw EnterpriseVaultUnrecoverable()
        val bytes = reading { files.readGeneration(token, generation, true) } ?: throw EnterpriseVaultUnrecoverable()
        return decoding { VaultMaintenanceCodec.open(bytes, token, generation, key.secret) }
    }

    fun load(partition: EnterprisePartition): VaultSnapshot? {
        val token = EnterpriseVaultCodec.token(partition)
        val descriptor = descriptor(token) ?: return null
        if (descriptor.partition != partition) throw EnterpriseVaultUnrecoverable()
        val generation = head(token) ?: throw EnterpriseVaultUnrecoverable()
        val key = keys.existing(EnterpriseVaultGenerationNames.data(token, generation)) ?: throw EnterpriseVaultUnrecoverable()
        if (key.managedUnlockSeconds != descriptor.lease.time.managedUnlockSeconds) throw EnterpriseVaultUnrecoverable()
        val bytes = reading { files.readGeneration(token, generation, false) } ?: throw EnterpriseVaultUnrecoverable()
        val snapshot = decoding { EnterpriseVaultCodec.open(bytes, partition, key.secret, generation) }
        if (snapshot.bound != descriptor.lease || snapshot.lease != descriptor.lease.time) throw EnterpriseVaultUnrecoverable()
        return snapshot
    }

    private fun <T> reading(block: () -> T): T = try { block() } catch (_: EnterpriseVaultCorrupted) { throw EnterpriseVaultUnrecoverable() }

    private fun <T> decoding(block: () -> T): T = try { block() } catch (failure: Exception) {
        when (keys.classify(failure)) {
            EnterpriseKeyUse.UNLOCK_REQUIRED -> throw com.anpfuel.application.enterprise.EnterpriseUnlockRequired()
            EnterpriseKeyUse.INVALIDATED -> throw EnterpriseVaultUnrecoverable()
            EnterpriseKeyUse.OTHER -> if (failure is javax.crypto.AEADBadTagException || failure is IllegalArgumentException || failure is IOException) {
                throw EnterpriseVaultUnrecoverable()
            } else throw failure
        }
    }

    /** Data and metadata are durable before head creation; a failed preparation keeps the old head. */
    fun commit(snapshot: VaultSnapshot) {
        val bound = requireNotNull(snapshot.bound); require(bound.time == snapshot.lease)
        val token = EnterpriseVaultCodec.token(snapshot.partition)
        val previous = head(token)
        if (!cleanup(token, previous)) error("Enterprise generation cleanup incomplete")
        val generation = Math.addExact(previous ?: 0L, 1L)
        val data = keys.create(EnterpriseVaultGenerationNames.data(token, generation), bound.time.managedUnlockSeconds)
        val maintenance = keys.create(EnterpriseVaultGenerationNames.maintenance(token, generation), null)
        val sealed = try { EnterpriseVaultCodec.seal(snapshot, data.secret, generation) } catch (failure: Exception) {
            when (keys.classify(failure)) {
                EnterpriseKeyUse.UNLOCK_REQUIRED -> throw com.anpfuel.application.enterprise.EnterpriseUnlockRequired()
                EnterpriseKeyUse.INVALIDATED -> throw EnterpriseVaultUnrecoverable()
                EnterpriseKeyUse.OTHER -> throw failure
            }
        }
        val descriptor = VaultMaintenanceCodec.seal(VaultMaintenance(snapshot.partition, bound), token, generation, maintenance.secret)
        files.replaceGeneration(token, generation, false, sealed)
        files.replaceGeneration(token, generation, true, descriptor)
        keys.create(EnterpriseVaultGenerationNames.head(token, generation), null)
        // The acknowledged current generation remains valid even if obsolete cleanup needs a retry.
        cleanup(token, generation)
    }

    /** Never removes the trusted head or its pair. Failed cleanup blocks another preparation. */
    fun cleanup(token: String, keep: Long? = head(token)): Boolean {
        val aliases = aliases(token)
        val generations = (files.generations(token) + aliases.mapNotNull { EnterpriseVaultGenerationNames.generation(it, token) }).distinct()
        check(generations.size <= 256)
        var success = true
        generations.filter { it != keep }.forEach { generation ->
            try { files.eraseGeneration(token, generation) } catch (_: Exception) { success = false }
            aliases.filter { EnterpriseVaultGenerationNames.generation(it, token) == generation }.forEach { alias ->
                try { keys.delete(alias) } catch (_: Exception) { success = false }
            }
        }
        return success
    }
}
