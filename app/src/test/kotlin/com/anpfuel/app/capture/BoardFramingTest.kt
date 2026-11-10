package com.anpfuel.app.capture

import com.anpfuel.application.port.ImagePriceOcr
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Review photo auto-centring: pure geometry, no pixels. */
class BoardFramingTest {
    @Test fun `centred board is enlarged until it fills the crop frame`() {
        // 4:3 photo in a 4:3 container: fit = 1. Board is the middle quarter in each direction.
        val (scale, offset) = boardFraming(ImagePriceOcr.BoardFrame(0.375f, 0.375f, 0.625f, 0.625f), 800, 600, 800f, 600f)!!
        assertEquals(0.85f / 0.25f * 0.96f, scale, 0.001f)
        assertEquals(0f, offset.x, 0.001f)
        assertEquals(0f, offset.y, 0.001f)
    }

    @Test fun `off-centre board is brought to the middle`() {
        val (scale, offset) = boardFraming(ImagePriceOcr.BoardFrame(0.6f, 0.2f, 0.8f, 0.4f), 800, 600, 800f, 600f)!!
        assertEquals(0.85f / 0.2f * 0.96f, scale, 0.001f)
        // Board centre is (+160, -120) px from the container centre before zoom.
        assertEquals(-160f * scale, offset.x, 0.01f)
        assertEquals(120f * scale, offset.y, 0.01f)
    }

    @Test fun `zoom and pan stay inside the gesture limits`() {
        val tiny = boardFraming(ImagePriceOcr.BoardFrame(0.0f, 0.0f, 0.02f, 0.02f), 800, 600, 800f, 600f)!!
        assertEquals(5f, tiny.first)
        assertEquals((800f * 5f - 800f) / 2f, tiny.second.x, 0.01f)
        assertEquals((600f * 5f - 600f) / 2f, tiny.second.y, 0.01f)
        val whole = boardFraming(ImagePriceOcr.BoardFrame(0f, 0f, 1f, 1f), 800, 600, 800f, 600f)!!
        assertEquals(1f, whole.first)
        assertEquals(0f, whole.second.x, 0.001f)
        assertEquals(0f, whole.second.y, 0.001f)
    }

    @Test fun `portrait photo in a landscape container uses the fitted size`() {
        // 600x800 photo letterboxed in 800x600: fit = 0.75, shown 450x600.
        val (scale, offset) = boardFraming(ImagePriceOcr.BoardFrame(0.25f, 0.5f, 0.75f, 0.75f), 600, 800, 800f, 600f)!!
        val boardW = 0.5f * 600 * 0.75f
        val boardH = 0.25f * 800 * 0.75f
        assertEquals(minOf(680f / boardW, 510f / boardH) * 0.96f, scale, 0.001f)
        assertEquals(0f, offset.x, 0.01f)
        assertTrue(offset.y < 0f)
    }

    @Test fun `nothing laid out or an invalid frame gives no framing`() {
        assertNull(boardFraming(ImagePriceOcr.BoardFrame(0.1f, 0.1f, 0.9f, 0.9f), 800, 600, 0f, 0f))
        assertNull(boardFraming(ImagePriceOcr.BoardFrame(0.1f, 0.1f, 0.9f, 0.9f), 0, 0, 800f, 600f))
        assertThrows(IllegalArgumentException::class.java) { ImagePriceOcr.BoardFrame(0.5f, 0.1f, 0.4f, 0.9f) }
        assertThrows(IllegalArgumentException::class.java) { ImagePriceOcr.BoardFrame(-0.1f, 0.1f, 0.4f, 0.9f) }
    }
}
