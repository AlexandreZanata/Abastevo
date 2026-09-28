package com.anpfuel.domain.event

import com.anpfuel.domain.state.SyncJobState
import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.Cnpj
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import com.anpfuel.domain.valueobject.SurveyWeekSelectionMode
import com.anpfuel.domain.valueobject.VehiclePriceSource
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import java.time.Instant

class DomainEventCatalogTest {

    @Test
    fun catalogMatchesGlossaryExactly() {
        val catalog = sampleEvents().map { it::class.simpleName }.toSet()

        assertEquals(GLOSSARY_EVENT_NAMES, catalog)
    }

    @Test
    fun everyEventHasIdTimestampAndPayload() {
        val fixedId = DomainId.generate()
        val fixedTimestamp = Instant.parse("2026-06-18T12:00:00Z")

        sampleEvents(fixedId, fixedTimestamp).forEach { event ->
            assertEquals(fixedId, event.id)
            assertEquals(fixedTimestamp, event.timestamp)
            assertNotNull(extractPayload(event))
        }
    }

    @Test
    fun exhaustiveWhenCoversAllGlossaryEvents() {
        sampleEvents().forEach { event ->
            val name = when (event) {
                is PriceTableDiscovered -> "PriceTableDiscovered"
                is PriceTableDownloaded -> "PriceTableDownloaded"
                is PriceTableImported -> "PriceTableImported"
                is PriceTableImportFailed -> "PriceTableImportFailed"
                is SyncJobCompleted -> "SyncJobCompleted"
                is CitySelected -> "CitySelected"
                is FuelProductSelected -> "FuelProductSelected"
                is SyncRequested -> "SyncRequested"
                is StationDetailRequested -> "StationDetailRequested"
                is PreferencesUpdated -> "PreferencesUpdated"
                is CacheCleared -> "CacheCleared"
                is SurveyWeekSelected -> "SurveyWeekSelected"
                is VehicleRegistered -> "VehicleRegistered"
                is VehicleUpdated -> "VehicleUpdated"
                is VehicleRemoved -> "VehicleRemoved"
                is PriceDropAlertConfigured -> "PriceDropAlertConfigured"
                is DeviceLocationResolved -> "DeviceLocationResolved"
                is StationNavigationRequested -> "StationNavigationRequested"
            }
            assertTrue(name in GLOSSARY_EVENT_NAMES)
        }
    }

    @Test
    fun createFactoriesGenerateUniqueIds() {
        val first = SyncRequested.create(SyncRequested.Payload(SyncRequestSource.MANUAL))
        val second = SyncRequested.create(SyncRequested.Payload(SyncRequestSource.SCHEDULED))

        assertTrue(first.id != second.id)
    }

