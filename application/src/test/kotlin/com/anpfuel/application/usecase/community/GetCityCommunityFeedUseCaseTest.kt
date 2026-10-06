package com.anpfuel.application.usecase.community

import com.anpfuel.application.usecase.location.PreferredLocation
import com.anpfuel.application.usecase.location.SelectLocationUseCase
import com.anpfuel.domain.community.*
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.valueobject.*
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class GetCityCommunityFeedUseCaseTest {
    private val reader = mockk<CityCommunityFeedReader>()
    private val locations = mockk<SelectLocationUseCase>()
    private val catalog = mockk<MunicipalityCatalogRepository>()
    private val useCase = GetCityCommunityFeedUseCase(reader, locations, catalog)
    @Test fun `manual preference resolves canonical city without GPS or network`() = runTest {
        coEvery { locations.getPreferredLocation() } returns PreferredLocation(BrazilianState.MATO_GROSSO, "Sorriso")
        coEvery { catalog.findCatalogEntry(BrazilianState.MATO_GROSSO, "Sorriso") } returns MunicipalityCatalogEntry(BrazilianState.MATO_GROSSO, "Sorriso", "5107925")
        assertEquals(FeedCity("5107925", BrazilianState.MATO_GROSSO, "Sorriso"), useCase.city())
        coVerify(exactly = 0) { reader.read(any(), any()) }
    }
    @Test fun `missing canonical code is not replaced by a name or invented code`() = runTest {
        coEvery { locations.getPreferredLocation() } returns PreferredLocation(BrazilianState.MATO_GROSSO, "Sorriso")
        coEvery { catalog.findCatalogEntry(BrazilianState.MATO_GROSSO, "Sorriso") } returns MunicipalityCatalogEntry(BrazilianState.MATO_GROSSO, "Sorriso")
        assertNull(useCase.city()); coVerify(exactly = 0) { reader.read(any(), any()) }
    }
}
