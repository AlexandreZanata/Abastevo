package com.anpfuel.domain.portable

import kotlin.math.abs
import kotlin.math.cos
import kotlin.math.max
import kotlin.math.min
import kotlin.math.roundToInt
import kotlin.math.sin

/**
 * Finds the price board in a photo from spatial OCR tokens (B-BR-PC02
 * framing): where to crop, how far to zoom and how much to straighten before
 * reading again. It only chooses pixels to look at; it never reads, repairs
 * or assigns a price.
 */
object FuelBoardFraming {
    /** Board rectangle in the token coordinate space, with the digit height and text tilt found there. */
    data class Frame(
        val left: Int,
        val top: Int,
        val right: Int,
        val bottom: Int,
        val priceHeight: Int,
        val angleDegrees: Double,
        val labeled: Boolean,
    ) {
        val width get() = right - left
        val height get() = bottom - top
    }

    /** Digit height the framed reading aims for; small digits are the main cause of missed rows. */
    const val TARGET_PRICE_HEIGHT = 64
    /** Digit height of the compact framed reading; it may shrink a board whose digits are larger. */
    const val COMPACT_PRICE_HEIGHT = 40
    const val MAX_ZOOM = 6.0
    const val MIN_ZOOM = 0.4
    /** Below this tilt the axis-aligned association is unaffected. */
    const val MIN_SKEW_DEGREES = 2.5
    /** Beyond this the reported angle is not trusted as a board tilt. */
    const val MAX_SKEW_DEGREES = 35.0

    private val separatedPrice = Regex("^(?:R\\$ ?)?[0-9]{1,2}[.,] ?[0-9]{2,3}$")
    private val barePrice = Regex("^[0-9]{3}$")
    private val zeroPrice = Regex("^(?:R\\$ ?)?0?0[.,] ?0{2,3}$")
    private val digits = Regex("^[0-9]{1,3}[.,]?$")

    fun locate(tokens: List<FuelBoardOcr.Token>, width: Int, height: Int): Frame? {
        if (width <= 0 || height <= 0) return null
        val usable = tokens.take(512).filter { it.right > it.left && it.bottom > it.top && it.text.length <= 256 }
        val labels = usable.filter { FuelBoardOcr.isFuelLabel(it.text) && !isPrice(it) }
        val separated = usable.filter { isPrice(it) }
        // LED boards drop the separator ("659"); such a number counts only beside a fuel name.
        val bare = usable.filter { barePrice.matches(it.text.trim()) }.filter { value ->
            labels.any { label ->
                val h = max(textHeight(label), textHeight(value))
                abs(label.cy - value.cy) <= h * 1.5 && value.left >= label.left
            }
        }
        val candidates = (separated + bare).distinct()
        if (candidates.isEmpty()) return null
        val tallest = candidates.maxOf { textHeight(it) }
        // Board digits are the largest prices in the scene; small ones are ads or watermarks.
        val prices = candidates.filter { textHeight(it) >= tallest * 0.5 }
        val near = labels.filter { label ->
            prices.any { value ->
                val h = max(textHeight(label), textHeight(value))
                abs(value.cy - label.cy) <= h * 3 && abs(value.cx - label.cx) <= width * 0.65
            }
        }
        if (near.isEmpty() && prices.size < 2) return null
        val priceHeight = median(prices.map { textHeight(it) })
        val box = (prices + near).toMutableList()
        var left = box.minOf { it.left }
        var top = box.minOf { it.top }
        var right = box.maxOf { it.right }
        var bottom = box.maxOf { it.bottom }
        // Rows whose price was read in pieces ("6." + "45") or not at all still belong to the
        // board: take in names and board-sized digit groups stacked with what was found.
        val stackable = labels + usable.filter { digits.matches(it.text.trim()) && textHeight(it) >= priceHeight * 0.5 }
        repeat(6) {
            val reach = priceHeight * 4
            val spread = (right - left) / 2
            val added = stackable.filter { token ->
                token !in box && token.right >= left - spread && token.left <= right + spread &&
                    token.bottom >= top - reach && token.top <= bottom + reach
            }
            if (added.isEmpty()) return@repeat
            box += added
            left = box.minOf { it.left }; top = box.minOf { it.top }
            right = box.maxOf { it.right }; bottom = box.maxOf { it.bottom }
        }
        if (near.isNotEmpty()) {
            val horizontal = max(24, priceHeight)
            val vertical = max(24, priceHeight * 3 / 2)
            left -= horizontal; right += horizontal; top -= vertical; bottom += vertical
        } else {
            // Names were too small to read: boards print them left of or above the digits.
            val priceWidth = median(prices.map { it.right - it.left })
            left -= priceWidth * 4; right += priceWidth
            top -= priceHeight * 2; bottom += priceHeight * 2
        }
        left = left.coerceAtLeast(0); top = top.coerceAtLeast(0)
        right = right.coerceAtMost(width); bottom = bottom.coerceAtMost(height)
        if (right - left < 8 || bottom - top < 8) return null
        val angles = box.map { it.angle.toDouble() }.filter { abs(it) <= MAX_SKEW_DEGREES }.sorted()
        val angle = if (angles.isEmpty()) 0.0 else angles[angles.size / 2]
        return Frame(left, top, right, bottom, priceHeight, if (abs(angle) >= MIN_SKEW_DEGREES) angle else 0.0, near.isNotEmpty())
    }

    /**
     * Scale that brings the digits to [digitHeight] without exceeding [maxSide] pixels. The
     * target reading only enlarges; the compact reading may also shrink oversized digits.
     */
    fun zoomFor(frame: Frame, maxSide: Int = 2048, digitHeight: Int = TARGET_PRICE_HEIGHT): Double {
        val floor = if (digitHeight >= TARGET_PRICE_HEIGHT) 1.0 else MIN_ZOOM
        val wanted = (digitHeight.toDouble() / max(1, frame.priceHeight)).coerceIn(floor, MAX_ZOOM)
        val fit = maxSide.toDouble() / max(1, max(frame.width, frame.height))
        return min(wanted, fit)
    }

    /** Height of the text itself: a tilted token's axis-aligned box is taller than its glyphs. */
    internal fun textHeight(token: FuelBoardOcr.Token): Int {
        val radians = Math.toRadians(abs(token.angle.toDouble()).coerceAtMost(MAX_SKEW_DEGREES))
        val c = cos(radians)
        val s = sin(radians)
        val boxWidth = (token.right - token.left).toDouble()
        val boxHeight = (token.bottom - token.top).toDouble()
        val denominator = c * c - s * s
        if (s < 0.02 || denominator < 0.2) return max(1, boxHeight.roundToInt())
        val straight = (boxHeight * c - boxWidth * s) / denominator
        return if (straight >= 1 && straight <= boxHeight) straight.roundToInt() else max(1, boxHeight.roundToInt())
    }

    private fun isPrice(token: FuelBoardOcr.Token): Boolean {
        val text = token.text.trim()
        return separatedPrice.matches(text) && !zeroPrice.matches(text)
    }

    private fun median(values: List<Int>): Int = values.sorted()[values.size / 2]
}
