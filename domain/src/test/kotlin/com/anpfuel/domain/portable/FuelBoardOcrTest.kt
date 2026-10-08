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
    @Test fun `real rota-do-sol board keeps every row despite misread labels`() {
        fun t(text: String, l: Int, top: Int, r: Int, b: Int) = FuelBoardOcr.Token(text, l, top, r, b)
        val tokens=listOf(
            t("Oiginal",102,376,132,385),t("Etonol",103,389,136,397),
            t("PT Gasolina",84,476,148,488),t("PT",84,476,99,488),t("Gasolina",103,476,148,488),
            t("lpnax",102,553,131,562),t("Gosolino",103,565,147,574),t("Aditivoda",102,577,153,587),
            t("S500",103,665,129,673),t("Originol",99,731,129,739),
            t("I Diesel",68,739,134,756),t("I",68,739,76,756),t("Diesel",109,739,134,756),t("S10",102,757,120,765),
            t("4.66",161,377,258,421),t("6,84",164,463,260,507),t("6.89",178,551,261,596),
            t("6.77",182,641,272,686),t("6.83",166,733,262,771),
            t("AUTO P OST O",421,553,605,570),t("AUTO",421,554,491,570),t("P",524,554,527,568),
            t("OST",538,553,587,568),t("O",602,553,605,567),t("ROTA",386,577,649,624),t("DOGOL",379,629,654,684),
            t("20 de fev de 2026 08:19:05",311,974,684,1005),t("20",311,974,349,1005),t("de",358,974,389,1005),
            t("fev",401,974,442,1005),t("de",457,974,483,1005),t("2026",491,974,556,1005),t("08:19:05",568,974,684,1005),
            t("530.Avenida Blumenau Sul",324,1004,682,1039),t("530.Avenida",324,1004,494,1037),
            t("Blumenau",501,1006,635,1038),t("Sul",646,1008,682,1039),
            t("Rota do Sol",535,1037,687,1071),t("Rota",535,1037,594,1071),t("do",611,1037,639,1071),t("Sol",647,1037,687,1071),
            t("Sorriso",590,1073,685,1095),t("Mato GTOSSO",511,1099,684,1137),t("Mato",511,1099,579,1130),t("GTOSSO",588,1105,684,1137),
            t("Altitude.345.3nsnm",408,1134,683,1161),t("Velocidade:0.2kn/h",414,1168,683,1200),
            t("Número de indice 6905",363,1196,684,1230),t("Número",363,1196,471,1230),t("de",481,1196,511,1230),
            t("indice",527,1196,603,1230),t("6905",620,1196,684,1230),
        )
        val result=FuelBoardOcr.associate(tokens)
        assertEquals(
            mapOf(FuelProduct.ETHANOL to 4660L,FuelProduct.GASOLINE_REGULAR to 6840L,FuelProduct.GASOLINE_PREMIUM to 6890L,FuelProduct.DIESEL_S500 to 6770L),
            result.rows.associate{it.product to it.amountMilli})
        assertEquals(listOf(6830L),result.orphans)
        assertTrue(result.unresolved)
    }
}
