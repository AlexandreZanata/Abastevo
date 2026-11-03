package com.anpfuel.data.repository

import com.anpfuel.data.local.charging.ChargingSnapshotImport
import com.anpfuel.data.local.dao.ChargingSnapshotDao
import com.anpfuel.data.local.entity.ChargingSnapshotMetaEntity
import com.anpfuel.data.local.entity.ChargingSnapshotPlaceEntity
import com.anpfuel.data.remote.charging.ChargingSnapshotHttpClient
import com.anpfuel.domain.charging.ChargingCatalogConfidence
import com.anpfuel.domain.charging.ChargingCoverage
import com.anpfuel.domain.charging.ChargingSourceAttribution
import com.anpfuel.domain.charging.ChargingAccess
import com.anpfuel.domain.charging.ChargingConnector
import com.anpfuel.domain.charging.ChargingCurrent
import com.anpfuel.domain.charging.ChargingPlace
import com.anpfuel.domain.charging.ChargingPlacePage
import com.anpfuel.domain.charging.Compatibility
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test

class ChargingDiscoveryRepositoryTest {

    private val client = mockk<ChargingSnapshotHttpClient>()
    private val origin = com.anpfuel.data.remote.ApiEnvironment.PREVIEW

    private class FakeDao : ChargingSnapshotDao {
        val places = mutableMapOf<Triple<String, Long, String>, ChargingSnapshotPlaceEntity>()
        val metas = mutableMapOf<String, ChargingSnapshotMetaEntity>()
        var removeCalls = 0

        override suspend fun upsertPlaces(places: List<ChargingSnapshotPlaceEntity>) {
            places.forEach { this.places[Triple(it.placeId, it.generation, it.profile)] = it }
        }

        override suspend fun upsertMeta(meta: ChargingSnapshotMetaEntity) {
            metas[meta.key] = meta
        }

        override suspend fun deleteOlderGenerations(profile: String, generation: Long) {
            places.keys.filter { it.third == profile && it.second != generation }.forEach { places.remove(it) }
        }

        override suspend fun deleteProfilePlaces(profile: String) {
            removeCalls++
            places.keys.filter { it.third == profile }.forEach { places.remove(it) }
        }

        override suspend fun clearMeta(key: String) {
            metas.remove(key)
        }

        override suspend fun markTombstones(ids: List<String>, profile: String, generation: Long) {
            ids.forEach { id ->
                val key = Triple(id, generation, profile)
                places[key]?.let { places[key] = it.copy(tombstone = true) }
            }
        }

        override suspend fun findMeta(key: String) = metas[key]

        override suspend fun readPlaces(profile: String, generation: Long, limit: Int, offset: Int) =
            places.values.filter { it.profile == profile && it.generation == generation && !it.tombstone }
                .sortedBy { it.placeId }.drop(offset).take(limit)

        override suspend fun findPlace(placeId: String, profile: String, generation: Long) =
            places.values.singleOrNull { it.placeId == placeId && it.profile == profile && it.generation == generation && !it.tombstone }

        override suspend fun readPlacesInBounds(
            profile: String,
            generation: Long,
            minLatitude: Double,
            maxLatitude: Double,
            minLongitude: Double,
            maxLongitude: Double,
            limit: Int,
        ) = places.values.filter {
            it.profile == profile && it.generation == generation && !it.tombstone &&
                it.latitude?.toDoubleOrNull()?.let { lat -> lat in minLatitude..maxLatitude } == true &&
                it.longitude?.toDoubleOrNull()?.let { lon -> lon in minLongitude..maxLongitude } == true
        }.sortedBy { it.placeId }.take(limit)

        override suspend fun countPlaces(profile: String, generation: Long) =
            places.values.count { it.profile == profile && it.generation == generation && !it.tombstone }.toLong()

        override suspend fun estimatedBytes(profile: String, generation: Long) = 1024L
    }

    private val dao = FakeDao()
    private lateinit var repo: ChargingDiscoveryRepository

    private val key = "a".repeat(64)
    private fun place(id: String, connector: ChargingConnector = ChargingConnector.CCS2) = ChargingPlace(
        id = id, entityKey = key, externalId = "OCM-1", name = "Eletroposto $id", country = "BR",
        latitude = null, longitude = null, publicAccess = ChargingAccess.PUBLIC,
        reportedPoints = 2,
        groups = listOf(
            com.anpfuel.domain.charging.ChargingConnectionGroup(
                id = "22222222-2222-4222-8222-222222222222", entityKey = key, externalId = "OCM-1",
                connector = connector, rawConnectorCode = connector.name, current = ChargingCurrent.DC,
                powerW = 150000, reportedQuantity = 2,
            ),
        ),
    )

