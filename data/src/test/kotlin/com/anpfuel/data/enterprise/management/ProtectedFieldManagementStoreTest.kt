package com.anpfuel.data.enterprise.management

import com.anpfuel.application.enterprise.EnterprisePersistenceUnavailable
import com.anpfuel.application.enterprise.management.FieldComplianceNotice
import com.anpfuel.application.enterprise.management.FieldManagementBook
import com.anpfuel.application.enterprise.management.FieldMaintenanceRequestDraft
import com.anpfuel.data.enterprise.EnterpriseVaultCodec
import com.anpfuel.data.enterprise.ProtectedQueueFixture.attempt
import com.anpfuel.data.enterprise.ProtectedQueueFixture.id
import com.anpfuel.data.enterprise.VaultFixture
import com.anpfuel.data.enterprise.VaultFixture.Companion.B
import com.anpfuel.data.enterprise.VaultFixture.Companion.ORIGIN
import com.anpfuel.data.enterprise.VaultFixture.Companion.partition
import com.anpfuel.domain.enterprise.maintenance.ComplianceState
import java.nio.file.Files
import java.nio.file.Path
import java.time.Instant
import java.time.LocalDate
import kotlinx.coroutines.runBlocking
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.io.TempDir

class ProtectedFieldManagementStoreTest {
    @TempDir
    lateinit var root: Path

    @Test
    fun `persists and isolates F6 field management records per partition inside encrypted vault`() = runBlocking {
        val f = VaultFixture(root)
        f.confirm(partition())
        f.confirm(partition(org = B))
        val store = ProtectedFieldManagementStore(f.store)
        val firstAttempt = attempt(f)
        val foreignAttempt = attempt(f, partition(org = B))

        assertNull(store.read(firstAttempt))

        val book = FieldManagementBook(
            membershipId = firstAttempt.enrollment.target.membershipId,
            enrollmentId = firstAttempt.enrollment.enrollmentId,
            complianceNotices = listOf(
                FieldComplianceNotice(id(801), "ASSET", "CRLV", LocalDate.of(2026, 12, 1), ComplianceState.DUE_SOON, 30),
            ),
            maintenanceRequests = listOf(
                FieldMaintenanceRequestDraft(id(802), id(803), "Vazamento radiador", 180_000L, Instant.parse("2026-11-18T11:00:00Z")),
            ),
            internalDispenseQueueCount = 1,
            updatedAt = Instant.parse("2026-11-18T11:05:00Z"),
        )
        store.save(firstAttempt, book)
        // Idempotent replay succeeds with no change
        store.save(firstAttempt, book)

        val readBack = store.read(firstAttempt)
        assertNotNull(readBack)
        assertEquals(book, readBack)
        assertNull(store.read(foreignAttempt))
        assertTrue(store.toString().contains("redacted"))

        val token = EnterpriseVaultCodec.token(partition())
        assertTrue(f.files.tokens(EnterpriseVaultCodec.accountTag(partition().ownerAccount, ORIGIN)).contains(token))
        Files.walk(root).use { paths ->
            paths.filter(Files::isRegularFile).forEach { file ->
                assertFalse(Files.readAllBytes(file).decodeToString().contains("Vazamento radiador"))
            }
        }

        // Rejects mismatched enrollment binding fail-closed
        val wrongBook = book.copy(membershipId = id(999))
        assertThrows(EnterprisePersistenceUnavailable::class.java) {
            runBlocking { store.save(firstAttempt, wrongBook) }
        }
    }
}
