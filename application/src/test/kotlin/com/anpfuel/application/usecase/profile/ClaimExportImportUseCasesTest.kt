package com.anpfuel.application.usecase.profile

import com.anpfuel.domain.profile.ClaimDeclaration
import com.anpfuel.domain.profile.ClaimExportDecision
import com.anpfuel.domain.profile.ClaimExportImportRule
import com.anpfuel.domain.profile.ClaimExportRequest
import com.anpfuel.domain.profile.ClaimImportDecision
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P32-T02 — Export/import orchestration RED→GREEN.
 */
class ClaimExportImportUseCasesTest {

    private class FakeStore : ClaimDeclarationStore {
        val seen = mutableSetOf<String>()
        var exports = 0

        override suspend fun export(request: ClaimExportRequest): ClaimDeclaration {
            exports++
            return ClaimDeclaration(
                version = 1,
                accountId = request.accountId,
                stationId = request.stationId,
                operatorName = "Posto Exemplo LTDA",
                scopes = request.scopes,
                nonce = "n-$exports",
                issuedAtEpochSeconds = 1_700_000_000L,
                ttlSeconds = 900L,
            )
        }

        override suspend fun markNonceSeen(nonce: String) {
            seen.add(nonce)
        }

        override suspend fun isNonceSeen(nonce: String): Boolean = seen.contains(nonce)
    }

    private val request = ClaimExportRequest(
        accountId = "acc-001",
        stationId = "9f6d8a2e-3b4c-4d5e-8f90-1234567890ab",
        scopes = setOf("edit_hours"),
    )

    @Test
    fun `guest export denied without store touch`() = runTest {
        val store = FakeStore()
        val decision = RequestClaimExportUseCase(store).invoke(false, request)
        assertEquals(ClaimExportDecision.DeniedGuest, decision)
        assertEquals(0, store.exports)
    }

    @Test
    fun `replayed nonce denied on retry`() = runTest {
        val store = FakeStore()
        val export = RequestClaimExportUseCase(store).invoke(true, request) as ClaimExportDecision.Ready
        val first = SubmitSignedClaimUseCase(store).invoke(
            true, export.declaration, 1_700_000_010L, "acc-001", "application/pdf", 50_000L,
        )
        assertEquals(ClaimImportOutcome.AcceptedPendingReview, first)
        val retry = SubmitSignedClaimUseCase(store).invoke(
            true, export.declaration, 1_700_000_020L, "acc-001", "application/pdf", 50_000L,
        )
        val denied = retry as ClaimImportOutcome.Denied
        assertEquals(ClaimImportDecision.DeniedReplay, denied.reason)
    }

    @Test
    fun `offline-sized wrong type denied`() = runTest {
        val store = FakeStore()
        val export = RequestClaimExportUseCase(store).invoke(true, request) as ClaimExportDecision.Ready
        val outcome = SubmitSignedClaimUseCase(store).invoke(
            true, export.declaration, 1_700_000_010L, "acc-001", "image/png", 50_000L,
        )
        assertTrue(outcome is ClaimImportOutcome.Denied)
        assertTrue(ClaimExportImportRule.MAX_SIGNED_BYTES > 0)
    }
}
