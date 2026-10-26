package com.anpfuel.domain.map.saved

import java.nio.ByteBuffer
import java.nio.charset.CharacterCodingException
import java.nio.charset.CodingErrorAction

class XmlNode(val name: String, val attributes: Map<String, String>) {
    val children = ArrayList<XmlNode>()
    val text = StringBuilder()

    fun child(local: String): XmlNode? = children.firstOrNull { it.local == local }
    fun all(local: String): List<XmlNode> = children.filter { it.local == local }
    val local: String get() = name.substringAfter(':')
    fun textOf(local: String): String? = child(local)?.text?.toString()?.trim()?.takeIf { it.isNotEmpty() }
}

/**
 * Strict, bounded XML subset for GPX/KML exchange (B-BR-MAP25-04). It exists so that
 * XXE, entity expansion and DTD tricks are impossible by construction: any DOCTYPE,
 * ENTITY, non-leading processing instruction, non-UTF-8 declaration or user-defined
 * entity is refused. No namespace resolution, no external reference, no recursion.
 */
object StrictXml {
    fun parse(bytes: ByteArray): XmlNode {
        if (bytes.size > SavedBounds.MAX_FILE_BYTES) throw SavedValidationException("file-too-large")
        val text = decode(bytes)
        return Reader(text).document()
    }

    private fun decode(bytes: ByteArray): String {
        var start = 0
        if (bytes.size >= 3 && bytes[0] == 0xEF.toByte() && bytes[1] == 0xBB.toByte() && bytes[2] == 0xBF.toByte()) start = 3
        if (bytes.size - start >= 2 && ((bytes[start] == 0xFE.toByte() && bytes[start + 1] == 0xFF.toByte()) ||
                (bytes[start] == 0xFF.toByte() && bytes[start + 1] == 0xFE.toByte()))
        ) throw SavedValidationException("xml-encoding-unsupported")
        return try {
            Charsets.UTF_8.newDecoder()
                .onMalformedInput(CodingErrorAction.REPORT)
                .onUnmappableCharacter(CodingErrorAction.REPORT)
                .decode(ByteBuffer.wrap(bytes, start, bytes.size - start)).toString()
        } catch (_: CharacterCodingException) {
            throw SavedValidationException("xml-encoding-unsupported")
        }
    }

    private class Reader(private val s: String) {
        private var i = 0
        private var elements = 0

        fun document(): XmlNode {
            declaration()
            var root: XmlNode? = null
            while (true) {
                skipSpace()
                if (i >= s.length) break
                if (s.startsWith("<!--", i)) { comment(); continue }
                if (s.startsWith("<?", i)) throw SavedValidationException("xml-pi-refused")
                if (s.startsWith("<!", i)) throw SavedValidationException("xml-doctype-refused")
                if (s[i] != '<') throw SavedValidationException("xml-malformed")
                if (root != null) throw SavedValidationException("xml-malformed")
                root = element(1)
            }
            return root ?: throw SavedValidationException("xml-malformed")
        }

        private fun declaration() {
            if (!s.startsWith("<?xml", i)) return
            val end = s.indexOf("?>", i)
            if (end < 0) throw SavedValidationException("xml-malformed")
            val body = s.substring(i + 5, end)
            if (body.isNotEmpty() && !body[0].isWhitespace()) throw SavedValidationException("xml-pi-refused")
            Regex("encoding\\s*=\\s*([\"'])(.*?)\\1").find(body)?.let {
                if (!it.groupValues[2].equals("UTF-8", ignoreCase = true)) throw SavedValidationException("xml-encoding-unsupported")
            }
            i = end + 2
        }

        private fun skipSpace() { while (i < s.length && s[i].isWhitespace()) i++ }

        private fun comment() {
            val end = s.indexOf("-->", i + 4)
            if (end < 0) throw SavedValidationException("xml-malformed")
            i = end + 3
        }

        private fun nameChar(c: Char) = c.isLetterOrDigit() || c == '_' || c == '-' || c == '.' || c == ':'

