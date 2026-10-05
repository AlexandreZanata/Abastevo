package com.anpfuel.domain.profile

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P32-T02 — Claim declaration export/import RED→GREEN.
 *
 * Locks: guest cannot export; expired declarations never import; owner
 * mismatch denied; wrong file type and oversize bytes refused; replayed
 * nonces rejected; the client never certifies signature or powers.
 */
class ClaimExportImportRuleTest {

    private val fresh = ClaimDeclaration(
        version = 1,
        accountId = "acc-001",
        stationId = "9f6d8a2e-3b4c-4d5e-8f90-1234567890ab",
        operatorName = "Posto Exemplo LTDA",
        scopes = setOf("edit_hours", "edit_services"),
        nonce = "n-abc-123",
        issuedAtEpochSeconds = 1_700_000_000L,
        ttlSeconds = 900L,
    )

    @Test
    fun `guest cannot build export request`() {
        val outcome = ClaimExportImportRule.canExport(
            signedIn = false,
            request = ClaimExportRequest(accountId = "", stationId = fresh.stationId, scopes = fresh.scopes),
        )
        assertEquals(ClaimExportDecision.DeniedGuest, outcome)
    }

    @Test
    fun `expired declaration never imports`() {
        val outcome = ClaimExportImportRule.canImport(
            signedIn = true,
            declaration = fresh,
            nowEpochSeconds = fresh.issuedAtEpochSeconds + fresh.ttlSeconds + 1,
            callerAccountId = fresh.accountId,
            fileMime = "application/pdf",
            fileBytes = 100_000L,
            seenNonces = emptySet(),
        )
        assertEquals(ClaimImportDecision.DeniedExpired, outcome)
    }

    @Test
    fun `owner mismatch denied`() {
        val outcome = ClaimExportImportRule.canImport(
            signedIn = true,
            declaration = fresh,
            nowEpochSeconds = fresh.issuedAtEpochSeconds + 10,
            callerAccountId = "acc-other",
            fileMime = "application/pdf",
            fileBytes = 100_000L,
            seenNonces = emptySet(),
        )
        assertEquals(ClaimImportDecision.DeniedOwnerMismatch, outcome)
    }

    @Test
    fun `wrong file type refused`() {
        val outcome = ClaimExportImportRule.canImport(
            signedIn = true,
            declaration = fresh,
            nowEpochSeconds = fresh.issuedAtEpochSeconds + 10,
            callerAccountId = fresh.accountId,
            fileMime = "image/png",
            fileBytes = 100_000L,
            seenNonces = emptySet(),
        )
        assertEquals(ClaimImportDecision.DeniedFileType, outcome)
    }

    @Test
    fun `oversize bytes refused`() {
        val outcome = ClaimExportImportRule.canImport(
            signedIn = true,
            declaration = fresh,
            nowEpochSeconds = fresh.issuedAtEpochSeconds + 10,
            callerAccountId = fresh.accountId,
            fileMime = "application/pdf",
            fileBytes = ClaimExportImportRule.MAX_SIGNED_BYTES + 1,
            seenNonces = emptySet(),
        )
        assertEquals(ClaimImportDecision.DeniedTooLarge, outcome)
    }

    @Test
    fun `replayed nonce rejected`() {
        val outcome = ClaimExportImportRule.canImport(
            signedIn = true,
            declaration = fresh,
            nowEpochSeconds = fresh.issuedAtEpochSeconds + 10,
            callerAccountId = fresh.accountId,
            fileMime = "application/pdf",
            fileBytes = 100_000L,
            seenNonces = setOf(fresh.nonce),
        )
        assertEquals(ClaimImportDecision.DeniedReplay, outcome)
    }

    @Test
    fun `fresh matching import accepted without certifying powers`() {
        val outcome = ClaimExportImportRule.canImport(
            signedIn = true,
            declaration = fresh,
            nowEpochSeconds = fresh.issuedAtEpochSeconds + 10,
            callerAccountId = fresh.accountId,
            fileMime = "application/pdf",
            fileBytes = 100_000L,
            seenNonces = emptySet(),
        )
        assertTrue(outcome is ClaimImportDecision.Accepted)
        assertFalse((outcome as ClaimImportDecision.Accepted).certifiesPowers)
    }
}
