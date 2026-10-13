package com.anpfuel.app.ui.stations

import com.anpfuel.app.ui.model.StationPriceUiModel
import com.anpfuel.domain.discovery.ServerStation
import com.anpfuel.domain.discovery.StationLocationQuality
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class StationDirectoryVisibilityStateTest {
    private val station = ServerStation.create(
        "d6c74c23-63db-4c24-a2e5-408cb23bad26", "Legal name", StationLocationQuality.UNKNOWN,
        null, null, "04218406000104", "3550308", "SP", null,
    )
    private val state = StationsUiState(
        isLoading = false,
        stations = listOf(StationPriceUiModel("04218406000104", "Trade name", null, "Test address", "6,00", null, "Test address")),
        serverStations = listOf(station),
    )

    @Test
    fun `cached directory supplement recomputes on price and visibility changes`() {
        assertEquals(emptyList<ServerStation>(), state.directoryOnlyStations)
        assertEquals(listOf(station), state.copy(stations = emptyList()).directoryOnlyStations)
        assertEquals(listOf(station), state.copy(showDownloadPrompt = true).directoryOnlyStations)
        assertEquals(listOf(station), state.copy(showNoLocation = true).directoryOnlyStations)
        assertEquals(listOf(station), state.copy(showEmpty = true).directoryOnlyStations)
        assertEquals(listOf(station), state.copy(searchNoMatch = "missing").directoryOnlyStations)
        assertEquals(listOf(station), state.serverStations)
    }
}
