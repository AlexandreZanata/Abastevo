package com.anpfuel.domain.enterprise.maintenance.checklist

import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.TimeDeclaration
import com.anpfuel.domain.enterprise.sync.EnterpriseOperation
import com.anpfuel.domain.enterprise.sync.SyncBinding
import com.anpfuel.domain.enterprise.sync.SyncProtocolException
import java.io.File
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertArrayEquals
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Assertions.fail
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/**
 * The Kotlin consumer of run-v1 (EMP-CHECK-002, B-BR-EMP-439, EDR-283/286); the Go `ParseRun` consumes the same rows.
 * The builder produces every OK row byte for byte and refuses every row it can express with the row's class; the
 * strict decoder accepts exactly the OK rows and refuses every other envelope with its class; PROTOCOL rows are refused
 * by the shared envelope profile itself.
 */
class EnterpriseChecklistRunVectorsTest {
    private val hash = "fbbea99443fd5aadf49f7d55bf132b34208d2ee7e545188a078af56dff14a24e"
    private val binding = SyncBinding.of("7b0e2a54-0000-4000-8000-000000009501", "https://api.example.invalid", "7b0e2a54-0000-4000-8000-000000009502")
    private class Row(val id: String, val args: Map<String, String>, val outcome: String, val bytes: ByteArray)

    private fun hex(raw: String): ByteArray {
        require(raw.length % 2 == 0)
        return ByteArray(raw.length / 2) { raw.substring(it * 2, it * 2 + 2).toInt(16).toByte() }
    }

