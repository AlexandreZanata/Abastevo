package com.anpfuel.domain.enterprise.attachment

/*
 * EMP-DOC-004 — capture and import of a synthetic C1 original (B-BR-EMP-450..454, EDR-291..293). The type is
 * decided by content with the EMP-OFFL-005 JPEG policy, the accepted bytes are kept exactly as read, and the
 * legibility check is a hint that never blocks, never validates and is never stored.
 */

/** How an original reached the device (B-BR-EMP-450). */
enum class ReceiptSource { CAMERA, IMPORT }

/** Why a selected file is not an acceptable original (B-BR-EMP-451); nothing is saved. */
enum class ReceiptRefusal {
    /** No readable bytes: the file is gone, its access was revoked, or it is empty. */
    MISSING,
    /** More than the 5 MiB EMP-DOC-002 budget. */
    TOO_LARGE,
    /** Not a JPEG by content (PNG, HEIC, WebP, PDF, a renamed file). */
    UNSUPPORTED,
}

/** An accepted original: the exact bytes as read, never modified (B-BR-EMP-451). */
class ReceiptOriginal private constructor(bytes: ByteArray, val source: ReceiptSource, val sha256: String) {
    private val value = bytes.copyOf()
    val size: Long get() = value.size.toLong()
    /** A copy of the exact original bytes. */
    fun bytes(): ByteArray = value.copyOf()
    override fun toString(): String = "ReceiptOriginal(redacted)"

    internal companion object {
        fun of(bytes: ByteArray, source: ReceiptSource, sha256: String): ReceiptOriginal = ReceiptOriginal(bytes, source, sha256)
    }
}

/** The outcome of inspecting one selected file. */
sealed interface ReceiptInspection {
    class Accepted(val original: ReceiptOriginal) : ReceiptInspection {
        override fun toString(): String = "ReceiptAccepted(redacted)"
    }
    class Refused(val refusal: ReceiptRefusal) : ReceiptInspection {
        override fun toString(): String = "ReceiptRefused(${refusal.name})"
    }
}

object ReceiptIntake {
    /** One byte more than the budget, so a larger file is recognised without reading all of it. */
    const val READ_LIMIT: Long = AttachmentPolicy.MAX_BYTES + 1

    /** Judges [bytes] by content only; the accepted original is a copy of them, unchanged. */
    fun inspect(bytes: ByteArray?, source: ReceiptSource): ReceiptInspection {
        if (bytes == null || bytes.isEmpty()) return ReceiptInspection.Refused(ReceiptRefusal.MISSING)
        val copy = bytes.copyOf()
        try {
            // The EMP-OFFL-005 policy is the single judge: size first, then the JPEG markers.
            val sha256 = try { AttachmentPolicy.checkJpeg(copy) } catch (local: AttachmentLocalException) {
                return ReceiptInspection.Refused(if (local.failure == AttachmentLocalFailure.SIZE) ReceiptRefusal.TOO_LARGE else ReceiptRefusal.UNSUPPORTED)
            }
            return ReceiptInspection.Accepted(ReceiptOriginal.of(copy, source, sha256))
        } finally { copy.fill(0) }
    }
}

/** One legibility problem seen in the sample (EDR-292). */
enum class LegibilityIssue { LOW_RESOLUTION, TOO_DARK, TOO_BRIGHT, LOW_CONTRAST, BLURRED }

/** The legibility hint of one original: never a validation, never stored or sent (B-BR-EMP-452). */
class LegibilityHint private constructor(val checked: Boolean, issues: Set<LegibilityIssue>) {
    private val values = issues.toSortedSet()
    val issues: Set<LegibilityIssue> get() = values.toSet()
    /** Checked and nothing seen; still only a hint. */
    val looksSharp: Boolean get() = checked && values.isEmpty()
    override fun equals(other: Any?): Boolean = other is LegibilityHint && checked == other.checked && values == other.values
    override fun hashCode(): Int = 31 * checked.hashCode() + values.hashCode()
    override fun toString(): String = "LegibilityHint($checked, $values)"

