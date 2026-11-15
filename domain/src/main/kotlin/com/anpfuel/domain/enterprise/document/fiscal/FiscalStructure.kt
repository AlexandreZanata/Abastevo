package com.anpfuel.domain.enterprise.document.fiscal

import com.anpfuel.domain.enterprise.EnterpriseValueException
import com.anpfuel.domain.enterprise.EnterpriseValues
import com.anpfuel.domain.enterprise.document.DocumentDimension
import com.anpfuel.domain.enterprise.document.DocumentState
import com.anpfuel.domain.enterprise.document.DocumentTransition
import java.util.Collections

/** The single answer of a check. UNSUPPORTED means "not a handled layout", never "invalid". */
enum class FiscalOutcome { STRUCTURE_OK, STRUCTURE_INVALID, UNSUPPORTED }

/** What was checked. The XML envelope is checked by the server only (EDR-159). */
enum class FiscalInputKind { ACCESS_KEY, QR_CODE, COMPARISON }

/** Named structural checks, reported in the order they ran. */
enum class FiscalCheck { KEY_FORMAT, KEY_CHECK_DIGIT, KEY_FIELDS, QR_SIZE, QR_URL, QR_PARAMETERS, QR_VERSION, QR_FIELDS, QR_MODEL, KEY_MATCH }

/** Closed causes of a non-OK outcome; each has one fixed outcome. */
enum class FiscalReason(val outcome: FiscalOutcome) {
    KEY_LENGTH(FiscalOutcome.STRUCTURE_INVALID), KEY_CHARACTER(FiscalOutcome.STRUCTURE_INVALID),
    KEY_CHECK_DIGIT(FiscalOutcome.STRUCTURE_INVALID), KEY_STATE(FiscalOutcome.STRUCTURE_INVALID),
    KEY_MONTH(FiscalOutcome.STRUCTURE_INVALID), KEY_MODEL_UNSUPPORTED(FiscalOutcome.UNSUPPORTED),
    QR_TOO_LARGE(FiscalOutcome.STRUCTURE_INVALID), QR_NOT_URL(FiscalOutcome.UNSUPPORTED),
    QR_HOST_UNSUPPORTED(FiscalOutcome.UNSUPPORTED), QR_MALFORMED(FiscalOutcome.STRUCTURE_INVALID),
    QR_FORMAT_UNSUPPORTED(FiscalOutcome.UNSUPPORTED), QR_UNEXPECTED_PARAMETER(FiscalOutcome.STRUCTURE_INVALID),
    QR_VERSION_UNSUPPORTED(FiscalOutcome.UNSUPPORTED), QR_VARIANT_UNSUPPORTED(FiscalOutcome.UNSUPPORTED),
    QR_FIELDS(FiscalOutcome.STRUCTURE_INVALID), QR_MODEL_UNSUPPORTED(FiscalOutcome.UNSUPPORTED),
    KEY_MISMATCH(FiscalOutcome.STRUCTURE_INVALID),
}

enum class FiscalEnvironment { PRODUCTION, HOMOLOGATION }
enum class FiscalLayout { ONLINE, OFFLINE }

/** A comparison operand is not a STRUCTURE_OK result carrying an access key. */
class FiscalComparisonException : IllegalArgumentException("fiscal: not comparable")

private const val REDACTED = "EnterpriseFiscalValue(redacted)"

/** Exact decimal total as written; [centavos] is absent above the accepted bound, never rounded. */
class FiscalAmount internal constructor(val text: String, val centavos: Long?) {
    override fun toString(): String = REDACTED
}

/** A structurally checked 44-character access key; well-formed is not authorized (BR-TRUTH-002). */
class FiscalAccessKey internal constructor(val text: String) {
    val state: String get() = text.substring(0, 2)
    val yearMonth: String get() = text.substring(2, 6)
    val issuer: String get() = text.substring(6, 20)
    val model: String get() = text.substring(20, 22)
    val series: String get() = text.substring(22, 25)
    val number: String get() = text.substring(25, 34)
    val emissionType: String get() = text.substring(34, 35)
    val numericCode: String get() = text.substring(35, 43)
    val checkDigit: String get() = text.substring(43, 44)
    override fun toString(): String = REDACTED
}

