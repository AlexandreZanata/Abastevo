package com.anpfuel.domain.enterprise.fueling

import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class EnterpriseFuelProductsTest {
    @Test fun `owned catalog has the nine exact descriptors and no cooking cylinder`() {
        val expected = listOf(
            "ETHANOL:LIQUID_FUEL:L:ML:ETHANOL", "GASOLINE_REGULAR:LIQUID_FUEL:L:ML:GASOLINE",
            "GASOLINE_ADDITIVED:LIQUID_FUEL:L:ML:GASOLINE", "GASOLINE_PREMIUM_GRADE:LIQUID_FUEL:L:ML:GASOLINE",
            "DIESEL_S10:LIQUID_FUEL:L:ML:DIESEL", "DIESEL_S500:LIQUID_FUEL:L:ML:DIESEL", "CNG:GAS_FUEL:M3:MILLI_M3:NATURAL_GAS",
            "ELECTRICITY:ELECTRICITY:KWH:WH:ELECTRIC", "ARLA_32:ADDITIVE:L:ML:NONE")
        assertEquals("fuel-products-v1", FuelProducts.VERSION)
        assertEquals(expected, FuelProducts.all().map { "${it.name}:${it.category}:${it.unit}:${it.storage}:${it.family}" })
        assertTrue(FuelProducts.get("ARLA_32").isAdditive)
        assertFalse(FuelProducts.get("ETHANOL").isAdditive)
    }
    @Test fun `catalog lookups reject aliases and snapshots cannot rewrite the owner`() {
        val products = FuelProducts.all()
        try { (products as MutableList).clear() } catch (_: UnsupportedOperationException) { /* Immutable view is safe. */ }
        assertEquals(9, FuelProducts.all().size)
        for (code in listOf("LPG_P13", "", "ethanol", " ETHANOL", "ETHANOL ", "POWER", "OTHER"))
            assertEquals(FuelFailure.INVALID, assertThrows(FuelDomainException::class.java) { FuelProducts.get(code) }.failure)
    }
    @Test fun `fleet product descriptors reuse the owner without copying public vocabulary`() {
        for ((product, unit) in listOf("ELECTRICITY" to "KWH", "ARLA_32" to "L", "CNG" to "M3")) {
            val descriptor = FuelProducts.fleetProduct(product)
            assertNotNull(descriptor)
            assertEquals(unit, descriptor?.unit)
        }
        assertNull(FuelProducts.fleetProduct("LPG_P13"))
        assertNull(FuelProducts.fleetProduct("CAT_A"))
    }
}
