package com.anpfuel.domain.enterprise.document.fiscal

import com.anpfuel.domain.enterprise.document.DocumentDimension
import com.anpfuel.domain.enterprise.document.DocumentDomainException
import com.anpfuel.domain.enterprise.document.DocumentState
import com.anpfuel.domain.enterprise.document.DocumentTransition
import com.anpfuel.domain.enterprise.document.EnterpriseDocument
import java.time.Instant
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class FiscalStructureTest {
    private val issuer = "SYNTHE00000140"
    private val key = "432611SYNTHE00000140650010000001231100000010"
    private val qr = "https://www.sefaz.rs.gov.br/NFCE/NFCE-COM.aspx?p=$key|3|1"
    private val offline = "http://www.fazenda.pr.gov.br/nfce/qrcode?p=$key|2|1|14|57.30|AB|1|${"A".repeat(40)}"

    /** Independent modulo-11 helper (values ASCII − 48, weights 2..9 from the right). */
    private fun synthKey(body: String): String {
        var sum = 0
        var weight = 2
        for (c in body.reversed()) { sum += (c.code - 48) * weight; weight = if (weight == 9) 2 else weight + 1 }
        val r = sum % 11
        return body + (if (r < 2) 0 else 11 - r)
    }

    private fun samples(): Map<String, FiscalStructureResult> = mapOf(
        "key-ok" to FiscalStructure.checkAccessKey(key),
        "key-invalid" to FiscalStructure.checkAccessKey(key.dropLast(1) + "9"),
        "key-unsupported" to FiscalStructure.checkAccessKey(synthKey("432611${issuer}57001000000001140000004")),
        "qr-ok" to FiscalStructure.checkQrCode(qr), "qr-offline" to FiscalStructure.checkQrCode(offline),
        "qr-invalid" to FiscalStructure.checkQrCode("$qr#x"),
        "qr-unsupported" to FiscalStructure.checkQrCode("https://nfce.example.com/?p=$key|3|1"),
        "compare-ok" to FiscalStructure.compare(FiscalStructure.checkAccessKey(key), FiscalStructure.checkQrCode(qr)),
    )

    private fun unchecked() = EnterpriseDocument.create("9d000000-0000-4000-8000-000000000001", "9d000000-0000-4000-8000-000000000201", "1a".repeat(32))
        .apply(DocumentTransition(DocumentDimension.STRUCTURE, DocumentState.NOT_APPLICABLE, DocumentState.UNCHECKED))

    @Test fun `samples have the expected outcomes`() {
        val want = mapOf("key-ok" to FiscalOutcome.STRUCTURE_OK, "key-invalid" to FiscalOutcome.STRUCTURE_INVALID,
            "key-unsupported" to FiscalOutcome.UNSUPPORTED, "qr-ok" to FiscalOutcome.STRUCTURE_OK, "qr-offline" to FiscalOutcome.STRUCTURE_OK,
            "qr-invalid" to FiscalOutcome.STRUCTURE_INVALID, "qr-unsupported" to FiscalOutcome.UNSUPPORTED, "compare-ok" to FiscalOutcome.STRUCTURE_OK)
        for ((name, r) in samples()) assertEquals(want[name], r.outcome, name)
    }

    @Test fun `results expose no authorization surface`() {
        val forbidden = listOf("authoriz", "cancel", "consult", "fiscal", "valid", "protocol", "signature", "cstat", "status")
        for (type in listOf(FiscalStructureResult::class.java, FiscalAccessKey::class.java, FiscalQrReference::class.java, FiscalAmount::class.java)) {
            val names = type.declaredMethods.map { it.name } + type.declaredFields.map { it.name }
            for (name in names) for (word in forbidden) assertFalse(name.lowercase().contains(word), "${type.simpleName} exposes $name")
        }
    }

    @Test fun `a result commands only the structure dimension and never fiscal consultation`() {
        for ((name, r) in samples()) {
            val command = r.structureTransition()
            assertEquals(r.outcome != FiscalOutcome.UNSUPPORTED, command != null, name)
            if (command == null) continue
            val target = if (r.outcome == FiscalOutcome.STRUCTURE_OK) DocumentState.STRUCTURE_OK else DocumentState.STRUCTURE_INVALID
            assertEquals(DocumentDimension.STRUCTURE, command.dimension); assertEquals(DocumentState.UNCHECKED, command.from)
            assertEquals(target, command.to); assertNull(command.consultedAt)
            val after = unchecked().apply(command)
            assertEquals(target, after.statuses.structure); assertEquals(DocumentState.NOT_CONSULTED, after.statuses.fiscal)
            assertNull(after.consultedAt)
            // Re-aimed by a caller, the command still has no consultation instant: the document refuses it.
            for (to in listOf(DocumentState.AUTHORIZED_AS_OF, DocumentState.CANCELLED_AS_OF)) {
                assertThrows(DocumentDomainException::class.java) {
                    after.apply(DocumentTransition(DocumentDimension.FISCAL, DocumentState.NOT_CONSULTED, to, command.consultedAt))
                }
            }
        }
        // Only a real consultation instant reaches fiscal status, through the document domain itself.
        val consulted = unchecked().apply(DocumentTransition(DocumentDimension.FISCAL, DocumentState.NOT_CONSULTED,
            DocumentState.AUTHORIZED_AS_OF, Instant.parse("2026-11-14T12:00:00Z")))
        assertEquals(DocumentState.UNCHECKED, consulted.statuses.structure)
    }

    @Test fun `values are redacted`() {
        val r = FiscalStructure.checkQrCode(offline)
        val q = requireNotNull(r.qrCode)
        for (value in listOf<Any>(r, requireNotNull(r.accessKey), q, requireNotNull(q.total))) {
            val text = value.toString() + "$value"
            for (secret in listOf(key, issuer, "57.30", "5730", "2611")) assertFalse(text.contains(secret), "${value::class.simpleName} leaks")
        }
        assertEquals("5730", q.total?.centavos?.toString())
    }

    /** Remainders 0 and 1 both give digit 0, so only a digit-0 key can collide with a single substitution. */
    @Test fun `check digit refuses substitutions except digit-zero collisions`() {
        var collisions = 0
        for (body in listOf("432611${issuer}65001000000123110000001", "412611${issuer}65002000004567920000002",
            "350001" + "0".repeat(14) + "55999999999999999999999")) {
            val valid = synthKey(body)
            assertEquals(FiscalOutcome.STRUCTURE_OK, FiscalStructure.checkAccessKey(valid).outcome)
            for (i in valid.indices) {
                if (valid[i] !in '0'..'9') continue
                for (d in '0'..'9') {
                    if (d == valid[i]) continue
                    val mutated = valid.substring(0, i) + d + valid.substring(i + 1)
                    val reason = FiscalStructure.checkAccessKey(mutated).reason
                    if (i < 43 && synthKey(mutated.substring(0, 43)) == mutated) {
                        assertEquals('0', valid[43], "collision with a nonzero digit at $i:$d")
                        assertNotEquals(FiscalReason.KEY_CHECK_DIGIT, reason, "$i:$d")
                        collisions++
                    } else {
                        assertEquals(FiscalReason.KEY_CHECK_DIGIT, reason, "$i:$d")
                    }
                }
            }
        }
        assertTrue(collisions > 0, "expected the documented digit-zero collisions")
    }

    @Test fun `results are immutable and comparison requires structure-ok keys`() {
        val r = FiscalStructure.checkAccessKey(key)
        @Suppress("UNCHECKED_CAST")
        assertThrows(UnsupportedOperationException::class.java) { (r.checks as MutableList<FiscalCheck>).add(FiscalCheck.KEY_MATCH) }
        val bad = FiscalStructure.checkAccessKey("")
        assertThrows(FiscalComparisonException::class.java) { FiscalStructure.compare(bad, r) }
        assertThrows(FiscalComparisonException::class.java) { FiscalStructure.compare(r, FiscalStructure.checkQrCode("")) }
        val same = FiscalStructure.compare(r, FiscalStructure.checkQrCode(qr))
        assertEquals(FiscalInputKind.COMPARISON, same.kind); assertNull(same.qrCode); assertEquals(key, same.accessKey?.text)
        assertEquals(FiscalOutcome.STRUCTURE_OK, FiscalStructure.compare(same, r).outcome)
        assertEquals(44, FiscalStructure.KEY_LENGTH); assertEquals(2953, FiscalStructure.MAX_QR_BYTES)
    }
}
