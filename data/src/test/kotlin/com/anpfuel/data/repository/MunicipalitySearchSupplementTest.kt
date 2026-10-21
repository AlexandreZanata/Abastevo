package com.anpfuel.data.repository

import com.anpfuel.data.local.catalog.MunicipalityCatalogSeeder
import com.anpfuel.data.local.dao.MunicipalityCatalogDao
import com.anpfuel.data.local.dao.MunicipalityFtsDao
import com.anpfuel.data.local.entity.MunicipalityCatalogEntity
import com.anpfuel.data.local.model.MunicipalityFtsRow
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.async
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows

/**
 * MEX-05 — Bounded typo supplement. The retired per-keystroke full-catalog
 * scan is replaced by indexed normalized-prefix ranges; ranking still
 * filters by match type off the main thread.
 */
class MunicipalitySearchSupplementTest {

    private val municipalityFtsDao = mockk<MunicipalityFtsDao>()
    private val municipalityCatalogDao = mockk<MunicipalityCatalogDao>()
    private val catalogSeeder = mockk<MunicipalityCatalogSeeder>()

    private lateinit var repository: MunicipalitySearchRepositoryImpl

    private val sorocabaEntity = MunicipalityCatalogEntity(
        id = "sp-3509502",
        ibgeCode = "3509502",
        state = "SP",
        municipality = "Sorocaba",
        normalizedName = "SOROCABA",
    )
    private val sorrisoEntity = MunicipalityCatalogEntity(
        id = "mt-5107925",
        ibgeCode = "5107925",
        state = "MT",
        municipality = "Sorriso",
        normalizedName = "SORRISO",
    )

    @BeforeEach
    fun setUp() {
        repository = MunicipalitySearchRepositoryImpl(
            municipalityFtsDao = municipalityFtsDao,
            municipalityCatalogDao = municipalityCatalogDao,
            catalogSeeder = catalogSeeder,
        )
        coEvery { catalogSeeder.seedIfEmpty() } returns Unit
        coEvery { municipalityFtsDao.search(any(), any()) } returns emptyList()
        coEvery {
            municipalityCatalogDao.findByNormalizedPrefix(any(), any(), any())
        } returns emptyList()
    }

    @Test
    fun supplementUsesBoundedStemsInsteadOfFullScan() = runTest {
        coEvery {
            municipalityCatalogDao.findByNormalizedPrefix("SO", "SO\uFFFF", 1000)
        } returns listOf(sorocabaEntity, sorrisoEntity)

        val results = repository.search("SORISO", limit = 20)

        // Documented typo survives: SORISO resolves Sorriso (MT 5107925).
        assertTrue(results.any { it.municipality == "Sorriso" })
        coVerify(exactly = 1) {
            municipalityCatalogDao.findByNormalizedPrefix("SO", "SO\uFFFF", 1000)
        }
    }

    @Test
    fun shortTokensUseSingleCharacterStems() = runTest {
        repository.search("SAO", limit = 20)

        // "SAO" (3 chars) uses the "S" stem; single-char "S*" stays bounded.
        coVerify(exactly = 1) {
            municipalityCatalogDao.findByNormalizedPrefix("S", "S\uFFFF", 1000)
        }
    }

    @Test
    fun multiTokenQueryQueriesFirstTwoStems() = runTest {
        repository.search("san paolo", limit = 20)

        coVerify(exactly = 1) {
            municipalityCatalogDao.findByNormalizedPrefix("S", "S\uFFFF", 1000)
        }
        coVerify(exactly = 1) {
            municipalityCatalogDao.findByNormalizedPrefix("PA", "PA\uFFFF", 1000)
        }
    }

    @Test
    fun adversarialQueryStaysBounded() = runTest {
        val adversarial = "z".repeat(128)

        val results = repository.search(adversarial, limit = 20)

        assertEquals(0, results.size)
        // Only the 2-char stem reaches the database, never the 128-char input.
        coVerify(exactly = 1) {
            municipalityCatalogDao.findByNormalizedPrefix("ZZ", "ZZ\uFFFF", 1000)
        }
    }

    @Test
    fun supplementSkipsFtsDuplicates() = runTest {
        coEvery {
            municipalityFtsDao.search(any(), any())
        } returns listOf(MunicipalityFtsRow(municipality = "Sorocaba", state = "SP"))
        coEvery {
            municipalityCatalogDao.findByStateMunicipalityKeys(any())
        } returns listOf(sorocabaEntity)
        coEvery {
            municipalityCatalogDao.findByNormalizedPrefix(any(), any(), any())
        } returns listOf(sorocabaEntity, sorrisoEntity)

        val results = repository.search("SOR", limit = 20)

        assertEquals(1, results.count { it.municipality == "Sorocaba" })
        assertTrue(results.any { it.municipality == "Sorriso" })
    }

    @Test
    fun searchIsCancellable() = runTest {
        coEvery {
            municipalityCatalogDao.findByNormalizedPrefix(any(), any(), any())
        } coAnswers {
            delay(5_000)
            emptyList()
        }

        val job = async { repository.search("SORISO", limit = 20) }
        job.cancel()

        assertThrows<CancellationException> {
            job.await()
        }
    }
}