    private fun page(vararg ids: String, next: String? = null) = ChargingPlacePage(
        ids.map { place(it) }, next, "2026-10-26T00:00:00Z",
    )

    private suspend fun assertRefreshRefused(refresh: suspend () -> Unit) {
        val failure = runCatching { refresh() }.exceptionOrNull()
        assertTrue(failure is IllegalStateException, "expected refresh refusal, got $failure")
    }

    @BeforeEach
    fun setUp() {
        repo = ChargingDiscoveryRepository(client, dao, origin)
    }

    @Test fun `refresh commits a generation and lists with verdicts`() = runTest {
        coEvery { client.places("ocm-br-20261021-v1", null, 100, null) } returns page("11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", next = "c2")
        coEvery { client.places("ocm-br-20261021-v1", null, 100, "c2") } returns page("33333333-3333-4333-8333-333333333333")
        val meta = repo.refresh("ocm-br-20261021-v1")
        assertEquals(1L, meta.generation)
        assertEquals(3, dao.places.size)
        val rows = repo.list("ocm-br-20261021-v1", null, ChargingConnector.CCS2, null, null, 100, 0)
        assertEquals(3, rows.size)
        assertTrue(rows.all { it.verdict == Compatibility.COMPATIBLE })
        assertEquals("ocm-br-20261021-v1", rows.single { it.place.id == "11111111-1111-4111-8111-111111111111" }.profile)
    }

    @Test fun `power filtered windows skip sparse raw pages and continue exactly once`() = runTest {
        val profile = ChargingDiscoveryRepository.OCM_PROFILE
        val places = (1..104).map { number ->
            val base = place("%08x-1111-4111-8111-111111111111".format(number))
            val power = when (number) {
                101 -> null
                102, 104 -> 150000L
                else -> 22000L
            }
            base.copy(groups = listOf(base.groups.single().copy(powerW = power)))
        }
        val stamp = "2026-10-26T00:00:00Z"
        coEvery { client.places(profile, null, 100, null) } returns ChargingPlacePage(places.take(100), "c2", stamp)
        coEvery { client.places(profile, null, 100, "c2") } returns ChargingPlacePage(places.drop(100), null, stamp)
        val meta = repo.refresh(profile)

        suspend fun read(offset: Int) = repo.explorePage(
            profile, meta.generation, ChargingConnector.CCS2, ChargingCurrent.DC, null, 150000, 1, offset,
        )
        val first = read(0)
        val second = read(requireNotNull(first.nextOffset))
        assertEquals(listOf(places[101].id), first.rows.map { it.place.id })
        assertEquals(listOf(places[103].id), second.rows.map { it.place.id })
        assertEquals(null, second.nextOffset)
    }

