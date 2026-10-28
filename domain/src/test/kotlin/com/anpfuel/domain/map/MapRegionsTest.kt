package com.anpfuel.domain.map

import java.io.File
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotNull
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertThrows
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class MapRegionsTest {
    private val asset = File("../data/src/main/assets/map_regions_brazil.txt")
    private val brazil by lazy { MapRegionIndex.parse(asset.readText()) }

    @Test fun `bundled asset covers exactly the 23 CoMaps Brazil leaves`() {
        assertEquals(23, brazil.countryIds.size)
        assertEquals(MapRegionCatalog.countryIds, brazil.countryIds.toSet())
        assertTrue(asset.length() < 200_000, "region asset must stay compact")
    }

    @Test fun `known city per region resolves to its CoMaps leaf`() {
        val cities = mapOf(
            "Sorriso" to Triple(-12.545, -55.711, "Brazil_Mato Grosso"),
            "São Paulo" to Triple(-23.5505, -46.6333, "Brazil_Southeast Region_Sao Paulo_City"),
            "Campinas" to Triple(-22.9056, -47.0608, "Brazil_Southeast Region_Sao Paulo_Campinas"),
            "Ribeirão Preto" to Triple(-21.17, -47.81, "Brazil_Southeast Region_Sao Paulo_West"),
            "Curitiba" to Triple(-25.4284, -49.2733, "Brazil_Parana_East"),
            "Londrina" to Triple(-23.31, -51.16, "Brazil_Parana_West"),
            "Brasília" to Triple(-15.7939, -47.8828, "Brazil_Goias_Brasilia"),
            "Porangatu" to Triple(-13.44, -49.15, "Brazil_Goias_North"),
            "Manaus" to Triple(-3.119, -60.0217, "Brazil_North Region_West"),
            "Belém" to Triple(-1.45, -48.49, "Brazil_North Region_East"),
            "Porto Alegre" to Triple(-30.0346, -51.2177, "Brazil_South Region_East"),
            "Santa Maria" to Triple(-29.68, -53.81, "Brazil_South Region_West"),
            "Florianópolis" to Triple(-27.59, -48.55, "Brazil_Santa Catarina"),
            "Campo Grande" to Triple(-20.47, -54.62, "Brazil_Mato Grosso Do Sul"),
            "Belo Horizonte" to Triple(-19.9167, -43.9345, "Brazil_Southeast Region_Minas Gerais_Contagem"),
            "Montes Claros" to Triple(-16.73, -43.86, "Brazil_Southeast Region_Minas Gerais_North"),
            "Vitória" to Triple(-20.32, -40.34, "Brazil_Southeast Region_Espirito Santo"),
            "Rio de Janeiro" to Triple(-22.9068, -43.1729, "Brazil_Southeast Region_Rio de Janeiro"),
            "Salvador" to Triple(-12.9777, -38.5016, "Brazil_Bahia"),
            "Recife" to Triple(-8.0476, -34.877, "Brazil_Northeast Region_East"),
            "Fortaleza" to Triple(-3.7172, -38.5433, "Brazil_Northeast Region_West"),
            "Natal" to Triple(-5.79, -35.21, "Brazil_Rio Grande do Norte"),
            "João Pessoa" to Triple(-7.12, -34.86, "Brazil_Paraiba"),
        )
        assertEquals(23, cities.values.map { it.third }.toSet().size, "one city per region")
        for ((city, expected) in cities) {
            assertEquals(expected.third, brazil.regionFor(expected.first, expected.second), city)
        }
    }

    @Test fun `ocean and neighbouring countries are outside every region`() {
        assertNull(brazil.regionFor(-30.0, -20.0), "South Atlantic")
        assertNull(brazil.regionFor(-25.0, -40.0), "offshore Santos basin")
        assertNull(brazil.regionFor(-34.6, -58.4), "Buenos Aires")
        assertNull(brazil.regionFor(-12.05, -77.04), "Lima")
        assertNull(brazil.regionFor(-25.28, -57.63), "Asunción")
        assertNull(brazil.regionFor(Double.NaN, -50.0))
        assertNull(brazil.regionFor(-95.0, -50.0))
    }

    @Test fun `border vertex resolves to a region instead of falling into a sliver`() {
        // Foz do Iguaçu sits on the triple frontier; the region edge still owns it.
        assertEquals("Brazil_Parana_West", brazil.regionFor(-25.54, -54.58))
        // Every vertex of every simplified ring lies on a border and must resolve.
        val text = asset.readText()
        val firstRing = text.lineSequence().first { it.startsWith("O\t") }.substring(2).split(';').first().split(',')
        val vertex = brazil.regionFor(firstRing[1].toDouble() / 10_000, firstRing[0].toDouble() / 10_000)
        assertNotNull(vertex)
    }

    private val synthetic = listOf(
        "abastevo.map-regions-v1",
        "# two regions: A is a square with a hole plus a second island, B is east of A",
        "R\tBrazil_A",
        "O\t0,0;10000,0;0,10000;-10000,0",
        "H\t2500,2500;5000,0;0,5000;-5000,0",
        "O\t30000,0;10000,0;0,10000;-10000,0",
        "R\tBrazil_B",
        "O\t10100,0;10000,0;0,10000;-10000,0",
    ).joinToString("\n")

    @Test fun `even-odd rings handle holes, multipolygons and edge slivers`() {
        val index = MapRegionIndex.parse(synthetic)
        assertEquals("Brazil_A", index.regionFor(0.1, 0.1))
        assertNull(index.regionFor(0.5, 0.5), "inside the hole")
        assertEquals("Brazil_A", index.regionFor(0.5, 3.5), "second polygon")
        assertEquals("Brazil_B", index.regionFor(0.5, 1.5))
        assertEquals("Brazil_A", index.regionFor(0.5, 1.003), "sliver closer to A")
        assertEquals("Brazil_B", index.regionFor(0.5, 1.008), "sliver closer to B")
        assertNull(index.regionFor(0.5, 2.5), "beyond the edge tolerance")
        assertNull(index.regionFor(5.0, 5.0))
    }

    @Test fun `malformed index is refused`() {
        for (bad in listOf(
            "",
            "abastevo.map-regions-v0\nR\tBrazil_A\nO\t0,0;1,0;0,1",
            "abastevo.map-regions-v1\nO\t0,0;1,0;0,1",
            "abastevo.map-regions-v1\nR\tBrazil_A",
            "abastevo.map-regions-v1\nR\tBrazil_A\nO\t0,0;1,0",
            "abastevo.map-regions-v1\nR\tBrazil_A\nO\t0,0;x,0;0,1",
            "abastevo.map-regions-v1\nR\tArgentina_A\nO\t0,0;1,0;0,1",
            "abastevo.map-regions-v1\nR\tBrazil_A\nO\t0,0;1,0;0,1\nR\tBrazil_A\nO\t0,0;1,0;0,1",
            "abastevo.map-regions-v1\nR\tBrazil_A\nO\t0,0;1,0;0,1\nX\t1",
            "abastevo.map-regions-v1\nR\tBrazil_A\nO\t1900000,0;1,0;0,1",
        )) {
            assertThrows(MapContractException::class.java) { MapRegionIndex.parse(bad) }
        }
    }

    @Test fun `slug follows the distribution contract for every region`() {
        assertEquals("brazil-mato-grosso", MapRegionCatalog.slugOf("Brazil_Mato Grosso"))
        assertEquals("brazil-northeast-region-west", MapRegionCatalog.slugOf("Brazil_Northeast Region_West"))
        assertEquals(
            "brazil-southeast-region-sao-paulo-city",
            MapRegionCatalog.slugOf("Brazil_Southeast Region_Sao Paulo_City"),
        )
        assertEquals("brazil-sao-paulo-city", MapRegionCatalog.slugOf("Brazil_Sao Paulo_City"))
        assertEquals("brazil-mato-grosso-do-sul", MapRegionCatalog.slugOf("Brazil_Mato Grosso Do Sul"))
        assertEquals("brazil-a-b", MapRegionCatalog.slugOf("Brazil_A _ B"))
        val slugs = MapRegionCatalog.countryIds.map { MapRegionCatalog.slugOf(it) }
        assertEquals(23, slugs.toSet().size)
        slugs.forEach { assertTrue(Regex("^brazil-[a-z0-9-]+$").matches(it) && "--" !in it, it) }
        MapRegionCatalog.countryIds.forEach {
            assertEquals(it, MapRegionCatalog.countryIdForSlug(MapRegionCatalog.slugOf(it)))
            assertEquals("$it.mwm", MapRegionCatalog.nativeFileName(it))
            assertTrue(isSafeMapPackagePath(MapRegionCatalog.nativeFileName(it)), it)
        }
        assertNull(MapRegionCatalog.countryIdForSlug("brazil-atlantis"))
        assertThrows(MapContractException::class.java) { MapRegionCatalog.slugOf("Argentina_Buenos Aires") }
        assertThrows(MapContractException::class.java) { MapRegionCatalog.slugOf("Brazil_São Paulo") }
    }

    @Test fun `display names are human readable Portuguese`() {
        assertEquals("Mato Grosso", MapRegionCatalog.displayName("Brazil_Mato Grosso"))
        assertEquals("São Paulo (capital e litoral)", MapRegionCatalog.displayName("Brazil_Southeast Region_Sao Paulo_City"))
        assertEquals("Nordeste – Oeste", MapRegionCatalog.displayName("Brazil_Northeast Region_West"))
        MapRegionCatalog.countryIds.forEach { assertTrue(!MapRegionCatalog.displayName(it).startsWith("Brazil_"), it) }
        assertEquals("Brazil_Unknown", MapRegionCatalog.displayName("Brazil_Unknown"))
    }

    @Test fun `city point prefers the camera centered on the city, then stations, then built-in`() {
        val camera = MapRegionPoint(-12.5, -55.7)
        val builtIn = MapRegionPoint(-12.0, -55.0)
        val stations = listOf(MapRegionPoint(-10.0, -50.0), MapRegionPoint(-12.0, -52.0))
        assertEquals(camera, chooseCityRegionPoint(camera, stations, builtIn))
        assertEquals(MapRegionPoint(-11.0, -51.0), chooseCityRegionPoint(null, stations, builtIn))
        assertEquals(
            MapRegionPoint(-11.0, -51.0),
            chooseCityRegionPoint(MapRegionPoint(Double.NaN, 0.0), stations + MapRegionPoint(99.0, 0.0), builtIn),
        )
        assertEquals(builtIn, chooseCityRegionPoint(null, emptyList(), builtIn))
        assertNull(chooseCityRegionPoint(null, emptyList(), null))
    }
}
