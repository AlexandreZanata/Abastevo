package com.anpfuel.app.ui.stations

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Test

class StationsListKeysTest {

    @Test
    fun `rows are namespaced by origin`() {
        assertEquals("anp:12345678000195", StationsListKeys.anpRow("12345678000195"))
        assertEquals(
            "dir:d6c74c23-63db-4c24-a2e5-408cb23bad26",
            StationsListKeys.directoryRow("d6c74c23-63db-4c24-a2e5-408cb23bad26"),
        )
    }

    @Test
    fun `same station identity never collides across origins`() {
        assertNotEquals(
            StationsListKeys.anpRow("12345678000195"),
            StationsListKeys.directoryRow("12345678000195"),
        )
    }

    @Test
    fun `keys are stable for the same input`() {
        assertEquals(StationsListKeys.anpRow("x"), StationsListKeys.anpRow("x"))
        assertEquals(StationsListKeys.directoryRow("y"), StationsListKeys.directoryRow("y"))
    }

    @Test
    fun `header slots are unique`() {
        val slots = listOf(
            StationsListKeys.OFFLINE_BANNER,
            StationsListKeys.FUEL_LABEL,
            StationsListKeys.FUEL_SELECTOR,
            StationsListKeys.SEARCH,
            StationsListKeys.SUMMARY,
            StationsListKeys.LOADING,
            StationsListKeys.ERROR,
            StationsListKeys.EMPTY_STATE,
            StationsListKeys.DOWNLOAD_ACTION,
            StationsListKeys.LIST_ERROR,
            StationsListKeys.NEAREST_BUTTON,
            StationsListKeys.NAVIGATE_HINT,
            StationsListKeys.ATTRIBUTION,
            StationsListKeys.ANP_MORE,
            StationsListKeys.ANP_MORE_LOADING,
            StationsListKeys.ANP_MORE_ERROR,
            StationsListKeys.ANP_MORE_RETRY,
            StationsListKeys.DIRECTORY_TITLE,
            StationsListKeys.DIRECTORY_CACHE_NOTE,
            StationsListKeys.DIRECTORY_MORE,
            StationsListKeys.DIRECTORY_MORE_LOADING,
            StationsListKeys.DIRECTORY_MORE_ERROR,
            StationsListKeys.DIRECTORY_MORE_RETRY,
            StationsListKeys.SUGGEST_ACTION,
            StationsListKeys.SERVER_ERROR,
        )
        assertEquals(slots.size, slots.distinct().size)
    }
}
