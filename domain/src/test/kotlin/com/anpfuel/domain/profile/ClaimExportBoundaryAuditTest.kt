package com.anpfuel.domain.profile

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ClaimExportBoundaryAuditTest {
    @Test
    fun declarationExportDoesNotGrantAuthorityOrUnsupportedScopes() {
        val allowed = ClaimExportRequest("account", "station", setOf("edit_hours"))
        assertEquals(ClaimExportDecision.DeniedGuest, ClaimExportImportRule.canExport(false, allowed))
        assertEquals(ClaimExportDecision.DeniedGuest, ClaimExportImportRule.canExport(true, allowed.copy(accountId = " ")))
        assertEquals(ClaimExportDecision.DeniedInvalid, ClaimExportImportRule.canExport(true, allowed.copy(stationId = "")))
        assertEquals(ClaimExportDecision.DeniedInvalid, ClaimExportImportRule.canExport(true, allowed.copy(scopes = emptySet())))
        assertEquals(ClaimExportDecision.DeniedInvalid, ClaimExportImportRule.canExport(true, allowed.copy(scopes = setOf("edit_hours", "grant_role"))))
        val prepared = ClaimExportImportRule.canExport(true, allowed) as ClaimExportDecision.Ready
        assertEquals(allowed.scopes, prepared.declaration.scopes)
        assertEquals(900L, prepared.declaration.ttlSeconds)
        assertEquals("", prepared.declaration.operatorName)
    }
}
