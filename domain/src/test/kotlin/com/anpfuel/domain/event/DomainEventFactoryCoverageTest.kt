package com.anpfuel.domain.event

import com.anpfuel.domain.valueobject.BrazilianState
import com.anpfuel.domain.valueobject.DomainId
import com.anpfuel.domain.valueobject.FuelProduct
import com.anpfuel.domain.valueobject.PriceTableType
import com.anpfuel.domain.valueobject.SurveyWeek
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test
import java.time.Instant

class DomainEventFactoryCoverageTest {

    private val fixedId = DomainId.generate()
    private val fixedTimestamp = Instant.parse("2026-06-18T12:00:00Z")
    private val surveyWeek = SurveyWeek.fromIsoDates("2026-06-07", "2026-06-13")

    @Test
    fun cacheClearedCreateHonorsExplicitIdAndTimestamp() {
        val event = CacheCleared.create(
            payload = CacheCleared.Payload(scope = CacheClearScope.STATION_DETAIL_ONLY),
            id = fixedId,
            timestamp = fixedTimestamp,
        )

        assertEquals(fixedId, event.id)
        assertEquals(fixedTimestamp, event.timestamp)
        assertEquals(CacheClearScope.STATION_DETAIL_ONLY, event.payload.scope)
    }

    @Test
    fun preferencesUpdatedCreateHonorsExplicitIdAndTimestamp() {
        val event = PreferencesUpdated.create(
            payload = PreferencesUpdated.Payload(changedKeys = setOf("localeTag")),
            id = fixedId,
            timestamp = fixedTimestamp,
        )

        assertEquals(fixedId, event.id)
        assertEquals(fixedTimestamp, event.timestamp)
        assertEquals(setOf("localeTag"), event.payload.changedKeys)
    }

    @Test
    fun stationDetailRequestedCreateHonorsExplicitIdAndTimestamp() {
        val event = StationDetailRequested.create(
            payload = StationDetailRequested.Payload(
                surveyWeekId = DomainId.forSurveyWeek(surveyWeek),
                municipality = "CURITIBA",
                state = BrazilianState.PARANA,
            ),
            id = fixedId,
            timestamp = fixedTimestamp,
        )

        assertEquals(fixedId, event.id)
        assertEquals(fixedTimestamp, event.timestamp)
        assertEquals("CURITIBA", event.payload.municipality)
    }

    @Test
    fun priceTableDiscoveredCreateHonorsExplicitIdAndTimestamp() {
        val event = PriceTableDiscovered.create(
            payload = PriceTableDiscovered.Payload(
                url = "https://example.com/resumo.xlsx",
                tableType = PriceTableType.WEEKLY_SUMMARY,
                surveyWeek = surveyWeek,
            ),
            id = fixedId,
            timestamp = fixedTimestamp,
        )

        assertEquals(fixedId, event.id)
        assertEquals(fixedTimestamp, event.timestamp)
        assertEquals(PriceTableType.WEEKLY_SUMMARY, event.payload.tableType)
    }
}
