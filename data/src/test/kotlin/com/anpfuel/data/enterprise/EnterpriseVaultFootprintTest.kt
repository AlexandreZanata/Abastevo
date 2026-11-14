package com.anpfuel.data.enterprise

import com.anpfuel.application.enterprise.*
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.domain.enterprise.*
import java.nio.file.Files
import java.nio.file.Path
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

/**
 * Only real protected material or a durable denial is prior enterprise data. Discovering a
 * personal-only account, or a bare coordination lock, must never later surface the false
 * "previous local data cannot be reopened" outcome after a revoked session.
 */
class EnterpriseVaultFootprintTest {
    @TempDir lateinit var root: Path
    private val p = partition()

    private fun entries(directory: Path): List<String> =
        if (!Files.exists(directory)) emptyList() else Files.list(directory).use { stream -> stream.map { it.fileName.toString() }.toList() }

    /** Refuses broad marker prefixes exactly like the actual Android provider (AndroidEnterpriseVaultKeys). */
    private class ProviderPrefixKeys : TestVaultKeys() {
        override fun aliases(prefix: String): List<String> {
            require(prefix == EnterpriseVaultGenerationNames.PREFIX || prefix == "abastevo_enterprise_v1_" ||
                Regex("abastevo_enterprise_v[12]_[0-9a-f]{64}-").matches(prefix) ||
                Regex("abastevo_enterprise_v2_(denied|lock)_[0-9a-f]{64}-").matches(prefix) ||
                Regex("abastevo_enterprise_v2_[0-9a-f]{64}-[0-9a-f]{64}_").matches(prefix)) { "provider refuses this prefix" }
            return super.aliases(prefix)
        }
    }

    @Test fun neverEnterpriseAccountDiscoveryCreatesNothingAndLaterRevocationNeedsNoLossAcknowledgement(): Unit = runBlocking {
        val directory = root.resolve("vault")
        val f = VaultFixture(directory, ProviderPrefixKeys())
        val page = f.store.discover(ProtectedQueueFixture.account(f))
        assertTrue(page.enrollments.isEmpty()); assertNull(page.nextAfterOpaque); assertEquals(0, page.unavailable)
        assertEquals(emptyList<String>(), entries(directory), "Discovery left a local artifact for a personal-only account")
        assertTrue(f.erasure.accountRevoked(A, ORIGIN))
        assertFalse(f.erasure.cleanupRequired)
        f.confirm(p)
        f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(3)))
        assertArrayEquals(byteArrayOf(3), f.store.record(p, A)!!.envelope)
    }

    @Test fun bareLockFileIsCoordinationNotPriorEnterpriseData(): Unit = runBlocking {
        // An interrupted first contact or an earlier app version can leave only the account lock file.
        val directory = root.resolve("vault")
        val f = VaultFixture(directory, ProviderPrefixKeys())
        f.files.locked(EnterpriseVaultCodec.accountTag(A, ORIGIN)) { }
        assertEquals(1, entries(directory).size)
        assertTrue(f.erasure.accountRevoked(A, ORIGIN))
        assertFalse(f.erasure.cleanupRequired)
        f.confirm(p)
        assertNull(f.store.record(p, A))
        assertEquals(0, f.store.discover(ProtectedQueueFixture.account(f)).unavailable)
    }

    @Test fun genuinelyUnrecoverableVaultStillRequiresExplicitLossAcknowledgement(): Unit = runBlocking {
        val f = VaultFixture(root.resolve("vault"), ProviderPrefixKeys())
        f.confirm(p); f.store.put(EnterpriseLocalRecord(p, A, byteArrayOf(7)))
        assertEquals(1, f.store.discover(ProtectedQueueFixture.account(f)).enrollments.size)
        assertTrue(f.erasure.accountRevoked(A, ORIGIN))
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { f.confirm(p) } }
        assertThrows(EnterpriseVaultUnrecoverable::class.java) { runBlocking { f.confirm(partition(org = VaultFixture.B)) } }
        f.server++
        f.confirm(p, acknowledgeErasedWork = true)
        assertNull(f.store.record(p, A))
        assertFalse(f.erasure.cleanupRequired)
    }
}
