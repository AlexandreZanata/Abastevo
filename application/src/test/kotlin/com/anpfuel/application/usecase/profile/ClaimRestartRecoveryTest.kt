package com.anpfuel.application.usecase.profile

import com.anpfuel.domain.profile.ClaimExportRequest
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P32-T04 — Restart recovery without stale replay.
 */
class ClaimRestartRecoveryTest {

    private class MemoryStore : ClaimDeclarationStore {
        val seen = mutableSetOf<String>()
        var exports = 0

        override suspend fun export(request: ClaimExportRequest) =
            com.anpfuel.domain.profile.ClaimDeclaration(
                version = 1,
                accountId = request.accountId,
                stationId = request.stationId,
                operatorName = "Posto Exemplo LTDA",
                scopes = request.scopes,
                nonce = "restart-n-$exports".also { exports++ },
                issuedAtEpochSeconds = 1_700_000_000L,
                ttlSeconds = 900L,
            )

        override suspend fun markNonceSeen(nonce: String) {
            seen.add(nonce)
        }

        override suspend fun isNonceSeen(nonce: String): Boolean = seen.contains(nonce)
    }

    @Test
    fun `restarted usecase instance honors already-seen nonce`() = runTest {
        val store = MemoryStore()
        val request = ClaimExportRequest("acc-001", "station-1", setOf("edit_hours"))
        val export = RequestClaimExportUseCase(store).invoke(true, request)
            as com.anpfuel.domain.profile.ClaimExportDecision.Ready

        val first = SubmitSignedClaimUseCase(store).invoke(
            true, export.declaration, 1_700_000_010L, "acc-001", "application/pdf", 10_000L,
        )
        assertEquals(ClaimImportOutcome.AcceptedPendingReview, first)

        val restarted = SubmitSignedClaimUseCase(store)
        val retry = restarted.invoke(
            true, export.declaration, 1_700_000_020L, "acc-001", "application/pdf", 10_000L,
        )
        assertTrue(retry is ClaimImportOutcome.Denied)
    }
}
