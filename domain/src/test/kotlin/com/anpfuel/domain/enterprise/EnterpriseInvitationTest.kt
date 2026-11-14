package com.anpfuel.domain.enterprise

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseInvitationTest {
    private val id = "7b0e2a54-0000-4000-8000-000000000001"
    private fun preview(grants: List<EnterpriseInvitationGrant> = listOf(EnterpriseInvitationGrant("DRIVER", "SELF"))) =
        EnterpriseInvitationPreview("Synthetic company", false, grants, true, "a".repeat(64))

    @Test fun `preview freezes literal grants and keeps consent private`() {
        val grants = mutableListOf(EnterpriseInvitationGrant("DRIVER", "SELF"))
        val value = preview(grants)
        grants.clear()
        assertEquals(1, value.grants.size)
        assertEquals(EnterpriseInvitationContract.NOTICE_VERSION, value.noticeVersion)
        assertFalse(value.toString().contains("Synthetic"))
        assertFalse(value.toString().contains("a".repeat(64)))
        assertFalse(value.grants.single().toString().contains("DRIVER"))
    }

    @Test fun `unsupported duplicate and improperly scoped grants refuse`() {
        for ((role, scope) in listOf("UNKNOWN" to "ORG", "OWNER" to "SELF", "DRIVER" to "STATION")) {
            assertThrows(IllegalArgumentException::class.java) { EnterpriseInvitationGrant(role, scope) }
        }
        assertThrows(IllegalArgumentException::class.java) { preview(emptyList()) }
        assertThrows(IllegalArgumentException::class.java) { preview(List(2) { EnterpriseInvitationGrant("DRIVER", "SELF") }) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseInvitationPreview("", false, listOf(EnterpriseInvitationGrant("DRIVER", "SELF")), false, "a".repeat(64)) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseInvitationPreview("Company", false, listOf(EnterpriseInvitationGrant("DRIVER", "SELF")), false, "A".repeat(64)) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseInvitationPreview("Company", false, listOf(EnterpriseInvitationGrant("DRIVER", "SELF")), false, "a".repeat(64), "unknown") }
    }

    @Test fun `acknowledgement never upgrades waiting admission to membership authority`() {
        val waiting = EnterpriseInvitationAcknowledgement(id, id, id, "ACCEPTED", "INVITED", false)
        assertTrue(waiting.awaitingConfirmation)
        assertFalse(waiting.active)
        assertTrue(EnterpriseInvitationAcknowledgement(id, id, id, "ACCEPTED", "ACTIVE", true).active)
        assertFalse(EnterpriseInvitationAcknowledgement(id, id, id, "DECLINED", "ENDED", false).active)
        for ((state, membership) in listOf("ACCEPTED" to "ENDED", "DECLINED" to "ACTIVE", "WAITING" to "INVITED")) {
            assertThrows(IllegalArgumentException::class.java) { EnterpriseInvitationAcknowledgement(id, id, id, state, membership, false) }
        }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseInvitationAcknowledgement("foreign", id, id, "ACCEPTED", "ACTIVE", false) }
        assertFalse(waiting.toString().contains(id))
    }

    @Test fun `only the reviewed notice version and text permit informed consent`() {
        assertTrue(EnterpriseInvitationContract.knownNotice(EnterpriseInvitationContract.NOTICE_VERSION, EnterpriseInvitationContract.NOTICE_TEXT))
        assertFalse(EnterpriseInvitationContract.knownNotice("future", EnterpriseInvitationContract.NOTICE_TEXT))
        assertFalse(EnterpriseInvitationContract.knownNotice(EnterpriseInvitationContract.NOTICE_VERSION, "Changed collection policy"))
        assertEquals("AIN1.$id.opaque-secret", EnterpriseInvitationContract.normalizeCode(" AIN1.${id.replace("-", "")}.opaque-secret "))
        assertEquals("bad-code", EnterpriseInvitationContract.normalizeCode(" bad-code "))
    }
}
