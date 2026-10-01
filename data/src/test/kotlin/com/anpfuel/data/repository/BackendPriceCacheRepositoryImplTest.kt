package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.BackendPriceCacheDao
import com.anpfuel.data.local.entity.BackendPriceCacheEntity
import com.anpfuel.data.remote.BackendPriceGroupsJsonCodec
import com.anpfuel.domain.model.BackendPriceGroups
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class BackendPriceCacheRepositoryImplTest {

    private val stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26"
    private val dao: BackendPriceCacheDao = mockk(relaxed = true)

    private fun groups() = BackendPriceGroups.create(
        stationId = stationId,
        fuelFilterWire = "ETHANOL",
        groups = emptyList(),
        fetchedAtMillis = 1_000_000L,
        expiresAtMillis = 1_060_000L,
    )

    @Test
    fun `save stores source version expiry and load decodes`() = runTest {
        val repo = BackendPriceCacheRepositoryImpl(dao)
        val saved = slot<BackendPriceCacheEntity>()
        coEvery { dao.upsert(capture(saved)) } returns Unit
        coEvery { dao.findByKey("$stationId|ETHANOL") } answers {
            saved.captured
        }

        repo.save(groups())
        val loaded = repo.load(stationId, com.anpfuel.domain.valueobject.FuelProduct.ETHANOL)

        coVerify { dao.upsert(any()) }
        checkNotNull(loaded)
        assertEquals("backend", loaded.source)
        assertEquals("v1", loaded.version)
        assertEquals(1_000_000L, loaded.fetchedAtMillis)
        assertEquals(1_060_000L, loaded.expiresAtMillis)
        assertTrue(saved.captured.payloadJson.contains("items"))
    }

    @Test
    fun `no cache returns null`() = runTest {
        coEvery { dao.findByKey(any()) } returns null
        val repo = BackendPriceCacheRepositoryImpl(dao)

        assertNull(repo.load(stationId, null))
    }

    @Test
    fun `codec round-trip keeps payload parseable`() {
        val encoded = BackendPriceGroupsJsonCodec.encode(groups())
        val decoded = BackendPriceGroupsJsonCodec.decode(
            payload = encoded,
            stationId = stationId,
            fuelFilterWire = "ETHANOL",
            fetchedAtMillis = 1_000_000L,
            expiresAtMillis = 1_060_000L,
        )

        assertEquals("ETHANOL", decoded.fuelFilterWire)
    }
}
