package com.anpfuel.domain.rule

import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test
import org.junit.jupiter.params.ParameterizedTest
import org.junit.jupiter.params.provider.Arguments
import org.junit.jupiter.params.provider.MethodSource
import java.util.stream.Stream

class FuelProductNormalizationRuleTest {

    @ParameterizedTest(name = "summary label \"{0}\" maps to {1}")
    @MethodSource("summaryLabels")
    fun everySummaryLabelInMappingTableMapsToFuelProduct(
        rawLabel: String,
        expected: FuelProduct,
    ) {
        val result = FuelProductNormalizationRule.normalize(rawLabel)

        assertTrue(result.isSuccess)
        assertEquals(expected, result.getOrNull())
    }

    @ParameterizedTest(name = "station label \"{0}\" maps to {1}")
    @MethodSource("stationLabels")
    fun everyStationLabelInMappingTableMapsToFuelProduct(
        rawLabel: String,
        expected: FuelProduct,
    ) {
        val result = FuelProductNormalizationRule.normalize(rawLabel)

        assertTrue(result.isSuccess)
        assertEquals(expected, result.getOrNull())
    }

    @Test
    fun ethanolSummaryAndStationLabelsMapToSameFuelProduct() {
        val summary = FuelProductNormalizationRule.normalize("ETANOL HIDRATADO")
        val station = FuelProductNormalizationRule.normalize("ETANOL")

        assertEquals(FuelProduct.ETHANOL, summary.getOrNull())
        assertEquals(FuelProduct.ETHANOL, station.getOrNull())
        assertEquals(summary.getOrNull(), station.getOrNull())
    }

    @Test
    fun unknownLabelReturnsFailureWithoutThrowing() {
        val result = FuelProductNormalizationRule.normalize("COMBUSTIVEL DESCONHECIDO")

        assertTrue(result.isFailure)
        assertNull(result.getOrNull())
    }

    @Test
    fun unknownLabelNormalizeOrNullReturnsNullWithoutThrowing() {
        assertNull(FuelProductNormalizationRule.normalizeOrNull("COMBUSTIVEL DESCONHECIDO"))
    }

    @Test
    fun blankLabelReturnsFailureWithoutThrowing() {
        val result = FuelProductNormalizationRule.normalize("   ")

        assertTrue(result.isFailure)
        assertNull(result.getOrNull())
    }

    @Test
    fun labelsAreMatchedCaseInsensitively() {
        val result = FuelProductNormalizationRule.normalize("etanol hidratado")

        assertEquals(FuelProduct.ETHANOL, result.getOrNull())
    }

    companion object {
        @JvmStatic
        fun summaryLabels(): Stream<Arguments> = Stream.of(
            Arguments.of("ETANOL HIDRATADO", FuelProduct.ETHANOL),
            Arguments.of("GASOLINA COMUM", FuelProduct.GASOLINE_REGULAR),
            Arguments.of("GASOLINA ADITIVADA", FuelProduct.GASOLINE_PREMIUM),
            Arguments.of("OLEO DIESEL", FuelProduct.DIESEL_S500),
            Arguments.of("OLEO DIESEL S10", FuelProduct.DIESEL_S10),
            Arguments.of("GNV", FuelProduct.CNG),
            Arguments.of("GLP", FuelProduct.LPG_P13),
        )

        @JvmStatic
        fun stationLabels(): Stream<Arguments> = Stream.of(
            Arguments.of("ETANOL", FuelProduct.ETHANOL),
            Arguments.of("GASOLINA COMUM", FuelProduct.GASOLINE_REGULAR),
            Arguments.of("GASOLINA ADITIVADA", FuelProduct.GASOLINE_PREMIUM),
            Arguments.of("DIESEL S500", FuelProduct.DIESEL_S500),
            Arguments.of("DIESEL S10", FuelProduct.DIESEL_S10),
            Arguments.of("GNV", FuelProduct.CNG),
            Arguments.of("GLP", FuelProduct.LPG_P13),
        )
    }
}
