package com.anpfuel.domain.enterprise.attachment

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

/**
 * EMP-DOC-004 (B-BR-EMP-451/452, EDR-291/292): content-decided intake of an unmodified original and the pure
 * legibility hint. The legibility expectations come from the independent oracle
 * f4-logs/doc004-scripts/legibility_oracle.py over the same synthetic grids.
 */
class ReceiptCaptureTest {
    private fun jpeg(size: Int, fill: Byte = 9): ByteArray = ByteArray(size) { fill }.also {
        it[0] = 0xff.toByte(); it[1] = 0xd8.toByte(); it[2] = 0xff.toByte(); it[size - 2] = 0xff.toByte(); it[size - 1] = 0xd9.toByte()
    }
    private fun refusal(bytes: ByteArray?): ReceiptRefusal? =
        (ReceiptIntake.inspect(bytes, ReceiptSource.IMPORT) as? ReceiptInspection.Refused)?.refusal

    @Test
    fun missingOrEmptySelectionsAreMissingAndNeverAnOriginal() {
        assertEquals(ReceiptRefusal.MISSING, refusal(null))
        assertEquals(ReceiptRefusal.MISSING, refusal(ByteArray(0)))
    }

    @Test
    fun theTypeIsDecidedByContentNotByName() {
        val png = byteArrayOf(0x89.toByte(), 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 0x0d) + ByteArray(64)
        val pdf = "%PDF-1.4\n% synthetic\n%%EOF".toByteArray()
        val heic = byteArrayOf(0, 0, 0, 0x18) + "ftypheic".toByteArray() + ByteArray(64)
        val webp = "RIFF".toByteArray() + byteArrayOf(64, 0, 0, 0) + "WEBPVP8 ".toByteArray() + ByteArray(32)
        val text = "not a photo.jpg".toByteArray()
        val startOnly = jpeg(64).copyOf(62) + byteArrayOf(0, 0)
        val tiny = byteArrayOf(0xff.toByte(), 0xd8.toByte(), 0xff.toByte())
        for (bytes in listOf(png, pdf, heic, webp, text, startOnly, tiny)) assertEquals(ReceiptRefusal.UNSUPPORTED, refusal(bytes))
    }

    @Test
    fun theBudgetIsFiveMebibytesAndTheReadLimitOneByteMore() {
        val limit = AttachmentPolicy.MAX_BYTES.toInt()
        assertEquals(AttachmentPolicy.MAX_BYTES + 1, ReceiptIntake.READ_LIMIT)
        assertEquals(ReceiptRefusal.TOO_LARGE, refusal(jpeg(limit + 1)))
        val accepted = ReceiptIntake.inspect(jpeg(limit), ReceiptSource.CAMERA) as ReceiptInspection.Accepted
        assertEquals(limit.toLong(), accepted.original.size)
        // Size is judged before type, like the EMP-OFFL-005 policy.
        assertEquals(ReceiptRefusal.TOO_LARGE, refusal(ByteArray(limit + 1) { 1 }))
    }

    @Test
    fun anAcceptedOriginalIsTheExactBytesAndNeverAliased() {
        val input = jpeg(128, fill = 42)
        val expected = input.copyOf()
        val accepted = ReceiptIntake.inspect(input, ReceiptSource.CAMERA) as ReceiptInspection.Accepted
        val original = accepted.original
        assertEquals(ReceiptSource.CAMERA, original.source)
        assertArrayEquals(expected, original.bytes())
        assertEquals(AttachmentPolicy.sha256(expected), original.sha256)
        input.fill(0)
        original.bytes().fill(1)
        assertArrayEquals(expected, original.bytes())
        assertEquals("ReceiptOriginal(redacted)", original.toString())
        assertFalse(accepted.toString().contains(original.sha256))
    }

    // ---- legibility (EDR-292) ----

    private val w = 96
    private val h = 128
    private fun grid(f: (Int, Int) -> Int): Array<IntArray> = Array(h) { y -> IntArray(w) { x -> f(x, y) } }
    private fun stripes(bg: Int, fg: Int) = grid { x, y -> if (y % 6 < 2 && x % 16 < 12) fg else bg }
    private fun box(g: Array<IntArray>, r: Int): Array<IntArray> = grid { x, y ->
        var acc = 0; var count = 0
        for (dy in -r..r) for (dx in -r..r) { acc += g[(y + dy).coerceIn(0, h - 1)][(x + dx).coerceIn(0, w - 1)]; count++ }
        acc / count
    }
    private fun sample(g: Array<IntArray>, sourceWidth: Int = 960, sourceHeight: Int = 1280): LumaSample =
        LumaSample(w, h, sourceWidth, sourceHeight, ByteArray(w * h) { g[it / w][it % w].toByte() })
    private fun issues(g: Array<IntArray>, sourceWidth: Int = 960, sourceHeight: Int = 1280): Set<LegibilityIssue> =
        ReceiptLegibility.assess(sample(g, sourceWidth, sourceHeight)).also { assertTrue(it.checked) }.issues

