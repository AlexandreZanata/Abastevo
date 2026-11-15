package com.anpfuel.domain.enterprise.maintenance.checklist

import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class EnterpriseChecklistTest {
    private val categoryA = "9c000000-0000-4000-8000-000000001001"
    private val categoryB = "9c000000-0000-4000-8000-000000001002"

    private fun failure(block: () -> Unit) = assertThrows(ChecklistDomainException::class.java) { block() }.failure

    @Test fun `closed vocabularies and the default policy`() {
        assertEquals(ChecklistContext.AD_HOC, ChecklistContext.parse("AD_HOC"))
        assertEquals(TargetKind.ASSETS, TargetKind.parse("ASSETS"))
        assertEquals(PolicyMode.WARN, PolicyMode.parse("WARN"))
        listOf("", "departure", " DEPARTURE", "PARKING").forEach { assertEquals(ChecklistFailure.INVALID, failure { ChecklistContext.parse(it) }) }
        assertEquals(ChecklistFailure.INVALID, failure { TargetKind.parse("UNITS") })
        assertEquals(ChecklistFailure.INVALID, failure { PolicyMode.parse("ALLOW") })
        val p = ChecklistPolicy.default()
        assertEquals(PolicyMode.BLOCK, p.mode)
        assertEquals(12 * 3_600L, p.validitySeconds)
    }

    @Test fun `a definition is a canonical independent copy`() {
        val ids = mutableListOf(categoryB, categoryA)
        val items = mutableListOf(ChecklistItem("Pneus", critical = true, photoOnNotOk = true), ChecklistItem("Estepe", optional = true))
        val d = ChecklistDefinition.of("Saída diária", "DEPARTURE", ChecklistTarget("CATEGORIES", ids), items)
        assertEquals(listOf(categoryA, categoryB), d.targetIds)
        ids[0] = "mutated"; items.clear()
        assertEquals(listOf(categoryA, categoryB), d.targetIds)
        assertEquals(2, d.items.size)
        assertTrue(ChecklistDefinition.of("Padrão", "RETURN", ChecklistTarget("ORGANIZATION", emptyList()), listOf(ChecklistItem("x"))).targetIds.isEmpty())
        assertEquals(ChecklistFailure.INVALID, failure { ChecklistDefinition.of("x", "DEPARTURE", ChecklistTarget("ORGANIZATION", null), listOf(ChecklistItem("x", critical = true, optional = true))) })
        assertEquals(ChecklistFailure.INVALID, failure { ChecklistDefinition.of("x\uD800", "DEPARTURE", ChecklistTarget("ORGANIZATION", null), listOf(ChecklistItem("x"))) })
    }

    @Test fun `evaluation lists issues and acknowledgement under WARN`() {
        val items = listOf(ChecklistItem("Freios", critical = true), ChecklistItem("Buzina"))
        val e = Checklist.evaluate(items, "WARN", 3_600, listOf(ChecklistResponse(ChecklistAnswer.NOT_OK), ChecklistResponse(ChecklistAnswer.NOT_OK, "Fraca")))
        assertEquals(ChecklistEvaluation(ChecklistOutcome.COMPLETED_WITH_ISSUES, listOf(1, 2), listOf(1), true), e)
        val clean = Checklist.evaluate(items, "BLOCK", 43_200, listOf(ChecklistResponse("OK"), ChecklistResponse("OK")))
        assertEquals(ChecklistEvaluation(ChecklistOutcome.COMPLETED_OK, emptyList(), emptyList(), false), clean)
    }

    @Test fun `a response note is never rendered`() {
        val r = ChecklistResponse(ChecklistAnswer.NOT_OK, "Placa SYN1A23 motorista fictício", true)
        assertFalse(r.toString().contains("SYN1A23"))
        assertFalse(listOf(r).toString().contains("fictício"))
    }

    @Test fun `identifiers and instants`() {
        assertTrue(Checklist.canonicalId("9c000000-0000-4000-8000-000000002001"))
        listOf("9c000000-0000-4000-8000-00000000200g", "9c000000x0000-4000-8000-000000002001", "9C000000-0000-4000-8000-000000002001", "00000000-0000-0000-0000-000000000000", "")
            .forEach { assertFalse(Checklist.canonicalId(it), it) }
        val history = listOf(ChecklistPolicyRevision(1, "BLOCK", 43_200, Instant.parse("2026-11-01T00:00:00Z")))
        assertEquals(ChecklistFailure.INVALID, failure { Checklist.policyAt(history, Instant.parse("+10000-01-01T00:00:00Z")) })
        assertNull(Checklist.policyAt(history, Instant.parse("2026-10-31T23:00:00Z")))
    }
}
