package com.anpfuel.domain.enterprise.expense

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import com.anpfuel.domain.enterprise.ReportingZones
import com.anpfuel.domain.enterprise.TimeDeclaration
import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** Loads the same expense-v1 oracle as Go; expectations are never computed here. */
class EnterpriseExpenseVectorsTest {
    private val corpusHash = "b411e1cfa55c0aca4e4c9cadfab2f286ec22c3215e3103c119a767e013885dfe"
    private val wantCounts = mapOf("ENUM" to 40, "AMOUNTS" to 36, "CATEGORY" to 24, "PERIOD" to 17, "EXPENSE" to 57, "TOTALS" to 48)
    private val errorCodes = ExpenseFailure.entries.map { it.name }.toSet()
    private val aliases = mapOf("A" to 1, "B" to 2, "E" to 11, "E2" to 12, "E3" to 13, "E4" to 14, "E5" to 15, "E6" to 16,
        "E7" to 17, "E8" to 18, "E9" to 19, "F" to 21, "F2" to 22, "F3" to 23, "W" to 31, "W2" to 32)
    private val expenseKeys = "org id id_org origin source source_org category paid company at entry offset zone unit_zone omit"
    private val allowedArgs = mapOf("ENUM" to "kind value_hex", "AMOUNTS" to "paid company", "CATEGORY" to "code",
        "PERIOD" to "start end", "EXPENSE" to expenseKeys, "TOTALS" to "org start end basis zone unit_zone items")
        .mapValues { it.value.split(' ').toSet() }
    private val omittable = setOf("id", "origin", "category", "amounts", "competence", "zone")
    private val enumSelectors = setOf("ORIGIN", "STATE", "BASIS")

