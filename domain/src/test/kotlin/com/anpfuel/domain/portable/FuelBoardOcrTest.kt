package com.anpfuel.domain.portable

import com.anpfuel.domain.valueobject.FuelProduct
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class FuelBoardOcrTest {
    private fun box(text: String, x: Int, y: Int, w: Int = 100) = FuelBoardOcr.Token(text, x, y, x + w, y + 30)

    @Test fun `real board typo retains ethanol conflict without stealing additive gasoline`() {
        fun t(text: String,l: Int,top: Int,r: Int,b: Int)=FuelBoardOcr.Token(text,l,top,r,b)
        val result=FuelBoardOcr.associate(listOf(t("Etanol",404,591,466,621),t("omom3.99",388,602,678,658),
            t("Efcnnol",388,674,467,700),t("Adtivado",385,701,506,734),t("Gasolina",391,762,493,790),
            t("Aditivada",390,795,500,824),t("4.11",530,680,667,750),t("|7.12",532,765,671,835)))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.GASOLINE_PREMIUM,7120L)),result.rows)
        assertEquals(setOf(3990L,4110L),result.orphans.toSet())
        assertEquals(setOf(FuelProduct.ETHANOL),result.conflictingProducts)
    }

    @Test fun `low-confidence label can withhold a conflicting product but never assign it`() {
        val uncertain = listOf(box("EFCNNOL",0,80),box("ADTIVADO",0,105,130),box("4,11",200,80))
        val alone = FuelBoardOcr.associate(uncertain)
        assertTrue(alone.rows.isEmpty())
        assertEquals(listOf(4110L),alone.orphans)
        val conflict = FuelBoardOcr.associate(listOf(box("ETANOL",0,0),box("3,99",200,0)) + uncertain)
        assertTrue(conflict.rows.isEmpty())
        assertEquals(setOf(3990L,4110L),conflict.orphans.toSet())
        assertEquals(setOf(FuelProduct.ETHANOL),conflict.conflictingProducts)
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.GASOLINE_REGULAR,6980L)),
            FuelBoardOcr.associate(listOf(box("GASOLINA",0,0),box("6,98",200,0)) + uncertain).rows)
    }

    @Test fun `same amount across pixel views cannot invent a second fuel association`() {
        val specific = FuelBoardOcr.Result(listOf(FuelBoardOcr.Row(FuelProduct.DIESEL_S10,6750L)),false,false)
        val generic = FuelBoardOcr.Result(listOf(FuelBoardOcr.Row(FuelProduct.DIESEL_S500,6750L)),false,false)
        val ambiguous = FuelBoardOcr.reconcile(listOf(specific,generic,generic))
        assertTrue(ambiguous.rows.isEmpty())
        assertEquals(listOf(6750L),ambiguous.orphans)
        val shared = FuelBoardOcr.Result(specific.rows+generic.rows,false,false)
        assertEquals(shared.rows,FuelBoardOcr.reconcile(listOf(shared,shared)).rows)
    }

    @Test fun `separate ethanol icon and dotted-I brand label form one fuel heading`() {
        val result = FuelBoardOcr.associate(listOf(box("E",0,0,25),box("GRİD",50,0,80),box("4,34",180,0)))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.ETHANOL,4340L)),result.rows)
    }

    @Test fun `inline diesel specification never joins the price integer`() {
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.DIESEL_S10,7750L)),
            FuelBoardOcr.associate(listOf(box("D DIESELS10 7,75",0,0,350))).rows)
    }

    @Test fun `total and volume parent context excludes numeric child elements`() {
        val result = FuelBoardOcr.associate(listOf(box("TOTAL R$ 549,60",0,0,350),box("549,60",220,0,100),
            box("VOLUME 74,270 LITROS",0,80,400),box("74,270",200,80,100)))
        assertTrue(result.rows.isEmpty())
        assertTrue(result.orphans.isEmpty())
    }

    @Test fun `pixel view consensus corrects a lone disagreeing reading without averaging money`() {
        fun view(amount: Long) = FuelBoardOcr.Result(listOf(FuelBoardOcr.Row(FuelProduct.DIESEL_S10, amount)), false, false)
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.DIESEL_S10, 7570L)),
            FuelBoardOcr.reconcile(listOf(view(17570L), view(7570L), view(7570L))).rows)
    }

    @Test fun `explicit conflicting product in any pixel view cannot be hidden by missed rows`() {
        val single = FuelBoardOcr.Result(listOf(FuelBoardOcr.Row(FuelProduct.ETHANOL,3990L)), false, false)
        val conflict = FuelBoardOcr.Result(emptyList(),false,true,listOf(3990L,4110L),setOf(FuelProduct.ETHANOL))
        val result = FuelBoardOcr.reconcile(listOf(single,single,conflict))
        assertTrue(result.rows.isEmpty())
        assertEquals(listOf(3990L,4110L),result.orphans)
        assertEquals(setOf(FuelProduct.ETHANOL),result.conflictingProducts)
    }

    @Test fun `disagreeing pixel views stay manual and retain conditions`() {
        val first = FuelBoardOcr.Result(listOf(FuelBoardOcr.Row(FuelProduct.ETHANOL, 4060L)), true, false)
        val second = FuelBoardOcr.Result(listOf(FuelBoardOcr.Row(FuelProduct.ETHANOL, 4050L)), false, false)
        val result = FuelBoardOcr.reconcile(listOf(first, second, FuelBoardOcr.Result(emptyList(), false, true)))
        assertTrue(result.rows.isEmpty())
        assertEquals(listOf(4060L, 4050L), result.orphans)
        assertTrue(result.unresolved)
        assertTrue(result.conditional)
    }

    @Test fun `consistent pixel views preserve precision and one-view fallback remains a suggestion`() {
        val result = FuelBoardOcr.Result(listOf(FuelBoardOcr.Row(FuelProduct.CNG, 4321L)), false, true, listOf(7400L))
        assertEquals(result, FuelBoardOcr.reconcile(listOf(result)))
        assertEquals(result, FuelBoardOcr.reconcile(listOf(result, result)))
        assertThrows(IllegalArgumentException::class.java) { FuelBoardOcr.reconcile(emptyList()) }
        assertThrows(IllegalArgumentException::class.java) { FuelBoardOcr.reconcile(List(4) { result }) }
    }

    @Test fun `brand sublabel inherits ethanol instead of creating gasoline ambiguity`() {
        val result = FuelBoardOcr.associate(listOf(box("ETANOL", 0, 0),
            box("GRID", 20, 25, 60), box("4,32", 180, 0)))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.ETHANOL, 4320L)), result.rows)
    }

    @Test fun `small generic caption cannot override mixed-case additive brand heading`() {
        val result = FuelBoardOcr.associate(listOf(box("GGRiD", 0, 0, 140),
            FuelBoardOcr.Token("GASOLINA", 5, 35, 65, 43), box("7,19", 180, 0)))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.GASOLINE_PREMIUM, 7190L)), result.rows)
    }

    @Test fun `offset multiline diesel spec and additive caption preserve explicit S10`() {
        val result = FuelBoardOcr.associate(listOf(box("I Diesel", 0, 0, 100),
            FuelBoardOcr.Token("Diesel", 50, 0, 100, 30), box("S10", 45, 32, 35),
            box("ADITIVADO", 45, 60, 70), FuelBoardOcr.Token("6,83", 180, 0, 280, 80)))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.DIESEL_S10, 6830L)), result.rows)
    }

    @Test fun `unknown explicit diesel specifications never default to S500`() {
        for (spec in listOf("S50", "S5", "S100", "S1O", "S-50", "SIO")) {
            val result = FuelBoardOcr.associate(listOf(box("DIESEL $spec", 0, 0, 150), box("7,40", 200, 0)))
            assertTrue(result.rows.isEmpty(), spec)
            assertEquals(listOf(7400L), result.orphans, spec)
            assertTrue(result.unresolved, spec)
        }
    }

    @Test fun `competing normalized prices remain manual without choosing the cheapest`() {
        val result = FuelBoardOcr.associate(listOf(box("ETANOL COMUM", 0, 0, 150), box("3,99", 200, 0),
            box("ETANOL ADITIVADO", 0, 80, 150), box("4,11", 200, 80)))
        assertTrue(result.rows.isEmpty())
        assertEquals(listOf(3990L, 4110L), result.orphans)
        assertTrue(result.unresolved)
    }

    @Test fun `separate integer and fraction blocks recover all aligned fuel rows`() {
        val tokens = listOf(
            box("ETANOL", 0, 0), box("4. 44", 150, 0, 130),
            box("GASOLINA", 0, 60), box("6.", 150, 60, 25), box("91", 195, 60, 45),
            box("DIESEL", 0, 120), box("6", 150, 120, 25), box("35", 195, 120, 45),
            box("DIESEL S10", 0, 180), box("6", 150, 180, 25), box("45", 195, 180, 45),
        )
        val result = FuelBoardOcr.associate(tokens)
        assertEquals(mapOf(FuelProduct.ETHANOL to 4440L, FuelProduct.GASOLINE_REGULAR to 6910L,
            FuelProduct.DIESEL_S500 to 6350L, FuelProduct.DIESEL_S10 to 6450L),
            result.rows.associate { it.product to it.amountMilli })
        assertFalse(result.unresolved)
        assertTrue(result.orphans.isEmpty())
    }

    @Test fun `split fragments require a unique nearby same-row labeled partner`() {
        val cases = listOf(
            listOf(box("6",150,0,25),box("91",195,0,45)),
            listOf(box("GASOLINA",0,0),box("6",150,0,25),box("91",195,60,45)),
            listOf(box("GASOLINA",0,0),box("6",150,0,25),box("91",400,0,45)),
            listOf(box("GASOLINA",0,0),box("6",150,0,25),box("91",195,0,10),box("35",210,0,20)),
            listOf(box("GASOLINA",0,0),box("6",150,0,25),FuelBoardOcr.Token("91",195,10,210,20)),
            listOf(box("GASOLINA",0,0),box("0",150,0,25),box("91",195,0,45)),
            listOf(box("GASOLINA",0,0),box("6",150,0,10),box("7",165,0,10),box("91",195,0,45)),
        )
        for (tokens in cases) {
            val result = FuelBoardOcr.associate(tokens)
            assertTrue(result.rows.isEmpty(), tokens.toString())
            assertTrue(result.orphans.isEmpty(), tokens.toString())
        }
    }

    @Test fun `complete price geometry takes precedence over split child elements`() {
        val result = FuelBoardOcr.associate(listOf(box("GASOLINA",0,0),
            box("6,920",150,0,120),box("6",150,0,25),box("91",195,0,45)))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.GASOLINE_REGULAR,6920L)),result.rows)
        assertFalse(result.unresolved)
    }

    @Test fun `split comma and three fractional digits retain milli precision`() {
        val result = FuelBoardOcr.associate(listOf(box("ETANOL",0,0),
            box("4,",150,0,25),box("321",195,0,55)))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.ETHANOL,4321L)),result.rows)
    }

    @Test fun `associates rows and preserves different variants without choosing minimum`() {
        val result = FuelBoardOcr.associate(listOf(box("GASOLINA",0,0),box("6,59",200,0),box("ADITIVADA",0,50),box("6.74",200,50),box("ETANOL",0,100),box("4,32",200,100)))
        assertEquals(mapOf(FuelProduct.GASOLINE_REGULAR to 6590L,FuelProduct.GASOLINE_PREMIUM to 6740L,FuelProduct.ETHANOL to 4320L),result.rows.associate{it.product to it.amountMilli})
    }
    @Test fun `rejects totals dates zero and unknown specs while preserving bare diesel`() {
        val result = FuelBoardOcr.associate(listOf(box("Diesel S50",0,0),box("7.400",200,0),box("DIESEL",0,50),box("6,15",200,50),box("PODIUM",0,100),box("9,19",200,100),box("DIESEL S500",0,150),box("0,00",200,150),box("TOTAL R$ 549,60",0,200),box("07/10/2026",0,250)))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.DIESEL_S500,6150L)),result.rows)
        assertEquals(listOf(7400L,9190L),result.orphans)
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
    @Test fun `bare diesel reads as common S500`() {
        val result=FuelBoardOcr.associate(listOf(box("DIESEL S500",0,0),box("0.00",150,0),box("DIESEL",0,80),box("7.98",150,80)))
        assertEquals(mapOf(FuelProduct.DIESEL_S500 to 7980L),result.rows.associate{it.product to it.amountMilli})
    }
    @Test fun `recovers board prices missing the decimal separator`() {
        val result=FuelBoardOcr.associate(listOf(box("ETANOL",0,0),box("4 35",200,0),box("GASOLINA",0,50),box("675",200,50)))
        assertEquals(mapOf(FuelProduct.ETHANOL to 4350L,FuelProduct.GASOLINE_REGULAR to 6750L),result.rows.associate{it.product to it.amountMilli})
    }
    @Test fun `distributor and spelled-out variants map without guessing premium`() {
        val result=FuelBoardOcr.associate(listOf(
            box("GRID",0,0),box("6,59",200,0),
            box("GAS NATURAL",0,50,150),box("4,99",200,50),
            box("GASOLINA COMUN",0,100,150),box("5,99",200,100),
            box("DISEL",0,150),box("6,15",200,150)))
        assertEquals(
            mapOf(FuelProduct.GASOLINE_PREMIUM to 6590L,FuelProduct.CNG to 4990L,FuelProduct.GASOLINE_REGULAR to 5990L,FuelProduct.DIESEL_S500 to 6150L),
            result.rows.associate{it.product to it.amountMilli})
        assertTrue(result.orphans.isEmpty())
    }
    @Test fun `unqualified premium brands stay manual instead of vanishing`() {
        val result=FuelBoardOcr.associate(listOf(
            box("PREMIUM",0,0,150),box("7,99",200,0),
            box("PODIUM",0,50),box("9,19",200,50),
            box("OCTAPRO",0,100),box("8,19",200,100)))
        assertTrue(result.rows.isEmpty())
        assertEquals(listOf(7990L,9190L,8190L),result.orphans)
        assertTrue(result.unresolved)
    }
    @Test fun `common diesel wording maps directly to S500`() {
        val result=FuelBoardOcr.associate(listOf(box("DIESEL COMUM",0,0,150),box("6,15",200,0)))
        assertEquals(mapOf(FuelProduct.DIESEL_S500 to 6150L),result.rows.associate{it.product to it.amountMilli})
        assertTrue(result.orphans.isEmpty())
    }
    @Test fun `unrecognized label keeps the value as an orphan for manual choice`() {
        val result=FuelBoardOcr.associate(listOf(box("LUBRIFICANTE XPTO",0,0,150),box("6,15",200,0)))
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
            mapOf(FuelProduct.ETHANOL to 4660L,FuelProduct.GASOLINE_REGULAR to 6840L,FuelProduct.GASOLINE_PREMIUM to 6890L,FuelProduct.DIESEL_S500 to 6770L,FuelProduct.DIESEL_S10 to 6830L),
            result.rows.associate{it.product to it.amountMilli})
        assertTrue(result.orphans.isEmpty())
        assertFalse(result.unresolved)
    }
    @Test fun `explicit premium gasoline has its own grade and ambiguous brands stay manual`() {
        val grade = FuelProduct.valueOf("GASOLINE_PREMIUM_GRADE")
        for (label in listOf("GASOLINA PREMIUM", "Gasolina Podium", "GASOLINA PREMIUM ADITIVADA", "GASOLINA OCTAPRO", "GASOLINA V-POWER RACING")) {
            val result = FuelBoardOcr.associate(listOf(box(label,0,0,180),box("9,19",220,0)))
            assertEquals(listOf(FuelBoardOcr.Row(grade,9190L)), result.rows, label)
        }
        for (label in listOf("PODIUM", "PREMIUM", "RACING", "OCTAPRO", "GASOLINA DIESEL PODIUM")) {
            val result = FuelBoardOcr.associate(listOf(box(label,0,0,180),box("9,19",220,0)))
            assertTrue(result.rows.isEmpty(), label)
            assertEquals(listOf(9190L),result.orphans,label)
        }
    }
    @Test fun `premium brands never convert explicit diesel to gasoline`() {
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.DIESEL_S10,7190L)),
            FuelBoardOcr.associate(listOf(box("DIESEL S10 PODIUM",0,0,180),box("7,19",220,0))).rows)
        assertTrue(FuelBoardOcr.associate(listOf(box("DIESEL PREMIUM",0,0,180),box("7,19",220,0))).rows.isEmpty())
    }

    @Test fun `premium caption joins only its immediate gasoline heading`() {
        val grade=FuelProduct.GASOLINE_PREMIUM_GRADE
        val result=FuelBoardOcr.associate(listOf(box("GASOLINA",0,0,160),box("PODIUM",0,24,160),box("9,19",220,0)))
        assertEquals(listOf(FuelBoardOcr.Row(grade,9190L)),result.rows)
        val distant=FuelBoardOcr.associate(listOf(box("GASOLINA COMUM",0,0,180),box("5,99",220,0),box("PODIUM",0,70,160),box("9,19",220,70)))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.GASOLINE_REGULAR,5990L)),distant.rows)
        assertEquals(listOf(9190L),distant.orphans)
    }
    @Test fun `mixed gasoline board preserves three separate products and conflicting premium prices`() {
        val result=FuelBoardOcr.associate(listOf(box("GASOLINA COMUM",0,0,180),box("5,99",220,0),box("GASOLINA ADITIVADA",0,70,180),box("6,19",220,70),box("GASOLINA PODIUM",0,140,180),box("9,19",220,140)))
        assertEquals(mapOf(FuelProduct.GASOLINE_REGULAR to 5990L,FuelProduct.GASOLINE_PREMIUM to 6190L,FuelProduct.GASOLINE_PREMIUM_GRADE to 9190L),result.rows.associate { it.product to it.amountMilli })
        val conflict=FuelBoardOcr.associate(listOf(box("GASOLINA PREMIUM",0,0,180),box("9,19",220,0),box("GASOLINA PODIUM",0,70,180),box("9,39",220,70)))
        assertTrue(conflict.rows.isEmpty())
        assertTrue(FuelProduct.GASOLINE_PREMIUM_GRADE in conflict.conflictingProducts)
    }

    @Test fun `premium horizontal elements override their enclosing common gasoline element`() {
        val input=listOf(box("GASOLINA PODIUM 9,19",0,0,300),box("GASOLINA",0,0,100),box("PODIUM",110,0,85),box("9,19",220,0))
        assertEquals(listOf(FuelBoardOcr.Row(FuelProduct.GASOLINE_PREMIUM_GRADE,9190L)),FuelBoardOcr.associate(input).rows)
    }

}
