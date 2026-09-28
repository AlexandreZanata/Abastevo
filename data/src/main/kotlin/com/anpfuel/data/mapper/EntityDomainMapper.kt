package com.anpfuel.data.mapper

import com.anpfuel.data.local.entity.AveragePriceEntity
import com.anpfuel.data.local.entity.StationPriceEntity
import com.anpfuel.data.local.entity.SurveyWeekEntity
import com.anpfuel.data.parser.dto.AveragePriceRow
import com.anpfuel.data.parser.dto.StationPriceRow
import com.anpfuel.domain.model.AveragePrice
import com.anpfuel.domain.model.PriceSurvey
import com.anpfuel.domain.model.RetailStation
import com.anpfuel.domain.model.StationPrice
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeographicScope
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.SurveyWeek
import java.time.Instant
import java.time.LocalDate
import java.util.UUID

/**
 * Maps Room infrastructure entities ↔ domain models at the data boundary.
 */
object EntityDomainMapper {

    fun toSurveyWeekEntity(
        surveyWeek: SurveyWeek,
        summaryImportedAt: Long,
        stationImportedAt: Long? = null,
    ): SurveyWeekEntity =
        SurveyWeekEntity(
            id = DomainId.forSurveyWeek(surveyWeek).value,
            startDate = surveyWeek.startDate.toString(),
            endDate = surveyWeek.endDate.toString(),
            summaryImportedAt = summaryImportedAt,
            stationImportedAt = stationImportedAt,
        )

    fun toAveragePriceEntity(row: AveragePriceRow, surveyWeekId: String): AveragePriceEntity {
        val state = AnpStateMapper.toAbbreviation(row.state)
        val fuelProduct = AnpProductMapper.toFuelProductOrThrow(row.productLabel)
        return AveragePriceEntity(
            id = averagePriceId(surveyWeekId, state, row.municipality, fuelProduct.name),
            surveyWeekId = surveyWeekId,
            state = state,
            municipality = row.municipality.trim(),
            fuelProduct = fuelProduct.name,
            stationCount = row.stationCount,
            unit = row.unit?.trim(),
            avgPrice = row.averagePrice?.toDouble(),
            minPrice = row.minimumPrice?.toDouble(),
            maxPrice = row.maximumPrice?.toDouble(),
            stdDev = row.standardDeviation?.toDouble(),
        )
    }

    fun toStationPriceEntity(row: StationPriceRow, surveyWeekId: String): StationPriceEntity {
        val state = AnpStateMapper.toAbbreviation(row.stateName)
        val fuelProduct = AnpProductMapper.toFuelProductOrThrow(row.productLabel)
        val cnpjDigits = AnpCnpjMapper.normalizeDigits(row.cnpj)
        return StationPriceEntity(
            id = stationPriceId(surveyWeekId, cnpjDigits, fuelProduct.name),
            surveyWeekId = surveyWeekId,
            cnpj = cnpjDigits,
            legalName = row.legalName?.trim(),
            tradeName = row.tradeName?.trim(),
            address = formatStationAddress(row),
            municipality = row.municipality.trim(),
            state = state,
            brand = row.brand?.trim(),
            fuelProduct = fuelProduct.name,
            price = row.price.toDouble(),
            collectedAt = row.collectedAt?.toString(),
        )
    }

    fun toPriceSurvey(entity: SurveyWeekEntity): PriceSurvey =
        PriceSurvey.restore(
            id = DomainId.from(entity.id),
            surveyWeek = SurveyWeek.fromIsoDates(entity.startDate, entity.endDate),
            summaryImportedAt = Instant.ofEpochMilli(entity.summaryImportedAt),
            stationImportedAt = entity.stationImportedAt?.let(Instant::ofEpochMilli),
        )

