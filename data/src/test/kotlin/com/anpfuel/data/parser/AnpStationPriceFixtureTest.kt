package com.anpfuel.data.parser

import com.anpfuel.data.mapper.AnpProductMapper
import com.anpfuel.domain.valueobject.FuelProduct
import java.io.File
import java.math.BigDecimal
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

/** Cross-language check against the same synthetic ANP XLSX used by Rust. */
class AnpStationPriceFixtureTest {

    @Test
    fun `Android parser and mapper agree with the MAP-05 goldens`() {
        val fixture = File("../contracts/testdata/map/anp-station-prices-v1.xlsx")
        val rows = StationDetailSheetParser().parseToList(fixture)
        assertEquals(15, rows.size)

        val gasoline = rows[0]
        assertEquals("01122233300000", gasoline.cnpj)
        assertEquals(BigDecimal("5.999"), gasoline.price)
        assertEquals(FuelProduct.GASOLINE_REGULAR, AnpProductMapper.toFuelProductOrThrow(gasoline.productLabel))

        val cng = rows[2]
        assertEquals("M3", cng.unit)
        assertEquals(FuelProduct.CNG, AnpProductMapper.toFuelProductOrThrow(cng.productLabel))

        val lpg = rows[3]
        assertEquals("KG_13", lpg.unit)
        assertEquals(FuelProduct.LPG_P13, AnpProductMapper.toFuelProductOrThrow(lpg.productLabel))

        val premium = rows[12]
        assertEquals(FuelProduct.GASOLINE_PREMIUM_GRADE, AnpProductMapper.toFuelProductOrThrow(premium.productLabel))
    }
}
