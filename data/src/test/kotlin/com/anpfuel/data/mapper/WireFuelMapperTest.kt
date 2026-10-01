package com.anpfuel.data.mapper

import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

/**
 * P10-T01: wire vocabulary bridge (A05).
 *
 * The wire enum uses GASOLINE_ADDITIVED; the legacy Android enum keeps
 * GASOLINE_PREMIUM. This adapter owns the bridge in one place so no
 * silent coercion or rename can drift in elsewhere.
 */
class WireFuelMapperTest {

    @Test
    fun legacyPremiumMapsToWireAdditived() {
        assertEquals("GASOLINE_ADDITIVED", WireFuelMapper.toWire(FuelProduct.GASOLINE_PREMIUM))
    }

    @Test
    fun otherProductsMapToOwnWireName() {
        assertEquals("ETHANOL", WireFuelMapper.toWire(FuelProduct.ETHANOL))
        assertEquals("GASOLINE_REGULAR", WireFuelMapper.toWire(FuelProduct.GASOLINE_REGULAR))
        assertEquals("DIESEL_S500", WireFuelMapper.toWire(FuelProduct.DIESEL_S500))
        assertEquals("DIESEL_S10", WireFuelMapper.toWire(FuelProduct.DIESEL_S10))
        assertEquals("CNG", WireFuelMapper.toWire(FuelProduct.CNG))
        assertEquals("LPG_P13", WireFuelMapper.toWire(FuelProduct.LPG_P13))
    }

    @Test
    fun wireAdditivedMapsBackToLegacyPremium() {
        val result = WireFuelMapper.fromWire("GASOLINE_ADDITIVED")
        assertTrue(result.isSuccess)
        assertEquals(FuelProduct.GASOLINE_PREMIUM, result.getOrNull())
    }

    @Test
    fun allWireNamesRoundTrip() {
        WireFuelMapper.wireValues().forEach { wire ->
            val back = WireFuelMapper.fromWire(wire)
            assertTrue(back.isSuccess, "wire value refused: $wire")
            assertEquals(wire, WireFuelMapper.toWire(requireNotNull(back.getOrNull())))
        }
    }

    @Test
    fun legacyPremiumNameNeverAppearsOnWire() {
        val result = WireFuelMapper.fromWire("GASOLINE_PREMIUM")
        assertTrue(result.isFailure)
    }

    @Test
    fun unknownWireEnumRefusedWithoutCoercion() {
        assertTrue(WireFuelMapper.fromWire("COMBUSTIVEL DESCONHECIDO").isFailure)
        assertTrue(WireFuelMapper.fromWire("").isFailure)
        assertTrue(WireFuelMapper.fromWire("gasoline_additived").isFailure)
    }
}
