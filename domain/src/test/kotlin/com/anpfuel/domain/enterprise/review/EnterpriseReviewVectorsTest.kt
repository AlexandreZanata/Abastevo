package com.anpfuel.domain.enterprise.review

import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.DynamicTest
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.TestFactory

/** Independent consumer of the shared review-v1 oracle; expectations come only from the corpus. */
class EnterpriseReviewVectorsTest {
    private val corpus = File("../contracts/testdata/enterprise/review/review-v1.psv")
    private val corpusSha256 = "fac6c92c9c6929dc59f724b6f9912149d2ec92206138ada14beeec2739dffa08"
    private val operations = mapOf("STATE" to 16, "COMMAND" to 15, "APPLY" to 155, "REPLAY" to 22, "PRESENT" to 24)
    private val allowed = mapOf(
        "STATE" to setOf("value_hex"),
        "COMMAND" to setOf("value_hex"),
        "APPLY" to setOf("steps", "command", "revision", "reason_hex", "reason_repeat", "actor", "author", "beneficiary", "permitted"),
        "REPLAY" to setOf("history"),
        "PRESENT" to setOf("server", "local"),
    )
    private val required = mapOf("STATE" to setOf("value_hex"), "COMMAND" to setOf("value_hex"), "APPLY" to setOf("command"),
        "REPLAY" to setOf("history"), "PRESENT" to setOf("server", "local"))
    private val codes = ReviewFailure.entries.map { it.name }.toSet()
    private val setupReason = "Synthetic reason"
    private val authorSpec = "a:1,m:11,e:21"
    private val reviewerSpec = "a:2,m:12,e:22"

    private class Fixture(message: String) : IllegalStateException(message)

