package com.anpfuel.data.local.saved

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.application.usecase.mapsaved.ExchangeFormat
import com.anpfuel.application.usecase.mapsaved.ExportExchangeUseCase
import com.anpfuel.application.usecase.mapsaved.ImportExchangeUseCase
import com.anpfuel.application.usecase.mapsaved.SavedOwnerResolver
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.domain.map.saved.*
import com.anpfuel.domain.portable.PortableAuth
import java.io.ByteArrayOutputStream
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/** MAP-25 adversarial exchange files through the real import use case and Room on the device runtime. */
@RunWith(AndroidJUnit4::class)
class SavedExchangeDeviceTest {
    private lateinit var db: AnpFuelDatabase
    private lateinit var store: RoomSavedStore
    private lateinit var importer: ImportExchangeUseCase
    private lateinit var exporter: ExportExchangeUseCase
    private val owner = SavedOwner(SavedOwner.GUEST, "https://api.example.test")

    @Before fun prepare() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AnpFuelDatabase::class.java).build()
        store = RoomSavedStore(db)
        val resolver = SavedOwnerResolver(object : AuthSessionStore {
            override fun save(session: PortableAuth.Session) = Unit
            override fun load(): PortableAuth.Session? = null
            override fun clear() = Unit
        }, owner.origin)
        importer = ImportExchangeUseCase(store, resolver)
        exporter = ExportExchangeUseCase(store, resolver)
    }

    @After fun finish() { db.close() }

    private fun refused(bytes: ByteArray, prefix: String) = runBlocking {
        try { importer(bytes); fail("must refuse ($prefix)") } catch (e: SavedValidationException) {
            assertTrue("${e.code} should start with $prefix", e.code.startsWith(prefix))
        }
        assertTrue(store.places(owner).isEmpty())
        assertTrue(store.trips(owner).isEmpty())
    }

    private fun zip(vararg entries: Pair<String, ByteArray>): ByteArray {
        val out = ByteArrayOutputStream()
        ZipOutputStream(out).use { z -> entries.forEach { (n, b) -> z.putNextEntry(ZipEntry(n)); z.write(b); z.closeEntry() } }
        return out.toByteArray()
    }

    @Test fun hostileFilesAreRefusedWithoutCreatingAnyRow() {
        refused("""<?xml version="1.0"?><!DOCTYPE a [<!ENTITY x SYSTEM "file:///proc/self/environ">]><gpx><wpt lat="1" lon="1"><name>&x;</name></wpt></gpx>""".toByteArray(), "xml-doctype")
        refused("""<!DOCTYPE lolz [<!ENTITY lol "lol"><!ENTITY l2 "&lol;&lol;&lol;&lol;&lol;&lol;&lol;&lol;&lol;&lol;">]><gpx><wpt lat="1" lon="1"><name>&l2;</name></wpt></gpx>""".toByteArray(), "xml-doctype")
        refused(("<a>".repeat(200) + "</a>".repeat(200)).toByteArray(), "xml-too-deep")
        refused("<gpx>${"<wpt lat=\"1\" lon=\"1\"/>".repeat(5_001)}</gpx>".toByteArray(), "too-many")
        refused("<gpx><wpt lat=\"1e9\" lon=\"x\"/></gpx>".toByteArray(), "coordinate-")
        refused(ByteArray(SavedBounds.MAX_FILE_BYTES + 1) { 'a'.code.toByte() }, "file-too-large")
        val kml = GeoExchange.writeKml(listOf(SavedPlace("11111111-1111-4111-8111-111111111111", SavedPlaceKind.PLACE, "x", GeoPoint(1, 1), null, SavedSourceKind.MANUAL, null, null, 0)), emptyList())
        refused(zip("../../evil.kml" to kml), "archive-name")
        refused(zip("doc.kml" to kml, "inner.kmz" to zip("doc.kml" to kml)), "archive-nested")
        refused(zip("doc.kml" to ByteArray(SavedBounds.MAX_EXPANDED_BYTES + 1)), "archive-too-large")
    }

    @Test fun validGpxKmlAndKmzImportOnceAndExportBackOnlyThisOwnersData() = runBlocking {
        val place = SavedPlace("11111111-1111-4111-8111-111111111111", SavedPlaceKind.STATION, "Posto A & B", GeoPoint.parse("-25.4295", "-49.2719"),
            "33333333-3333-4333-8333-333333333333", SavedSourceKind.ANP, "ANP-7", "nota", 0)
        val track = SavedTrip("22222222-2222-4222-8222-222222222222", SavedTripKind.RECORDED_TRACK, "Trilha",
            List(300) { TripPoint(GeoPoint(it.toLong() * 100, it.toLong() * 50), it, 1_000L + it) }, createdAtMillis = 0)
        val gpx = GeoExchange.writeGpx(listOf(place), listOf(track))
        assertEquals(1, importer(gpx).importedPlaces)
        assertTrue(importer(gpx).alreadyImported)
        assertEquals(place.copy(createdAtMillis = store.places(owner).single().createdAtMillis), store.places(owner).single())
        assertEquals(track.points, store.trip(owner, track.id)!!.points)
        // The same content as KML and as KMZ is recognised as existing ids (skipped), never duplicated.
        val kml = GeoExchange.writeKml(listOf(place), listOf(track))
        assertEquals(0, importer(kml).importedPlaces)
        assertEquals(0, importer(zip("doc.kml" to kml, "files/icon.png" to byteArrayOf(1, 2))).importedTrips)
        assertEquals(1, store.places(owner).size)
        assertEquals(1, store.trips(owner).size)
        val exportedGpx = exporter(ExchangeFormat.GPX).decodeToString()
        assertTrue(exportedGpx.contains("Posto A &amp; B"))
        assertTrue(exportedGpx.contains("ANP-7"))
        assertTrue(exportedGpx.contains(GeoExchange.LICENSE_NOTICE.take(30)))
        assertEquals(track.id, GeoExchange.parse(exporter(ExchangeFormat.KML, tripId = track.id), 1) { "aaaaaaaa-0000-4000-8000-000000000001" }.trips.single().id)
    }
}