/** A structurally checked NFC-e QR payload; its hash and digest are never verified, its URL never opened. */
class FiscalQrReference internal constructor(
    val accessKey: FiscalAccessKey,
    val version: String,
    val environment: FiscalEnvironment,
    val layout: FiscalLayout,
    val total: FiscalAmount?,
) {
    override fun toString(): String = REDACTED
}

/** One immutable structure answer without any authorization, cancellation or consultation field. */
class FiscalStructureResult internal constructor(
    val kind: FiscalInputKind,
    val outcome: FiscalOutcome,
    val reason: FiscalReason?,
    checks: List<FiscalCheck>,
    val accessKey: FiscalAccessKey?,
    val qrCode: FiscalQrReference?,
) {
    /** Checks that ran, in order; the last decides a non-OK outcome. */
    val checks: List<FiscalCheck> = Collections.unmodifiableList(ArrayList(checks))

    /** The only document command a result can feed: Structure from UNCHECKED; UNSUPPORTED commands nothing. */
    fun structureTransition(): DocumentTransition? = when (outcome) {
        FiscalOutcome.STRUCTURE_OK -> DocumentTransition(DocumentDimension.STRUCTURE, DocumentState.UNCHECKED, DocumentState.STRUCTURE_OK)
        FiscalOutcome.STRUCTURE_INVALID -> DocumentTransition(DocumentDimension.STRUCTURE, DocumentState.UNCHECKED, DocumentState.STRUCTURE_INVALID)
        FiscalOutcome.UNSUPPORTED -> null
    }

    override fun toString(): String = REDACTED
}

/** Accumulates the checks that ran for one result. */
private class Checker(private val kind: FiscalInputKind) {
    val checks = mutableListOf<FiscalCheck>()
    fun run(check: FiscalCheck) { checks += check }
    fun fail(reason: FiscalReason) = FiscalStructureResult(kind, reason.outcome, reason, checks, null, null)
    fun pass(key: FiscalAccessKey, qr: FiscalQrReference? = null) = FiscalStructureResult(kind, FiscalOutcome.STRUCTURE_OK, null, checks, key, qr)
}

/** A failed check inside a step; carries the closed reason to the result. */
private class Refusal(val reason: FiscalReason) : Exception(null, null, false, false)

/** Pure, offline access-key, NFC-e QR and comparison checks mirrored from the Go server (EMP-DOC-006). */
object FiscalStructure {
    const val KEY_LENGTH = 44
    const val MAX_QR_BYTES = 2953

    /** The 27 IBGE state codes that open an access key. */
    private val ufCodes = setOf("11", "12", "13", "14", "15", "16", "17", "21", "22", "23", "24", "25", "26", "27", "28", "29",
        "31", "32", "33", "35", "41", "42", "43", "50", "51", "52", "53")

    private fun digits(s: String) = s.isNotEmpty() && s.all { it in '0'..'9' }
    private fun hexValue(c: Char): Int = when (c) {
        in '0'..'9' -> c - '0'
        in 'a'..'f' -> c - 'a' + 10
        in 'A'..'F' -> c - 'A' + 10
        else -> -1
    }
    private fun hexDigits(s: String, minimum: Int, maximum: Int) = s.length in minimum..maximum && s.all { hexValue(it) >= 0 }
    private fun asciiLower(s: String) = buildString(s.length) { for (c in s) append(if (c in 'A'..'Z') c + ('a' - 'A') else c) }

    /** Modulo 11 of the first 43 characters valued ASCII − 48 (NT 2025.001), weights 2..9 from the right. */
    private fun keyDigit(body: String): Char {
        var sum = 0
        var weight = 2
        for (i in body.indices.reversed()) {
            sum += (body[i].code - 48) * weight
            weight = if (weight == 9) 2 else weight + 1
        }
        val r = sum % 11
        return if (r > 1) '0' + (11 - r) else '0'
    }