    private fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map {
            val p = it.split('=', limit = 2)
            if (p.size != 2 || p[0].isBlank() || p[0] != p[0].trim()) throw Fixture("malformed field")
            p[0] to p[1]
        }
        if (pairs.map { it.first }.distinct().size != pairs.size) throw Fixture("duplicate field")
        return pairs.toMap()
    }

    private fun decode(raw: String): String {
        if (raw.length % 2 != 0 || !raw.all { it in '0'..'9' || it in 'a'..'f' }) throw Fixture("malformed hex")
        return Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(raw.chunked(2).map { it.toInt(16).toByte() }.toByteArray())).toString()
    }

    private fun hex(raw: String) = raw.toByteArray(Charsets.UTF_8).joinToString("") { "%02x".format(it.toInt() and 255) }

    private fun uuid(n: Long) = "9e000000-0000-4000-8000-" + n.toString().padStart(12, '0')

    private fun decimal(raw: String): Long? = if (raw.isNotEmpty() && raw.length <= 12 && raw.all { it in '0'..'9' }) raw.toLong() else null

    private fun identity(spec: String): ReviewIdentity {
        if (spec.isEmpty()) return ReviewIdentity()
        val accounts = mutableListOf<String>()
        val memberships = mutableListOf<String>()
        val employees = mutableListOf<String>()
        for (part in spec.split(',')) {
            val p = part.split(':', limit = 2)
            if (p.size != 2) throw Fixture("malformed identity")
            val value = decimal(p[1])?.let(::uuid) ?: p[1]
            when (p[0]) {
                "a" -> accounts += value
                "m" -> memberships += value
                "e" -> employees += value
                else -> throw Fixture("unknown identity kind")
            }
        }
        return ReviewIdentity(accounts, memberships, employees)
    }

    private fun defaultFacts(kind: ReviewCommandKind?): ReviewFacts = ReviewFacts(
        actor = identity(if (kind?.role == ReviewRole.AUTHOR) authorSpec else reviewerSpec),
        author = identity(authorSpec),
        permitted = true,
    )

    private fun historyText(entries: List<ReviewEntry>) = entries.joinToString("~") { "${it.kind.name}:${it.revision}:${it.state.name}:${hex(it.reason)}" }

    /** Raw stored tokens: an unknown kind/state is domain input, so it yields the INVALID result. */
    private fun parseHistory(raw: String): List<Triple<String, Long, Pair<String, String>>> {
        if (raw.isEmpty()) return emptyList()
        return raw.split('~').map {
            val p = it.split(':')
            if (p.size != 4) throw Fixture("malformed history")
            Triple(p[0], p[1].toLongOrNull() ?: throw Fixture("malformed revision"), p[2] to decode(p[3]))
        }
    }

    private data class Row(val id: String, val op: String, val args: Map<String, String>, val want: Map<String, String>)

    private fun load(lines: List<String>): List<Row> {
        if (lines.firstOrNull() != "id|operation|args|expected") throw Fixture("header")
        val ids = mutableSetOf<String>()
        return lines.drop(1).map { line ->
            val p = line.split('|')
            if (p.size != 4 || p[0].isBlank() || p[0] != p[0].trim() || !ids.add(p[0])) throw Fixture("row")
            val keys = allowed[p[1]] ?: throw Fixture("operation")
            val args = fields(p[2])
            val want = fields(p[3])
            if (!keys.containsAll(args.keys) || !args.keys.containsAll(required.getValue(p[1]))) throw Fixture("keys")
            want["error"]?.let { if (it !in codes || want.size != 1) throw Fixture("error") }
            Row(p[0], p[1], args, want)
        }
    }

    private val pairs = mutableSetOf<Pair<ReviewState, ReviewCommandKind>>()

    private fun snapshot(c: ReviewCase) = Triple(c.state, c.revision, c.history)

    private fun apply(a: Map<String, String>): Map<String, String> {
        var case = ReviewCase.draft()
        a["steps"]?.takeIf { it.isNotEmpty() }?.split(',')?.forEach { step ->
            val kind = ReviewCommandKind.parse(step)
            val reason = if (kind == ReviewCommandKind.RETURN || kind == ReviewCommandKind.REJECT) setupReason else ""
            case = try { case.apply(ReviewCommand(kind, case.revision, reason), defaultFacts(kind)) }
            catch (failure: ReviewDomainException) { throw Fixture("setup step $step refused: ${failure.failure}") }
        }
        val kind = try { ReviewCommandKind.parse(a.getValue("command")) } catch (_: ReviewDomainException) { null }
        val revision = a["revision"]?.let { it.toLongOrNull() ?: throw Fixture("revision") } ?: case.revision
        val reason = a["reason_hex"]?.let { unit ->
            val repeat = a["reason_repeat"]?.let { r -> decimal(r)?.takeIf { it >= 1 }?.toInt() ?: throw Fixture("repeat") } ?: 1
            decode(unit).repeat(repeat)
        } ?: if ("reason_repeat" in a) throw Fixture("repeat without reason") else ""
        var facts = defaultFacts(kind)
        a["actor"]?.let { facts = facts.copy(actor = identity(it)) }
        a["author"]?.let { facts = facts.copy(author = identity(it)) }
        a["beneficiary"]?.takeUnless { it == "?" }?.let { facts = facts.copy(beneficiary = identity(it)) }
        when (a["permitted"]) {
            null, "true" -> Unit
            "false", "?" -> facts = facts.copy(permitted = false)
            else -> throw Fixture("permission")
        }
        // An unknown command token cannot be represented: it is refused at the parse boundary.
        if (kind == null) throw ReviewDomainException(ReviewFailure.INVALID)
        pairs += case.state to kind
        val before = snapshot(case)
        val next = try { case.apply(ReviewCommand(kind, revision, reason), facts) } catch (failure: ReviewDomainException) {
            assertEquals(before, snapshot(case), "a refused command changed the input case")
            throw failure
        }
        assertEquals(before, snapshot(case), "an accepted command changed the input case")
        val replayed = ReviewCase.replay(next.history)
        assertEquals(snapshot(next), snapshot(replayed), "accepted history does not replay to the same case")
        return mapOf("state" to next.state.name, "revision" to next.revision.toString(), "history" to historyText(next.history))
    }

    private fun execute(op: String, a: Map<String, String>): Map<String, String> = when (op) {
        "STATE" -> ReviewState.parse(decode(a.getValue("value_hex"))).let {
            mapOf("state" to it.name, "terminal" to it.terminal.toString(), "basis" to it.basis.name)
        }
        "COMMAND" -> ReviewCommandKind.parse(decode(a.getValue("value_hex"))).let {
            mapOf("command" to it.name, "role" to it.role.name, "reason" to it.reason.name)
        }
        "APPLY" -> apply(a)
        "REPLAY" -> {
            val entries = parseHistory(a.getValue("history")).map { (kind, revision, rest) ->
                ReviewEntry(ReviewCommandKind.parse(kind), revision, ReviewState.parse(rest.first), rest.second)
            }
            ReviewCase.replay(entries).let { mapOf("state" to it.state.name, "revision" to it.revision.toString(), "entries" to it.history.size.toString()) }
        }
        "PRESENT" -> {
            val server = a.getValue("server").takeUnless { it == "?" }?.let(ReviewState::parse)
            val local = a.getValue("local").takeUnless { it == "?" }?.let(ReviewCommandKind::parse)
            ReviewPresenter.present(server, local).let {
                mapOf("state" to it.state.name, "server_confirmed" to it.serverConfirmed.toString(), "ready_to_send" to it.readyToSend.toString())
            }
        }
        else -> throw Fixture("operation")
    }

    @Test fun `malformed fixture grammar is rejected before domain dispatch`() {
        val header = "id|operation|args|expected"
        listOf(
            listOf("x|STATE|value_hex=|error=INVALID"),
            listOf(header, "x|OTHER|value_hex=|error=INVALID"),
            listOf(header, "x|STATE|value=|error=INVALID"),
            listOf(header, "x|APPLY|steps=SUBMIT|error=INVALID"),
            listOf(header, "x|STATE|value_hex=|error=INVALID", "x|STATE|value_hex=|error=INVALID"),
            listOf(header, "x|STATE|value_hex=;value_hex=|error=INVALID"),
            listOf(header, " |STATE|value_hex=|error=INVALID"),
            listOf(header, "x |STATE|value_hex=|error=INVALID"),
            listOf(header, "x|STATE|value_hex=|error=UNEXPECTED"),
            listOf(header, "x|STATE|value_hex=|error=INVALID;state=DRAFT"),
            listOf(header, "x|STATE|value_hex=|error=INVALID|"),
            listOf(header, "x|STATE|value_hex|error=INVALID"),
        ).forEach { assertThrows(Fixture::class.java) { load(it) } }
        assertThrows(CharacterCodingException::class.java) { decode("ff") }
        assertThrows(CharacterCodingException::class.java) { decode("eda080") }
        listOf("4", "4A").forEach { assertThrows(Fixture::class.java) { decode(it) } }
        listOf("x:1", "a1").forEach { assertThrows(Fixture::class.java) { identity(it) } }
        assertThrows(Fixture::class.java) { identity("a:1,") }
        listOf("SUBMIT:1:SUBMITTED", "SUBMIT:x:SUBMITTED:").forEach { assertThrows(Fixture::class.java) { parseHistory(it) } }
    }

    @TestFactory fun allSharedReviewVectors(): List<DynamicTest> {
        assertEquals(corpusSha256, MessageDigest.getInstance("SHA-256").digest(corpus.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val rows = load(corpus.readLines())
        assertEquals(232, rows.size)
        assertEquals(operations, rows.groupingBy { it.op }.eachCount())
        assertEquals(codes, rows.mapNotNull { it.want["error"] }.toSet(), "every closed error code needs a vector")
        val tests = rows.map { row ->
            DynamicTest.dynamicTest(row.id) {
                val actual = try { execute(row.op, row.args) } catch (failure: ReviewDomainException) { mapOf("error" to failure.failure.name) }
                assertEquals(row.want, actual)
            }
        }
        val coverage = DynamicTest.dynamicTest("every state and command pair is exercised") {
            val missing = ReviewState.entries.flatMap { s -> ReviewCommandKind.entries.map { s to it } }.filterNot { it in pairs }
            assertTrue(missing.isEmpty(), "uncovered pairs: $missing")
        }
        return tests + coverage
    }
}