    private fun sampleEvents(
        id: DomainId = DomainId.generate(),
        timestamp: Instant = Instant.now(),
    ): List<DomainEvent> {
        val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")
        val surveyWeekId = DomainId.forSurveyWeek(surveyWeek)

        return listOf(
            PriceTableDiscovered.create(
                payload = PriceTableDiscovered.Payload(
                    url = "https://example.com/resumo.xlsx",
                    tableType = PriceTableType.WEEKLY_SUMMARY,
                    surveyWeek = surveyWeek,
                ),
                id = id,
                timestamp = timestamp,
            ),
            PriceTableDownloaded.create(
                payload = PriceTableDownloaded.Payload(
                    surveyWeekId = surveyWeekId,
                    tableType = PriceTableType.WEEKLY_SUMMARY,
                    localPath = "/cache/resumo.xlsx",
                    sourceUrl = "https://example.com/resumo.xlsx",
                ),
                id = id,
                timestamp = timestamp,
            ),
            PriceTableImported.create(
                payload = PriceTableImported.Payload(
                    surveyWeekId = surveyWeekId,
                    tableType = PriceTableType.WEEKLY_SUMMARY,
                    rowCount = 2344,
                ),
                id = id,
                timestamp = timestamp,
            ),
            PriceTableImportFailed.create(
                payload = PriceTableImportFailed.Payload(
                    surveyWeekId = surveyWeekId,
                    tableType = PriceTableType.STATION_DETAIL,
                    detail = "Parse error",
                ),
                id = id,
                timestamp = timestamp,
            ),
            SyncJobCompleted.create(
                payload = SyncJobCompletedPayload(
                    finalState = SyncJobState.COMPLETED,
                    outcome = SyncJobOutcome.SUCCESS,
                ),
                id = id,
                timestamp = timestamp,
            ),
            CitySelected.create(
                payload = CitySelected.Payload(
                    municipality = "CURITIBA",
                    state = BrazilianState.PARANA,
                    surveyWeekId = surveyWeekId,
                ),
                id = id,
                timestamp = timestamp,
            ),
            FuelProductSelected.create(
                payload = FuelProductSelected.Payload(
                    fuelProduct = FuelProduct.ETHANOL,
                    municipality = "CURITIBA",
                    state = BrazilianState.PARANA,
                ),
                id = id,
                timestamp = timestamp,
            ),
            SyncRequested.create(
                payload = SyncRequested.Payload(source = SyncRequestSource.MANUAL),
                id = id,
                timestamp = timestamp,
            ),
            StationDetailRequested.create(
                payload = StationDetailRequested.Payload(
                    surveyWeekId = surveyWeekId,
                    municipality = "CURITIBA",
                    state = BrazilianState.PARANA,
                ),
                id = id,
                timestamp = timestamp,
            ),
            PreferencesUpdated.create(
                payload = PreferencesUpdated.Payload(changedKeys = setOf("locale")),
                id = id,
                timestamp = timestamp,
            ),
            CacheCleared.create(
                payload = CacheCleared.Payload(scope = CacheClearScope.ALL),
                id = id,
                timestamp = timestamp,
            ),
            SurveyWeekSelected.create(
                payload = SurveyWeekSelected.Payload(
                    surveyWeek = surveyWeek,
                    selectionMode = SurveyWeekSelectionMode.SPECIFIC,
                ),
                id = id,
                timestamp = timestamp,
            ),
            VehicleRegistered.create(
                payload = VehicleRegistered.Payload(
                    vehicleId = DomainId.generate(),
                    displayName = "Gol",
                    fuelProduct = FuelProduct.GASOLINE_REGULAR,
                ),
                id = id,
                timestamp = timestamp,
            ),
            VehicleUpdated.create(
                payload = VehicleUpdated.Payload(
                    vehicleId = DomainId.generate(),
                    displayName = "Gol 1.0",
                    fuelProduct = FuelProduct.GASOLINE_REGULAR,
                ),
                id = id,
                timestamp = timestamp,
            ),
            VehicleRemoved.create(
                payload = VehicleRemoved.Payload(vehicleId = DomainId.generate()),
                id = id,
                timestamp = timestamp,
            ),
            PriceDropAlertConfigured.create(
                payload = PriceDropAlertConfigured.Payload(
                    vehicleId = DomainId.generate(),
                    enabled = true,
                    alertPriceSource = VehiclePriceSource.cheapest(),
                ),
                id = id,
                timestamp = timestamp,
            ),
            DeviceLocationResolved.create(
                payload = DeviceLocationResolved.Payload(
                    state = BrazilianState.PARANA,
                    municipality = "CURITIBA",
                ),
                id = id,
                timestamp = timestamp,
            ),
            StationNavigationRequested.create(
                payload = StationNavigationRequested.Payload(
                    cnpj = Cnpj.parse("12345678000195"),
                    navigationQuery = "RUA A, CURITIBA - PR, Brazil",
                ),
                id = id,
                timestamp = timestamp,
            ),
        )
    }

    private fun extractPayload(event: DomainEvent): Any? = when (event) {
        is PriceTableDiscovered -> event.payload
        is PriceTableDownloaded -> event.payload
        is PriceTableImported -> event.payload
        is PriceTableImportFailed -> event.payload
        is SyncJobCompleted -> event.payload
        is CitySelected -> event.payload
        is FuelProductSelected -> event.payload
        is SyncRequested -> event.payload
        is StationDetailRequested -> event.payload
        is PreferencesUpdated -> event.payload
        is CacheCleared -> event.payload
        is SurveyWeekSelected -> event.payload
        is VehicleRegistered -> event.payload
        is VehicleUpdated -> event.payload
        is VehicleRemoved -> event.payload
        is PriceDropAlertConfigured -> event.payload
        is DeviceLocationResolved -> event.payload
        is StationNavigationRequested -> event.payload
    }

    private companion object {
        val GLOSSARY_EVENT_NAMES = setOf(
            "PriceTableDiscovered",
            "PriceTableDownloaded",
            "PriceTableImported",
            "PriceTableImportFailed",
            "SyncJobCompleted",
            "CitySelected",
            "FuelProductSelected",
            "SyncRequested",
            "StationDetailRequested",
            "PreferencesUpdated",
            "CacheCleared",
            "SurveyWeekSelected",
            "VehicleRegistered",
            "VehicleUpdated",
            "VehicleRemoved",
            "PriceDropAlertConfigured",
            "DeviceLocationResolved",
            "StationNavigationRequested",
        )
    }
}
