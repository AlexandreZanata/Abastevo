package com.anpfuel.data.local.charging

import com.anpfuel.domain.charging.ChargingCoverage
import com.anpfuel.domain.charging.ChargingPackMeta
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingPlacePage
import java.security.MessageDigest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ChargingSnapshotImportTest {

    private val placeId = "11111111-1111-4111-8111-111111111111"
    private val placeTwo = "33333333-3333-4333-8333-333333333333"
    private val retiredId = "44444444-4444-4444-8444-444444444444"
    private val key = "d".repeat(64)

    private fun place(id: String = placeId) = ChargingPlace(
        id = id, entityKey = key, externalId = "OCM-7", name = "Eletroposto DF-7",
        country = "BR", latitude = "-15.7939", longitude = "-47.8828",
        publicAccess = com.anpfuel.domain.charging.ChargingAccess.PUBLIC,
        reportedPoints = 2, groups = emptyList(),
    )

    private fun meta() = ChargingPackMeta(
        profile = "ocm-br-20261021-v1", sourceRevision = "rev-1",
        origin = "https://api.example.test", license = "CC-BY-4.0 (provider metadata, attribution required)",
        schemaVersion = 1, baseGeneration = 3, generatedAt = "2026-10-22T00:00:00Z",
    )

    private fun sha(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

    private fun pageBytes(vararg ids: String): ByteArray =
        ids.joinToString(",").toByteArray()

    @Test
    fun `begin admits pinned pack and refuses wrong base source origin`() {
        val session = ChargingSnapshotImport.beginPack(
            meta = meta(), expectedOrigin = "https://api.example.test", currentGeneration = 3,
            expectedHashes = listOf("a".repeat(64)), expectedPlaces = 1,
        )
        assertEquals(4L, session.newGeneration)
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.beginPack(meta(), "https://api.example.test", 2, emptyList(), 0)
        }
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.beginPack(meta(), "https://other.example/", 3, emptyList(), 0)
        }
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.beginPack(meta().copy(profile = "licensed-operator-v1"), "https://api.example.test", 3, emptyList(), 0)
        }
    }

    @Test
    fun `pages validate hash budget and duplicates`() {
        val first = pageBytes(placeId)
        val session = ChargingSnapshotImport.beginPack(meta(), "https://api.example.test", 3, listOf(sha(first)), 1)
        ChargingSnapshotImport.addPage(session, first, ChargingPlacePage(listOf(place()), null, "2026-10-22T00:00:00Z"), sha(first))
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.addPage(session, first, ChargingPlacePage(listOf(place()), null, "2026-10-22T00:00:00Z"), "0".repeat(64))
        }
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.addPage(session, first, ChargingPlacePage(listOf(place()), null, "2026-10-22T00:00:00Z"), sha(first))
        }
        assertThrows(IllegalStateException::class.java) {
            val big = ByteArray(1024 * 1024 + 1)
            ChargingSnapshotImport.addPage(session, big, ChargingPlacePage(emptyList(), null, "2026-10-22T00:00:00Z"), sha(big))
        }
    }

    @Test
    fun `commit validates manifest tombstones and coverage`() {
        val first = pageBytes(placeId)
        val second = pageBytes(placeTwo)
        val session = ChargingSnapshotImport.beginPack(
            meta(), "https://api.example.test", 3, listOf(sha(first), sha(second)), 2,
        )
        ChargingSnapshotImport.addPage(session, first, ChargingPlacePage(listOf(place()), "c2", "2026-10-22T00:00:00Z"), sha(first))
        ChargingSnapshotImport.addPage(session, second, ChargingPlacePage(listOf(place(placeTwo)), null, "2026-10-22T00:00:00Z"), sha(second))
        val coverage = ChargingCoverage("ocm-br-20261021-v1", "rev-1", 1298, 3387, 181, "2026-10-22T00:00:00Z")
        val committed = ChargingSnapshotImport.commitPack(session, listOf(retiredId), coverage)
        assertEquals(4L, committed.meta.generation)
        assertEquals(3, committed.places.size)
        assertTrue(committed.places.single { it.placeId == retiredId }.tombstone)
        assertEquals("COMPLETE", committed.meta.state)
        // Tombstone overlapping an added place refuses.
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.commitPack(session, listOf(placeId), coverage)
        }
        // Coverage smaller than the pack refuses.
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.commitPack(session, emptyList(), coverage.copy(places = 1))
        }
        // Coverage from another profile or revision refuses.
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.commitPack(session, emptyList(), coverage.copy(profile = "pbe-2026-pdf-v1"))
        }
        // Manifest mismatch refuses.
        val short = ChargingSnapshotImport.beginPack(meta(), "https://api.example.test", 3, listOf(sha(first)), 9)
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.commitPack(short, emptyList(), coverage)
        }
    }

    @Test
    fun `interrupted delta resumes from cursor without touching committed generation`() {
        val first = pageBytes(placeId)
        val session = ChargingSnapshotImport.beginPack(meta(), "https://api.example.test", 3, listOf(sha(first), "b".repeat(64)), 2)
        ChargingSnapshotImport.addPage(session, first, ChargingPlacePage(listOf(place()), "resume-here", "2026-10-22T00:00:00Z"), sha(first))
        assertEquals("resume-here", session.resumeCursor())
        assertThrows(IllegalStateException::class.java) {
            ChargingSnapshotImport.commitPack(session, emptyList(), null)
        }
        assertEquals(1, session.pendingPlaces())
    }
}
