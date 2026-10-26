package com.anpfuel.domain.map.saved

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class StrictXmlTest {
    private fun refusal(xml: String): String = try { StrictXml.parse(xml.toByteArray()); "ok" } catch (e: SavedValidationException) { e.code }
    private fun refusalBytes(bytes: ByteArray): String = try { StrictXml.parse(bytes); "ok" } catch (e: SavedValidationException) { e.code }

    @Test fun `plain document with predefined entities cdata and comments parses`() {
        val root = StrictXml.parse("""<?xml version="1.0" encoding="UTF-8"?><!-- c --><a x="1 &amp; 2"><b>a&lt;b&#x41;&#66;</b><c><![CDATA[<raw & text>]]></c><d/></a>""".toByteArray())
        assertEquals("1 & 2", root.attributes["x"])
        assertEquals("a<bAB", root.child("b")!!.text.toString())
        assertEquals("<raw & text>", root.child("c")!!.text.toString())
        assertNotNull(root.child("d"))
    }

    @Test fun `xxe doctype entity and billion laughs are refused before expansion`() {
        assertEquals("xml-doctype-refused", refusal("""<?xml version="1.0"?><!DOCTYPE a [<!ENTITY x SYSTEM "file:///etc/passwd">]><a>&x;</a>"""))
        assertEquals("xml-doctype-refused", refusal("""<!DOCTYPE lolz [<!ENTITY lol "lol"><!ENTITY lol2 "&lol;&lol;">]><lolz>&lol2;</lolz>"""))
        assertEquals("xml-doctype-refused", refusal("""<a><!DOCTYPE b></a>"""))
        assertEquals("xml-entity-refused", refusal("<a>&xxe;</a>"))
        assertEquals("xml-entity-refused", refusal("<a>&amp</a>"))
        assertEquals("xml-entity-refused", refusal("<a>&#0;</a>"))
        assertEquals("xml-entity-refused", refusal("<a>&#xD800;</a>"))
        assertEquals("xml-entity-refused", refusal("<a>&#99999999999;</a>"))
        assertEquals("xml-entity-refused", refusal("""<a b="&ext;"/>"""))
    }

    @Test fun `processing instructions other than the declaration and foreign encodings are refused`() {
        assertEquals("xml-pi-refused", refusal("""<?xml version="1.0"?><?xml-stylesheet href="x"?><a/>"""))
        assertEquals("xml-pi-refused", refusal("<a><?php echo 1 ?></a>"))
        assertEquals("xml-encoding-unsupported", refusal("""<?xml version="1.0" encoding="ISO-8859-1"?><a/>"""))
        assertEquals("xml-encoding-unsupported", refusalBytes(byteArrayOf(0xFF.toByte(), 0xFE.toByte(), 0x3C, 0x00)))
        assertEquals("xml-encoding-unsupported", refusalBytes(byteArrayOf(0x3C, 0x61, 0x3E, 0xC3.toByte(), 0x28, 0x3C, 0x2F, 0x61, 0x3E)))
        assertEquals("ok", refusalBytes(byteArrayOf(0xEF.toByte(), 0xBB.toByte(), 0xBF.toByte(), 0x3C, 0x61, 0x2F, 0x3E)))
    }

    @Test fun `structure errors are refused`() {
        for (bad in listOf("", "text", "<a>", "<a></b>", "<a><b></a></b>", "<a/><b/>", "<a x=1/>", "<a x='1' x='2'/>", "<a>&lt;</a>trailing", "<1a/>", "<a x=\"<\"/>", "<a><!-- unterminated</a>", "<a><![CDATA[x</a>")) {
            assertNotEquals("ok", refusal(bad), bad)
        }
    }

    @Test fun `depth element count and size limits hold`() {
        assertEquals("ok", refusal("<a>".repeat(64) + "</a>".repeat(64)))
        assertEquals("xml-too-deep", refusal("<a>".repeat(65) + "</a>".repeat(65)))
        val wide = "<r>" + "<e/>".repeat(SavedBounds.MAX_XML_ELEMENTS) + "</r>"
        assertEquals("xml-too-large", refusal(wide))
        assertEquals("file-too-large", try { StrictXml.parse(ByteArray(SavedBounds.MAX_FILE_BYTES + 1)); "ok" } catch (e: SavedValidationException) { e.code })
        val attrs = "<a " + (1..33).joinToString(" ") { "k$it=\"v\"" } + "/>"
        assertEquals("xml-malformed", refusal(attrs))
    }
}
