package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class PortablePhotoTest {

    @Test
    fun budgetsMirrorFrozenForwardContract() {
        assertEquals("image/jpeg", PortablePhoto.WIRE_MIME)
        assertEquals(153600L, PortablePhoto.TARGET_BYTES)
        assertEquals(262144L, PortablePhoto.CAP_BYTES)
        assertEquals(1600, PortablePhoto.MAX_EDGE_PIXELS)
        assertEquals(2000000L, PortablePhoto.MAX_PIXELS)
        assertEquals(3, PortablePhoto.MAX_ATTEMPTS)
        assertEquals(33554432L, PortablePhoto.WORKING_MEMORY_HYPOTHESIS_BYTES)
        assertEquals(86400000L, PortablePhoto.TRANSIENT_TTL_MILLIS)
    }

    @Test
    fun intentAllowlistIsExact() {
        assertTrue(PortablePhoto.isSupportedIntentMime("image/jpeg"))
        assertTrue(PortablePhoto.isSupportedIntentMime("image/png"))
        assertTrue(PortablePhoto.isSupportedIntentMime("image/heic"))
        assertTrue(PortablePhoto.isSupportedIntentMime("image/heif"))
        assertTrue(PortablePhoto.isSupportedIntentMime("  IMAGE/PNG  "))
        assertFalse(PortablePhoto.isSupportedIntentMime("image/gif"))
        assertFalse(PortablePhoto.isSupportedIntentMime("image/webp"))
        assertFalse(PortablePhoto.isSupportedIntentMime("image/bmp"))
        assertFalse(PortablePhoto.isSupportedIntentMime("image/svg+xml"))
        assertFalse(PortablePhoto.isSupportedIntentMime(""))
        assertFalse(PortablePhoto.isSupportedIntentMime("image/jpeg2000"))
    }

    @Test
    fun sampleSizeKeepsDecodedFrameInsideBudgets() {
        // Small frames decode as-is: no upscale, no work.
        assertEquals(1, PortablePhoto.sampleSizeForBounds(1200, 900))
        assertEquals(1, PortablePhoto.sampleSizeForBounds(1600, 1200))
        // 4000x3000 (12 MP): 4x → 1000x750 (0.75 MP) inside both bounds.
        assertEquals(4, PortablePhoto.sampleSizeForBounds(4000, 3000))
        // 8000x6000 (48 MP): 8x → 1000x750 inside both bounds.
        assertEquals(8, PortablePhoto.sampleSizeForBounds(8000, 6000))
        // Long edge rules even when megapixels fit: 3200x400 needs 2x.
        assertEquals(2, PortablePhoto.sampleSizeForBounds(3200, 400))
        // Megapixel rule with fitting edges: 1600x1600 (2.56 MP) needs 2x.
        assertEquals(2, PortablePhoto.sampleSizeForBounds(1600, 1600))
        // Results are powers of two.
        for ((w, h) in listOf(4000 to 3000, 3200 to 400, 1600 to 1600, 5000 to 5000)) {
            val size = PortablePhoto.sampleSizeForBounds(w, h)
            assertTrue(size and (size - 1) == 0, "power of two for $w x $h")
            val (sw, sh) = PortablePhoto.sampledDims(w, h, size)
            assertTrue(sw <= 1600 && sh <= 1600, "edge inside for $w x $h")
            assertTrue(sw.toLong() * sh <= 2000000L, "pixels inside for $w x $h")
        }
    }

    @Test
    fun sampleSizeRefusesNonPositiveFrames() {
        assertThrows(IllegalArgumentException::class.java) {
            PortablePhoto.sampleSizeForBounds(0, 600)
        }
        assertThrows(IllegalArgumentException::class.java) {
            PortablePhoto.sampleSizeForBounds(600, -1)
        }
    }

    @Test
    fun wireCapBoundsBytes() {
        assertTrue(PortablePhoto.fitsWireCap(1L))
        assertTrue(PortablePhoto.fitsWireCap(153600L))
        assertTrue(PortablePhoto.fitsWireCap(262144L))
        assertFalse(PortablePhoto.fitsWireCap(0L))
        assertFalse(PortablePhoto.fitsWireCap(-5L))
        assertFalse(PortablePhoto.fitsWireCap(262145L))
    }

    @Test
    fun transientExpiryIsCapturePlus24h() {
        assertFalse(PortablePhoto.isTransientExpired(0L, 86399999L))
        assertTrue(PortablePhoto.isTransientExpired(0L, 86400000L))
        assertTrue(PortablePhoto.isTransientExpired(0L, 90000000L))
    }
}
