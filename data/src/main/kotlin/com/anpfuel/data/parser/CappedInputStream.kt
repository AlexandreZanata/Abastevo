package com.anpfuel.data.parser

import java.io.FilterInputStream
import java.io.IOException
import java.io.InputStream

/**
 * Counting stream cap for untrusted ZIP entries (SEC06-B): actual
 * inflated bytes refuse past [limit], so lying size headers cannot
 * smuggle expansion past the declared-size gate.
 */
internal class CappedInputStream(
    input: InputStream,
    private val limit: Long,
) : FilterInputStream(input) {

    private var total = 0L

    override fun read(): Int {
        if (total >= limit) {
            throw IOException("ZIP entry exceeds $limit bytes")
        }
        val byte = super.read()
        if (byte >= 0) total++
        return byte
    }

    override fun read(buffer: ByteArray, offset: Int, length: Int): Int {
        if (total >= limit) {
            throw IOException("ZIP entry exceeds $limit bytes")
        }
        val read = super.read(buffer, offset, length)
        if (read > 0) {
            total += read
            if (total > limit) {
                throw IOException("ZIP entry exceeds $limit bytes")
            }
        }
        return read
    }
}
