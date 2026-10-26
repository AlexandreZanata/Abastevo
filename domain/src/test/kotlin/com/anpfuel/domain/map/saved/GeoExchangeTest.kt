package com.anpfuel.domain.map.saved

import java.io.ByteArrayOutputStream
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class GeoExchangeTest {
    private var seq = 0
    private fun newId() = "aaaaaaaa-0000-4000-8000-" + (++seq).toString().padStart(12, '0')
    private fun parse(text: String) = GeoExchange.parse(text.toByteArray(), 1_000L, ::newId)
    private fun parseBytes(bytes: ByteArray) = GeoExchange.parse(bytes, 1_000L, ::newId)
    private fun refusal(bytes: ByteArray): String = try { parseBytes(bytes); "ok" } catch (e: SavedValidationException) { e.code }
    private fun refusal(text: String) = refusal(text.toByteArray())

    private val stationId = "33333333-3333-4333-8333-333333333333"
    private val place = SavedPlace("11111111-1111-4111-8111-111111111111", SavedPlaceKind.STATION, "Posto Ipiranga & Cia <A>",
        GeoPoint.parse("-25.4295", "-49.2719"), stationId, SavedSourceKind.ANP, "ANP-123", "Aberto 24h", 5)
    private val plan = SavedTrip("22222222-2222-4222-8222-222222222222", SavedTripKind.ROUTE_PLAN, "Casa → Trabalho",
        listOf(TripPoint(GeoPoint.parse("-25.4295", "-49.2719")), TripPoint(GeoPoint.parse("-25.43", "-49.27")), TripPoint(GeoPoint.parse("-25.4222", "-49.264"))),
        setOf("TOLL", "FERRY"), "plan-1", "nota", 6)
    private val track = SavedTrip("44444444-4444-4444-8444-444444444444", SavedTripKind.RECORDED_TRACK, "Ride",
        listOf(TripPoint(GeoPoint(1, 2), 12345, 1_700_000_000_000), TripPoint(GeoPoint(3, 4), -50, 1_700_000_005_000)), createdAtMillis = 7)

    @Test fun `gpx round trip keeps ids source ids notes exact coordinates and the license notice`() {
        val bytes = GeoExchange.writeGpx(listOf(place), listOf(plan, track))
        val text = bytes.decodeToString()
        assertTrue(text.contains(GeoExchange.LICENSE_NOTICE.take(40)))
        assertTrue(text.contains("lat=\"-25.429500\""))
        val back = parseBytes(bytes)
        assertEquals(listOf(place.copy(createdAtMillis = 1_000)), back.places)
        assertEquals(listOf(plan.copy(createdAtMillis = 1_000), track.copy(createdAtMillis = 1_000)).sortedBy { it.id }, back.trips.sortedBy { it.id })
        assertEquals(0, back.skippedUnsupported)
    }

    @Test fun `kml round trip keeps ids source ids and avoid options`() {
        val bytes = GeoExchange.writeKml(listOf(place), listOf(plan, track.copy(points = listOf(TripPoint(GeoPoint(1, 2), 12345), TripPoint(GeoPoint(3, 4))))))
        val back = parseBytes(bytes)
        assertEquals(place.copy(createdAtMillis = 1_000), back.places.single())
        assertEquals(setOf("TOLL", "FERRY"), back.trips.single { it.kind == SavedTripKind.ROUTE_PLAN }.avoid)
        assertEquals("Casa → Trabalho", back.trips.single { it.kind == SavedTripKind.ROUTE_PLAN }.name)
        assertEquals(12345, back.trips.single { it.kind == SavedTripKind.RECORDED_TRACK }.points[0].elevationCm)
    }

    @Test fun `foreign gpx without abastevo extensions imports as plain places and imported tracks`() {
        val gpx = """<?xml version="1.0"?><gpx version="1.0" creator="Other"><wpt lat="-25.43" lon="-49.27"><name>Cafe</name><cmt>nice</cmt></wpt>
            <trk><name>Morning</name><trkseg><trkpt lat="-25.43" lon="-49.27"><ele>900.5</ele><time>2026-10-01T10:00:00Z</time></trkpt>
            <trkpt lat="-25.44" lon="-49.28"><ele>bad</ele><time>garbage</time></trkpt></trkseg><trkseg><trkpt lat="-25.45" lon="-49.29"/></trkseg></trk>
            <rte><name>Plan</name><rtept lat="1" lon="1"/><rtept lat="2" lon="2"/></rte><unknownThing/></gpx>"""
        val c = parse(gpx)
        assertEquals(SavedPlaceKind.PLACE, c.places.single().kind)
        assertEquals(SavedSourceKind.IMPORT, c.places.single().sourceKind)
        assertEquals("nice", c.places.single().note)
        val morning = c.trips.single { it.name == "Morning" }
        assertEquals(SavedTripKind.IMPORTED_TRACK, morning.kind)
        assertEquals(3, morning.points.size)
        assertEquals(90_050, morning.points[0].elevationCm)
        assertEquals(1_790_848_800_000L, morning.points[0].timeMillis)
        assertNull(morning.points[1].elevationCm)
        assertNull(morning.points[1].timeMillis)
        assertEquals(SavedTripKind.IMPORTED_TRACK, c.trips.single { it.name == "Plan" }.kind) // foreign rte never becomes a plan
        assertEquals(1, c.skippedUnsupported)
    }

    @Test fun `foreign kml with folders multigeometry html description and unsupported shapes`() {
        val kml = """<kml xmlns="http://www.opengis.net/kml/2.2" xmlns:gx="x"><Document><Folder><name>f</name>
            <Placemark><name>P1</name><description><![CDATA[<b>Hello</b>  <i>world</i>]]></description><Point><coordinates>-49.2719,-25.4295,900</coordinates></Point></Placemark>
            <Placemark><name>Multi</name><MultiGeometry><Point><coordinates>-49.1,-25.1</coordinates></Point><LineString><coordinates>-49.1,-25.1 -49.2,-25.2</coordinates></LineString></MultiGeometry></Placemark>
            <Placemark><name>Area</name><Polygon><outerBoundaryIs><LinearRing><coordinates>0,0 1,1 1,0 0,0</coordinates></LinearRing></outerBoundaryIs></Polygon></Placemark>
            <Placemark><name>T</name><gx:Track/></Placemark></Folder></Document></kml>"""
        val c = parse(kml)
        assertEquals(listOf("P1", "Multi"), c.places.map { it.name })
        assertEquals("Hello world", c.places[0].note)
        assertEquals(1, c.trips.size)
        assertEquals(2, c.trips.single().points.size)
        assertTrue(c.skippedUnsupported >= 2)
    }

    @Test fun `too long names and notes are truncated and counted never silently exceed bounds`() {
        val c = parse("""<gpx version="1.1"><wpt lat="1" lon="1"><name>${"n".repeat(200)}</name><desc>${"d".repeat(900)}</desc></wpt></gpx>""")
        assertEquals(120, c.places.single().name.length)
        assertEquals(500, c.places.single().note!!.length)
        assertEquals(2, c.truncatedFields)
    }

    @Test fun `malformed coordinates and structural problems refuse the whole file`() {
        val ok = """<wpt lat="1" lon="1"><name>ok</name></wpt>"""
        for (bad in listOf("""<wpt lat="abc" lon="1"/>""", """<wpt lat="1"/>""", """<wpt lat="91" lon="0"/>""", """<wpt lat="1e2" lon="0"/>""", """<wpt lat="NaN" lon="0"/>""",
            """<wpt lat="0" lon="181"/>""", """<trk><trkseg><trkpt lat="0" lon="x"/></trkseg></trk>""")) {
            assertNotEquals("ok", refusal("<gpx version=\"1.1\">$ok$bad</gpx>"), bad)
        }
        for (bad in listOf("<Placemark><Point><coordinates>1</coordinates></Point></Placemark>", "<Placemark><Point><coordinates>a,b</coordinates></Point></Placemark>",
            "<Placemark><Point><coordinates>1,2 3,4</coordinates></Point></Placemark>", "<Placemark><Point/></Placemark>")) {
            assertNotEquals("ok", refusal("<kml><Document>$bad</Document></kml>"), bad)
        }
        assertEquals("format-unsupported", refusal("<html/>"))
        assertEquals("file-empty", refusal(""))
        assertEquals("file-has-no-records", refusal("<gpx version=\"1.1\"/>"))
        assertEquals("duplicate-record-id", refusal("""<gpx><wpt lat="1" lon="1"><extensions><abv:id>${place.id}</abv:id></extensions></wpt><wpt lat="2" lon="2"><extensions><abv:id>${place.id}</abv:id></extensions></wpt></gpx>"""))
    }

    @Test fun `point and record limits are enforced and xxe never reaches records`() {
        val big = "<gpx><trk><trkseg>" + (1..10_001).joinToString("") { """<trkpt lat="1" lon="1"/>""" } + "</trkseg></trk></gpx>"
        assertEquals("track-too-many-points", refusal(big))
        val exact = "<gpx><trk><trkseg>" + (1..10_000).joinToString("") { """<trkpt lat="1" lon="1"/>""" } + "</trkseg></trk></gpx>"
        assertEquals(10_000, parse(exact).trips.single().points.size)
        val many = "<gpx>" + (1..5_001).joinToString("") { """<wpt lat="1" lon="1"/>""" } + "</gpx>"
        assertEquals("too-many-records", refusal(many))
        assertEquals("xml-doctype-refused", refusal("""<?xml version="1.0"?><!DOCTYPE gpx [<!ENTITY xxe SYSTEM "file:///data/data/x">]><gpx><wpt lat="1" lon="1"><name>&xxe;</name></wpt></gpx>"""))
    }

    @Test fun `same bytes give the same content hash and different bytes differ`() {
        val a = GeoExchange.writeGpx(listOf(place), emptyList())
        assertEquals(parseBytes(a).contentSha256, parseBytes(a.copyOf()).contentSha256)
        assertNotEquals(parseBytes(a).contentSha256, parseBytes(GeoExchange.writeKml(listOf(place), emptyList())).contentSha256)
    }

    // ---- KMZ -------------------------------------------------------------------

    private fun zip(vararg entries: Pair<String, ByteArray>): ByteArray {
        val out = ByteArrayOutputStream()
        ZipOutputStream(out).use { z -> entries.forEach { (n, b) -> z.putNextEntry(ZipEntry(n)); z.write(b); z.closeEntry() } }
        return out.toByteArray()
    }
    private val kml = GeoExchange.writeKml(listOf(place), emptyList())

    @Test fun `kmz with root doc kml imports and prefers doc kml`() {
        assertEquals(place.name, parseBytes(zip("other.kml" to GeoExchange.writeKml(listOf(place.copy(name = "other", id = "55555555-5555-4555-8555-555555555555")), emptyList()), "doc.kml" to kml)).places.single().name)
        assertEquals(1, parseBytes(zip("files/icon.png" to byteArrayOf(1, 2, 3), "doc.kml" to kml)).places.size)
    }

    @Test fun `kmz traversal absolute backslash drive and nested archive names are refused`() {
        for (name in listOf("../evil.kml", "a/../../evil.kml", "/abs.kml", "..\\evil.kml", "C:evil.kml", "x\\y.kml")) {
            assertEquals("archive-name-refused", refusal(zip(name to kml)), name)
        }
        for (name in listOf("inner.kmz", "a.zip", "lib.jar", "x.gz")) {
            assertEquals("archive-nested-refused", refusal(zip("doc.kml" to kml, name to byteArrayOf(1))), name)
        }
        assertEquals("archive-nested-refused", refusal(zip("doc.kml" to kml, "inner.kmz" to zip("doc.kml" to kml))))
    }

    @Test fun `kmz without a root kml or with only nested kml is refused`() {
        assertEquals("archive-no-kml", refusal(zip("readme.txt" to byteArrayOf(1))))
        assertEquals("archive-no-kml", refusal(zip("deep/doc.kml" to kml)))
    }

    @Test fun `kmz zip bomb entry count and expanded size limits hold without writing files`() {
        val bomb = zip("doc.kml" to ByteArray(SavedBounds.MAX_EXPANDED_BYTES + 1))
        assertTrue(bomb.size < 100_000, "the bomb must be tiny to be a real bomb: ${bomb.size}")
        assertEquals("archive-too-large", refusal(bomb))
        val padding = ByteArray(4 * 1024 * 1024)
        val big = zip("a.bin" to padding, "b.bin" to padding, "c.bin" to padding, "d.bin" to padding, "e.bin" to padding, "f.bin" to padding, "doc.kml" to kml)
        assertEquals("archive-too-large", refusal(big))
        val many = zip(*Array(65) { "f$it.txt" to byteArrayOf(1) })
        assertEquals("archive-too-many-entries", refusal(many))
        assertTrue(refusal(byteArrayOf(0x50, 0x4B, 0x03, 0x04, 1, 2, 3)).startsWith("archive-"))
        assertTrue(refusal(zip("doc.kml" to kml).let { it.copyOf(it.size / 2) }).startsWith("archive-"))
    }

    @Test fun `a kml inside a kmz still gets xxe protection`() {
        val evil = """<?xml version="1.0"?><!DOCTYPE kml [<!ENTITY x SYSTEM "file:///etc/passwd">]><kml><Document/></kml>""".toByteArray()
        assertEquals("xml-doctype-refused", refusal(zip("doc.kml" to evil)))
    }
}
