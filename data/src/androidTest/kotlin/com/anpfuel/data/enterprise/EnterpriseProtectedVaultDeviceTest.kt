package com.anpfuel.data.enterprise

import android.app.KeyguardManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.application.enterprise.*
import com.anpfuel.domain.enterprise.*
import java.io.File
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/** Deferred device batch: actual AndroidKeyStore, filesystem, managed policy and cryptographic erasure. */
@RunWith(AndroidJUnit4::class)
class EnterpriseProtectedVaultDeviceTest {
    @Test
    fun realKeysSealAllSurfacesReopenAndBecomeUnusableAfterRevocation(): Unit = runBlocking {
        val context = ApplicationProvider.getApplicationContext<Context>()
        val p = EnterprisePartition.of(UUID.randomUUID().toString(), "https://api.example.invalid", UUID.randomUUID().toString())
        val family = UUID.randomUUID().toString()
        val id = UUID.randomUUID().toString()
        val root = File(context.noBackupFilesDir, "enterprise-vault-test-" + UUID.randomUUID())
        val keys = AndroidEnterpriseVaultKeys()
        val files = AtomicEnterpriseVaultFiles(root)
        val fence = EnterpriseSessionFence()
        val erase = EnterpriseVaultErasure(files, keys, fence)
        val clock = AndroidEnterpriseDeviceClock(context)
        val owner = EnterpriseLocalIdentity(p.ownerAccount, p.origin, family)
        val target = EnterpriseDeviceTarget(p, UUID.randomUUID().toString())
        val enrollment = EnterpriseEnrollmentBinding(target, UUID.randomUUID().toString(), 1)
        val authority = EnterpriseContactAuthority { EnterpriseContactReply.Active(EnterpriseContactGrant(enrollment, family, System.currentTimeMillis())) }
        fun store() = ProtectedEnterprisePersistence(files, keys, fence, erase, { owner }, clock::read, authority)
        val token = EnterpriseVaultCodec.token(p)
        val payload = ("PRIVATE-SYNTHETIC-DEVICE-" + id).toByteArray()
        try {
            assertTrue(root.canonicalPath.startsWith(context.noBackupFilesDir.canonicalPath + File.separator))
            val s = store()
            fun binding() = EnterpriseAccessBinding(p.ownerAccount, p.origin, family, fence.epoch)
            val local = EnterpriseLocalContext({ owner.account }, { owner.origin }, s, { fence.epoch })
            val before = binding()
            s.confirm(target, before, local.reserveContact(local.acquireOwner(), before, fence, ::binding))
            s.put(EnterpriseLocalRecord(p, id, payload))
            s.enqueue(EnterprisePendingOperation(p, id, id, payload))
            s.putPrivateFile(p, id, payload)
            fun generation() = EnterpriseVaultGenerations(files, keys).head(token)!!
            fun ciphertext() = files.readGeneration(token, generation(), false)!!
            val first = ciphertext()
            s.put(EnterpriseLocalRecord(p, id, payload))
            val current = generation()
            val alias = EnterpriseVaultGenerationNames.data(token, current)
            val key = keys.existing(alias)!!
            assertNull(key.secret.encoded)
            val second = ciphertext()
            assertFalse(first.contentEquals(second))
            assertArrayEquals(payload, store().record(p, id)!!.envelope)
            assertArrayEquals(payload, store().privateFile(p, id))
            root.walkTopDown().filter { it.isFile }.forEach {
                val text = String(it.readBytes(), Charsets.ISO_8859_1)
                assertFalse(text.contains(String(payload)))
                assertFalse(text.contains(p.ownerAccount))
                assertFalse(text.contains(p.organizationId))
            }
            assertTrue(erase.partitionRevoked(p))
            assertNull(keys.existing(alias))
            try { store().pending(p, 1); fail("Revoked partition reopened") }
            catch (_: EnterpriseLocalDenied) { /* expected */ }
            try { EnterpriseVaultCodec.open(second, p, key.secret, current); fail("Deleted key decrypted") }
            catch (_: Exception) { /* actual Keystore refuses old handle */ }
        } finally {
            keys.aliases(EnterpriseVaultGenerationNames.PREFIX).filter { EnterpriseVaultGenerationNames.token(it) == token }.forEach(keys::delete)
            root.deleteRecursively()
        }
    }

    @Test
    fun managedKeyRequiresConfiguredDeviceAuthenticationWithoutExportingKeyMaterial() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        assertTrue("Configure a secure device credential for the acceptance batch",
            context.getSystemService(KeyguardManager::class.java).isDeviceSecure)
        val p = EnterprisePartition.of(UUID.randomUUID().toString(), "https://api.example.invalid", UUID.randomUUID().toString())
        val keys = AndroidEnterpriseVaultKeys()
        val alias = EnterpriseVaultCodec.alias(EnterpriseVaultCodec.token(p))
        try {
            val key = keys.create(alias, 60)
            assertNull(key.secret.encoded)
            assertEquals(60, keys.existing(alias)!!.managedUnlockSeconds)
            assertFalse(keys.aliases(EnterpriseVaultCodec.alias(EnterpriseVaultCodec.accountTag(p.ownerAccount, p.origin) + "-")).isEmpty())
        } finally { keys.delete(alias) }
    }
}