    @Test fun `national refresh requires a complete stable revision before replacing cache`() = runTest {
        val profile = "charging-br-national-v1"
        val revision = "c".repeat(64)
        val stamp = "2026-10-26T00:00:00Z"
        val id = "11111111-1111-4111-8111-111111111111"
        val second = "33333333-3333-4333-8333-333333333333"
        val codec = com.anpfuel.data.remote.charging.ChargingSnapshotJsonCodec
        fun coverage(
            rev: String = revision,
            groups: Int = 2,
            base: String = "/v1/charging/catalog/$rev/offer",
            data: String = "$base/derived-database.ndjson",
        ): ChargingCoverage =
            codec.decodeCoverage(org.json.JSONObject("""
                {"profile":"$profile","source_revision":"$rev","places":2,"groups":$groups,
                 "vehicles":0,"generated_at":"$stamp",
                 "offer":{"notice":"$base/derived-database-offer.json","data":"$data"}}
            """.trimIndent()))
        fun national(id: String): ChargingPlace {
            val base = place(id)
            return base.copy(
                catalogConfidence = ChargingCatalogConfidence.SINGLE_SOURCE,
                sources = listOf(ChargingSourceAttribution("OSM", "node:7", "synthetic", null, "ODbL-1.0", "Synthetic OSM")),
                groups = base.groups.map { it.copy(sourceKey = "osm:node:7") },
            )
        }
        val firstPage = ChargingPlacePage(listOf(national(id)), "c2", stamp)
        val lastPage = ChargingPlacePage(listOf(national(second)), null, stamp)
        coEvery { client.coverage(profile) } returns coverage()
        coEvery { client.places(profile, revision, 100, null) } returns firstPage
        coEvery { client.places(profile, revision, 100, "c2") } returns lastPage

        val original = repo.refresh(profile)
        assertEquals(revision, original.sourceRevision)
        assertEquals(2L, original.groupCount)
        assertEquals(national(id), repo.detail(id, profile, null)?.place)

        coEvery { client.places(profile, revision, 100, "c2") } returns lastPage.copy(items = emptyList())
        assertRefreshRefused { repo.refresh(profile) }
        coEvery { client.places(profile, revision, 100, "c2") } returns lastPage
        coEvery { client.coverage(profile) } returns coverage(data = "https://foreign.example/offer")
        assertRefreshRefused { repo.refresh(profile) }
        coEvery { client.coverage(profile) } returns coverage(groups = 3)
        assertRefreshRefused { repo.refresh(profile) }
        coEvery { client.coverage(profile) } returnsMany listOf(coverage(), coverage(rev = "d".repeat(64)))
        assertRefreshRefused { repo.refresh(profile) }
        assertEquals(original.generation, repo.generationMeta(profile)?.generation)
        assertEquals(national(id), repo.detail(id, profile, null)?.place)
    }

    @Test fun `pbe profile refuses while mixed sources coexist without merging`() = runTest {
        try {
            repo.refresh("pbe-2026-pdf-v1")
            error("pbe admitted")
        } catch (e: ChargingDiscoveryUnavailable) {
            assertEquals("pbe-rights-pending", e.code)
        }
        assertTrue(dao.places.isEmpty())
        coEvery { client.places("ocm-br-20261021-v1", null, 100, null) } returns page("44444444-4444-4444-8444-444444444444")
        repo.refresh("ocm-br-20261021-v1")
        val rows = repo.list("ocm-br-20261021-v1", null, null, null, null, 100, 0)
        assertEquals(1, rows.size)
        // Unknown connector: the row stays with UNKNOWN verdict instead of vanishing.
        assertEquals(Compatibility.UNKNOWN, rows.single().verdict)
        // An incompatible connector filter excludes the row rather than coercing it.
        assertTrue(repo.list("ocm-br-20261021-v1", null, ChargingConnector.NACS, null, null, 100, 0).isEmpty())
    }

    @Test fun `corrupt page and wrong base leave the committed generation intact`() = runTest {
        var failNext = false
        coEvery { client.places("ocm-br-20261021-v1", null, 100, null) } answers {
            if (failNext) throw com.anpfuel.data.remote.charging.ChargingSnapshotUnavailable("boom")
            page("11111111-1111-4111-8111-111111111111")
        }
        repo.refresh("ocm-br-20261021-v1")
        failNext = true
        try {
            repo.refresh("ocm-br-20261021-v1")
            error("corrupt admitted")
        } catch (e: com.anpfuel.data.remote.charging.ChargingSnapshotUnavailable) {
        }
        assertEquals(1, repo.list("ocm-br-20261021-v1", null, null, null, null, 100, 0).size)
        try {
            repo.refresh("ocm-br-20261021-v1", baseGeneration = 99L)
            error("wrong base admitted")
        } catch (e: IllegalStateException) {
        }
        assertEquals(1, repo.list("ocm-br-20261021-v1", null, null, null, null, 100, 0).size)
    }

    @Test fun `detail resolves offline and equipment counts stay distinct`() = runTest {
        coEvery { client.places("ocm-br-20261021-v1", null, 100, null) } returns page("11111111-1111-4111-8111-111111111111")
        repo.refresh("ocm-br-20261021-v1")
        coEvery { client.places(any(), any(), any(), any()) } throws AssertionError("network must stay idle offline")
        val detail = repo.detail("11111111-1111-4111-8111-111111111111", "ocm-br-20261021-v1", null)
        assertEquals("Eletroposto", detail?.place?.name?.take(11))
        assertEquals(2L, detail?.place?.reportedPoints)
        assertEquals(1, detail?.place?.groups?.size)
    }
}
