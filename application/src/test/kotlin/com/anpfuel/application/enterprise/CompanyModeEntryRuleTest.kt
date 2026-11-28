package com.anpfuel.application.enterprise

import com.anpfuel.domain.enterprise.EnterpriseInvitationAcknowledgement
import com.anpfuel.domain.enterprise.EnterprisePartition
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** B-BR-EMP-ENTRY-01: the Perfil "Company mode" entry opens only for an account with a company link. */
class CompanyModeEntryRuleTest {
    private val account = "7b0e2a54-0000-4000-8000-000000000001"
    private val other = "7b0e2a54-0000-4000-8000-000000000002"
    private val origin = "https://api.example.invalid"
    private val binding = EnterpriseAccessBinding(account, origin, account, 0)
    private val company = EnterpriseContextSummary(EnterprisePartition.of(account, origin, other), other, "Synthetic company")

    @Test
    fun `signed out keeps the entry closed`() {
        assertFalse(CompanyModeEntryRule.enabled(binding = null, hint = true, state = EnterpriseContextState(contexts = listOf(company), catalogConfirmed = true)))
    }

    @Test
    fun `no link known keeps the entry closed`() {
        assertFalse(CompanyModeEntryRule.enabled(binding, hint = null, state = EnterpriseContextState()))
        assertFalse(CompanyModeEntryRule.enabled(binding, hint = false, state = EnterpriseContextState()))
    }

    @Test
    fun `a remembered link opens the entry until the server says otherwise`() {
        assertTrue(CompanyModeEntryRule.enabled(binding, hint = true, state = EnterpriseContextState()))
        assertFalse(CompanyModeEntryRule.enabled(binding, hint = true, state = EnterpriseContextState(catalogConfirmed = true)))
    }

    @Test
    fun `a confirmed company list opens the entry`() {
        assertTrue(
            CompanyModeEntryRule.enabled(binding, hint = false, state = EnterpriseContextState(contexts = listOf(company), catalogConfirmed = true)),
        )
    }

    @Test
    fun `an active company context always keeps the way back to personal mode`() {
        assertTrue(CompanyModeEntryRule.enabled(binding, hint = false, state = EnterpriseContextState(selected = company, catalogConfirmed = true)))
    }

    @Test
    fun `only a confirmed, idle list is recorded`() {
        assertNull(CompanyModeEntryRule.linkToRecord(EnterpriseContextState(contexts = listOf(company))))
        assertNull(CompanyModeEntryRule.linkToRecord(EnterpriseContextState(contexts = listOf(company), catalogConfirmed = true, busy = true)))
        assertEquals(true, CompanyModeEntryRule.linkToRecord(EnterpriseContextState(contexts = listOf(company), catalogConfirmed = true)))
        assertEquals(false, CompanyModeEntryRule.linkToRecord(EnterpriseContextState(catalogConfirmed = true)))
    }

    @Test
    fun `an accepted invitation counts only once the membership is active`() {
        val active = EnterpriseInvitationAcknowledgement(other, other, other, "ACCEPTED", "ACTIVE", false)
        val waiting = EnterpriseInvitationAcknowledgement(other, other, other, "ACCEPTED", "INVITED", false)
        val declined = EnterpriseInvitationAcknowledgement(other, other, other, "DECLINED", "ENDED", false)

        assertTrue(CompanyModeEntryRule.linkedBy(EnterpriseInvitationState(stage = EnterpriseInvitationStage.RESULT, acknowledgement = active)))
        assertFalse(CompanyModeEntryRule.linkedBy(EnterpriseInvitationState(stage = EnterpriseInvitationStage.RESULT, acknowledgement = waiting)))
        assertFalse(CompanyModeEntryRule.linkedBy(EnterpriseInvitationState(stage = EnterpriseInvitationStage.RESULT, acknowledgement = declined)))
        assertFalse(CompanyModeEntryRule.linkedBy(EnterpriseInvitationState(stage = EnterpriseInvitationStage.PREVIEW)))
    }

    @Test
    fun `the remembered link is probed only when unknown and nothing is selected`() {
        assertTrue(CompanyModeEntryRule.shouldProbe(binding, hint = null, state = EnterpriseContextState()))
        assertFalse(CompanyModeEntryRule.shouldProbe(binding, hint = false, state = EnterpriseContextState()))
        assertFalse(CompanyModeEntryRule.shouldProbe(null, hint = null, state = EnterpriseContextState()))
        assertFalse(CompanyModeEntryRule.shouldProbe(binding, hint = null, state = EnterpriseContextState(busy = true)))
        assertFalse(CompanyModeEntryRule.shouldProbe(binding, hint = null, state = EnterpriseContextState(selected = company)))
    }
}