    @Test
    fun aSharpWellLitTextLikeImageLooksSharp() {
        val hint = ReceiptLegibility.assess(sample(stripes(230, 30)))
        assertTrue(hint.looksSharp)
        assertEquals(emptySet<LegibilityIssue>(), hint.issues)
    }

    @Test
    fun eachProblemIsReportedAndNoneBlocks() {
        assertEquals(setOf(LegibilityIssue.TOO_DARK, LegibilityIssue.LOW_CONTRAST), issues(stripes(50, 0)))
        assertEquals(setOf(LegibilityIssue.TOO_BRIGHT), issues(stripes(255, 200)))
        assertEquals(setOf(LegibilityIssue.LOW_CONTRAST, LegibilityIssue.BLURRED), issues(grid { _, _ -> 128 }))
        assertEquals(setOf(LegibilityIssue.BLURRED), issues(grid { x, _ -> 40 + 180 * x / (w - 1) }))
        assertEquals(setOf(LegibilityIssue.LOW_CONTRAST, LegibilityIssue.BLURRED), issues(box(box(stripes(230, 30), 2), 2)))
        assertEquals(setOf(LegibilityIssue.LOW_RESOLUTION), issues(stripes(230, 30), sourceWidth = 400))
    }

    @Test
    fun thresholdsAreStrictAndExact() {
        assertEquals(setOf(LegibilityIssue.TOO_DARK, LegibilityIssue.LOW_CONTRAST, LegibilityIssue.BLURRED), issues(grid { _, _ -> 59 }))
        assertEquals(setOf(LegibilityIssue.LOW_CONTRAST, LegibilityIssue.BLURRED), issues(grid { _, _ -> 60 }))
        assertEquals(setOf(LegibilityIssue.LOW_CONTRAST, LegibilityIssue.BLURRED), issues(grid { _, _ -> 225 }))
        assertEquals(setOf(LegibilityIssue.TOO_BRIGHT, LegibilityIssue.LOW_CONTRAST, LegibilityIssue.BLURRED), issues(grid { _, _ -> 226 }))
        // Two equal halves: the standard deviation is exactly half their difference (24 is enough, 23 is not).
        assertEquals(setOf(LegibilityIssue.BLURRED), issues(grid { x, _ -> if (x < w / 2) 100 else 148 }))
        assertEquals(setOf(LegibilityIssue.LOW_CONTRAST, LegibilityIssue.BLURRED), issues(grid { x, _ -> if (x < w / 2) 100 else 146 }))
        assertEquals(emptySet<LegibilityIssue>(), issues(stripes(230, 30), sourceWidth = 480))
        assertEquals(setOf(LegibilityIssue.LOW_RESOLUTION), issues(stripes(230, 30), sourceWidth = 1280, sourceHeight = 479))
    }

    @Test
    fun anUndecodedImageIsNotCheckedAndStillNotARefusal() {
        val hint = ReceiptLegibility.assess(null)
        assertFalse(hint.checked)
        assertFalse(hint.looksSharp)
        assertTrue(hint.issues.isEmpty())
        assertEquals(LegibilityHint.NOT_CHECKED, hint)
    }

    @Test
    fun lumaFollowsTheIntegerBt601Formula() {
        val pixels = intArrayOf(0xffffffff.toInt(), 0xffff0000.toInt(), 0xff00ff00.toInt(), 0xff0000ff.toInt(), 0x00000000, 0x80102030.toInt(),
            0xff7f7f7f.toInt(), 0xff010101.toInt(), 0xfffefefe.toInt())
        val sample = LumaSample.fromArgb(3, 3, 3, 3, pixels)
        fun bt601(p: Int) = (299 * (p shr 16 and 255) + 587 * (p shr 8 and 255) + 114 * (p and 255) + 500) / 1000
        for (i in pixels.indices) assertEquals(bt601(pixels[i]), sample.at(i % 3, i / 3))
        assertEquals(255, sample.at(0, 0))
        assertEquals(76, sample.at(1, 0))
        assertEquals(150, sample.at(2, 0))
        assertEquals(29, sample.at(0, 1))
    }

    @Test
    fun samplesAreBoundedAndConsistent() {
        assertThrows<IllegalArgumentException> { LumaSample(2, 3, 2, 3, ByteArray(6)) }
        assertThrows<IllegalArgumentException> { LumaSample(513, 3, 513, 3, ByteArray(513 * 3)) }
        assertThrows<IllegalArgumentException> { LumaSample(3, 3, 2, 3, ByteArray(9)) }
        assertThrows<IllegalArgumentException> { LumaSample(3, 3, 3, 3, ByteArray(8)) }
        assertThrows<IllegalArgumentException> { LumaSample.fromArgb(3, 3, 3, 3, IntArray(8)) }
        val source = ByteArray(9) { 7 }
        val sample = LumaSample(3, 3, 3, 3, source)
        source.fill(0)
        assertEquals(7, sample.at(1, 1))
        assertEquals("LumaSample(redacted)", sample.toString())
    }
}
