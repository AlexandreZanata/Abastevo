package com.anpfuel.data.repository

import com.anpfuel.data.local.entity.StationPriceEntity
import com.anpfuel.data.mapper.EntityDomainMapper
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.ServerStationPage
import com.anpfuel.domain.discovery.StationLocationQuality
import com.anpfuel.domain.repository.MunicipalityCatalogRepository
import com.anpfuel.domain.repository.ServerStationCache
import com.anpfuel.domain.repository.ServerStationGateway
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.MunicipalityCatalogEntry
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.io.IOException
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class CanonicalStationNamesTest {
    private val gateway = mockk<ServerStationGateway>()
    private val cache = mockk<ServerStationCache>()
    private val catalog = mockk<MunicipalityCatalogRepository>()
    private val state = BrazilianState.SAO_PAULO
    private val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val weekId = DomainId.forSurveyWeek(week).value
    private val original = EntityDomainMapper.toStationPrice(StationPriceEntity(
        id = "$weekId|61602199002409|GASOLINE_REGULAR", surveyWeekId = weekId,
        cnpj = "61602199002409", legalName = "Old Legal", tradeName = "Old label",
        address = "Old Street", municipality = "SAO PAULO", state = "SP", brand = "BR",
        fuelProduct = "GASOLINE_REGULAR", price = 5.19, collectedAt = "2026-06-10",
    ), week)
    private val canonical = ServerStation.create("00000000-0000-4000-8000-000000000001", "Synthetic Trade",
        StationLocationQuality.UNKNOWN, null,null,"61602199002409","3550308","SP",null,
        mapOf("street" to "Synthetic Avenue", "number" to "1"),"Synthetic Legal")
    private var stored: ServerStationPage? = null
    private fun resolver(): CanonicalStationNames {
        coEvery { catalog.findCatalogEntry(state,"SAO PAULO") } returns MunicipalityCatalogEntry(state,"SAO PAULO","3550308")
        coEvery { cache.loadScopedPage("station-names:3550308") } answers { stored }
        coEvery { cache.saveScopedPage("station-names:3550308",any()) } answers { stored=secondArg();Unit }
        return CanonicalStationNames(gateway,cache,catalog)
    }

    @Test fun `one bounded city traversal updates all consumers without changing price identity`() = runTest {
        val names=resolver()
        coEvery { gateway.listInMunicipality("3550308",100,null) } returns ServerStationPage(listOf(canonical),"next")
        coEvery { gateway.listInMunicipality("3550308",100,"next") } returns ServerStationPage(emptyList(),null)
        val updated=names.enrich(listOf(original),state,"SAO PAULO").single()
        assertEquals("Synthetic Trade",updated.station.displayName())
        assertEquals("Synthetic Legal",updated.station.secondaryLegalName)
        assertEquals("Synthetic Avenue, 1",updated.station.address)
        assertSame(original.price,updated.price)
        assertEquals(original.id,updated.id)
        assertEquals(original.collectedAt,updated.collectedAt)
        assertEquals(original.surveyWeek,updated.surveyWeek)
        assertEquals(updated.station.displayName(),names.enrich(listOf(original),state,"SAO PAULO").single().station.displayName())
        coVerify(exactly=2) { gateway.listInMunicipality(any(),any(),any()) }
        coVerify(exactly=0) { gateway.detail(any()) }
    }

    @Test fun `partial failed traversal keeps complete offline names and never saves a partial page`() = runTest {
        stored=ServerStationPage(listOf(canonical),null)
        val names=resolver()
        coEvery { gateway.listInMunicipality("3550308",100,null) } returns ServerStationPage(emptyList(),"next")
        coEvery { gateway.listInMunicipality("3550308",100,"next") } throws IOException("synthetic offline")
        assertEquals("Synthetic Trade",names.enrich(listOf(original),state,"SAO PAULO").single().station.displayName())
        coVerify(exactly=0) { cache.saveScopedPage(any(),any()) }
    }

    @Test fun `foreign CNPJ and source cancellation cannot rename a price row`() = runTest {
        val names=resolver()
        coEvery { gateway.listInMunicipality("3550308",100,null) } returns ServerStationPage(listOf(
            ServerStation.create("00000000-0000-4000-8000-000000000002","Wrong branch",StationLocationQuality.UNKNOWN,
                null,null,"61602199000104","3550308","SP",null)),null)
        assertSame(original,names.enrich(listOf(original),state,"SAO PAULO").single())
        val cancelled=resolver()
        coEvery { gateway.listInMunicipality("3550308",100,null) } throws CancellationException("synthetic cancellation")
        try { cancelled.enrich(listOf(original),state,"SAO PAULO");fail("cancellation swallowed") }
        catch (_: CancellationException) { /* propagated */ }
    }
}
