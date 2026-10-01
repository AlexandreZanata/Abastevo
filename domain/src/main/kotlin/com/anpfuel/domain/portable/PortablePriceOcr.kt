package com.anpfuel.domain.portable

/**
 * P10-T04 portable price-OCR values (B-BR-010/011/015/016, new Android
 * UC-CAPTURE-01).
 *
 * Pure Kotlin with zero `java.*` imports so this file moves unchanged into
 * a future `commonMain` source set. It only parses OCR text into price
 * candidates in exact milli-BRL ([PortableMoney], 1..1000000); the server
 * always revalidates and the client only hints.
 *
 * Hard rules (frozen for G10-LOCAL):
 * - Candidates carry price + raw + confidence only. Fuel product and
 *   station condition are NEVER inferred from OCR; the contributor picks
 *   them explicitly before any upload (P10-T05 owns the outbox).
 * - No automatic lowest-price choice: callers present every candidate in
 *   text order and wait for human selection. There is no `min()` here.
 * - OCR text is on-device only; nothing is uploaded by this file.
 * - Grammar is tolerant at the edges (`R$`, `RS`, `BRL`, `$` optional;
 *   `.` or `,` decimal separator with 2..3 fraction digits) but exact at
 *   the core: the normalized `int,frac` form must pass [PortableMoney].
 *   Over-precision, over-range, negative and zero values are skipped,
 *   never rounded or clamped.
 * - Confidence is a local hint: 0.90 when the line carries a currency
 *   marker, 0.50 (below [LOW_CONFIDENCE_THRESHOLD]) for a bare number.
 *   [isLowConfidence] forces manual entry; it is never a guarantee.
 * - P21-T02 human-typed entries ([OcrCandidate.manualEntry]) skip the OCR
 *   confidence gate: a typed price was read by the contributor, not the
 *   recognizer. Their confidence value is unused.
 */
object PortablePriceOcr {

    /** Below this the UI must require manual price entry. */
    const val LOW_CONFIDENCE_THRESHOLD: Double = 0.60

    /** Bounded candidate set per capture (never unbounded OCR output). */
    const val MAX_CANDIDATES: Int = 10

    /** One OCR price candidate: price only, never product/condition. */
    data class OcrCandidate(
        val priceMilli: Long,
        val raw: String,
        val confidence: Double,
        val hasCurrencyMarker: Boolean,
        val manualEntry: Boolean = false,
    )

    /** True when [candidate] confidence is below the manual-entry bar. */
    fun isLowConfidence(candidate: OcrCandidate): Boolean =
        !candidate.manualEntry && candidate.confidence < LOW_CONFIDENCE_THRESHOLD

    /**
     * Parses OCR text into at most [MAX_CANDIDATES] candidates in text
     * order. Empty/blank text yields an empty list (manual entry). Invalid
     * fragments are skipped; valid ones keep document order (no sorting,
     * no minimum-picking).
     */
    fun parseCandidates(ocrText: String): List<OcrCandidate> {
        if (ocrText.isBlank()) return emptyList()
        val out = ArrayList<OcrCandidate>(4)
        val lines = splitLines(ocrText)
        for (line in lines) {
            if (out.size >= MAX_CANDIDATES) break
            val marked = lineHasCurrencyMarker(line)
            val raws = scanPriceRaws(line)
            for (raw in raws) {
                if (out.size >= MAX_CANDIDATES) break
                val milli = toMilliOrNull(raw) ?: continue
                val confidence = if (marked) 0.90 else 0.50
                out.add(
                    OcrCandidate(
                        priceMilli = milli,
                        raw = raw,
                        confidence = confidence,
                        hasCurrencyMarker = marked,
                    ),
                )
            }
        }
        return out
    }

    private fun splitLines(text: String): List<String> {
        val lines = ArrayList<String>(8)
        var start = 0
        for (i in text.indices) {
            val c = text[i]
            if (c == '\n' || c == '\r') {
                if (i > start) lines.add(text.substring(start, i))
                start = i + 1
            }
        }
        if (start < text.length) lines.add(text.substring(start))
        return lines
    }

    private fun lineHasCurrencyMarker(line: String): Boolean {
        val lower = line.lowercase()
        if (lower.contains("r$") || lower.contains("brl")) return true
        var i = 0
        while (i < lower.length) {
            val c = lower[i]
            if (c == '$') return true
            if (c == 'r' && i + 1 < lower.length && lower[i + 1] == 's') return true
            i += 1
        }
        return false
    }

    /**
     * Scans one line for `<int><sep><frac>` fragments where int is 1..4
     * digits, sep is `.` or `,` and frac is 2..3 digits. Returns the raw
     * matched slices in order.
     */
    private fun scanPriceRaws(line: String): List<String> {
        val raws = ArrayList<String>(2)
        var i = 0
        while (i < line.length) {
            if (!isDigit(line[i])) {
                i += 1
                continue
            }
            var j = i
            while (j < line.length && isDigit(line[j])) j += 1
            val intLen = j - i
            if (intLen in 1..4 && j < line.length && (line[j] == '.' || line[j] == ',')) {
                var k = j + 1
                while (k < line.length && isDigit(line[k])) k += 1
                val fracLen = k - (j + 1)
                if (fracLen in 2..3) {
                    raws.add(line.substring(i, k))
                    i = k
                    continue
                }
            }
            i = j
        }
        return raws
    }

    private fun toMilliOrNull(raw: String): Long? {
        val sep = raw.indexOf('.').let { dot ->
            if (dot >= 0) dot else raw.indexOf(',')
        }
        if (sep < 0) return null
        val intPart = raw.substring(0, sep)
        val fracPart = raw.substring(sep + 1)
        if (intPart.isEmpty() || intPart.length > 4) return null
        if (fracPart.length !in 2..3) return null
        val normalized = intPart + "," + fracPart
        return try {
            PortableMoney.parse(normalized)
        } catch (e: PortableMoneyException) {
            null
        }
    }

    private fun isDigit(c: Char): Boolean = c in '0'..'9'
}
