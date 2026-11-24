package com.anpfuel.domain.account

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PublicNameTest {

    @Test
    fun `shared vectors match the account API rule`() {
        var cases = 0
        File("../contracts/testdata/account/public-names-v1.tsv").readLines().forEachIndexed { index, line ->
            if (line.isEmpty() || line.startsWith("#")) return@forEachIndexed
            val fields = line.split('\t')
            assertEquals(3, fields.size, "line ${index + 1}: want 3 tab-separated fields")
            val input = unescape(fields[1])
            cases++
            when (fields[0]) {
                "normalize" -> assertEquals(fields[2], PublicName.normalize(input), "line ${index + 1}: normalize($input)")
                "valid" -> assertEquals(fields[2].toBooleanStrict(), PublicName.isValid(input), "line ${index + 1}: isValid($input)")
                else -> error("line ${index + 1}: unknown kind ${fields[0]}")
            }
        }
        assertTrue(cases >= 30, "want at least 30 public-name vectors, got $cases")
    }

    /** Decodes the vector escapes shared with Go's strconv.Unquote. */
    private fun unescape(field: String): String {
        val out = StringBuilder()
        var i = 0
        while (i < field.length) {
            val c = field[i]
            if (c != '\\') {
                out.append(c)
                i++
                continue
            }
            when (val escape = field[i + 1]) {
                't' -> out.append('\t').also { i += 2 }
                'n' -> out.append('\n').also { i += 2 }
                'r' -> out.append('\r').also { i += 2 }
                '\\' -> out.append('\\').also { i += 2 }
                'u' -> out.appendCodePoint(field.substring(i + 2, i + 6).toInt(16)).also { i += 6 }
                'U' -> out.appendCodePoint(field.substring(i + 2, i + 10).toInt(16)).also { i += 10 }
                else -> error("unsupported escape \\$escape")
            }
        }
        return out.toString()
    }
}
