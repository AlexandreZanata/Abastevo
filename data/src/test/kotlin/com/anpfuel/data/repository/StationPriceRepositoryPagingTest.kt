package com.anpfuel.data.repository

import com.anpfuel.data.local.dao.StationPriceDao
import com.anpfuel.data.local.dao.SurveyWeekDao
import com.anpfuel.data.local.entity.StationPriceEntity
import com.anpfuel.domain.discovery.DiscoveryQuery
import com.anpfuel.domain.discovery.DiscoverySort
import com.anpfuel.domain.discovery.DiscoveryStationsRule
import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class StationPriceRepositoryPagingTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value

    private val dao: StationPriceDao = mockk()
    private val weekDao: SurveyWeekDao = mockk(relaxed = true)
    private val repository = StationPriceRepositoryImpl(dao, weekDao)

    private fun entity(
        cnpj: String = "61602199002409",
        price: Double = 5.19,
        tradeName: String? = "Posto Centro",
    ) = StationPriceEntity(
        id = "$surveyWeekId|$cnpj|GASOLINE_REGULAR",
        surveyWeekId = surveyWeekId,
        cnpj = cnpj,
        legalName = "Posto Legal",
        tradeName = tradeName,
        address = "Rua A",
        municipality = "SAO PAULO",
        state = "SP",
        brand = "BR",
        fuelProduct = "GASOLINE_REGULAR",
        price = price,
        collectedAt = "2026-06-10",
    )

    @Test
    fun `page routes price sort with match-all search`() = runTest {
        coEvery {
            dao.findPageByPrice(surveyWeekId, "SP", "SAO PAULO", "GASOLINE_REGULAR", "", 20, 0)
        } returns listOf(entity())
        coEvery {
            dao.countPage(surveyWeekId, "SP", "SAO PAULO", "GASOLINE_REGULAR", "")
        } returns 1

        val page = repository.getStationPricesPage(
            state = BrazilianState.SAO_PAULO,
            municipality = "SAO PAULO",
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            surveyWeek = surveyWeek,
            search = null,
            sort = DiscoverySort.PRICE_ASC,
            page = 0,
            pageSize = 20,
        )

        assertEquals(1, page.items.size)
        assertEquals(1, page.totalCount)
        assertEquals(false, page.hasMore)
    }

    @Test
    fun `short search matches everything and wildcards stay literal`() = runTest {
        val search = slot<String>()
        coEvery {
            dao.findPageByPrice(surveyWeekId, "SP", "SAO PAULO", "GASOLINE_REGULAR", capture(search), 20, 0)
        } returns emptyList()
        coEvery {
            dao.countPage(surveyWeekId, "SP", "SAO PAULO", "GASOLINE_REGULAR", any())
        } returns 0

        repository.getStationPricesPage(
            BrazilianState.SAO_PAULO, "São Paulo", FuelProduct.GASOLINE_REGULAR, surveyWeek,
            search = "a", sort = DiscoverySort.PRICE_ASC, page = 0, pageSize = 20,
        )
        assertEquals("", search.captured)

        repository.getStationPricesPage(
            BrazilianState.SAO_PAULO, "São Paulo", FuelProduct.GASOLINE_REGULAR, surveyWeek,
            search = "100%", sort = DiscoverySort.PRICE_ASC, page = 0, pageSize = 20,
        )
        assertEquals("100\\%", search.captured)
    }

    @Test
    fun `recency sort routes to the recency window`() = runTest {
        coEvery {
            dao.findPageByRecency(any(), any(), any(), any(), any(), any(), any())
        } returns listOf(entity())
        coEvery { dao.countPage(any(), any(), any(), any(), any()) } returns 1

        val page = repository.getStationPricesPage(
            BrazilianState.SAO_PAULO, "São Paulo", FuelProduct.GASOLINE_REGULAR, surveyWeek,
            search = null, sort = DiscoverySort.RECENCY_DESC, page = 1, pageSize = 20,
        )

        coVerify {
            dao.findPageByRecency(surveyWeekId, "SP", "SAO PAULO", "GASOLINE_REGULAR", "", 20, 20)
        }
        assertTrue(page.items.isNotEmpty())
    }

    @Test
    fun `hasMore reflects the total count`() = runTest {
        coEvery {
            dao.findPageByPrice(any(), any(), any(), any(), any(), 20, 0)
        } returns List(20) { entity(cnpj = it.toString().padStart(14, '0')) }
        coEvery { dao.countPage(any(), any(), any(), any(), any()) } returns 21

        val page: DiscoveryStationsRule.DiscoveryPage = repository.getStationPricesPage(
            BrazilianState.SAO_PAULO, "São Paulo", FuelProduct.GASOLINE_REGULAR, surveyWeek,
            search = null, sort = DiscoverySort.PRICE_ASC, page = 0, pageSize = 20,
        )

        assertEquals(21, page.totalCount)
        assertTrue(page.hasMore)
    }

    @Test
    fun `invalid windows are refused before IO`() = runTest {
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking {
                repository.getStationPricesPage(
                    BrazilianState.SAO_PAULO, "São Paulo", FuelProduct.GASOLINE_REGULAR, surveyWeek,
                    search = null, sort = DiscoverySort.PRICE_ASC, page = -1, pageSize = 20,
                )
            }
        }
        assertThrows(DomainException::class.java) {
            kotlinx.coroutines.runBlocking {
                repository.getStationPricesPage(
                    BrazilianState.SAO_PAULO, "São Paulo", FuelProduct.GASOLINE_REGULAR, surveyWeek,
                    search = null, sort = DiscoverySort.PRICE_ASC, page = 0,
                    pageSize = DiscoveryQuery.MAX_PAGE_SIZE + 1,
                )
            }
        }
        coVerify(exactly = 0) { dao.findPageByPrice(any(), any(), any(), any(), any(), any(), any()) }
    }

    @Test
    fun `targeted lookup maps the row or null`() = runTest {
        coEvery {
            dao.findByCnpj(surveyWeekId, "SP", "SAO PAULO", "GASOLINE_REGULAR", "61602199002409")
        } returns entity()

        val found: StationPrice? = repository.findStationPrice(
            BrazilianState.SAO_PAULO, "São Paulo", FuelProduct.GASOLINE_REGULAR, surveyWeek, "61602199002409",
        )
        assertEquals("61602199002409", found?.station?.cnpj?.digits)

        coEvery {
            dao.findByCnpj(any(), any(), any(), any(), any())
        } returns null
        assertNull(
            repository.findStationPrice(
                BrazilianState.SAO_PAULO, "São Paulo", FuelProduct.GASOLINE_REGULAR, surveyWeek, "00000000000000",
            ),
        )
    }
}
