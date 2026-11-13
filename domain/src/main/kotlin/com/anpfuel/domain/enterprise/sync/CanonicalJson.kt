package com.anpfuel.domain.enterprise.sync

import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction

enum class SyncFailure { INVALID, LIMIT, BINDING, DIGEST, REUSED }
class SyncProtocolException internal constructor(val failure: SyncFailure) : IllegalArgumentException("orgsync: ${failure.name}") {
    val code: String get() = failure.name
}
internal fun invalid(failure: SyncFailure = SyncFailure.INVALID): SyncProtocolException = SyncProtocolException(failure)

/** Exact JSON values. Numbers never pass through floating point or a fixed-width integer. */
sealed interface SyncJson {
    class Object(fields: Map<String, SyncJson>) : SyncJson {
        private val content = fields.toMap()
        init { content.keys.forEach { if (!segmented(it)) throw invalid() } }
        val fields: Map<String, SyncJson> get() = content.toMap()
        internal fun entries(): Set<Map.Entry<String, SyncJson>> = content.entries
        internal operator fun get(key: String): SyncJson? = content[key]
        override fun toString(): String = "SyncObject(redacted)"
    }
    class Array(values: List<SyncJson>) : SyncJson {
        private val content = values.toList()
        val values: List<SyncJson> get() = content.toList()
        internal fun items(): List<SyncJson> = content
        override fun toString(): String = "SyncArray(redacted)"
    }
    class Text(val value: String) : SyncJson {
        init { requireUnicode(value) }
        override fun toString(): String = "SyncText(redacted)"
    }
    class Integer(val lexeme: String) : SyncJson {
        init { if (!integerToken.matches(lexeme)) throw invalid() }
        override fun toString(): String = "SyncInteger(redacted)"
    }
    class Bool(val value: Boolean) : SyncJson { override fun toString(): String = "SyncBoolean" }
    data object Null : SyncJson
}

private val integerToken = Regex("0|-?[1-9][0-9]*")
internal fun requireUnicode(value: String) {
    var index = 0
    while (index < value.length) {
        val character = value[index++]
        if (Character.isHighSurrogate(character)) {
            if (index >= value.length || !Character.isLowSurrogate(value[index++])) throw invalid()
        } else if (Character.isLowSurrogate(character)) throw invalid()
    }
}

/** Small enterprise canonical profile, deliberately not a claim of RFC 8785 compliance. */
object CanonicalJson {
    const val MAX_BATCH_BYTES = 65536
    const val MAX_OPERATION_BYTES = 8192
    const val MAX_TRANSPORT_DEPTH = 64
    const val MAX_OPERATION_DEPTH = 62

    fun parse(raw: ByteArray, maxBytes: Int = MAX_BATCH_BYTES, maxDepth: Int = MAX_TRANSPORT_DEPTH): SyncJson =
        parseSpanned(raw, maxBytes, maxDepth).value

