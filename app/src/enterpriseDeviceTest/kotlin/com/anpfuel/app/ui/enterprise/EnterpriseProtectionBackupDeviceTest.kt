package com.anpfuel.app.ui.enterprise

import android.content.Context
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.anpfuel.application.enterprise.EnterpriseAccessBinding
import com.anpfuel.application.enterprise.EnterpriseContactAuthority
import com.anpfuel.application.enterprise.EnterpriseContactGrant
import com.anpfuel.application.enterprise.EnterpriseContactReply
import com.anpfuel.application.enterprise.EnterpriseDeviceTarget
import com.anpfuel.application.enterprise.EnterpriseEnrollmentBinding
import com.anpfuel.application.enterprise.EnterpriseLocalContext
import com.anpfuel.application.enterprise.EnterpriseLocalIdentity
import com.anpfuel.application.enterprise.EnterpriseLocalDenied
import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.EnterpriseSessionFence
import com.anpfuel.data.enterprise.AndroidEnterpriseDeviceClock
import com.anpfuel.data.enterprise.AndroidEnterpriseVaultKeys
import com.anpfuel.data.enterprise.AtomicEnterpriseVaultFiles
import com.anpfuel.data.enterprise.EnterpriseVaultErasure
import com.anpfuel.data.enterprise.ProtectedEnterprisePersistence
import com.anpfuel.domain.enterprise.EnterpriseLocalRecord
import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterprisePendingOperation
import java.io.File
import java.security.KeyStore
import javax.crypto.SecretKey
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Explicit multi-process OS acceptance, invoked one method at a time without
 * Orchestrator data clearing. The external test sequence backs up, uninstalls
 * and reinstalls only the isolated validation target between these methods.
 * A synthetic authority tests storage custody; the separate signed-contact
 * acceptance owns actual source proof. No Hilt fake supplies this store.
 */
@RunWith(AndroidJUnit4::class)
class EnterpriseProtectionBackupDeviceTest {
    private val context: Context = InstrumentationRegistry.getInstrumentation().targetContext.also {
        check(it.packageName == "com.anpfuel.app.enterprisevalidation") {
            "OS acceptance requires the isolated enterprise validation target"
        }
    }
    private val partition = EnterprisePartition.of(ACCOUNT, ORIGIN, ORGANIZATION)
    private val root = File(context.noBackupFilesDir, "enterprise-sealed-v1")
    private val payload = "SYNTHETIC-PRIVATE-ENTERPRISE-BACKUP-CONTROL".toByteArray()

    private fun store(): Pair<ProtectedEnterprisePersistence, EnterpriseLocalContext> {
        val fence = EnterpriseSessionFence()
        val files = AtomicEnterpriseVaultFiles(root)
        val keys = AndroidEnterpriseVaultKeys()
        val clock = AndroidEnterpriseDeviceClock(context)
        val target = EnterpriseDeviceTarget(partition, MEMBERSHIP)
        val enrollment = EnterpriseEnrollmentBinding(target, ENROLLMENT, 1)
        val identity = EnterpriseLocalIdentity(ACCOUNT, ORIGIN, FAMILY)
        val authority = EnterpriseContactAuthority { request ->
            check(request.target == target && request.binding.familyId == FAMILY)
            EnterpriseContactReply.Active(EnterpriseContactGrant(enrollment, FAMILY, System.currentTimeMillis()))
        }
        val persistence = ProtectedEnterprisePersistence(files, keys, fence, EnterpriseVaultErasure(files, keys, fence),
            { identity }, clock::read, authority)
        val local = EnterpriseLocalContext({ ACCOUNT }, { ORIGIN }, persistence, { fence.epoch })
        // Retain these ports only for this synthetic fixture, never in app persistence.
        fixtureFence = fence
        return persistence to local
    }

    private lateinit var fixtureFence: EnterpriseSessionFence

