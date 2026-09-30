package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PortableTextTest {

    @Test
    fun countsAsciiAndAccentedScalars() {
        assertEquals(3, PortableText.countScalars("abc"))
        assertEquals(4, PortableText.countScalars("café"))
    }

    @Test
    fun countsAstralEmojiAsOneScalar() {
        // U+1F697 is a surrogate pair in UTF-16 (length 2) but one scalar.
        assertEquals(1, PortableText.countScalars("🚗"))
        assertEquals(2, "🚗".length)
    }

    @Test
    fun countsCombiningSequenceAsBasePlusMark() {
        // e + U+0301 COMBINING ACUTE ACCENT: two scalars, like Go rune count.
        assertEquals(2, PortableText.countScalars("é"))
    }

    @Test
    fun normalizesCrlfBeforeCounting() {
        assertEquals("a\nb", PortableText.normalize("a\r\nb"))
        assertEquals(3, PortableText.countScalars(PortableText.normalize("a\r\nb")))
    }

    @Test
    fun acceptsExactly280AndRejects281() {
        val max = "a".repeat(280)
        val over = "a".repeat(281)
        assertTrue(PortableText.isValidComment(max))
        assertFalse(PortableText.isValidComment(over))
    }

    @Test
    fun rejectsEmptyAndBlank() {
        assertFalse(PortableText.isValidComment(""))
        assertFalse(PortableText.isValidComment("   "))
    }

    @Test
    fun acceptsEmojiCommentWithinLimit() {
        assertTrue(PortableText.isValidComment("Preço bom ⛽🚗"))
    }
}
