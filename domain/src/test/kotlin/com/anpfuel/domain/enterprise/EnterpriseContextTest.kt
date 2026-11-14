package com.anpfuel.domain.enterprise

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseContextTest {
    private val a = "7b0e2a54-0000-4000-8000-000000000001"
    private val b = "7b0e2a54-0000-4000-8000-000000000002"
    private fun summary(owner: String = a, organization: String = a, origin: String = "https://api.example.invalid") =
        EnterpriseContextSummary(EnterprisePartition.of(owner, origin, organization), b, "Synthetic company")

    @Test fun `context metadata is typed redacted and never membership authority`() {
        val value = summary()
        assertEquals(a, value.partition.ownerAccount)
        assertEquals(a, value.partition.organizationId)
        assertEquals(b, value.membershipId)
        assertEquals("Synthetic company", value.organizationName)
        assertFalse(value.toString().contains(a))
        assertFalse(value.toString().contains("Synthetic"))
        assertThrows(IllegalArgumentException::class.java) { EnterpriseContextSummary(value.partition, "foreign", "Company") }
        for (name in listOf("", " ", "Private\nCompany", "x".repeat(121))) {
            assertThrows(IllegalArgumentException::class.java) { EnterpriseContextSummary(value.partition, b, name) }
        }
        assertEquals(120, EnterpriseContextSummary(value.partition, b, "🚚".repeat(120)).organizationName.codePointCount(0, 240))
    }

    @Test fun `a bounded page freezes metadata and permits empty scanned pages with a next cursor`() {
        val values = mutableListOf(summary())
        val page = EnterpriseContextPage(values, b)
        values.clear()
        assertEquals(1, page.contexts.size)
        assertEquals(b, page.nextAfterOrganizationId)
        assertTrue(EnterpriseContextPage(emptyList(), b).contexts.isEmpty())
        assertNull(EnterpriseContextPage(emptyList()).nextAfterOrganizationId)
        assertFalse(page.toString().contains(a))
    }

    @Test fun `duplicate foreign oversized and backward pages refuse`() {
        assertThrows(IllegalArgumentException::class.java) { EnterpriseContextPage(listOf(summary(), summary())) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseContextPage(listOf(summary(), summary(owner = b, organization = b))) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseContextPage(listOf(summary(), summary(organization = b, origin = "https://other.example.invalid"))) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseContextPage(List(101) { summary() }) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseContextPage(listOf(summary(organization = b)), a) }
        assertThrows(IllegalArgumentException::class.java) { EnterpriseContextPage(emptyList(), "") }
    }
}