    @Test
    fun seedProtectedWorkAndPublicBackupControl(): Unit = runBlocking {
        assertFalse("Start the OS sequence with an empty isolated vault", root.exists())
        assertTrue(vaultAliases().isEmpty())
        val contactStart = AndroidEnterpriseDeviceClock(context).read()
        val (persistence, local) = store()
        fun binding() = EnterpriseAccessBinding(ACCOUNT, ORIGIN, FAMILY, fixtureFence.epoch)
        val before = binding()
        val owner = local.reserveContact(local.acquireOwner(), before, fixtureFence, ::binding)
        val handoff = persistence.confirm(EnterpriseDeviceTarget(partition, MEMBERSHIP), before, owner)
        val selected = local.select(local.consumeContact(owner, handoff, binding()), partition)
        selected.put(EnterpriseLocalRecord(partition, RECORD, payload))
        assertTrue(selected.enqueue(EnterprisePendingOperation(partition, OPERATION, RECORD, payload)))
        persistence.putPrivateFile(partition, ATTACHMENT, payload)
        persistence.putPrivateFile(partition, CACHE, payload, cache = true)
        assertArrayEquals(payload, selected.record(RECORD)!!.envelope)
        assertArrayEquals(payload, persistence.privateFile(partition, ATTACHMENT))
        assertArrayEquals(payload, persistence.privateFile(partition, CACHE, cache = true))
        assertEquals(1, selected.pending().size)
        assertTrue(root.canonicalPath.startsWith(context.noBackupFilesDir.canonicalPath + File.separator))
        assertTrue(root.walkTopDown().any { it.isFile && it.extension == "data" })
        root.walkTopDown().filter { it.isFile }.forEach { file ->
            val text = file.readBytes().toString(Charsets.ISO_8859_1)
            for (privateValue in listOf(ACCOUNT, ORGANIZATION, MEMBERSHIP, ENROLLMENT, FAMILY, payload.toString(Charsets.UTF_8))) {
                assertFalse("Private value entered a filesystem name", file.name.contains(privateValue))
                assertFalse("Private value entered a plaintext file", text.contains(privateValue))
            }
        }
        val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        assertTrue(vaultAliases().isNotEmpty())
        vaultAliases().forEach { alias ->
            val key = keyStore.getKey(alias, null)
            assertTrue(key is SecretKey)
            assertNull("A vault/head key was exportable", key.encoded)
        }
        File(context.filesDir, PUBLIC_CONTROL).writeText(PUBLIC_VALUE)
        // Public platform observations let the external reboot/clock sequence
        // prove it exercised the real OS condition, without a private lease.
        File(context.filesDir, CLOCK_CONTROL).writeText(JSONObject().put("boot", contactStart.boot)
            .put("wall", contactStart.wallMillis).toString())
        assertTrue(context.getSharedPreferences(PRIVATE_PREF, Context.MODE_PRIVATE).edit()
            .putString("synthetic-private", "EXCLUDED-SYNTHETIC-ACCOUNT-PROOF").commit())
    }

    @Test
    fun reopenAcknowledgedWorkAfterProcessDeath(): Unit = runBlocking {
        assertEquals(PUBLIC_VALUE, File(context.filesDir, PUBLIC_CONTROL).readText())
        val (persistence, _) = store()
        assertArrayEquals(payload, persistence.record(partition, RECORD)!!.envelope)
        assertArrayEquals(payload, persistence.pending(partition, 50).single().envelope)
        assertArrayEquals(payload, persistence.privateFile(partition, ATTACHMENT))
        assertArrayEquals(payload, persistence.privateFile(partition, CACHE, cache = true))
    }

    @Test
    fun capacityRefusalPreservesAcknowledgedPendingWorkOnAndroid(): Unit = runBlocking {
        val (persistence, _) = store()
        val retained = root.walkTopDown().filter { it.isFile && (it.extension == "data" || it.extension == "meta") }
            .associate { it.name to it.readBytes() }
        assertTrue(retained.isNotEmpty())
        val oversized = ByteArray(32 * 1024 * 1024) { 7 }
        try {
            persistence.putPrivateFile(partition, ATTACHMENT, oversized)
            fail("An oversized replacement evicted held work or exceeded the allocation budget")
        } catch (_: EnterprisePersistenceUnavailable) { /* Explicit refusal; no automatic eviction. */ }
        finally { oversized.fill(0) }
        retained.forEach { (name, bytes) -> assertArrayEquals(bytes, File(root, name).readBytes()) }
        assertArrayEquals(payload, persistence.pending(partition, 50).single().envelope)
        assertArrayEquals(payload, persistence.privateFile(partition, ATTACHMENT))
    }

    @Test
    fun publicControlRestoredButPrivateVaultAndKeysExcluded() {
        assertEquals("The OS must really restore the positive control", PUBLIC_VALUE,
            File(context.filesDir, PUBLIC_CONTROL).readText())
        assertTrue("The OS restored enterprise vault files", !root.exists() || root.listFiles().orEmpty().isEmpty())
        assertTrue("Uninstall/restore retained enterprise Keystore custody", vaultAliases().isEmpty())
        assertTrue("The OS restored private account preferences",
            context.getSharedPreferences(PRIVATE_PREF, Context.MODE_PRIVATE).all.isEmpty())
    }

