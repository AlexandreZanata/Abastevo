package com.anpfuel.domain.enterprise.sync

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CanonicalJsonTest {
    @Test fun `canonical bytes retain exact integers Unicode and control spelling`() {
        val raw = """ {"z":[9007199254740993,9223372036854775808,-123456789012345678901234567890],"a":"<é😀>\n\t\u0000"} """
        assertEquals("""{"a":"<é😀>\u000a\u0009\u0000","z":[9007199254740993,9223372036854775808,-123456789012345678901234567890]}""", canonical(raw))
        assertEquals("""{"a":1}""", canonical("""{"\u0061":1}"""))
        assertNotEquals(canonical("""{"a":"é"}"""), canonical("""{"a":"é"}"""))
    }

    @Test fun `malformed numbers keys strings and grammar are rejected`() {
        val cases = listOf("01", "-0", "1.0", "1e2", "+1", "--1", "1 2", "[1,]", "{\"a\":1,}", "{\"a\":1,\"a\":2}", "{\"a\":1,\"\\u0061\":2}", "{\"A\":1}", "{\"a-b\":1}", "{\"é\":1}", "\"\\ud800\"", "\"\\udc00\"", "\"\\ud800a\"", "\"\\x00\"", "\"\n\"", "nullx", "\uFEFFnull", "")
        cases.forEach { raw -> assertThrows(SyncProtocolException::class.java, { canonical(raw) }, raw) }
        assertThrows(SyncProtocolException::class.java) { CanonicalJson.parse(byteArrayOf(0xc3.toByte(), 0x28)) }
    }

    @Test fun `container levels and raw byte limits include empty containers`() {
        listOf("0", "[]").forEach { leaf ->
            val at = "[".repeat(63) + leaf + "]".repeat(63)
            assertEquals(at, canonical(at))
        }
        val empty64 = "[".repeat(64) + "]".repeat(64)
        assertEquals(empty64, canonical(empty64))
        val over = "[".repeat(65) + "]".repeat(65)
        assertThrows(SyncProtocolException::class.java) { canonical(over) }
        assertEquals("null", String(CanonicalJson.encode(CanonicalJson.parse(" null ".toByteArray(), 6))))
        assertThrows(SyncProtocolException::class.java) { CanonicalJson.parse(" null ".toByteArray(), 5) }
    }

    @Test fun `AST snapshots collections and rejects invalid programmatic values`() {
        val source = mutableListOf<SyncJson>(SyncJson.Integer("1"))
        val array = SyncJson.Array(source)
        source.clear()
        runCatching { (array.values as? MutableList<SyncJson>)?.clear() }
        assertEquals("[1]", String(CanonicalJson.encode(array)))
        val fields = mutableMapOf("a" to array)
        val obj = SyncJson.Object(fields)
        fields.clear()
        runCatching { (obj.fields as? MutableMap<String, SyncJson>)?.clear() }
        assertEquals("{\"a\":[1]}", String(CanonicalJson.encode(obj)))
        assertFalse(obj.toString().contains("[1]"))
        assertThrows(SyncProtocolException::class.java) { SyncJson.Integer("-0") }
        assertThrows(SyncProtocolException::class.java) { SyncJson.Text("\uD800") }
        assertThrows(SyncProtocolException::class.java) { SyncJson.Object(mapOf("Bad" to SyncJson.Null)) }
    }

    @Test fun `programmatic AST cannot bypass depth or induce recursive key parsing`() {
        var node: SyncJson = SyncJson.Null
        repeat(64) { node = SyncJson.Array(listOf(node)) }
        assertNotNull(CanonicalJson.encode(node))
        node = SyncJson.Array(listOf(node))
        assertEquals("LIMIT", assertThrows(SyncProtocolException::class.java) { CanonicalJson.encode(node) }.code)
        val segmentedKey = "a_".repeat(10000) + "a"
        val raw = "{\"$segmentedKey\":null}"
        assertEquals(raw, canonical(raw))
        assertEquals("INVALID", assertThrows(SyncProtocolException::class.java) { CanonicalJson.parse("null".toByteArray(), maxBytes = 65537) }.code)
    }

    private fun canonical(raw: String): String = String(CanonicalJson.encode(CanonicalJson.parse(raw.toByteArray(Charsets.UTF_8))), Charsets.UTF_8)
}