    private fun rows(): List<Row> {
        val file = File("../contracts/testdata/enterprise/checklist/run-v1.psv")
        assertEquals(hash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected|bytes_hex", lines.first())
        return lines.drop(1).map { line ->
            val cols = line.split('|')
            require(cols.size == 5 && cols[0].isNotEmpty() && cols[1] == "RUN")
            val pairs = cols[2].split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotEmpty()); p[0] to p[1] }
            require(pairs.map { it.first }.distinct().size == pairs.size)
            Row(cols[0], pairs.toMap(), cols[3], hex(cols[4]))
        }
    }

    /** `ANSWER[:hexnote][+photo]`; no `:` is a null note and `:` with nothing is the empty note. */
    private fun answers(row: Row): List<ChecklistRunAnswerInput> {
        val raw = row.args.getValue("answers")
        if (raw.isEmpty()) return emptyList()
        return raw.split(',').map { item ->
            val photo = item.endsWith("+photo")
            val body = item.removeSuffix("+photo")
            val parts = body.split(':', limit = 2)
            ChecklistRunAnswerInput(parts[0], if (parts.size == 2) String(hex(parts[1]), Charsets.UTF_8) else null, photo)
        }
    }

    /** Only rows a typed builder can express: no payload shape, dependency, expected revision or entity override, integer counters. */
    private fun expressible(row: Row): Boolean = listOf("shape", "dep", "revision", "entity").none { it in row.args } &&
        row.args.getValue("version").toLongOrNull() != null && row.args.getValue("policy").toLongOrNull() != null

    private fun envelope(row: Row) = ChecklistRunEnvelope(binding, "7b0e2a54-0000-4000-8000-000000009503", "7b0e2a54-0000-4000-8000-000000009504",
        row.args.getValue("op"), TimeDeclaration(EnterpriseTime.parseInstant(row.args["at"] ?: "2026-11-15T12:00:00.123Z"),
            row.args["zone"] ?: "America/Cuiaba", (row.args["offset"] ?: "-14400").toLong()))

    private fun build(row: Row): EnterpriseOperation = ChecklistRunOperations.complete(envelope(row), row.args.getValue("run"),
        row.args.getValue("asset"), row.args.getValue("context"), row.args.getValue("template"), row.args.getValue("version").toLong(),
        row.args.getValue("policy").toLong(), row.args.getValue("started"), answers(row), row.args.getValue("device"), row.args.getValue("ack") == "true")

    private fun accepted(row: Row) {
        assertTrue(expressible(row), "every OK row is producible by the builder")
        assertArrayEquals(row.bytes, build(row).canonicalBytes, "builder bytes")
        val fact = ChecklistRunOperations.read(EnterpriseOperation.parse(row.bytes, binding))
        assertEquals(listOf(row.args["op"], row.args["run"], row.args["asset"], row.args["context"], row.args["template"]),
            listOf(fact.operationId, fact.runId, fact.assetId, fact.context.name, fact.templateId))
        assertEquals(row.args.getValue("version").toLong(), fact.templateVersion)
        assertEquals(row.args.getValue("policy").toLong(), fact.policyRevision)
        assertEquals(EnterpriseTime.parseInstant(row.args.getValue("started")), fact.startedAt)
        assertEquals(row.args["device"], fact.deviceOutcome.name)
        assertEquals(row.args["ack"] == "true", fact.acknowledged)
        assertEquals(answers(row).map { ChecklistRunAnswer(it.answer, it.note) }, fact.answers)
        assertFalse(fact.startedAt.isAfter(fact.declared.occurredAt))
    }

    private fun refused(row: Row) {
        val built = if (!expressible(row)) null else try { build(row); "BUILT" } catch (refusal: ChecklistRunOperationException) { refusal.failure.name }
        if (row.outcome == "PROTOCOL") {
            if (built != null) assertTrue(built != "BUILT", "the builder accepted an envelope-profile refusal ${row.id}")
            try { EnterpriseOperation.seal(row.bytes, binding); fail("the envelope profile accepted ${row.id}") } catch (_: SyncProtocolException) { }
            return
        }
        if (built != null) assertEquals(row.outcome, built, "builder refusal class of ${row.id}")
        val sealed = EnterpriseOperation.seal(row.bytes, binding)
        val decoded = try { ChecklistRunOperations.read(sealed); "DECODED" } catch (refusal: ChecklistRunOperationException) { refusal.failure.name }
        assertEquals(row.outcome, decoded, "decoder refusal class of ${row.id}")
    }

    @Test fun `the corpus is complete and every class, context, outcome and answer is covered`() {
        val all = rows()
        assertEquals(56, all.size)
        assertEquals(all.size, all.map { it.id }.distinct().size)
        assertEquals(mapOf("OK" to 15, "MALFORMED" to 37, "UNSUPPORTED" to 2, "PROTOCOL" to 2), all.groupingBy { it.outcome }.eachCount())
        val ok = all.filter { it.outcome == "OK" }
        assertEquals(ChecklistContext.entries.map { it.name }.toSet(), ok.map { it.args.getValue("context") }.toSet())
        assertEquals(ChecklistOutcome.entries.map { it.name }.toSet(), ok.map { it.args.getValue("device") }.toSet())
        assertEquals(setOf(ChecklistAnswer.OK, ChecklistAnswer.NOT_OK, ChecklistAnswer.NOT_APPLICABLE), ok.flatMap { answers(it) }.map { it.answer }.toSet())
        assertTrue(ok.flatMap { answers(it) }.none { it.photo }, "the builder never produces a photo claim")
        // Most refusals are expressible by the builder; the rest are envelope shapes only the decoder sees.
        assertEquals(26, all.count { it.outcome != "OK" && expressible(it) })
        assertEquals(15, all.count { it.outcome != "OK" && !expressible(it) })
        assertEquals(ChecklistRunOperations.COMPLETE, "check.run.complete")
        assertEquals(ChecklistRunOperations.ENTITY, "checklist_run")
    }

    @TestFactory fun vectors(): List<DynamicTest> = rows().map { row ->
        DynamicTest.dynamicTest(row.id) { if (row.outcome == "OK") accepted(row) else refused(row) }
    }

    @Test fun `a photo claim is never produced and a photo on an answer other than NOT_OK is malformed`() {
        val base = rows().first { it.id == "run-blocking-critical-with-note" }
        fun attempt(photoOn: Int, answer: String): ChecklistRunOperationFailure? = try {
            val inputs = answers(base).mapIndexed { i, a -> if (i == photoOn) ChecklistRunAnswerInput(answer, a.note, true) else a }
            ChecklistRunOperations.complete(envelope(base), base.args.getValue("run"), base.args.getValue("asset"), "DEPARTURE",
                base.args.getValue("template"), 1, 1, base.args.getValue("started"), inputs, "BLOCKING", false); null
        } catch (refusal: ChecklistRunOperationException) { refusal.failure }
        assertEquals(ChecklistRunOperationFailure.UNSUPPORTED, attempt(1, ChecklistAnswer.NOT_OK))
        assertEquals(ChecklistRunOperationFailure.MALFORMED, attempt(0, ChecklistAnswer.OK))
        assertEquals(ChecklistRunOperationFailure.MALFORMED, attempt(0, ChecklistAnswer.NOT_APPLICABLE))
    }
}
