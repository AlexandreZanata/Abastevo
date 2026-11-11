package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.domain.enterprise.*
import java.nio.file.Path
import javax.crypto.KeyGenerator

internal class TestVaultKeys : EnterpriseVaultKeys {
    val values = linkedMapOf<String, EnterpriseVaultKey>()
    var creates = 0
    var failDelete = false
    var failAliases = emptySet<String>()
    var locked = false
    @Synchronized override fun existing(alias: String): EnterpriseVaultKey? {
        if (locked) error("private key-provider diagnostic")
        return values[alias]
    }
    @Synchronized override fun create(alias: String, managedUnlockSeconds: Int?): EnterpriseVaultKey {
        creates++
        check(alias !in values)
        val generator = KeyGenerator.getInstance("AES").apply { init(256) }
        return EnterpriseVaultKey(generator.generateKey(), managedUnlockSeconds).also { values[alias] = it }
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
        EnterpriseContactAuthority { p ->
            contacts++
            if (denyContact) error("private authority-provider diagnostic")
            EnterpriseContactGrant(p, identity!!.family, server, managedUnlockSeconds = unlock)
        })

    fun file(directory: Path, p: EnterprisePartition = partition()): Path =
        directory.resolve(EnterpriseVaultCodec.token(p) + ".vault")

    companion object {
        const val A = "7b0e2a54-0000-4000-8000-000000000001"
        const val B = "7b0e2a54-0000-4000-8000-000000000002"
        const val C = "7b0e2a54-0000-4000-8000-000000000003"
        const val ORIGIN = "https://api.example.invalid"
        fun partition(owner: String = A, origin: String = ORIGIN, org: String = A) =
            EnterprisePartition.of(owner, origin, org)
    }
}
