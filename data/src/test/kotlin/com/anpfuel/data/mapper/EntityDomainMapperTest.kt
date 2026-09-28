package com.anpfuel.data.mapper

import com.anpfuel.data.local.entity.AveragePriceEntity
import com.anpfuel.data.parser.dto.AveragePriceRow
import com.anpfuel.data.parser.dto.StationPriceRow
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.math.BigDecimal
import java.time.LocalDate

class EntityDomainMapperTest {

    @Test
    fun averagePriceEntityRoundTripPreservesBusinessFields() {
        val surveyWeek = SurveyWeek(LocalDate.parse("2026-06-07"), LocalDate.parse("2026-06-13"))
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        val row = AveragePriceRow(
            startDate = surveyWeek.startDate,
            endDate = surveyWeek.endDate,
            state = "SAO PAULO",
            municipality = "CAMPINAS",
            productLabel = "ETANOL HIDRATADO",
            stationCount = 42,
            unit = "R$/l",
            averagePrice = BigDecimal("3.42"),
            standardDeviation = BigDecimal("0.12"),
            minimumPrice = BigDecimal("3.10"),
            maximumPrice = BigDecimal("3.80"),
            variationCoefficient = BigDecimal("3.50"),
        )

        val entity = EntityDomainMapper.toAveragePriceEntity(row, surveyWeekId)
        val domain = EntityDomainMapper.toAveragePrice(entity, surveyWeek)

        assertEquals("SP", entity.state)
        assertEquals(FuelProduct.ETHANOL, domain.fuelProduct)
        assertEquals("CAMPINAS", domain.municipality)
        assertEquals(42, domain.stationCount)
        assertEquals(BigDecimal("3.42"), domain.average?.value)
    }

    @Test
    fun stationPriceEntityRoundTripPreservesAddressAndPrice() {
        val surveyWeek = SurveyWeek(LocalDate.parse("2026-06-07"), LocalDate.parse("2026-06-13"))
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek).value
        val row = StationPriceRow(
            cnpj = "12.345.678/0001-99",
            legalName = "Example Ltd",
            tradeName = "Example Posto",
            address = "Rua Example",
            number = "100",
            complement = "Sala 1",
            neighborhood = "Centro",
            zipCode = "01001000",
            municipality = "SÃO PAULO",
            stateName = "SAO PAULO",
            brand = "Example",
            productLabel = "GLP",
            unit = "R$/kg",
            price = BigDecimal("95.50"),
            collectedAt = LocalDate.parse("2026-06-10"),
        )

        val entity = EntityDomainMapper.toStationPriceEntity(row, surveyWeekId)
        val domain = EntityDomainMapper.toStationPrice(entity, surveyWeek)

        assertEquals("12345678000199", entity.cnpj)
        assertTrueContains(entity.address, "Rua Example")
        assertTrueContains(entity.address, "100")
        assertEquals(FuelProduct.LPG_P13, domain.fuelProduct)
        assertEquals(BigDecimal("95.50"), domain.price.value)
        assertEquals("SÃO PAULO", domain.station.municipality)
    }

    @Test
    fun surveyWeekEntityMapsToPriceSurveyAggregate() {
        val surveyWeek = SurveyWeek(LocalDate.parse("2026-06-07"), LocalDate.parse("2026-06-13"))
        val entity = EntityDomainMapper.toSurveyWeekEntity(
            surveyWeek = surveyWeek,
            summaryImportedAt = 1_000L,
            stationImportedAt = 2_000L,
        )

        val restored = EntityDomainMapper.toPriceSurvey(entity)

        assertEquals(surveyWeek, restored.surveyWeek)
        assertEquals(1_000L, restored.summaryImportedAt?.toEpochMilli())
        assertEquals(2_000L, restored.stationImportedAt?.toEpochMilli())
    }

    private fun assertTrueContains(haystack: String, needle: String) {
        assertEquals(true, haystack.contains(needle))
    }
}