    companion object {
        /** The image could not be decoded; it may still be saved. */
        val NOT_CHECKED: LegibilityHint = LegibilityHint(false, emptySet())
        fun of(issues: Set<LegibilityIssue>): LegibilityHint = LegibilityHint(true, issues)
    }
}

/** A downscaled luminance sample of a decoded original (unsigned 0..255 per pixel, row-major). */
class LumaSample(val width: Int, val height: Int, val sourceWidth: Int, val sourceHeight: Int, luma: ByteArray) {
    private val values = luma.copyOf()
    init {
        require(width in MIN_SIDE..ReceiptLegibility.MAX_SAMPLE_SIDE && height in MIN_SIDE..ReceiptLegibility.MAX_SAMPLE_SIDE)
        require(sourceWidth >= width && sourceHeight >= height && values.size == width * height)
    }
    /** The luminance at ([x], [y]). */
    fun at(x: Int, y: Int): Int = values[y * width + x].toInt() and 255
    override fun toString(): String = "LumaSample(redacted)"

    companion object {
        const val MIN_SIDE = 3
        /** BT.601 integer luma of ARGB pixels: (299 R + 587 G + 114 B + 500) / 1000. */
        fun fromArgb(width: Int, height: Int, sourceWidth: Int, sourceHeight: Int, pixels: IntArray): LumaSample {
            require(width > 0 && height > 0 && pixels.size == width * height)
            val luma = ByteArray(pixels.size) { index ->
                val p = pixels[index]
                ((299 * (p shr 16 and 255) + 587 * (p shr 8 and 255) + 114 * (p and 255) + 500) / 1000).toByte()
            }
            return LumaSample(width, height, sourceWidth, sourceHeight, luma)
        }
    }
}

/** The pure legibility heuristic of EDR-292; every threshold is a hypothesis kept here. */
object ReceiptLegibility {
    const val MAX_SAMPLE_SIDE = 512
    const val MIN_SOURCE_SIDE = 480
    const val DARK_MEAN = 60
    const val BRIGHT_MEAN = 225
    const val MIN_DEVIATION = 24
    const val MIN_LAPLACIAN_VARIANCE = 60

    fun assess(sample: LumaSample?): LegibilityHint {
        if (sample == null) return LegibilityHint.NOT_CHECKED
        val issues = mutableSetOf<LegibilityIssue>()
        if (minOf(sample.sourceWidth, sample.sourceHeight) < MIN_SOURCE_SIDE) issues += LegibilityIssue.LOW_RESOLUTION
        // Exact integer statistics: no rounding decides a threshold.
        val n = sample.width.toLong() * sample.height
        var sum = 0L
        var squares = 0L
        for (y in 0 until sample.height) for (x in 0 until sample.width) {
            val v = sample.at(x, y).toLong()
            sum += v; squares += v * v
        }
        if (sum < DARK_MEAN * n) issues += LegibilityIssue.TOO_DARK
        if (sum > BRIGHT_MEAN * n) issues += LegibilityIssue.TOO_BRIGHT
        if (n * squares - sum * sum < MIN_DEVIATION.toLong() * MIN_DEVIATION * n * n) issues += LegibilityIssue.LOW_CONTRAST
        // Variance of the 4-neighbour Laplacian over interior pixels: little edge energy reads as blur.
        val m = (sample.width - 2).toLong() * (sample.height - 2)
        var edges = 0L
        var edgeSquares = 0L
        for (y in 1 until sample.height - 1) for (x in 1 until sample.width - 1) {
            val l = (4 * sample.at(x, y) - sample.at(x, y - 1) - sample.at(x, y + 1) - sample.at(x - 1, y) - sample.at(x + 1, y)).toLong()
            edges += l; edgeSquares += l * l
        }
        if (m * edgeSquares - edges * edges < MIN_LAPLACIAN_VARIANCE * m * m) issues += LegibilityIssue.BLURRED
        return LegibilityHint.of(issues)
    }
}
