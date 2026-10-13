package com.anpfuel.domain.discovery

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class StationDirectoryVisibilityRuleTest {
    private fun station(cnpj: String?, name: String = "Same trade name") = ServerStation.create(
        stationId = "d6c74c23-63db-4c24-a2e5-408cb23bad26",
        displayName = name,
        locationQuality = StationLocationQuality.UNKNOWN,
        latitude = null,
        longitude = null,
        cnpjNormalized = cnpj,
        municipalityCode = "3550308",
        state = "SP",
        currentRevisionId = null,
    )

    @Test
    fun `same establishment with a legal name is shown once`() {
        val legal = station("04218406000104", "Legal company name")
        val other = station("12ABC34501DE35")
        val unknown = station(null)
        assertEquals(listOf(other, unknown), StationDirectoryVisibilityRule.additionalStations(
            listOf(legal, other, unknown), listOf("04218406000104"),
        ))
    }

    @Test
    fun `different establishments sharing names are retained and empty prices restore directory`() {
        val first = station("04218406000104")
        val second = station("12ABC34501DE35")
        assertEquals(listOf(second), StationDirectoryVisibilityRule.additionalStations(
            listOf(first, second), listOf("04218406000104"),
        ))
        assertEquals(listOf(first, second), StationDirectoryVisibilityRule.additionalStations(
            listOf(first, second), emptyList(),
        ))
    }

    @Test
    fun `leading zeroes and full branch identities remain distinct`() {
        val zero = station("04218406000104")
        val otherBranch = station("04218406000200")
        assertEquals(listOf(zero, otherBranch), StationDirectoryVisibilityRule.additionalStations(
            listOf(zero, otherBranch), listOf("4218406000104", "04218406"),
        ))
    }
}
