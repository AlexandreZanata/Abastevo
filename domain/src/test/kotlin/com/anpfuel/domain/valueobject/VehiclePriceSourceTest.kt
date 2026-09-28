package com.anpfuel.domain.valueobject

import com.anpfuel.domain.exception.DomainException
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Test

class VehiclePriceSourceTest {

    @Test
    fun cheapestSourceHasNoCnpj() {
        val source = VehiclePriceSource.cheapest()

        assertEquals(VehiclePriceSourceMode.CHEAPEST_STATION, source.mode)
        assertEquals(null, source.specificStationCnpj)
    }

    @Test
    fun specificSourceRequiresCnpj() {
        val cnpj = Cnpj.parse("12345678000195")
        val source = VehiclePriceSource.specific(cnpj)

        assertEquals(VehiclePriceSourceMode.SPECIFIC_STATION, source.mode)
        assertEquals(cnpj, source.specificStationCnpj)
    }

    @Test
    fun specificSourceWithoutCnpjThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            VehiclePriceSource(
                mode = VehiclePriceSourceMode.SPECIFIC_STATION,
                specificStationCnpj = null,
            )
        }
    }

    @Test
    fun cheapestSourceWithCnpjThrowsDomainException() {
        assertThrows(DomainException::class.java) {
            VehiclePriceSource(
                mode = VehiclePriceSourceMode.CHEAPEST_STATION,
                specificStationCnpj = Cnpj.parse("12345678000195"),
            )
        }
    }
}
