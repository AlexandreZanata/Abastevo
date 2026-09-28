package com.anpfuel.domain.model

import com.anpfuel.domain.exception.DomainException
import com.anpfuel.domain.valueobject.DataAvailability
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.TankCapacity
import com.anpfuel.domain.valueobject.VehiclePriceSource
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.GeographicScope
import com.anpfuel.domain.valueobject.PriceAmount
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.time.Instant

class DomainModelCoverageTest {

    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @Test
    fun municipalitySearchResultHoldsStateAndMunicipality() {
        val result = MunicipalitySearchResult(
            municipality = "Curitiba",
            state = BrazilianState.PARANA,
            dataAvailability = DataAvailability.HAS_DATA,
        )

        assertEquals("Curitiba", result.municipality)
        assertEquals(BrazilianState.PARANA, result.state)
        assertEquals(result, result.copy())
        assertNotEquals(result, result.copy(municipality = "São Paulo"))
    }

    @Test
    fun storageUsageTracksImportedRowCounts() {
        val usage = StorageUsage(
            summaryRowCount = 2344,
            stationRowCount = 19676,
            importedWeekCount = 3,
        )

        assertEquals(2344, usage.summaryRowCount)
        assertEquals(19676, usage.stationRowCount)
        assertEquals(3, usage.importedWeekCount)
        assertEquals(usage, usage.copy())
    }

    @Test
    fun priceTableCreateStoresMetadata() {
        val id = DomainId.generate()
        val table = PriceTable.create(
            surveyWeek = surveyWeek,
            tableType = PriceTableType.WEEKLY_SUMMARY,
            sourceUrl = "https://example.com/resumo.xlsx",
            checksum = "abc123",
            id = id,
        )

        assertEquals(id, table.id)
        assertEquals(surveyWeek, table.surveyWeek)
        assertEquals(PriceTableType.WEEKLY_SUMMARY, table.tableType)
        assertEquals("https://example.com/resumo.xlsx", table.sourceUrl)
        assertEquals("abc123", table.checksum)
    }

    @Test
    fun retailStationDisplayNameFallsBackToLegalNameAndCnpj() {
        val legalOnly = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = "POSTO LEGAL",
            tradeName = null,
            address = "RUA A",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = " BRAND ",
        )
        val cnpjOnly = RetailStation.create(
            cnpj = Cnpj.parse("12345678000195"),
            legalName = null,
            tradeName = "  ",
            address = "RUA A",
            municipality = "CURITIBA",
            state = BrazilianState.PARANA,
            brand = null,
        )