    internal fun parseSpanned(raw: ByteArray, maxBytes: Int, maxDepth: Int): JsonNode {
        if (maxBytes !in 1..MAX_BATCH_BYTES || maxDepth !in 1..MAX_TRANSPORT_DEPTH) throw invalid()
        if (raw.size > maxBytes) throw invalid(SyncFailure.LIMIT)
        val text = try {
            Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
                .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(raw)).toString()
        } catch (_: java.nio.charset.CharacterCodingException) { throw invalid() }
        return Parser(text, maxDepth).parse()
    }

    fun encode(value: SyncJson, maxDepth: Int = MAX_TRANSPORT_DEPTH): ByteArray {
        if (maxDepth !in 1..MAX_TRANSPORT_DEPTH) throw invalid()
        return buildString { appendValue(value, 0, maxDepth) }.toByteArray(Charsets.UTF_8)
    }

    private fun StringBuilder.appendValue(value: SyncJson, containers: Int, limit: Int) {
        when (value) {
            is SyncJson.Object -> {
                if (containers >= limit) throw invalid(SyncFailure.LIMIT)
                append('{')
                value.entries().sortedBy { it.key }.forEachIndexed { index, entry ->
                    if (index > 0) append(',')
                    appendText(entry.key); append(':'); appendValue(entry.value, containers + 1, limit)
                }
                append('}')
            }
            is SyncJson.Array -> {
                if (containers >= limit) throw invalid(SyncFailure.LIMIT)
                append('[')
                value.items().forEachIndexed { index, item ->
                    if (index > 0) append(',')
                    appendValue(item, containers + 1, limit)
                }
                append(']')
            }
            is SyncJson.Text -> appendText(value.value)
            is SyncJson.Integer -> append(value.lexeme)
            is SyncJson.Bool -> append(if (value.value) "true" else "false")
            SyncJson.Null -> append("null")
        }
    }

    private fun StringBuilder.appendText(value: String) {
        append('"')
        for (character in value) when {
            character == '"' -> append("\\\"")
            character == '\\' -> append("\\\\")
            character < ' ' -> append("\\u00").append("0123456789abcdef"[character.code ushr 4]).append("0123456789abcdef"[character.code and 15])
            else -> append(character)
        }
        append('"')
    }

    internal class JsonNode(val value: SyncJson, val start: Int, val end: Int, val children: List<JsonNode> = emptyList())

    private class Parser(private val source: String, private val maxDepth: Int) {
        private var index = 0
        fun parse(): JsonNode {
            skipWhitespace()
            val node = value(0)
            skipWhitespace()
            if (index != source.length) throw invalid()
            return node
        }
        private fun value(containers: Int): JsonNode {
            skipWhitespace()
            if (index == source.length) throw invalid()
            val start = index
            val children = mutableListOf<JsonNode>()
            val result = when (source[index]) {
                '{' -> {
                    if (containers >= maxDepth) throw invalid(SyncFailure.LIMIT)
                    index++; skipWhitespace()
                    val fields = linkedMapOf<String, SyncJson>()
                    if (!take('}')) {
                        do {
                            skipWhitespace()
                            if (index >= source.length || source[index] != '"') throw invalid()
                            val key = text()
                            if (!segmented(key) || fields.containsKey(key)) throw invalid()
                            skipWhitespace(); expect(':')
                            val child = value(containers + 1)
                            children.add(child); fields[key] = child.value
                            skipWhitespace()
                            if (take('}')) break
                            expect(',')
                        } while (true)
                    }
                    SyncJson.Object(fields)
                }
                '[' -> {
                    if (containers >= maxDepth) throw invalid(SyncFailure.LIMIT)
                    index++; skipWhitespace()
                    if (!take(']')) {
                        do {
                            children.add(value(containers + 1)); skipWhitespace()
                            if (take(']')) break
                            expect(',')
                        } while (true)
                    }
                    SyncJson.Array(children.map { it.value })
                }
                '"' -> SyncJson.Text(text())
                't' -> { literal("true"); SyncJson.Bool(true) }
                'f' -> { literal("false"); SyncJson.Bool(false) }
                'n' -> { literal("null"); SyncJson.Null }
                else -> {
                    if (source[index] == '-') index++
                    val digits = index
                    while (index < source.length && source[index] in '0'..'9') index++
                    if (index == digits) throw invalid()
                    SyncJson.Integer(source.substring(start, index))
                }
            }
            return JsonNode(result, start, index, children)
        }
        private fun text(): String {
            expect('"')
            val result = StringBuilder()
            while (index < source.length) {
                val character = source[index++]
                when {
                    character == '"' -> return result.toString().also(::requireUnicode)
                    character < ' ' -> throw invalid()
                    character != '\\' -> result.append(character)
                    else -> {
                        if (index == source.length) throw invalid()
                        when (source[index++]) {
                            '"' -> result.append('"')
                            '\\' -> result.append('\\')
                            '/' -> result.append('/')
                            'b' -> result.append('\b')
                            'f' -> result.append('\u000c')
                            'n' -> result.append('\n')
                            'r' -> result.append('\r')
                            't' -> result.append('\t')
                            'u' -> {
                                if (source.length - index < 4) throw invalid()
                                val hex = source.substring(index, index + 4)
                                if (hex.any { it !in '0'..'9' && it !in 'a'..'f' && it !in 'A'..'F' }) throw invalid()
                                result.append(hex.toInt(16).toChar()); index += 4
                            }
                            else -> throw invalid()
                        }
                    }
                }
            }
            throw invalid()
        }
        private fun literal(token: String) {
            if (!source.startsWith(token, index)) throw invalid()
            index += token.length
        }
        private fun skipWhitespace() { while (index < source.length && source[index] in " \t\n\r") index++ }
        private fun take(character: Char): Boolean = if (index < source.length && source[index] == character) { index++; true } else false
        private fun expect(character: Char) { if (!take(character)) throw invalid() }
    }
}
