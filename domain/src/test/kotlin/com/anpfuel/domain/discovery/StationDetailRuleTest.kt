package com.anpfuel.domain.discovery

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
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class StationDetailRuleTest {

    private val freshWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
    private val today = LocalDate.of(2026, 6, 14)

    private fun stationPrice(
        collectedAt: LocalDate?,
        price: String = "5.19",
    ): StationPrice = StationPrice.create(
        priceSurveyId = DomainId.forSurveyWeek(freshWeek),
        surveyWeek = freshWeek,
        station = RetailStation.create(
            cnpj = Cnpj.parse("61602199002409"),
            legalName = "POSTO CENTRO LTDA",
            tradeName = "POSTO CENTRO",
            address = "RUA XV DE NOVEMBRO, 1000",
            municipality = "Curitiba",
            state = BrazilianState.PARANA,
            brand = "BR",
        ),
        fuelProduct = FuelProduct.GASOLINE_REGULAR,
        price = PriceAmount.of(price),
        collectedAt = collectedAt,
    )

    @Test
    fun `empty rows resolve to no coverage`() {
        val state = StationDetailRule.resolve(
            rows = emptyList(),
            surveyWeek = freshWeek,
            today = today,
            staleCache = false,
        )

        assertTrue(state is StationDetailState.NoCoverage)
    }

    @Test
    fun `fresh week resolves fresh rows with unknown community`() {
        val state = StationDetailRule.resolve(
            rows = listOf(stationPrice(LocalDate.of(2026, 6, 10))),
            surveyWeek = freshWeek,
            today = today,
            staleCache = false,
        )

        assertTrue(state is StationDetailState.AnpReference)
        state as StationDetailState.AnpReference
        assertFalse(state.weekStale)
        assertFalse(state.communityDisputed)
        assertEquals(listOf(StationRowFreshness.FRESH), state.rows.map { it.freshness })
    }

    @Test
    fun `old week marks rows stale`() {
        val oldWeek = SurveyWeek.fromIsoDates("2026-01-04", "2026-01-10")
        val state = StationDetailRule.resolve(
            rows = listOf(stationPrice(LocalDate.of(2026, 1, 8))),
            surveyWeek = oldWeek,
            today = today,
            staleCache = false,
        )

        assertTrue(state is StationDetailState.AnpReference)
        state as StationDetailState.AnpReference
        assertTrue(state.weekStale)
        assertEquals(listOf(StationRowFreshness.STALE), state.rows.map { it.freshness })
    }

    @Test
    fun `stale cache marks fresh week stale`() {
        val state = StationDetailRule.resolve(
            rows = listOf(stationPrice(LocalDate.of(2026, 6, 10))),
            surveyWeek = freshWeek,
            today = today,
            staleCache = true,
        )

        assertTrue(state is StationDetailState.AnpReference)
        state as StationDetailState.AnpReference
        assertTrue(state.weekStale)
        assertEquals(listOf(StationRowFreshness.STALE), state.rows.map { it.freshness })
    }

    @Test
    fun `missing collection date is unknown, not fresh`() {
        val state = StationDetailRule.resolve(
            rows = listOf(stationPrice(null)),
            surveyWeek = freshWeek,
            today = today,
            staleCache = false,
        )

        assertTrue(state is StationDetailState.AnpReference)
        state as StationDetailState.AnpReference
        assertFalse(state.weekStale)
        assertEquals(listOf(StationRowFreshness.UNKNOWN), state.rows.map { it.freshness })
    }

    @Test
    fun `disputed community passes through without inventing data`() {
        val state = StationDetailRule.resolve(
            rows = listOf(stationPrice(LocalDate.of(2026, 6, 10))),
            surveyWeek = freshWeek,
            today = today,
            staleCache = false,
            communityDisputed = true,
        )

        assertTrue(state is StationDetailState.AnpReference)
        assertTrue((state as StationDetailState.AnpReference).communityDisputed)
    }
}