        assertEquals("POSTO LEGAL", legalOnly.displayName())
        assertEquals("12.345.678/0001-95", cnpjOnly.displayName())
        assertEquals("BRAND", legalOnly.brand)
    }

    @Test
    fun retailStationRejectsBlankAddressOrMunicipality() {
        assertThrows(DomainException::class.java) {
            RetailStation.create(
                cnpj = Cnpj.parse("12345678000195"),
                legalName = null,
                tradeName = null,
                address = " ",
                municipality = "CURITIBA",
                state = BrazilianState.PARANA,
                brand = null,
            )
        }
        assertThrows(DomainException::class.java) {
            RetailStation.create(
                cnpj = Cnpj.parse("12345678000195"),
                legalName = null,
                tradeName = null,
                address = "RUA A",
                municipality = " ",
                state = BrazilianState.PARANA,
                brand = null,
            )
        }
    }

    @Test
    fun averagePriceValidatesMunicipalityAndStationCount() {
        val priceSurveyId = DomainId.forSurveyWeek(surveyWeek)

        assertThrows(DomainException::class.java) {
            AveragePrice.create(
                priceSurveyId = priceSurveyId,
                surveyWeek = surveyWeek,
                state = BrazilianState.PARANA,
                municipality = " ",
                fuelProduct = FuelProduct.ETHANOL,
            )
        }
        assertThrows(DomainException::class.java) {
            AveragePrice.create(
                priceSurveyId = priceSurveyId,
                surveyWeek = surveyWeek,
                state = BrazilianState.PARANA,
                municipality = "CURITIBA",
                fuelProduct = FuelProduct.ETHANOL,
                stationCount = -1,
            )
        }
    }

    @Test
    fun averagePriceHasStatisticsWhenMinimumOrMaximumPresent() {
        val priceSurveyId = DomainId.forSurveyWeek(surveyWeek)
        val withMinimum = AveragePrice.create(
            priceSurveyId = priceSurveyId,
            surveyWeek = surveyWeek,
            state = BrazilianState.PARANA,
            municipality = "CURITIBA",
            fuelProduct = FuelProduct.ETHANOL,
            minimum = PriceAmount.of("3.10"),
            unit = " R$/l ",
            geographicScope = GeographicScope.STATE,
        )

        assertTrue(withMinimum.hasPriceStatistics())
        assertEquals(GeographicScope.STATE, withMinimum.geographicScope)
        assertEquals("R$/l", withMinimum.unit)
    }

    @Test
    fun priceSurveyRestorePreservesImportedTimestamps() {
        val summaryAt = Instant.parse("2026-06-14T10:00:00Z")
        val stationAt = Instant.parse("2026-06-14T11:00:00Z")
        val restored = PriceSurvey.restore(
            id = DomainId.forSurveyWeek(surveyWeek),
            surveyWeek = surveyWeek,
            summaryImportedAt = summaryAt,
            stationImportedAt = stationAt,
        )

        assertTrue(restored.hasSummaryData)
        assertTrue(restored.hasStationData)
        assertEquals(summaryAt, restored.summaryImportedAt)
        assertEquals(stationAt, restored.stationImportedAt)
    }

    @Test
    fun priceSurveyRejectsBackwardStationImportTimestamp() {
        val survey = PriceSurvey.create(surveyWeek)
        survey.markSummaryImported(Instant.parse("2026-06-14T10:00:00Z"))
        survey.markStationImported(Instant.parse("2026-06-14T11:00:00Z"))

        assertThrows(DomainException::class.java) {
            survey.markStationImported(Instant.parse("2026-06-14T10:30:00Z"))
        }
    }

    @Test
    fun userPreferencesExposeDefaultRetentionWeeks() {
        val preferences = UserPreferences()

        assertEquals(UserPreferences.DEFAULT_RETENTION_WEEKS, preferences.stationDetailRetentionWeeks)
        assertEquals(UserPreferences.DEFAULT_NEAREST_STATION_RADIUS_KM, preferences.nearestStationRadiusKm)
        assertEquals(3000.0, preferences.getNearestStationRadiusMeters())
        assertEquals("", preferences.localeTag)
        assertFalse(preferences.localeUserSelected)
        assertTrue(preferences.syncStationDetail)
        assertTrue(preferences.autoDownloadLatestWeek)
    }

    @Test
    fun userPreferencesCopyUpdatesIndividualFields() {
        val updated = UserPreferences().copy(
            preferredState = BrazilianState.PARANA,
            preferredMunicipality = "Curitiba",
            preferredFuelProduct = FuelProduct.ETHANOL,
            localeTag = "pt-BR",
            syncStationDetail = true,
            stationDetailRetentionWeeks = 8,
            nearestStationRadiusKm = 10,
            autoSyncOnWifi = false,
            showPriceHistory = false,
            onboardingCompleted = true,
        )

        assertEquals(BrazilianState.PARANA, updated.preferredState)
        assertEquals("Curitiba", updated.preferredMunicipality)
        assertEquals(FuelProduct.ETHANOL, updated.preferredFuelProduct)
        assertEquals("pt-BR", updated.localeTag)
        assertTrue(updated.syncStationDetail)
        assertEquals(8, updated.stationDetailRetentionWeeks)
        assertEquals(10, updated.nearestStationRadiusKm)
        assertEquals(10_000.0, updated.getNearestStationRadiusMeters())
        assertFalse(updated.autoSyncOnWifi)
        assertFalse(updated.showPriceHistory)
        assertTrue(updated.onboardingCompleted)
    }

    @Test
    fun vehicleCreateTrimsDisplayNameAndStoresTankCapacity() {
        val vehicle = Vehicle.create(
            displayName = "  Gol 1.0  ",
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            priceSource = VehiclePriceSource.cheapest(),
        )

        assertEquals("Gol 1.0", vehicle.displayName)
        assertEquals(TankCapacity.of(50.0), vehicle.tankCapacity)
    }

    @Test
    fun tankFillCostEstimateHoldsComputedValues() {
        val vehicleId = DomainId.generate()
        val estimate = TankFillCostEstimate(
            vehicleId = vehicleId,
            displayName = "Gol",
            tankCapacity = TankCapacity.of(50.0),
            fuelProduct = FuelProduct.GASOLINE_REGULAR,
            unitPrice = PriceAmount.of("5.49"),
            totalCost = PriceAmount.of("274.50"),
            unitPriceSource = TankFillCostUnitPriceSource.CHEAPEST_STATION,
            stationDisplayName = "Posto Centro",
        )

        assertEquals(vehicleId, estimate.vehicleId)
        assertEquals("Posto Centro", estimate.stationDisplayName)
        assertEquals(estimate, estimate.copy())
    }

    @Test
    fun reverseGeocodeResultRequiresNonBlankMunicipality() {
        val result = ReverseGeocodeResult(
            state = BrazilianState.PARANA,
            municipality = "Curitiba",
            displayName = "Curitiba, Paraná, Brazil",
        )

        assertEquals("Curitiba", result.municipality)
        assertEquals(result, result.copy())
        assertThrows(DomainException::class.java) {
            ReverseGeocodeResult(
                state = BrazilianState.PARANA,
                municipality = " ",
            )
        }
    }

    @Test
    fun priceDropAlertNotificationHoldsVehicleAndFormattedPrice() {
        val vehicleId = DomainId.generate()
        val notification = PriceDropAlertNotification(
            vehicleId = vehicleId,
            vehicleDisplayName = "Gol",
            currentPriceFormatted = "R$ 5,40",
        )

        assertEquals(vehicleId, notification.vehicleId)
        assertEquals("Gol", notification.vehicleDisplayName)
        assertEquals(notification, notification.copy())
        assertNotEquals(notification, notification.copy(vehicleDisplayName = "Onix"))
    }
}
