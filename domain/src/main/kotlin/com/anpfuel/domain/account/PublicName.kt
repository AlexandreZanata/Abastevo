package com.anpfuel.domain.account

import java.text.Normalizer
import java.util.Locale

/**
 * Client copy of the account API public-name rule. It lets the app refuse a
 * name before an email code is requested. The server remains authoritative;
 * both sides run contracts/testdata/account/public-names-v1.tsv.
 */
object PublicName {
    const val MIN_LENGTH = 2
    const val MAX_LENGTH = 40
    private const val MIN_LETTERS = 2
    private const val MAX_DIGITS = 4

    private val reservedWords = setOf(
        "abastevo", "admin", "administrador", "moderador", "moderacao", "moderação", "suporte", "anp", "oficial",
    )

    /** NFC with surrounding whitespace removed and inner runs collapsed to one space. */
    fun normalize(name: String): String {
        val words = mutableListOf<String>()
        val word = StringBuilder()
        name.codePoints().forEach { codePoint ->
            if (isSpace(codePoint)) {
                if (word.isNotEmpty()) words += word.toString()
                word.setLength(0)
            } else {
                word.appendCodePoint(codePoint)
            }
        }
        if (word.isNotEmpty()) words += word.toString()
        return Normalizer.normalize(words.joinToString(" "), Normalizer.Form.NFC)
    }

    fun isValid(name: String): Boolean {
        if (name.codePoints().anyMatch { Character.isISOControl(it) || Character.getType(it) == FORMAT }) {
            return false
        }
        val normalized = normalize(name)
        if (normalized.codePointCount(0, normalized.length) !in MIN_LENGTH..MAX_LENGTH) return false
        if (containsReservedWord(normalized)) return false
        var letters = 0
        var digits = 0
        for (codePoint in normalized.codePoints().toArray()) {
            when {
                Character.isLetter(codePoint) -> letters++
                Character.isDigit(codePoint) -> digits++
                isMark(codePoint) || codePoint in allowedPunctuation -> Unit
                else -> return false
            }
        }
        return letters >= MIN_LETTERS && digits <= MAX_DIGITS
    }

    private fun containsReservedWord(name: String): Boolean {
        val word = StringBuilder()
        for (codePoint in name.lowercase(Locale.ROOT).codePoints().toArray() + SEPARATOR) {
            if (Character.isLetter(codePoint) || isMark(codePoint)) {
                word.appendCodePoint(codePoint)
            } else {
                if (word.toString() in reservedWords) return true
                word.setLength(0)
            }
        }
        return false
    }

    private fun isMark(codePoint: Int): Boolean = when (Character.getType(codePoint).toByte()) {
        Character.NON_SPACING_MARK, Character.COMBINING_SPACING_MARK, Character.ENCLOSING_MARK -> true
        else -> false
    }

    // Matches Go's unicode.IsSpace: ASCII and Latin-1 spaces plus Unicode
    // space separators, line and paragraph separators.
    private fun isSpace(codePoint: Int): Boolean =
        codePoint in asciiSpaces || Character.isSpaceChar(codePoint)

    private val asciiSpaces = setOf('\t'.code, '\n'.code, 0x0B, 0x0C, '\r'.code, 0x85)
    private val allowedPunctuation = setOf(' '.code, '\''.code, '’'.code, '-'.code, '.'.code)
    private val FORMAT = Character.FORMAT.toInt()
    private const val SEPARATOR = ' '.code
}
