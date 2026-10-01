package com.anpfuel.domain.discovery

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.LocalDate
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class DiscoveryStationsRuleTest {

    private val week = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    private fun stationPrice(
        tradeName: String,
        price: String,
        brand: String? = "BR",
        address: String = "RUA XV DE NOVEMBRO, 1000",
        collectedAt: LocalDate? = LocalDate.of(2026, 6, 10),
    ): StationPrice = StationPrice.create(
        priceSurveyId = DomainId.forSurveyWeek(week),
        surveyWeek = week,
        station = RetailStation.create(
            cnpj = Cnpj.parse("61602199002409"),
            legalName = "RAZAO $tradeName",
            tradeName = tradeName,
            address = address,
            municipality = "Curitiba",
            state = BrazilianState.PARANA,
            brand = brand,
        ),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        price = PriceAmount.of(price),
        collectedAt = collectedAt,
    )

    @Test
    fun `blank search keeps every station in price order`() {
        val stations = listOf(
            stationPrice("POSTO B", "5.99"),
            stationPrice("POSTO A", "5.19"),
        )

        val result = DiscoveryStationsRule.filterAndSort(stations, search = "  ", sort = DiscoverySort.PRICE_ASC)

        assertEquals(listOf("POSTO A", "POSTO B"), result.map { it.station.displayName() })
    }

    @Test
    fun `search below minimum length does not exclude stations`() {
        val stations = listOf(stationPrice("POSTO CENTRO", "5.19"))

        val result = DiscoveryStationsRule.filterAndSort(stations, search = "x", sort = DiscoverySort.PRICE_ASC)

        assertEquals(1, result.size)
    }

    @Test
    fun `search matches trade name brand and address case-insensitively`() {
        val stations = listOf(
            stationPrice("POSTO CENTRO", "5.19", brand = "BR"),
            stationPrice("POSTO LESTE", "5.29", brand = "SHELL", address = "AVENIDA CENTRO, 10"),
            stationPrice("POSTO OESTE", "5.39", brand = "IPIRANGA"),
        )

        assertEquals(
            listOf("POSTO CENTRO", "POSTO LESTE"),
            DiscoveryStationsRule.filterAndSort(stations, search = "centro", sort = DiscoverySort.PRICE_ASC)
                .map { it.station.displayName() },
        )
        assertEquals(
            listOf("POSTO LESTE"),
            DiscoveryStationsRule.filterAndSort(stations, search = "shell", sort = DiscoverySort.PRICE_ASC)
                .map { it.station.displayName() },
        )
    }

    @Test
    fun `search without match returns honest empty list`() {
        val stations = listOf(stationPrice("POSTO CENTRO", "5.19"))

        val result = DiscoveryStationsRule.filterAndSort(
            stations,
            search = "inexistente",
            sort = DiscoverySort.PRICE_ASC,
        )

        assertTrue(result.isEmpty())
    }

    @Test
    fun `recency sort puts newest first and unknown dates last`() {
        val stations = listOf(
            stationPrice("OLD", "5.19", collectedAt = LocalDate.of(2026, 6, 8)),
            stationPrice("UNKNOWN", "5.29", collectedAt = null),
            stationPrice("NEW", "5.39", collectedAt = LocalDate.of(2026, 6, 12)),
        )

        val result = DiscoveryStationsRule.filterAndSort(stations, search = null, sort = DiscoverySort.RECENCY_DESC)

        assertEquals(listOf("NEW", "OLD", "UNKNOWN"), result.map { it.station.displayName() })
    }

    @Test
    fun `paginate slices pages and reports hasMore`() {
        val stations = (1..5).map { stationPrice("POSTO $it", "5.1$it") }

        val first = DiscoveryStationsRule.paginate(stations, page = 0, pageSize = 2)
        val last = DiscoveryStationsRule.paginate(stations, page = 2, pageSize = 2)
        val beyond = DiscoveryStationsRule.paginate(stations, page = 3, pageSize = 2)

        assertEquals(2, first.items.size)
        assertTrue(first.hasMore)
        assertEquals(5, first.totalCount)
        assertEquals(1, last.items.size)
        assertFalse(last.hasMore)
        assertTrue(beyond.items.isEmpty())
        assertFalse(beyond.hasMore)
    }

    @Test
    fun `paginate rejects out-of-range bounds`() {
        val stations = listOf(stationPrice("POSTO CENTRO", "5.19"))

        assertThrows(DomainException::class.java) {
            DiscoveryStationsRule.paginate(stations, page = -1, pageSize = 2)
        }
        assertThrows(DomainException::class.java) {
            DiscoveryStationsRule.paginate(stations, page = 0, pageSize = 0)
        }
        assertThrows(DomainException::class.java) {
            DiscoveryStationsRule.paginate(
                stations,
                page = 0,
                pageSize = DiscoveryQuery.MAX_PAGE_SIZE + 1,
            )
        }
    }
}
