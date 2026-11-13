package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.*
import java.nio.file.Path
import java.security.InvalidKeyException
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey

/** Stand-ins for UserNotAuthenticatedException/KeyPermanentlyInvalidatedException. */
internal class TestUnlockRequired : InvalidKeyException("private unlock diagnostic")
internal class TestKeyInvalidated : InvalidKeyException("private invalidation diagnostic")

/** Like AndroidKeyStore, the alias still resolves; the cipher refuses the key only at use. */
private class RefusedKey(private val failure: () -> Exception) : SecretKey {
    override fun getAlgorithm() = "AES"
    override fun getFormat() = "RAW"
    override fun getEncoded(): ByteArray = throw failure()
}

internal open class TestVaultKeys : EnterpriseVaultKeys {
    val values = linkedMapOf<String, EnterpriseVaultKey>()
    var creates = 0
    var failDelete = false
    var failCreate = false
    var afterCreate: ((String) -> Unit)? = null
    var failAliases = emptySet<String>()
    var locked = false
    var invalidated = false
    var unavailable = false
    @Synchronized override fun existing(alias: String): EnterpriseVaultKey? {
        if (unavailable) error("private key-provider diagnostic")
        val key = values[alias] ?: return null
        return when {
            locked && key.managedUnlockSeconds != null -> key.copy(secret = RefusedKey { TestUnlockRequired() })
            invalidated && alias.endsWith("_d") -> key.copy(secret = RefusedKey { TestKeyInvalidated() })
            else -> key
        }
    }
    override fun classify(failure: Exception) = when (failure) {
        is TestUnlockRequired -> EnterpriseKeyUse.UNLOCK_REQUIRED
        is TestKeyInvalidated -> EnterpriseKeyUse.INVALIDATED
        else -> EnterpriseKeyUse.OTHER
    }
    @Synchronized override fun create(alias: String, managedUnlockSeconds: Int?): EnterpriseVaultKey {
        if (failCreate) error("private key-create diagnostic")
        creates++
        check(alias !in values)
        val generator = KeyGenerator.getInstance("AES").apply { init(256) }
        return EnterpriseVaultKey(generator.generateKey(), managedUnlockSeconds).also { values[alias] = it; afterCreate?.invoke(alias) }
    }
    @Synchronized override fun delete(alias: String) {
        if (failDelete || alias in failAliases) error("private key-delete diagnostic")
        values.remove(alias)
    }
    @Synchronized override fun aliases(prefix: String) = values.keys.filter { it.startsWith(prefix) }
}

internal class VaultFixture(
    directory: Path,
    val keys: TestVaultKeys = TestVaultKeys(),
    transform: (EnterpriseVaultFiles) -> EnterpriseVaultFiles = { it },
) {
    val files = transform(AtomicEnterpriseVaultFiles(directory.toFile()))
    val fence = EnterpriseSessionFence()
    val erasure = EnterpriseVaultErasure(files, keys, fence)
    @Volatile var identity: EnterpriseLocalIdentity? = EnterpriseLocalIdentity(A, ORIGIN, A)
    var now = EnterpriseDeviceClock("boot-7", 1000, 10000)
    var server = 10000L
    var unlock: Int? = null
    var denyContact = false
    var contacts = 0
    val store = ProtectedEnterprisePersistence(files, keys, fence, erasure, { identity }, { now },
        EnterpriseContactAuthority { request ->
            contacts++
            if (denyContact) error("private authority-provider diagnostic")
            testGrant(request.target.partition, identity!!.family, server, unlock, request.target.membershipId)
        })

    fun binding(): EnterpriseAccessBinding? = identity?.let { EnterpriseAccessBinding(it.account, it.origin, it.family, fence.epoch) }
    val local = EnterpriseLocalContext({ identity?.account }, { identity?.origin ?: ORIGIN }, store, { fence.epoch })
    suspend fun confirm(p: EnterprisePartition, acknowledgeErasedWork: Boolean = false,
        store: ProtectedEnterprisePersistence = this.store, membership: String = B) {
        val current = binding() ?: throw EnterpriseLocalDenied()
        val contact = local.reserveContact(local.acquireOwner(), current, fence, ::binding)
        store.confirm(EnterpriseDeviceTarget(p, membership), current, contact, acknowledgeErasedWork)
    }
    fun contentKeys() = keys.values.filterKeys { !it.contains("_denied_") && !it.contains("_lock_") }
    fun payloadAlias(p: EnterprisePartition = partition()): String = EnterpriseVaultGenerationNames.data(EnterpriseVaultCodec.token(p),
        EnterpriseVaultGenerations(files, keys).head(EnterpriseVaultCodec.token(p))!!)
    fun payloadKey(p: EnterprisePartition = partition()) = keys.values.getValue(payloadAlias(p))
    fun file(directory: Path, p: EnterprisePartition = partition()): Path {
        val token = EnterpriseVaultCodec.token(p)
        val generation = EnterpriseVaultGenerations(files, keys).head(token)!!
        return directory.resolve(token + ".g" + generation.toString().padStart(19, '0') + ".data")
    }

    companion object {
        const val A = "7b0e2a54-0000-4000-8000-000000000001"
        const val B = "7b0e2a54-0000-4000-8000-000000000002"
        const val C = "7b0e2a54-0000-4000-8000-000000000003"
        const val ORIGIN = "https://api.example.invalid"
        fun partition(owner: String = A, origin: String = ORIGIN, org: String = A) =
            EnterprisePartition.of(owner, origin, org)
    }
}

internal fun testGrant(p: EnterprisePartition, family: String, server: Long, unlock: Int? = null,
    membership: String = VaultFixture.B): EnterpriseContactReply.Active = EnterpriseContactReply.Active(
    EnterpriseContactGrant(EnterpriseEnrollmentBinding(EnterpriseDeviceTarget(p, membership), VaultFixture.C, 1), family, server, managedUnlockSeconds = unlock))
