package com.anpfuel.domain.map.saved

import java.io.ByteArrayInputStream
import java.io.IOException
import java.util.zip.ZipException
import java.util.zip.ZipInputStream

/**
 * Bounded in-memory KMZ reader (B-BR-MAP25-04): nothing is ever written to disk, so
 * traversal names are refused rather than sanitised, nested archives and oversized or
 * highly compressed entries are refused, and only a root `.kml` document is used.
 */
object KmzReader {
    fun looksLikeZip(bytes: ByteArray): Boolean =
        bytes.size >= 4 && bytes[0] == 0x50.toByte() && bytes[1] == 0x4B.toByte() &&
            (bytes[2] == 0x03.toByte() || bytes[2] == 0x05.toByte() || bytes[2] == 0x07.toByte())

    fun readKml(bytes: ByteArray): ByteArray {
        if (bytes.size > SavedBounds.MAX_FILE_BYTES) throw SavedValidationException("file-too-large")
        var entries = 0
        var expanded = 0L
        var chosen: ByteArray? = null
        var chosenIsDoc = false
        try {
            ZipInputStream(ByteArrayInputStream(bytes)).use { zip ->
                while (true) {
                    val entry = zip.nextEntry ?: break
                    if (++entries > SavedBounds.MAX_ARCHIVE_ENTRIES) throw SavedValidationException("archive-too-many-entries")
                    val name = entry.name
                    if (name.isEmpty() || name.startsWith("/") || name.contains('\\') || name.contains(':') ||
                        name.split("/").any { it == ".." }
                    ) throw SavedValidationException("archive-name-refused")
                    val lower = name.lowercase()
                    if (lower.endsWith(".kmz") || lower.endsWith(".zip") || lower.endsWith(".jar") || lower.endsWith(".gz")) {
                        throw SavedValidationException("archive-nested-refused")
                    }
                    val wanted = !entry.isDirectory && !name.contains('/') && lower.endsWith(".kml") &&
                        (chosen == null || (lower == "doc.kml" && !chosenIsDoc))
                    val buffer = ByteArray(16 * 1024)
                    val out = if (wanted) java.io.ByteArrayOutputStream() else null
                    var entryBytes = 0L
                    while (true) {
                        val read = zip.read(buffer)
                        if (read < 0) break
                        entryBytes += read
                        expanded += read
                        if (expanded > SavedBounds.MAX_EXPANDED_BYTES || entryBytes > SavedBounds.MAX_FILE_BYTES * 4L) {
                            throw SavedValidationException("archive-too-large")
                        }
                        out?.write(buffer, 0, read)
                    }
                    if (out != null) {
                        chosen = out.toByteArray()
                        chosenIsDoc = lower == "doc.kml"
                    }
                }
            }
        } catch (error: SavedValidationException) {
            throw error
        } catch (_: ZipException) {
            throw SavedValidationException("archive-malformed")
        } catch (_: IOException) {
            throw SavedValidationException("archive-malformed")
        }
        val kml = chosen ?: throw SavedValidationException("archive-no-kml")
        if (kml.size > SavedBounds.MAX_FILE_BYTES) throw SavedValidationException("file-too-large")
        return kml
    }
}
