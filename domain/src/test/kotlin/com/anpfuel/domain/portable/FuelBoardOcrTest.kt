package com.anpfuel.domain.portable

import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class FuelBoardOcrTest {
    private fun box(text: String, x: Int, y: Int, w: Int = 100) = FuelBoardOcr.Token(text, x, y, x + w, y + 30)

    @Test fun `associates rows and preserves different variants without choosing minimum`() {
        val result = FuelBoardOcr.associate(listOf(box("GASOLINA",0,0),box("6,59",200,0),box("ADITIVADA",0,50),box("6.74",200,50),box("ETANOL",0,100),box("4,32",200,100)))
        assertEquals(mapOf(FuelProduct.GASOLINE_REGULAR to 6590L,FuelProduct.GASOLINE_PREMIUM to 6740L,FuelProduct.ETHANOL to 4320L),result.rows.associate{it.product to it.amountMilli})
    }
    @Test fun `rejects total dates zero diesel ambiguity and true premium`() {
        val result = FuelBoardOcr.associate(listOf(box("Diesel S50",0,0),box("7.400",200,0),box("DIESEL",0,50),box("6,15",200,50),box("PODIUM",0,100),box("9,19",200,100),box("DIESEL S500",0,150),box("0,00",200,150),box("TOTAL R$ 549,60",0,200),box("07/10/2026",0,250)))
        assertTrue(result.rows.isEmpty())
        assertTrue(result.unresolved)
    }
    @Test fun `conditional and conflicting prices require review`() {
        val result = FuelBoardOcr.associate(listOf(box("A VISTA",0,0),box("ETANOL",0,50),box("4,06",200,50),box("ETANOL",0,100),box("4,20",200,100)))
        assertTrue(result.conditional)
        assertTrue(result.rows.isEmpty())
        assertTrue(result.unresolved)
    }
    @Test fun `inline label and price and vertical board columns`() {
        assertEquals(6190L,FuelBoardOcr.associate(listOf(box("DIESEL S10 6,19",0,0,300))).rows.single().amountMilli)
        val result=FuelBoardOcr.associate(listOf(box("ETANOL",0,0),box("3,18",0,45),box("GASOLINA",150,0),box("6,98",150,45)))
        assertEquals(2,result.rows.size)
    }
    @Test fun `overlapping multiline additive labels preserve fuel variants`() {
        val tokens=listOf(FuelBoardOcr.Token("GASOLINA",264,828,393,878),FuelBoardOcr.Token("ADITIVADA",266,865,404,915),FuelBoardOcr.Token("6.87",478,801,619,885),FuelBoardOcr.Token("DIESEL S-10",260,720,395,765),FuelBoardOcr.Token("ADITIVADO",260,756,393,808),FuelBoardOcr.Token("6.20",466,698,613,784))
        assertEquals(mapOf(FuelProduct.GASOLINE_PREMIUM to 6870L,FuelProduct.DIESEL_S10 to 6200L),FuelBoardOcr.associate(tokens).rows.associate{it.product to it.amountMilli})
    }
    @Test fun `unknown diesel blocks vertical reuse of preceding zero row`() {
        val result=FuelBoardOcr.associate(listOf(box("DIESEL S500",0,0),box("0.00",150,0),box("DIESEL",0,80),box("7.98",150,80)))
        assertTrue(result.rows.isEmpty())
    }
    @Test fun `recovers board prices missing the decimal separator`() {
        val result=FuelBoardOcr.associate(listOf(box("ETANOL",0,0),box("4 35",200,0),box("GASOLINA",0,50),box("675",200,50)))
        assertEquals(mapOf(FuelProduct.ETHANOL to 4350L,FuelProduct.GASOLINE_REGULAR to 6750L),result.rows.associate{it.product to it.amountMilli})
    }
    @Test fun `unrecognized label keeps the value as an orphan for manual choice`() {
        val result=FuelBoardOcr.associate(listOf(box("DIESEL COMUM",0,0,150),box("6,15",200,0)))
        assertTrue(result.rows.isEmpty())
        assertEquals(listOf(6150L),result.orphans)
        assertTrue(result.unresolved)
    }
    @Test fun `years and address numbers never become prices`() {
        val result=FuelBoardOcr.associate(listOf(box("11 DE JULHO DE 2025",0,0,200),box("20 RUA 8",0,50,150)))
        assertTrue(result.rows.isEmpty())
        assertTrue(result.orphans.isEmpty())
    }
}
