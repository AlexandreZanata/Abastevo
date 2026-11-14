package com.anpfuel.domain.enterprise.expense

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.ReportingZones
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.fueling.FuelingRef
import java.math.BigInteger
import java.util.Random
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class EnterpriseExpenseTest {
    private fun uuid(n: Int) = "9e000000-0000-4000-8000-" + n.toString().padStart(12, '0')
    private val org = uuid(1)
    private val foreign = uuid(2)
    private val november = ReportingPeriod.of("2026-11-01", "2026-12-01")

    private fun failure(block: () -> Unit): ExpenseFailure? =
        try { block(); null } catch (e: ExpenseDomainException) { e.failure }

    private fun input(n: Int, company: String = "97350", paid: String = company): ExpenseInput = ExpenseInput(
        reference = ExpenseRef(org, uuid(n)),
        origin = ExpenseOrigin.create(org, OriginKind.FUELING, SourceRef(org, uuid(1000 + n))),
        category = ExpenseCategory.parse("FUEL"),
        amounts = ExpenseAmounts.parse(paid, company),
        competence = TimeDeclaration(EnterpriseTime.parseInstant("2026-11-14T12:00:00Z"), "UTC", 0),
        zones = ReportingZones("America/Sao_Paulo"),
    )

    @Test fun `origin kinds are closed and settlement is refused`() {
        for (raw in listOf("FUELING", "WORK_ORDER", "DIRECT")) assertEquals(raw, OriginKind.parse(raw).name)
        for (raw in listOf("PAYMENT", "REIMBURSEMENT", "CARD_STATEMENT", "ADVANCE", "PAYABLE", "DOCUMENT", "STOCK_PURCHASE", "STOCK_TRANSFER")) {
            assertEquals(ExpenseFailure.NOT_COST_ORIGIN, failure { OriginKind.parse(raw) }, raw)
        }
        for (raw in listOf("", "fueling", "Payment", "MAINTENANCE", "MANUAL", " DIRECT", "DIRECT\n")) {
            assertEquals(ExpenseFailure.INVALID, failure { OriginKind.parse(raw) }, raw)
        }
    }

    @Test fun `fueling origin reuses the fueling identity and its scope`() {
        val origin = ExpenseOrigin.fueling(org, FuelingRef(org, uuid(21)))
        assertEquals(OriginKind.FUELING, origin.kind)
        assertEquals(SourceRef(org, uuid(21)), origin.source)
        assertEquals(ExpenseFailure.SCOPE, failure { ExpenseOrigin.fueling(org, FuelingRef(foreign, uuid(21))) })
        assertEquals(ExpenseFailure.SCOPE, failure { ExpenseOrigin.fueling("not-an-organization", FuelingRef(org, uuid(21))) })
        val value = Expense.create(org, input(11).copy(origin = origin))
        assertEquals(OriginKey(org, OriginKind.FUELING, uuid(21)), value.originKey)
    }

    @Test fun `origin shape and one cost key`() {
        val source = SourceRef(org, uuid(21))
        assertEquals(ExpenseFailure.INVALID, failure { ExpenseOrigin.create(org, OriginKind.DIRECT, source) })
        assertEquals(ExpenseFailure.INVALID, failure { ExpenseOrigin.create(org, OriginKind.FUELING, null) })
        assertEquals(ExpenseFailure.INVALID, failure { ExpenseOrigin.create(org, OriginKind.WORK_ORDER, null) })
        assertEquals(ExpenseFailure.SCOPE, failure { ExpenseOrigin.create(org, OriginKind.WORK_ORDER, SourceRef(foreign, uuid(21))) })
        assertEquals(ExpenseFailure.SCOPE, failure { ExpenseOrigin.create("bad", OriginKind.DIRECT, null) })
        val direct = ExpenseOrigin.create(org, OriginKind.DIRECT, null)
        assertNull(direct.source)
        val keys = listOf(direct, ExpenseOrigin.create(org, OriginKind.WORK_ORDER, source), ExpenseOrigin.create(org, OriginKind.FUELING, source))
            .mapIndexed { n, origin -> Expense.create(org, input(11 + n).copy(origin = origin)).originKey }.toSet()
        assertEquals(3, keys.size)
        val again = Expense.create(org, input(30).copy(origin = direct)).originKey
        assertEquals(OriginKey(org, OriginKind.DIRECT, uuid(30)), again)
        assertNotEquals(OriginKey(org, OriginKind.FUELING, uuid(21)), OriginKey(org, OriginKind.WORK_ORDER, uuid(21)))
        val foreignOrigin = ExpenseOrigin.create(foreign, OriginKind.FUELING, SourceRef(foreign, uuid(21)))
        assertEquals(ExpenseFailure.SCOPE, failure { Expense.create(org, input(31).copy(origin = foreignOrigin)) })
    }

    @Test fun `amounts never default round or recompute`() {
        assertEquals(ExpenseFailure.INVALID, failure { ExpenseAmounts.parse(null, "1") })
        assertEquals(ExpenseFailure.INVALID, failure { ExpenseAmounts.parse("1", null) })
        assertEquals(ExpenseFailure.INVALID, failure { ExpenseAmounts.parse("", "0") })
        assertEquals(ExpenseFailure.COMPANY_EXCEEDS_PAID, failure { ExpenseAmounts.parse("97350", "97351") })
        val zero = ExpenseAmounts.parse("0", "0")
        assertEquals(0L, zero.paidCentavos)
        assertEquals(0L, zero.companyCentavos)
        assertEquals(ExpenseFailure.INVALID, failure { Expense.create(org, input(11).copy(amounts = null)) })
        val kept = Expense.create(org, input(11, company = "90000", paid = "97350")).amounts
        assertEquals(97350L, kept.paidCentavos)
        assertEquals(90000L, kept.companyCentavos)
    }

    @Test fun `expense requires every fact and scope`() {
        val base = input(11)
        val cases = mapOf(
            "reference" to base.copy(reference = null),
            "origin" to base.copy(origin = null),
            "category" to base.copy(category = null),
            "competence" to base.copy(competence = null),
            "entry-zone" to base.copy(competence = base.competence!!.copy(entryZone = "Mars/Olympus")),
            "offset" to base.copy(competence = base.competence.copy(deviceOffsetSeconds = 64801)),
            "zones" to base.copy(zones = null),
            "unit-zone" to base.copy(zones = ReportingZones("America/Sao_Paulo", "Mars/Olympus")),
        )
        for ((name, value) in cases) assertEquals(ExpenseFailure.INVALID, failure { Expense.create(org, value) }, name)
        assertEquals(ExpenseFailure.SCOPE, failure { Expense.create(org, base.copy(reference = ExpenseRef(foreign, uuid(11)))) })
        assertEquals(ExpenseFailure.SCOPE, failure { Expense.create("not-an-organization", base) })
        for (raw in listOf("", "not-a-uuid", uuid(11).uppercase(), "00000000-0000-0000-0000-000000000000")) {
            assertEquals(ExpenseFailure.INVALID, failure { ExpenseRef(org, raw) }, raw)
            assertEquals(ExpenseFailure.INVALID, failure { SourceRef(org, raw) }, raw)
        }
        assertEquals(ExpenseFailure.SCOPE, failure { ExpenseRef("bad", uuid(11)) })
        for (raw in listOf("FUEL\n", "FU\u0000EL", "FUEL\u200b")) assertEquals(ExpenseFailure.INVALID, failure { ExpenseCategory.parse(raw) })
    }

    @Test fun `competence uses the operating zone not the entry zone`() {
        val declared = TimeDeclaration(EnterpriseTime.parseInstant("2026-11-01T02:30:00Z"), "Asia/Tokyo", 32400)
        val value = Expense.create(org, input(11).copy(competence = declared))
        assertEquals("2026-10-31", value.reporting.day)
        assertEquals("America/Sao_Paulo", value.reporting.zone)
        assertEquals("organization", value.reporting.source)
        assertEquals(declared, value.competence)
        val unit = Expense.create(org, input(11).copy(
            competence = TimeDeclaration(EnterpriseTime.parseInstant("2026-11-01T03:30:00Z"), "UTC", 0),
            zones = ReportingZones("America/Sao_Paulo", "America/Cuiaba"),
        ))
        assertEquals("2026-10-31", unit.reporting.day)
        assertEquals("America/Cuiaba", unit.reporting.zone)
        assertEquals("unit", unit.reporting.source)
    }

    @Test fun `private values are redacted`() {
        val value = Expense.create(org, input(11))
        val text = listOf(value, value.reference, value.origin, value.origin.source, value.originKey, value.amounts, value.category,
            input(11), ExpenseTotals.compute(org, november, TotalsBasis.APPROVED, listOf(ExpenseEntry(value, ReviewState.APPROVED))))
            .joinToString(" ")
        for (secret in listOf(uuid(11), uuid(1011), "97350", "FUEL")) assertFalse(text.contains(secret), secret)
    }

    @Test fun `value equality is typed and by value`() {
        assertEquals(ExpenseRef(org, uuid(11)), ExpenseRef(org, uuid(11)))
        assertEquals(ExpenseRef(org, uuid(11)).hashCode(), ExpenseRef(org, uuid(11)).hashCode())
        assertNotEquals(ExpenseRef(org, uuid(11)), SourceRef(org, uuid(11)))
        assertNotEquals(ExpenseRef(org, uuid(11)), ExpenseRef(foreign, uuid(11)))
        assertNotEquals(ExpenseRef(org, uuid(11)), ExpenseRef(org, uuid(12)))
        assertEquals(ExpenseCategory.parse("FUEL"), ExpenseCategory.parse("FUEL"))
        assertEquals(ExpenseCategory.parse("FUEL").hashCode(), ExpenseCategory.parse("FUEL").hashCode())
        assertNotEquals(ExpenseCategory.parse("FUEL"), ExpenseCategory.parse("TOLL"))
        val key = OriginKey(org, OriginKind.FUELING, uuid(21))
        assertEquals(key.hashCode(), OriginKey(org, OriginKind.FUELING, uuid(21)).hashCode())
        assertNotEquals(key, OriginKey(foreign, OriginKind.FUELING, uuid(21)))
        assertNotEquals(key, OriginKey(org, OriginKind.FUELING, uuid(22)))
        assertNotEquals(key, uuid(21))
        assertEquals(november.hashCode(), ReportingPeriod.of("2026-11-01", "2026-12-01").hashCode())
        assertNotEquals(november, ReportingPeriod.of("2026-11-01", "2026-11-30"))
        assertNotEquals(november, ReportingPeriod.of("2026-10-01", "2026-12-01"))
        assertNotEquals(november, "2026-11-01")
        assertEquals("ReportingPeriod[2026-11-01, 2026-12-01)", november.toString())
    }

    @Test fun `review states and bases are closed`() {
        val counting = mapOf("DRAFT" to "EXCLUDED", "SUBMITTED" to "PENDING", "IN_REVIEW" to "PENDING", "APPROVED" to "APPROVED",
            "RETURNED" to "EXCLUDED", "REJECTED" to "EXCLUDED", "CANCELLED" to "EXCLUDED", "VOIDED" to "EXCLUDED")
        assertEquals(counting, ReviewState.entries.associate { it.name to ReviewState.parse(it.name).counting.name })
        for (raw in listOf("", "approved", "PAID", "SETTLED", "PARTIALLY_APPROVED", "CANCELED")) {
            assertEquals(ExpenseFailure.INVALID, failure { ReviewState.parse(raw) }, raw)
        }
        for (raw in listOf("", "ALL", "PENDING", "approved")) assertEquals(ExpenseFailure.INVALID, failure { TotalsBasis.parse(raw) }, raw)
    }

    @Test fun `reporting period is half open`() {
        val membership = mapOf("2026-10-31" to false, "2026-11-01" to true, "2026-11-30" to true, "2026-12-01" to false,
            "" to false, "2026-11-1" to false, "2026-11-15T00:00:00Z" to false, "2026-11-31" to false)
        for ((day, inside) in membership) assertEquals(inside, november.contains(day), day)
        assertEquals(ReportingPeriod.of("2026-11-01", "2026-12-01"), november)
    }

    @Test fun `totals refuse foreign scope`() {
        val entries = listOf(ExpenseEntry(Expense.create(org, input(11)), ReviewState.APPROVED))
        assertEquals(ExpenseFailure.SCOPE, failure { ExpenseTotals.compute(foreign, november, TotalsBasis.APPROVED, entries) })
        assertEquals(ExpenseFailure.SCOPE, failure { ExpenseTotals.compute("not-an-organization", november, TotalsBasis.APPROVED, entries) })
    }

    @Test fun `totals are exact widened and partitioned`() {
        val random = Random(20261114)
        repeat(40) { round ->
            val entries = mutableListOf<ExpenseEntry>()
            var approved = BigInteger.ZERO
            var pending = BigInteger.ZERO
            var counts = Triple(0, 0, 0)
            repeat(random.nextInt(60)) { n ->
                val company = Math.floorMod(random.nextLong(), 1_000_000_000_001L)
                val state = ReviewState.entries[random.nextInt(ReviewState.entries.size)]
                entries += ExpenseEntry(Expense.create(org, input(100 + n, company.toString())), state)
                when (state.counting) {
                    CostCounting.APPROVED -> { approved += BigInteger.valueOf(company); counts = counts.copy(first = counts.first + 1) }
                    CostCounting.PENDING -> { pending += BigInteger.valueOf(company); counts = counts.copy(second = counts.second + 1) }
                    CostCounting.EXCLUDED -> counts = counts.copy(third = counts.third + 1)
                }
            }
            for (basis in TotalsBasis.entries) {
                val total = ExpenseTotals.compute(org, november, basis, entries.toList())
                val figure = if (basis == TotalsBasis.INCLUDES_PENDING) approved + pending else approved
                assertEquals(approved, total.approvedCentavos, "round $round")
                assertEquals(pending, total.pendingCentavos)
                assertEquals(figure, total.figureCentavos)
                assertEquals(counts, Triple(total.approvedCount, total.pendingCount, total.excludedCount))
                assertEquals(basis == TotalsBasis.INCLUDES_PENDING, total.includesPending)
                assertEquals(org, total.organizationId)
                assertEquals(november, total.period)
            }
        }
    }

    @Test fun `totals stay exact beyond the record ceiling`() {
        val entries = (0 until 5000).map { ExpenseEntry(Expense.create(org, input(10_000 + it, "1000000000000")), ReviewState.APPROVED) }
        val total = ExpenseTotals.compute(org, november, TotalsBasis.INCLUDES_PENDING, entries)
        assertEquals(BigInteger("5000000000000000"), total.approvedCentavos)
        assertEquals(BigInteger("5000000000000000"), total.figureCentavos)
        assertEquals(5000, total.approvedCount)
        assertTrue(total.includesPending)
    }
}