    @Test
    fun realRebootRequiresFreshContactAndPreservesAcknowledgedWork(): Unit = runBlocking {
        assertEquals(PUBLIC_VALUE, File(context.filesDir, PUBLIC_CONTROL).readText())
        val clockControl = JSONObject(File(context.filesDir, CLOCK_CONTROL).readText())
        assertNotEquals("This method requires an actual emulator reboot", clockControl.getString("boot"),
            AndroidEnterpriseDeviceClock(context).read().boot)
        val (persistence, local) = store()
        verifyReconfirmationAndPreserve(persistence, local)
    }

    @Test
    fun realWallRollbackKeepsCiphertextAndRequiresContact(): Unit = runBlocking {
        val control = JSONObject(File(context.filesDir, CLOCK_CONTROL).readText())
        val now = AndroidEnterpriseDeviceClock(context).read()
        assertEquals(control.getString("boot"), now.boot)
        assertTrue("This method requires an actual backward OS clock change", now.wallMillis < control.getLong("wall"))
        val (persistence, _) = store()
        val retained = encryptedFiles()
        try { persistence.pending(partition, 50); fail("A rolled-back clock opened protected work") }
        catch (_: EnterpriseLocalDenied) { /* Non-destructive reconfirmation. */ }
        retained.forEach { (name, bytes) -> assertArrayEquals(bytes, File(root, name).readBytes()) }
        assertTrue(vaultAliases().isNotEmpty())
    }

    @Test
    fun restoredClockStillRequiresContactBeforeReopeningWork(): Unit = runBlocking {
        val control = JSONObject(File(context.filesDir, CLOCK_CONTROL).readText())
        val now = AndroidEnterpriseDeviceClock(context).read()
        assertEquals(control.getString("boot"), now.boot)
        assertTrue("Restore the actual OS clock before this method", now.wallMillis >= control.getLong("wall"))
        val (persistence, local) = store()
        verifyReconfirmationAndPreserve(persistence, local)
    }

    private fun encryptedFiles(): Map<String, ByteArray> = root.walkTopDown()
        .filter { it.isFile && (it.extension == "data" || it.extension == "meta") }.associate { it.name to it.readBytes() }

    private suspend fun verifyReconfirmationAndPreserve(persistence: ProtectedEnterprisePersistence, local: EnterpriseLocalContext) {
        val retained = encryptedFiles()
        assertTrue("Seed protected work before rebooting this task's emulator", retained.isNotEmpty())
        try { persistence.pending(partition, 50); fail("An old-boot lease opened without source contact") }
        catch (_: EnterpriseLocalDenied) { /* Reboot reconfirms; it is not security erasure. */ }
        retained.forEach { (name, bytes) -> assertArrayEquals(bytes, File(root, name).readBytes()) }
        assertTrue(vaultAliases().isNotEmpty())
        fun binding() = EnterpriseAccessBinding(ACCOUNT, ORIGIN, FAMILY, fixtureFence.epoch)
        val before = binding()
        val owner = local.reserveContact(local.acquireOwner(), before, fixtureFence, ::binding)
        val handoff = persistence.confirm(EnterpriseDeviceTarget(partition, MEMBERSHIP), before, owner)
        val selected = local.select(local.consumeContact(owner, handoff, binding()), partition)
        assertArrayEquals(payload, selected.pending().single().envelope)
        assertArrayEquals(payload, selected.record(RECORD)!!.envelope)
    }

    private fun vaultAliases(): List<String> = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        .aliases().toList().filter { it.startsWith("abastevo_enterprise_") }

    companion object {
        private const val ACCOUNT = "7b0e2a54-1010-4000-8000-000000000001"
        private const val FAMILY = "7b0e2a54-1010-4000-8000-000000000002"
        private const val ORGANIZATION = "7b0e2a54-1010-4000-8000-000000000003"
        private const val MEMBERSHIP = "7b0e2a54-1010-4000-8000-000000000004"
        private const val ENROLLMENT = "7b0e2a54-1010-4000-8000-000000000005"
        private const val RECORD = "7b0e2a54-1010-4000-8000-000000000006"
        private const val OPERATION = "7b0e2a54-1010-4000-8000-000000000007"
        private const val ATTACHMENT = "7b0e2a54-1010-4000-8000-000000000008"
        private const val CACHE = "7b0e2a54-1010-4000-8000-000000000009"
        private const val ORIGIN = "https://api.anpfuel.example.invalid"
        private const val PUBLIC_CONTROL = "enterprise-backup-public-control.txt"
        private const val CLOCK_CONTROL = "enterprise-backup-public-clock-control.json"
        private const val PUBLIC_VALUE = "PUBLIC-SYNTHETIC-OS-RESTORE-CONTROL-V1"
        private const val PRIVATE_PREF = "enterprise-backup-private-control"
    }
}