    private fun id(index: Int) = "9e000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private fun alias(raw: String): String = aliases[raw]?.let(::id) ?: raw
    private fun label(raw: String): String = aliases.entries.firstOrNull { id(it.value) == raw }?.key ?: raw
    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotBlank()); p[0] to p[1] }
        require(pairs.map { it.first }.distinct().size == pairs.size)
        return pairs.toMap()
    }
    private fun decode(raw: String): String {
        require(raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(raw.chunked(2).map { it.toInt(16).toByte() }.toByteArray())).toString()
    }
    private fun caseShape(operation: String, args: Map<String, String>) {
        val allowed = requireNotNull(allowedArgs[operation])
        require(args.keys.all { it in allowed })
        if (operation == "ENUM") require(args.getValue("kind") in enumSelectors)
    }
    private fun present(raw: String?): String? = raw?.takeUnless { it == "?" }

    @Test fun `malformed fixture grammar is rejected before domain dispatch`() {
        assertThrows(IllegalArgumentException::class.java) { caseShape("ENUM", fields("kind=OTHER;value_hex=")) }
        assertThrows(IllegalArgumentException::class.java) { caseShape("OTHER", fields("code=FUEL")) }
        assertThrows(IllegalArgumentException::class.java) { caseShape("CATEGORY", fields("code=FUEL;extra=1")) }
        assertThrows(IllegalArgumentException::class.java) { fields("a=1;a=2") }
        assertThrows(IllegalArgumentException::class.java) { fields("novalue") }
        assertThrows(IllegalArgumentException::class.java) { decode("4F") }
        assertThrows(CharacterCodingException::class.java) { decode("ff") }
        assertThrows(CharacterCodingException::class.java) { decode("eda080") }
    }

    private fun expense(args: Map<String, String>): Expense {
        val f = mapOf("org" to "A", "id" to "E", "origin" to "FUELING", "source" to "F", "category" to "FUEL", "paid" to "97350",
            "company" to "97350", "at" to "2026-11-14T12:00:00Z", "entry" to "UTC", "offset" to "0", "zone" to "America/Sao_Paulo",
            "unit_zone" to "?") + args
        val omitted = f["omit"]?.split(',')?.toSet().orEmpty()
        require(omitted.all { it in omittable })
        val org = f.getValue("org")
        val reference = if ("id" in omitted) null else ExpenseRef(alias(f["id_org"] ?: org), alias(f.getValue("id")))
        val origin = if ("origin" in omitted) null else {
            val kind = OriginKind.parse(f.getValue("origin"))
            val source = present(f["source"])?.let { SourceRef(alias(f["source_org"] ?: org), alias(it)) }
            ExpenseOrigin.create(alias(org), kind, source)
        }
        val category = if ("category" in omitted) null else ExpenseCategory.parse(f.getValue("category"))
        val amounts = if ("amounts" in omitted) null else ExpenseAmounts.parse(present(f["paid"]), present(f["company"]))
        val competence = if ("competence" in omitted) null else TimeDeclaration(EnterpriseTime.parseInstant(f.getValue("at")),
            f.getValue("entry"), f.getValue("offset").toLongOrNull() ?: throw ExpenseDomainException())
        val zones = if ("zone" in omitted) null else ReportingZones(f.getValue("zone"), present(f["unit_zone"]))
        return Expense.create(alias(org), ExpenseInput(reference, origin, category, amounts, competence, zones))
    }

    private fun expenseFields(value: Expense): Map<String, String> {
        val key = value.originKey
        return mapOf("ready" to "true", "org" to label(value.reference.organizationId), "id" to label(value.reference.id),
            "origin" to value.origin.kind.name, "source" to (value.origin.source?.id?.let(::label) ?: "?"),
            "key" to "${label(key.organizationId)}:${key.kind.name}:${label(key.source)}", "category" to value.category.code,
            "paid" to value.amounts.paidCentavos.toString(), "company" to value.amounts.companyCentavos.toString(),
            "at" to EnterpriseTime.formatInstant(value.competence.occurredAt), "entry" to value.competence.entryZone,
            "offset" to value.competence.deviceOffsetSeconds.toString(), "day" to value.reporting.day,
            "zone" to value.reporting.zone, "reporting_source" to value.reporting.source)
    }

    private fun totals(a: Map<String, String>): Map<String, String> {
        val entries = a.getValue("items").takeIf { it.isNotEmpty() }?.split('~')?.map {
            val p = it.split(',')
            require(p.size == 8)
            val value = expense(mapOf("id" to p[0], "org" to p[1], "origin" to p[2], "source" to p[3], "paid" to p[4],
                "company" to p[5], "at" to p[6], "zone" to (a["zone"] ?: "America/Sao_Paulo"), "unit_zone" to (a["unit_zone"] ?: "?")))
            ExpenseEntry(value, ReviewState.parse(p[7]))
        }.orEmpty()
        val period = ReportingPeriod.of(a.getValue("start"), a.getValue("end"))
        val basis = TotalsBasis.parse(a.getValue("basis"))
        val total = ExpenseTotals.compute(alias(a.getValue("org")), period, basis, entries)
        return mapOf("org" to label(total.organizationId), "start" to total.period.start, "end" to total.period.end,
            "basis" to total.basis.name, "approved" to total.approvedCentavos.toString(),
            "approved_count" to total.approvedCount.toString(), "pending" to total.pendingCentavos.toString(),
            "pending_count" to total.pendingCount.toString(), "excluded_count" to total.excludedCount.toString(),
            "figure" to total.figureCentavos.toString(), "includes_pending" to total.includesPending.toString())
    }

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "ENUM" -> {
            val value = decode(a.getValue("value_hex"))
            when (a.getValue("kind")) {
                "ORIGIN" -> mapOf("value" to OriginKind.parse(value).name)
                "STATE" -> ReviewState.parse(value).let { mapOf("value" to it.name, "counting" to it.counting.name) }
                "BASIS" -> mapOf("value" to TotalsBasis.parse(value).name)
                else -> error("Unknown fixture enum namespace")
            }
        }
        "AMOUNTS" -> ExpenseAmounts.parse(present(a["paid"]), present(a["company"])).let {
            mapOf("paid" to it.paidCentavos.toString(), "company" to it.companyCentavos.toString())
        }
        "CATEGORY" -> mapOf("code" to ExpenseCategory.parse(a.getValue("code")).code)
        "PERIOD" -> ReportingPeriod.of(a.getValue("start"), a.getValue("end")).let { mapOf("start" to it.start, "end" to it.end) }
        "EXPENSE" -> expenseFields(expense(a))
        "TOTALS" -> totals(a)
        else -> error("Unknown fixture operation")
    }

    @TestFactory fun allSharedExpenseVectors(): List<DynamicTest> {
        val file = File("../contracts/testdata/enterprise/expense/expense-v1.psv")
        assertEquals(corpusHash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        assertEquals(222, rows.size)
        require(rows.all { it.size == 4 && it[0].isNotBlank() })
        assertEquals(rows.size, rows.map { it[0] }.distinct().size)
        assertEquals(wantCounts, rows.groupingBy { it[1] }.eachCount())
        val codes = mutableSetOf<String>()
        for (row in rows) {
            caseShape(row[1], fields(row[2]))
            val expected = fields(row[3])
            expected["error"]?.let { require(expected.size == 1); codes += it }
        }
        assertEquals(errorCodes, codes, "every error code needs a shared vector and no other code may appear")
        return rows.map { (id, operation, rawArgs, expected) ->
            DynamicTest.dynamicTest(id) {
                val actual = try { execute(operation, fields(rawArgs)) }
                catch (failure: ExpenseDomainException) { mapOf("error" to failure.failure.name) }
                catch (_: EnterpriseTimeException) { mapOf("error" to "INVALID") }
                assertEquals(fields(expected), actual)
            }
        }
    }
}