        private fun name(): String {
            val start = i
            while (i < s.length && nameChar(s[i])) i++
            if (i == start || s[start].isDigit() || s[start] == '-' || s[start] == '.') throw SavedValidationException("xml-malformed")
            if (i - start > 128) throw SavedValidationException("xml-malformed")
            return s.substring(start, i)
        }

        private fun element(depth: Int): XmlNode {
            if (depth > SavedBounds.MAX_XML_DEPTH) throw SavedValidationException("xml-too-deep")
            if (++elements > SavedBounds.MAX_XML_ELEMENTS) throw SavedValidationException("xml-too-large")
            i++ // '<'
            val tag = name()
            val attributes = LinkedHashMap<String, String>()
            while (true) {
                skipSpace()
                if (i >= s.length) throw SavedValidationException("xml-malformed")
                if (s[i] == '>') { i++; break }
                if (s.startsWith("/>", i)) { i += 2; return XmlNode(tag, attributes) }
                val key = name()
                skipSpace()
                if (i >= s.length || s[i] != '=') throw SavedValidationException("xml-malformed")
                i++
                skipSpace()
                if (i >= s.length || (s[i] != '"' && s[i] != '\'')) throw SavedValidationException("xml-malformed")
                val quote = s[i++]
                val end = s.indexOf(quote, i)
                if (end < 0 || end - i > 4096) throw SavedValidationException("xml-malformed")
                if (attributes.size >= 32 || attributes.containsKey(key)) throw SavedValidationException("xml-malformed")
                val raw = s.substring(i, end)
                if (raw.contains('<')) throw SavedValidationException("xml-malformed")
                attributes[key] = unescape(raw)
                i = end + 1
            }
            val node = XmlNode(tag, attributes)
            while (true) {
                if (i >= s.length) throw SavedValidationException("xml-malformed")
                if (s[i] != '<') {
                    val end = s.indexOf('<', i).let { if (it < 0) throw SavedValidationException("xml-malformed") else it }
                    node.text.append(unescape(s.substring(i, end)))
                    i = end
                    continue
                }
                when {
                    s.startsWith("<!--", i) -> comment()
                    s.startsWith("<![CDATA[", i) -> {
                        val end = s.indexOf("]]>", i + 9)
                        if (end < 0) throw SavedValidationException("xml-malformed")
                        node.text.append(s, i + 9, end)
                        i = end + 3
                    }
                    s.startsWith("<!", i) -> throw SavedValidationException("xml-doctype-refused")
                    s.startsWith("<?", i) -> throw SavedValidationException("xml-pi-refused")
                    s.startsWith("</", i) -> {
                        i += 2
                        val close = name()
                        skipSpace()
                        if (i >= s.length || s[i] != '>' || close != tag) throw SavedValidationException("xml-malformed")
                        i++
                        return node
                    }
                    else -> node.children.add(element(depth + 1))
                }
            }
        }

        private fun unescape(raw: String): String {
            if (!raw.contains('&')) return raw
            val out = StringBuilder(raw.length)
            var k = 0
            while (k < raw.length) {
                val c = raw[k]
                if (c != '&') { out.append(c); k++; continue }
                val end = raw.indexOf(';', k)
                if (end < 0 || end - k > 10) throw SavedValidationException("xml-entity-refused")
                when (val entity = raw.substring(k + 1, end)) {
                    "amp" -> out.append('&')
                    "lt" -> out.append('<')
                    "gt" -> out.append('>')
                    "quot" -> out.append('"')
                    "apos" -> out.append('\'')
                    else -> {
                        if (!entity.startsWith("#")) throw SavedValidationException("xml-entity-refused")
                        val code = try {
                            if (entity.startsWith("#x") || entity.startsWith("#X")) entity.substring(2).toInt(16)
                            else entity.substring(1).toInt(10)
                        } catch (_: NumberFormatException) {
                            throw SavedValidationException("xml-entity-refused")
                        }
                        val valid = code == 0x9 || code == 0xA || code == 0xD || code in 0x20..0xD7FF ||
                            code in 0xE000..0xFFFD || code in 0x10000..0x10FFFF
                        if (!valid) throw SavedValidationException("xml-entity-refused")
                        out.appendCodePoint(code)
                    }
                }
                k = end + 1
            }
            return out.toString()
        }
    }
}
