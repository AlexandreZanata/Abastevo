package com.anpfuel.domain.portable

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/** Hand-written synthetic tokens only; no photo-derived data. */
class FuelBoardFramingTest {
    private fun t(text: String, left: Int, top: Int, right: Int, bottom: Int, angle: Float = 0f) =
        FuelBoardOcr.Token(text, left, top, right, bottom, angle)

    @Test fun `labeled board is framed with a margin and excludes far text`() {
        val frame = FuelBoardFraming.locate(listOf(
            t("ETANOL", 300, 400, 420, 430), t("4,32", 500, 395, 600, 435),
            t("GASOLINA", 300, 480, 450, 510), t("6,59", 500, 475, 600, 515),
            t("Velocidade:0,4km/h", 600, 1150, 900, 1180), t("12 de out de 2026", 500, 1100, 900, 1130),
        ), 960, 1280)!!
        assertTrue(frame.labeled)
        assertEquals(40, frame.priceHeight)
        assertEquals(260, frame.left); assertEquals(335, frame.top)
        assertEquals(640, frame.right); assertEquals(575, frame.bottom)
        assertEquals(0.0, frame.angleDegrees)
    }

    @Test fun `distant board with unread names is framed from stacked prices and widened to the left`() {
        val frame = FuelBoardFraming.locate(listOf(
            t("3.97", 428, 970, 468, 986), t("6.98", 426, 998, 466, 1014), t("7.92", 425, 1083, 465, 1102),
            t("0.00", 420, 1055, 460, 1070), t("636", 1068, 1277, 1100, 1324),
        ), 1200, 1600)!!
        assertFalse(frame.labeled)
        assertEquals(16, frame.priceHeight)
        assertTrue(frame.left <= 425 - 150 && frame.right >= 468 + 30)
        assertTrue(frame.top <= 970 - 30 && frame.bottom >= 1102 + 30)
        assertEquals(4.0, FuelBoardFraming.zoomFor(frame), 0.01)
    }

    @Test fun `rows read in pieces or without a price still extend the frame`() {
        val frame = FuelBoardFraming.locate(listOf(
            t("ETANOL", 300, 400, 420, 430), t("4,32", 500, 395, 600, 435),
            t("GASOLINA", 300, 480, 450, 510), t("6,59", 500, 475, 600, 515),
            // Third row: the price came out as two digit groups. Fourth row: only the name was read.
            t("DIESEL S10", 300, 560, 470, 590), t("6.", 500, 555, 540, 595), t("19", 548, 555, 600, 595),
            t("DIESEL S500", 300, 640, 480, 670),
            // A phone-number-like digit group far below is not part of the stack.
            t("636", 500, 1100, 560, 1140),
        ), 960, 1280)!!
        assertTrue(frame.bottom >= 670 + 24)
        assertTrue(frame.bottom < 1100)
    }

    @Test fun `a single unlabeled number or a zero display is not a board`() {
        assertNull(FuelBoardFraming.locate(listOf(t("9,99", 100, 100, 180, 130)), 1000, 1000))
        assertNull(FuelBoardFraming.locate(listOf(t("0,00", 100, 100, 180, 130), t("0.000", 100, 150, 180, 180)), 1000, 1000))
        assertNull(FuelBoardFraming.locate(emptyList(), 1000, 1000))
        assertNull(FuelBoardFraming.locate(listOf(t("6,59", 10, 10, 90, 40), t("5,49", 10, 60, 90, 90)), 0, 0))
    }

    @Test fun `small price-like numbers away from the board digits are ignored`() {
        val frame = FuelBoardFraming.locate(listOf(
            t("DIESEL S10", 100, 300, 300, 340), t("6,19", 350, 290, 520, 370),
            t("ETANOL", 100, 420, 240, 460), t("4,06", 350, 410, 520, 490),
            t("2,50", 800, 900, 840, 915),
        ), 1000, 1000)!!
        assertTrue(frame.right < 800 && frame.bottom < 900)
        assertEquals(80, frame.priceHeight)
        assertEquals(1.0, FuelBoardFraming.zoomFor(frame), 0.01)
    }

    @Test fun `separator-less LED digits count only beside a fuel name`() {
        assertNull(FuelBoardFraming.locate(listOf(t("385", 500, 1100, 560, 1130), t("636", 500, 1150, 560, 1180)), 1000, 1300))
        val frame = FuelBoardFraming.locate(listOf(
            t("GASOLINA", 100, 300, 300, 340), t("659", 350, 290, 500, 360),
            t("ETANOL", 100, 420, 240, 460), t("435", 350, 410, 500, 480),
            t("2026", 700, 1200, 780, 1230),
        ), 1000, 1300)
        assertNotNull(frame)
        assertEquals(70, frame!!.priceHeight)
        assertTrue(frame.bottom < 1200)
    }

    @Test fun `tilt is the median text angle and tiny or absurd angles are ignored`() {
        val tilted = FuelBoardFraming.locate(listOf(
            t("ETANOL", 300, 400, 420, 450, 9f), t("4,32", 500, 420, 600, 470, 10f),
            t("GASOLINA", 300, 480, 450, 540, 11f), t("6,59", 500, 500, 600, 550, 88f),
        ), 960, 1280)!!
        assertEquals(10.0, tilted.angleDegrees, 0.01)
        val straight = FuelBoardFraming.locate(listOf(
            t("ETANOL", 300, 400, 420, 430, 1.5f), t("4,32", 500, 395, 600, 435, -1f),
        ), 960, 1280)!!
        assertEquals(0.0, straight.angleDegrees)
    }

    @Test fun `tilted token height is the glyph height, not the inflated box`() {
        // 100x40 text rotated 10 degrees occupies a 105x57 axis-aligned box.
        assertTrue(FuelBoardFraming.textHeight(t("6,59", 0, 0, 105, 57, 10f)) in 38..42)
        assertEquals(57, FuelBoardFraming.textHeight(t("6,59", 0, 0, 105, 57, 0f)))
    }

    @Test fun `zoom never exceeds the pixel budget or the maximum enlargement`() {
        val small = FuelBoardFraming.Frame(0, 0, 100, 100, 4, 0.0, true)
        assertEquals(FuelBoardFraming.MAX_ZOOM, FuelBoardFraming.zoomFor(small), 0.001)
        val wide = FuelBoardFraming.Frame(0, 0, 1024, 600, 16, 0.0, true)
        assertEquals(2.0, FuelBoardFraming.zoomFor(wide), 0.001)
        // The compact reading shrinks oversized digits; the target reading never shrinks.
        val large = FuelBoardFraming.Frame(0, 0, 1000, 1000, 80, 0.0, true)
        assertEquals(1.0, FuelBoardFraming.zoomFor(large), 0.001)
        assertEquals(0.5, FuelBoardFraming.zoomFor(large, digitHeight = FuelBoardFraming.COMPACT_PRICE_HEIGHT), 0.001)
        assertEquals(FuelBoardFraming.MIN_ZOOM, FuelBoardFraming.zoomFor(large.copy(priceHeight = 400), digitHeight = FuelBoardFraming.COMPACT_PRICE_HEIGHT), 0.001)
    }
}