    /** KEY_FORMAT, KEY_CHECK_DIGIT and KEY_FIELDS in order (B-BR-EMP-232); nothing is trimmed or folded. */
    private fun accessKey(c: Checker, text: String): FiscalAccessKey {
        c.run(FiscalCheck.KEY_FORMAT)
        if (text.any { it !in '0'..'9' && it !in 'A'..'Z' }) throw Refusal(FiscalReason.KEY_CHARACTER)
        if (text.length != KEY_LENGTH) throw Refusal(FiscalReason.KEY_LENGTH)
        if (!digits(text.substring(0, 6)) || !digits(text.substring(18))) throw Refusal(FiscalReason.KEY_CHARACTER)
        c.run(FiscalCheck.KEY_CHECK_DIGIT)
        if (keyDigit(text.substring(0, 43)) != text[43]) throw Refusal(FiscalReason.KEY_CHECK_DIGIT)
        c.run(FiscalCheck.KEY_FIELDS)
        if (text.substring(0, 2) !in ufCodes) throw Refusal(FiscalReason.KEY_STATE)
        if (text.substring(4, 6) !in "01".."12") throw Refusal(FiscalReason.KEY_MONTH)
        val model = text.substring(20, 22)
        if (model != "55" && model != "65") throw Refusal(FiscalReason.KEY_MODEL_UNSUPPORTED)
        return FiscalAccessKey(text)
    }

    private fun checked(c: Checker, block: () -> FiscalStructureResult): FiscalStructureResult =
        try { block() } catch (refusal: Refusal) { c.fail(refusal.reason) }

    fun checkAccessKey(text: String): FiscalStructureResult {
        val c = Checker(FiscalInputKind.ACCESS_KEY)
        return checked(c) { c.pass(accessKey(c, text)) }
    }

    /** NFC-e QR payload (B-BR-EMP-233): size, URL, single p, version, fields, key, model; nothing is fetched. */
    fun checkQrCode(payload: String): FiscalStructureResult {
        val c = Checker(FiscalInputKind.QR_CODE)
        return checked(c) {
            c.run(FiscalCheck.QR_SIZE)
            // Every char is at least one UTF-8 byte, so a long payload is refused before encoding it.
            if (payload.length > MAX_QR_BYTES || payload.toByteArray(Charsets.UTF_8).size > MAX_QR_BYTES) throw Refusal(FiscalReason.QR_TOO_LARGE)
            c.run(FiscalCheck.QR_URL)
            val rest = cutScheme(payload) ?: throw Refusal(FiscalReason.QR_NOT_URL)
            val end = rest.indexOfFirst { it == '/' || it == '?' }.let { if (it < 0) rest.length else it }
            if (!governmentHost(rest.substring(0, end))) throw Refusal(FiscalReason.QR_HOST_UNSUPPORTED)
            if (!visibleUrl(payload)) throw Refusal(FiscalReason.QR_MALFORMED)
            c.run(FiscalCheck.QR_PARAMETERS)
            val tail = rest.substring(end)
            val query = tail.indexOf('?').let { if (it < 0) "" else tail.substring(it + 1) }
            val parts = query.split('&')
            val values = parts.filter { it.substringBefore('=') == "p" }.map { it.substringAfter('=', "") }
            if (values.isEmpty()) throw Refusal(FiscalReason.QR_FORMAT_UNSUPPORTED)
            if (parts.size != 1) throw Refusal(FiscalReason.QR_UNEXPECTED_PARAMETER)
            c.run(FiscalCheck.QR_VERSION)
            val f = percentDecode(values.single()).split('|')
            if (f.size < 2 || (f[1] != "2" && f[1] != "3")) throw Refusal(FiscalReason.QR_VERSION_UNSUPPORTED)
            if (f[1] == "3" && f.size != 3) throw Refusal(FiscalReason.QR_VARIANT_UNSUPPORTED)
            c.run(FiscalCheck.QR_FIELDS)
            val layout = qrFields(f) ?: throw Refusal(FiscalReason.QR_FIELDS)
            val key = accessKey(c, f[0])
            c.run(FiscalCheck.QR_MODEL)
            if (key.model != "65") throw Refusal(FiscalReason.QR_MODEL_UNSUPPORTED)
            c.pass(key, FiscalQrReference(key, f[1], layout.first, layout.second, layout.third))
        }
    }

