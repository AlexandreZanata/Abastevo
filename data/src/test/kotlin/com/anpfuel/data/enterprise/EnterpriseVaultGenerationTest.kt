package com.anpfuel.data.enterprise

import com.anpfuel.domain.enterprise.*
import com.anpfuel.data.enterprise.VaultFixture.Companion.A
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.C
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import java.nio.file.Files
import java.nio.file.Path
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class EnterpriseVaultGenerationTest {
    @TempDir lateinit var root: Path
    private fun snapshot() = VaultSnapshot(partition(), EnterpriseOfflineLease(A, 10000, EnterpriseDeviceClock("boot", 1000, 10000))).also {
        it.bound = EnterpriseBoundOfflineLease(B, C, 1, it.lease)
        it.entries["RECORD:$A"] = VaultEntry(VaultKind.RECORD, A, "", byteArrayOf(7))
    }
    @Test fun restoredAcknowledgedFilesCannotSelectOlderTrustedHead() {
        val files = AtomicEnterpriseVaultFiles(root.toFile()); val keys = TestVaultKeys()
        val generations = EnterpriseVaultGenerations(files, keys); val value = snapshot(); val token = EnterpriseVaultCodec.token(value.partition)
        generations.commit(value)
        val old = Files.list(root).use { stream -> stream.filter { it.toString().endsWith(".data") || it.toString().endsWith(".meta") }.toList().associate { it.fileName.toString() to Files.readAllBytes(it) } }
        value.entries["RECORD:$A"] = VaultEntry(VaultKind.RECORD, A, "", byteArrayOf(8))
        generations.commit(value)
        Files.list(root).use { stream -> stream.filter { it.toString().endsWith(".data") || it.toString().endsWith(".meta") }.forEach(Files::delete) }
        old.forEach { (name, bytes) -> Files.write(root.resolve(name), bytes) }
        assertThrows(com.anpfuel.application.enterprise.EnterpriseVaultUnrecoverable::class.java) { generations.load(value.partition) }
        assertEquals(2L, generations.head(token))
    }
    @Test fun preparationFailurePreservesAcknowledgedGenerationAndDoesNotCommitMetadataOnly() {
        val real = AtomicEnterpriseVaultFiles(root.toFile()); var fail = false
        val files = object : EnterpriseVaultFiles by real {
            override fun replaceGeneration(token: String, generation: Long, descriptor: Boolean, ciphertext: ByteArray) {
                if (fail && descriptor) error("private fault")
                real.replaceGeneration(token, generation, descriptor, ciphertext)
            }
        }
        val keys = TestVaultKeys(); val generations = EnterpriseVaultGenerations(files, keys); val value = snapshot()
        generations.commit(value); fail = true
        value.entries["RECORD:$A"] = VaultEntry(VaultKind.RECORD, A, "", byteArrayOf(8))
        assertThrows(IllegalStateException::class.java) { generations.commit(value) }
        assertArrayEquals(byteArrayOf(7), generations.load(value.partition)!!.entries["RECORD:$A"]!!.bytes)
        fail = false; generations.commit(value)
        assertArrayEquals(byteArrayOf(8), generations.load(value.partition)!!.entries["RECORD:$A"]!!.bytes)
        assertEquals(3, keys.values.size, "Only current data, maintenance and trusted head remain")
    }
    @Test fun maintenanceBindingCanBeReadWithoutManagedPayloadUnlockAndCannotBeTransplanted() {
        val files = AtomicEnterpriseVaultFiles(root.toFile()); val keys = TestVaultKeys(); val generations = EnterpriseVaultGenerations(files, keys)
        val value = snapshot(); value.lease = value.lease.copy(managedUnlockSeconds = 60); value.bound = value.bound!!.copy(time = value.lease)
        generations.commit(value)
        val token = EnterpriseVaultCodec.token(value.partition)
        assertEquals(B, generations.descriptor(token)!!.lease.membershipId)
        val other = EnterpriseVaultCodec.token(partition(org = B))
        assertNull(generations.descriptor(other))
        assertEquals(60, keys.values[EnterpriseVaultGenerationNames.data(token, 1)]!!.managedUnlockSeconds)
        assertNull(keys.values[EnterpriseVaultGenerationNames.maintenance(token, 1)]!!.managedUnlockSeconds)
    }
    @Test fun missingTrustedHeadWithSurvivingAcknowledgedMaterialNeverLooksEmpty() {
        val files = AtomicEnterpriseVaultFiles(root.toFile()); val keys = TestVaultKeys()
        val generations = EnterpriseVaultGenerations(files, keys); val value = snapshot()
        generations.commit(value)
        val token = EnterpriseVaultCodec.token(value.partition)
        keys.delete(EnterpriseVaultGenerationNames.head(token, 1))
        assertThrows(com.anpfuel.application.enterprise.EnterpriseVaultUnrecoverable::class.java) { generations.descriptor(token) }
    }

    @Test fun copiedMaintenanceAndPayloadPairCannotBecomeAnotherPartitionsCatalog() {
        val files = AtomicEnterpriseVaultFiles(root.toFile()); val keys = TestVaultKeys(); val generations = EnterpriseVaultGenerations(files, keys)
        val original = snapshot(); val foreign = snapshot().let { VaultSnapshot(partition(org = B), it.lease, bound = it.bound) }
        generations.commit(original); generations.commit(foreign)
        val token = EnterpriseVaultCodec.token(original.partition); val other = EnterpriseVaultCodec.token(foreign.partition)
        files.replaceGeneration(other, 1, true, files.readGeneration(token, 1, true)!!)
        files.replaceGeneration(other, 1, false, files.readGeneration(token, 1, false)!!)
        assertThrows(com.anpfuel.application.enterprise.EnterpriseVaultUnrecoverable::class.java) { generations.descriptor(other) }
        assertThrows(com.anpfuel.application.enterprise.EnterpriseVaultUnrecoverable::class.java) { generations.load(foreign.partition) }
        assertArrayEquals(byteArrayOf(7), generations.load(original.partition)!!.entries["RECORD:$A"]!!.bytes)
    }

}
