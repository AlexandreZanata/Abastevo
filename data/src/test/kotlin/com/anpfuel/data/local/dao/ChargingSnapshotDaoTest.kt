package com.anpfuel.data.local.dao

import com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity
import com.anpfuel.data.local.entity.ChargingSnapshotPlaceEntity
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class ChargingSnapshotDaoTest {

    private class FakeDao : ChargingSnapshotDao {
        override suspend fun findPlace(placeId: String, profile: String, generation: Long) =
            places.values.singleOrNull { it.placeId == placeId && it.profile == profile && it.generation == generation && !it.tombstone }

        val places = mutableMapOf<Pair<String, Long>, ChargingSnapshotPlaceEntity>()
        val metas = mutableMapOf<String, ChargingSnapshotMetaEntity>()
        var tombstoneCalls = 0

        override suspend fun upsertPlaces(places: List<ChargingSnapshotPlaceEntity>) {
            places.forEach { this.places[it.placeId to it.generation] = it }
        }

        override suspend fun upsertMeta(meta: ChargingSnapshotMetaEntity) {
            metas[meta.key] = meta
        }

        override suspend fun deleteOlderGenerations(profile: String, generation: Long) {
            places.keys.filter { places[it]!!.profile == profile && it.second != generation }
                .forEach { places.remove(it) }
        }

        override suspend fun deleteProfilePlaces(profile: String) {
            places.keys.filter { places[it]!!.profile == profile }.forEach { places.remove(it) }
        }

        override suspend fun clearMeta(key: String) {
            metas.remove(key)
        }

        override suspend fun markTombstones(ids: List<String>, generation: Long) {
            tombstoneCalls++
            ids.forEach { id ->
                val row = places[id to generation] ?: return@forEach
                places[id to generation] = row.copy(tombstone = true)
            }
        }

        override suspend fun findMeta(key: String): ChargingSnapshotMetaEntity? = metas[key]

        override suspend fun readPlaces(profile: String, generation: Long, limit: Int, offset: Int) =
            places.values.filter { it.profile == profile && it.generation == generation && !it.tombstone }
                .sortedBy { it.placeId }.drop(offset).take(limit)

        override suspend fun countPlaces(generation: Long) =
            places.values.count { it.generation == generation && !it.tombstone }.toLong()

        override suspend fun estimatedBytes(generation: Long) =
            places.values.filter { it.generation == generation }
                .sumOf { (it.name.length + it.groupsJson.length + 512).toLong() }
    }

    private fun meta(generation: Long = 4) = ChargingSnapshotMetaEntity(
        key = ChargingSnapshotMetaEntity.packKey("ocm-br-20261021-v1"),
        generation = generation, profile = "ocm-br-20261021-v1", sourceRevision = "rev-1",
        origin = "https://api.example.test", license = "CC-BY-4.0 (provider metadata, attribution required)",
        schemaVersion = 1, baseGeneration = 3, placeCount = 1, groupCount = 0,
        cursor = null, state = ChargingSnapshotMetaEntity.STATE_COMPLETE, savedAtMillis = 1,
    )

    private fun place(generation: Long = 4, id: String = "11111111-1111-4111-8111-111111111111") = ChargingSnapshotPlaceEntity(
        placeId = id, entityKey = "e".repeat(64), externalId = "OCM-1", generation = generation,
        profile = "ocm-br-20261021-v1", sourceRevision = "rev-1",
        origin = "https://api.example.test", license = "CC-BY-4.0 (provider metadata, attribution required)",
        name = "Eletroposto DF-7", country = "BR", latitude = null, longitude = null,
        publicAccess = "PUBLIC", reportedPoints = 0, groupsJson = "[]",
        tombstone = false, savedAtMillis = 1,
    )

    @Test fun `replace swaps generation and clears the old one`() = runTest {
        val dao = FakeDao()
        dao.upsertPlaces(listOf(place(generation = 3)))
        dao.replaceGeneration(meta(), listOf(place()))
        assertEquals(listOf(place()), dao.readPlaces("ocm-br-20261021-v1", 4, 10, 0))
        assertTrue(dao.readPlaces("ocm-br-20261021-v1", 3, 10, 0).isEmpty())
        assertEquals(4L, dao.findMeta(ChargingSnapshotMetaEntity.packKey("ocm-br-20261021-v1"))?.generation)
    }

    @Test fun `budgets fail the transaction instead of trimming`() = runTest {
        val dao = FakeDao()
        val filler = (0 until 10_000).map { place(generation = 4, id = "%08x-1111-4111-8111-111111111111".format(it)) }
        val overCount = try {
            dao.replaceGeneration(meta(), filler + place())
            false
        } catch (_: IllegalStateException) {
            true
        }
        assertTrue(overCount)
        val heavy = FakeDao()
        val big = place().copy(groupsJson = "x".repeat(9 * 1024 * 1024))
        val overBytes = try {
            heavy.replaceGeneration(meta(), listOf(big))
            false
        } catch (_: IllegalStateException) {
            true
        }
        assertTrue(overBytes)
    }

    @Test fun `delta marks tombstones and empty tombstones skip the write`() = runTest {
        val dao = FakeDao()
        dao.replaceGeneration(meta(generation = 4), listOf(place(generation = 4)))
        dao.appendDelta(
            meta(generation = 4).copy(state = ChargingSnapshotMetaEntity.STATE_IN_PROGRESS, cursor = "c2"),
            emptyList(), listOf(place().placeId),
        )
        assertEquals(1, dao.tombstoneCalls)
        assertTrue(dao.readPlaces("ocm-br-20261021-v1", 4, 10, 0).isEmpty())
        val fresh = FakeDao()
        fresh.appendDelta(meta(generation = 5), listOf(place(generation = 5)), emptyList())
        assertEquals(0, fresh.tombstoneCalls)
    }

    @Test fun `removePack clears rows and meta for the profile only`() = runTest {
        val dao = FakeDao()
        dao.replaceGeneration(meta(), listOf(place()))
        dao.replaceGeneration(meta(generation = 7).copy(key = "pack:pbe-2026-pdf-v1", profile = "pbe-2026-pdf-v1"), listOf(place(generation = 7).copy(profile = "pbe-2026-pdf-v1")))
        dao.removePack("ocm-br-20261021-v1")
        assertTrue(dao.readPlaces("ocm-br-20261021-v1", 4, 10, 0).isEmpty())
        assertEquals(1, dao.readPlaces("pbe-2026-pdf-v1", 7, 10, 0).size)
    }
}