    fun toAveragePrice(entity: AveragePriceEntity, surveyWeek: SurveyWeek): AveragePrice =
        AveragePrice.create(
            id = DomainId.from(entity.id),
            priceSurveyId = DomainId.from(entity.surveyWeekId),
            surveyWeek = surveyWeek,
            state = BrazilianState.fromAbbreviation(entity.state)
                ?: error("Invalid stored state abbreviation: ${entity.state}"),
            municipality = entity.municipality,
            fuelProduct = FuelProduct.valueOf(entity.fuelProduct),
            geographicScope = GeographicScope.MUNICIPALITY,
            stationCount = entity.stationCount,
            unit = entity.unit,
            average = entity.avgPrice?.let(PriceAmount::of),
            minimum = entity.minPrice?.let(PriceAmount::of),
            maximum = entity.maxPrice?.let(PriceAmount::of),
            standardDeviation = entity.stdDev?.let(PriceAmount::of),
        )

    fun toStationPrice(entity: StationPriceEntity, surveyWeek: SurveyWeek): StationPrice {
        val state = BrazilianState.fromAbbreviation(entity.state)
            ?: error("Invalid stored state abbreviation: ${entity.state}")
        val station = RetailStation.create(
            cnpj = AnpCnpjMapper.parse(entity.cnpj),
            legalName = entity.legalName,
            tradeName = entity.tradeName,
            address = entity.address,
            municipality = entity.municipality,
            state = state,
            brand = entity.brand,
        )
        return StationPrice.create(
            id = DomainId.from(entity.id),
            priceSurveyId = DomainId.from(entity.surveyWeekId),
            surveyWeek = surveyWeek,
            station = station,
            fuelProduct = FuelProduct.valueOf(entity.fuelProduct),
            price = PriceAmount.of(entity.price),
            collectedAt = entity.collectedAt?.let(LocalDate::parse),
        )
    }

    fun toAveragePriceEntity(price: AveragePrice): AveragePriceEntity =
        AveragePriceEntity(
            id = price.id.value,
            surveyWeekId = price.priceSurveyId.value,
            state = price.state.abbreviation,
            municipality = price.municipality,
            fuelProduct = price.fuelProduct.name,
            stationCount = price.stationCount,
            unit = price.unit,
            avgPrice = price.average?.value?.toDouble(),
            minPrice = price.minimum?.value?.toDouble(),
            maxPrice = price.maximum?.value?.toDouble(),
            stdDev = price.standardDeviation?.value?.toDouble(),
        )

    fun toStationPriceEntity(price: StationPrice): StationPriceEntity =
        StationPriceEntity(
            id = price.id.value,
            surveyWeekId = price.priceSurveyId.value,
            cnpj = price.station.cnpj.digits,
            legalName = price.station.legalName,
            tradeName = price.station.tradeName,
            address = price.station.address,
            municipality = price.station.municipality,
            state = price.station.state.abbreviation,
            brand = price.station.brand,
            fuelProduct = price.fuelProduct.name,
            price = price.price.value.toDouble(),
            collectedAt = price.collectedAt?.toString(),
        )

    fun toSurveyWeekEntity(priceSurvey: PriceSurvey): SurveyWeekEntity =
        SurveyWeekEntity(
            id = priceSurvey.id.value,
            startDate = priceSurvey.surveyWeek.startDate.toString(),
            endDate = priceSurvey.surveyWeek.endDate.toString(),
            summaryImportedAt = priceSurvey.summaryImportedAt?.toEpochMilli()
                ?: error("PriceSurvey summaryImportedAt is required"),
            stationImportedAt = priceSurvey.stationImportedAt?.toEpochMilli(),
        )

    fun formatStationAddress(row: StationPriceRow): String {
        val parts = buildList {
            add(row.address.trim())
            row.number?.takeIf { it.isNotBlank() }?.let { add("nº $it") }
            row.complement?.takeIf { it.isNotBlank() }?.let { add(it.trim()) }
            row.neighborhood?.takeIf { it.isNotBlank() }?.let { add(row.neighborhood.trim()) }
        }
        return parts.joinToString(", ")
    }

    private fun averagePriceId(
        surveyWeekId: String,
        state: String,
        municipality: String,
        fuelProduct: String,
    ): String = deterministicId("$surveyWeekId|$state|$municipality|$fuelProduct")

    private fun stationPriceId(
        surveyWeekId: String,
        cnpjDigits: String,
        fuelProduct: String,
    ): String = deterministicId("$surveyWeekId|$cnpjDigits|$fuelProduct")

    private fun deterministicId(key: String): String =
        UUID.nameUUIDFromBytes(key.toByteArray()).toString()
}
