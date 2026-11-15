package com.anpfuel.domain.enterprise.document

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.EnterpriseTimeException
import java.io.File
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** Same shared oracle as the Go consumer; expectations are never derived from the implementation. */
class EnterpriseDocumentVectorsTest {
    private val aliases = mapOf("A" to 1, "B" to 2, "D" to 201, "N" to 202, "E1" to 211, "E2" to 212, "E3" to 213,
        "F1" to 221, "W1" to 231, "T1" to 241, "S1" to 251, "P1" to 261, "R1" to 271)
    private val digests = mapOf("H1" to "1a".repeat(32), "H2" to "2b".repeat(32))
    private fun id(index: Int) = "9d000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    private fun alias(raw: String): String = if (raw == "?") "" else aliases[raw]?.let(::id) ?: raw
    private fun label(raw: String): String = aliases.entries.firstOrNull { id(it.value) == raw }?.key ?: raw
    private fun digest(raw: String): String = if (raw == "?") "" else digests[raw] ?: raw
    private fun digestLabel(raw: String): String = digests.entries.firstOrNull { it.value == raw }?.key ?: raw
    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotBlank()); p[0] to p[1] }
        require(pairs.map { it.first }.distinct().size == pairs.size)
        return pairs.toMap()
    }
    private val operations = mapOf("ENUM" to setOf("kind", "value"), "ALLOWED" to setOf("dimension", "from"),
        "SCENARIO" to setOf("org", "id", "digest", "steps"), "REPLACE" to setOf("old_steps", "new", "new_org", "new_digest", "new_steps"))
    private val enumSelectors = setOf("DIMENSION", "STORAGE", "EXTRACTION", "STRUCTURE", "CONFERENCE", "RECONCILIATION", "FISCAL", "TARGET")
    private fun caseShape(operation: String, args: Map<String, String>) {
        val keys = requireNotNull(operations[operation])
        require(args.keys.all { it in keys })
        if (operation == "ENUM") require(args.getValue("kind") in enumSelectors)
    }

    @Test fun `unknown fixture namespaces are rejected before domain dispatch`() {
        assertThrows(IllegalArgumentException::class.java) { caseShape("ENUM", fields("kind=STATE;value=STORED")) }
        assertThrows(IllegalArgumentException::class.java) { caseShape("OTHER", fields("kind=STORAGE")) }
        assertThrows(IllegalArgumentException::class.java) { caseShape("SCENARIO", fields("steps=;extra=1")) }
        assertThrows(IllegalArgumentException::class.java) { fields("steps=;steps=") }
    }

    private fun target(raw: String, owner: String): DocumentTarget {
        val (kind, rest) = raw.split('/', limit = 2).also { require(it.size == 2) }
        val parts = rest.split('@', limit = 2)
        val parsed = DocumentTargetKind.parse(kind)
        return DocumentTarget(alias(if (parts.size == 2) parts[1] else owner), parsed, alias(parts[0]))
    }
    private fun targetText(value: DocumentTarget) = "${value.kind.name}/${label(value.id)}"
    private fun long(raw: String): Long = raw.toLongOrNull() ?: throw DocumentDomainException()

    private class Run(var document: EnterpriseDocument, var match: String = "?", val refused: MutableList<String> = mutableListOf())

    private fun step(run: Run, owner: String, raw: String) {
        val p = raw.split(',')
        val d = run.document
        run.document = when (p[0]) {
            "T" -> {
                require(p.size == 4 || p.size == 5)
                val dimension = DocumentDimension.parse(p[1])
                val from = DocumentState.parse(dimension, p[2])
                val to = DocumentState.parse(dimension, p[3])
                val at = if (p.size == 5) EnterpriseTime.parseInstant(p[4]) else null
                d.apply(DocumentTransition(dimension, from, to, at))
            }
            "AMOUNT" -> { require(p.size == 2); d.withAmount(long(p[1])) }
            "LINK" -> { require(p.size == 2); d.link(target(p[1], owner)) }
            "ALLOC" -> d.allocate(p.drop(1).map {
                val item = it.split(':').also { parts -> require(parts.size == 2) }
                val value = target(item[0], owner)
                DocumentAllocationLine(value, long(item[1]))
            })
            "SPLIT" -> d.splitEvenly(p.drop(1).map { target(it, owner) })
            "DIGEST" -> { require(p.size == 2); run.match = d.matchesOriginal(digest(p[1])).toString(); d }
            else -> error("Unknown fixture step")
        }
    }

    private fun refusal(block: () -> Unit): DocumentFailure? = try { block(); null }
        catch (failure: DocumentDomainException) { failure.failure }
        catch (_: EnterpriseTimeException) { DocumentFailure.INVALID }

    private fun run(org: String, id: String, digest: String, steps: String): Run {
        val result = Run(EnterpriseDocument.create(alias(org), alias(id), digest(digest)))
        steps.takeIf { it.isNotEmpty() }?.split('~')?.forEachIndexed { index, raw ->
            refusal { step(result, org, raw) }?.let { result.refused += "${index + 1}:${it.name}" }
        }
        return result
    }
    private fun dashed(values: List<String>) = if (values.isEmpty()) "-" else values.joinToString(",")
    private fun links(d: EnterpriseDocument) = dashed(d.links.map(::targetText))
    private fun allocations(d: EnterpriseDocument) = dashed(d.allocations.map { "${targetText(it.target)}:${it.centavos}" })
    private fun project(run: Run): Map<String, String> {
        val d = run.document
        val s = d.statuses
        return mapOf("org" to label(d.organizationId), "id" to label(d.id), "digest" to digestLabel(d.digest),
            "replaces" to (d.replaces?.let(::label) ?: "?"), "replaced_by" to (d.replacedBy?.let(::label) ?: "?"),
            "storage" to s.storage.token, "extraction" to s.extraction.token, "structure" to s.structure.token,
            "conference" to s.conference.token, "reconciliation" to s.reconciliation.token, "fiscal" to s.fiscal.token,
            "consulted_at" to (d.consultedAt?.let(EnterpriseTime::formatInstant) ?: "?"), "amount" to (d.amount?.toString() ?: "?"),
            "allocated" to d.allocated.toString(), "remaining" to (d.remaining?.toString() ?: "?"), "links" to links(d),
            "allocations" to allocations(d), "match" to run.match, "refused" to dashed(run.refused))
    }
    private fun build(org: String, id: String, digest: String, steps: String): EnterpriseDocument =
        run(org, id, digest, steps).also { require(it.refused.isEmpty()) }.document

    private fun execute(operation: String, a: Map<String, String>): Map<String, String> = when (operation) {
        "ENUM" -> {
            val value = a.getValue("value")
            when (val kind = a.getValue("kind")) {
                "DIMENSION" -> DocumentDimension.parse(value).let { mapOf("value" to it.name, "derived" to it.derived.toString()) }
                "TARGET" -> DocumentTargetKind.parse(value).let { mapOf("value" to it.name, "allocatable" to it.allocatable.toString()) }
                else -> {
                    val dimension = DocumentDimension.parse(kind)
                    val state = DocumentState.parse(dimension, value)
                    mapOf("value" to state.token, "initial" to (state == DocumentState.initial(dimension)).toString())
                }
            }
        }
        "ALLOWED" -> {
            val dimension = DocumentDimension.parse(a.getValue("dimension"))
            mapOf("to" to dashed(DocumentState.allowed(dimension, DocumentState.parse(dimension, a.getValue("from"))).map { it.token }))
        }
        "SCENARIO" -> project(run(a["org"] ?: "A", a["id"] ?: "D", a["digest"] ?: "H1", a.getValue("steps")))
        "REPLACE" -> {
            val old = build("A", "D", "H1", a.getValue("old_steps"))
            val successor = build(a["new_org"] ?: "A", a["new"] ?: "N", a["new_digest"] ?: "H2", a.getValue("new_steps"))
            val (o, n) = EnterpriseDocument.replace(old, successor)
            mapOf("old_id" to label(o.id), "old_digest" to digestLabel(o.digest), "old_storage" to o.statuses.storage.token,
                "old_replaced_by" to (o.replacedBy?.let(::label) ?: "?"), "old_conference" to o.statuses.conference.token,
                "old_fiscal" to o.statuses.fiscal.token, "old_reconciliation" to o.statuses.reconciliation.token,
                "old_links" to links(o), "old_allocations" to allocations(o), "new_id" to label(n.id),
                "new_digest" to digestLabel(n.digest), "new_storage" to n.statuses.storage.token,
                "new_replaces" to (n.replaces?.let(::label) ?: "?"), "new_links" to links(n))
        }
        else -> error("Unknown fixture operation")
    }

    @TestFactory fun allSharedDocumentVectors(): List<DynamicTest> {
        val file = File("../contracts/testdata/enterprise/document/document-v1.psv")
        assertEquals("27eaec5e45b641ab7ee31b70681d96690560a12a655f2f43d0f2cc2f55f1e383",
            MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        require(rows.all { it.size == 4 && it[0].isNotBlank() })
        assertEquals(rows.size, rows.map { it[0] }.distinct().size)
        assertEquals(mapOf("ENUM" to 64, "ALLOWED" to 29, "SCENARIO" to 146, "REPLACE" to 12), rows.groupingBy { it[1] }.eachCount())
        for (row in rows) caseShape(row[1], fields(row[2]))
        val seen = rows.flatMap { row ->
            val expected = fields(row[3])
            listOfNotNull(expected["error"]) + (expected["refused"]?.takeIf { it != "-" }?.split(',')?.map { it.substringAfter(':') } ?: emptyList())
        }.toSet()
        assertEquals(DocumentFailure.entries.map { it.name }.toSet(), seen)
        return rows.map { (id, operation, rawArgs, expected) -> DynamicTest.dynamicTest(id) {
            val actual = try { execute(operation, fields(rawArgs)) }
            catch (failure: DocumentDomainException) { mapOf("error" to failure.failure.name) }
            assertEquals(fields(expected), actual)
        } }
    }
}