    /** Version 2 online (5 fields) or offline (8 fields) and version 3 online (3 fields). */
    private fun qrFields(f: List<String>): Triple<FiscalEnvironment, FiscalLayout, FiscalAmount?>? {
        if (f[1] == "2" && f.size != 5 && f.size != 8) return null
        val environment = when (f[2]) {
            "1" -> FiscalEnvironment.PRODUCTION
            "2" -> FiscalEnvironment.HOMOLOGATION
            else -> return null
        }
        return when (f.size) {
            5 -> if (csc(f[3]) && hexDigits(f[4], 40, 40)) Triple(environment, FiscalLayout.ONLINE, null) else null
            8 -> {
                val total = parseAmount(f[4])
                val day = f[3].length == 2 && digits(f[3]) && f[3] in "01".."31"
                if (total != null && day && hexDigits(f[5], 1, 128) && csc(f[6]) && hexDigits(f[7], 40, 40)) {
                    Triple(environment, FiscalLayout.OFFLINE, total)
                } else null
            }
            else -> Triple(environment, FiscalLayout.ONLINE, null)
        }
    }

    private fun csc(s: String) = s.length <= 6 && digits(s)

    private fun cutScheme(s: String): String? = listOf("https://", "http://")
        .firstOrNull { s.length >= it.length && asciiLower(s.substring(0, it.length)) == it }?.let { s.substring(it.length) }

    /** A plain DNS host name under .gov.br with a nonempty label before it; no user info, port, IP, empty label or trailing dot. */
    private fun governmentHost(raw: String): Boolean {
        val host = asciiLower(raw)
        if (host.length > 253 || !host.endsWith(".gov.br")) return false
        return host.split('.').all { label ->
            label.isNotEmpty() && label.length <= 63 && label.first() != '-' && label.last() != '-' &&
                label.all { it in 'a'..'z' || it in '0'..'9' || it == '-' }
        }
    }

    /** Visible ASCII, no fragment, and percent escapes of two hex digits decoding to visible ASCII. */
    private fun visibleUrl(s: String): Boolean {
        var i = 0
        while (i < s.length) {
            val c = s[i]
            if (c.code < 0x21 || c.code > 0x7e || c == '#') return false
            if (c == '%') {
                if (i + 2 >= s.length) return false
                val hi = hexValue(s[i + 1])
                val lo = hexValue(s[i + 2])
                if (hi < 0 || lo < 0 || hi * 16 + lo !in 0x21..0x7e) return false
                i += 2
            }
            i++
        }
        return true
    }

    /** Decodes escapes already proven well formed by [visibleUrl]. */
    private fun percentDecode(s: String) = buildString {
        var i = 0
        while (i < s.length) {
            if (s[i] == '%') { append((hexValue(s[i + 1]) * 16 + hexValue(s[i + 2])).toChar()); i += 3 } else { append(s[i]); i++ }
        }
    }

    /** Exactly the NF-e TDec_1302 text; centavos only through the exact parser within its bound, never rounded. */
    private fun parseAmount(text: String): FiscalAmount? {
        val decimal = text.contains('.')
        val whole = text.substringBefore('.')
        val fraction = if (decimal) text.substringAfter('.') else "00"
        if (decimal && (fraction.length != 2 || !digits(fraction))) return null
        if (!digits(whole) || whole.length > 13 || (whole[0] == '0' && whole.length > 1)) return null
        val cents = (whole + fraction).trimStart('0').ifEmpty { "0" }
        val centavos = try { EnterpriseValues.parseCentavos(cents) } catch (_: EnterpriseValueException) { null }
        return FiscalAmount(text, centavos)
    }

    /** Two STRUCTURE_OK results carrying keys: equal keys are STRUCTURE_OK, else KEY_MISMATCH; nothing fiscal is implied. */
    fun compare(a: FiscalStructureResult, b: FiscalStructureResult): FiscalStructureResult {
        val left = a.accessKey
        val right = b.accessKey
        if (a.outcome != FiscalOutcome.STRUCTURE_OK || b.outcome != FiscalOutcome.STRUCTURE_OK || left == null || right == null) {
            throw FiscalComparisonException()
        }
        val c = Checker(FiscalInputKind.COMPARISON)
        c.run(FiscalCheck.KEY_MATCH)
        return if (left.text == right.text) c.pass(left) else c.fail(FiscalReason.KEY_MISMATCH)
    }
}
