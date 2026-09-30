package com.anpfuel.domain.portable

/**
 * Portable Unicode text rules for community comments (P12-T02, B-BR F03).
 *
 * Pure Kotlin with zero `java.*` imports so this file moves unchanged into a
 * future `commonMain` source set. Length is counted in Unicode scalar values
 * after trimming and CRLF normalization: an astral-plane emoji (a UTF-16
 * surrogate pair on every platform) counts one, exactly like Go's
 * range-over-string rune count. Never count UTF-16 units on one platform and
 * code points on another.
 */
object PortableText {

    /** Comments and replies hold at most 280 Unicode scalar values (F03). */
    const val MAX_COMMENT_SCALARS: Int = 280

    /**
     * Trims surrounding whitespace and normalizes CRLF/CR line breaks to LF,
     * mirroring the F03 counting rule.
     */
    fun normalize(text: String): String {
        return text.trim().replace("\r\n", "\n").replace('\r', '\n')
    }

    /**
     * Counts Unicode scalar values. A high surrogate followed by a low
     * surrogate counts one; lone surrogates each count one instead of
     * throwing, so malformed input fails validation, not the counter.
     */
    fun countScalars(text: String): Int {
        var count = 0
        var i = 0
        while (i < text.length) {
            val c = text[i]
            if (isHighSurrogate(c) && i + 1 < text.length && isLowSurrogate(text[i + 1])) {
                i += 2
            } else {
                i += 1
            }
            count += 1
        }
        return count
    }

    /** Nonempty after normalization and at most [MAX_COMMENT_SCALARS] scalars. */
    fun isValidComment(text: String): Boolean {
        val normalized = normalize(text)
        if (normalized.isEmpty()) return false
        return countScalars(normalized) <= MAX_COMMENT_SCALARS
    }

    private fun isHighSurrogate(c: Char): Boolean = c in '\uD800'..'\uDBFF'

    private fun isLowSurrogate(c: Char): Boolean = c in '\uDC00'..'\uDFFF'
}
