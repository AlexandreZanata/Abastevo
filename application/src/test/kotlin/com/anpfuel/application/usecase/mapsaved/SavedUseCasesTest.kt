package com.anpfuel.application.usecase.mapsaved

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.portable.PortableAuth
import com.anpfuel.domain.map.saved.*
import com.anpfuel.domain.routing.NavigationFix
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class SavedUseCasesTest {
    private var account: String? = null
    private val sessions = mockk<AuthSessionStore>().also {
        every { it.load() } answers { account?.let { id -> mockk<PortableAuth.Session>().also { s -> every { s.accountId } returns id } } }
    }
    private val owners = SavedOwnerResolver(sessions, "https://api.example.test")
    private var seq = 0
    private fun id() = String.format("00000000-0000-4000-8000-%012d", ++seq)
    private val station = "33333333-3333-4333-8333-333333333333"

    /** Small faithful in-memory store; the real one is proven on Room in the instrumented suite. */
    private class FakeStore : SavedPrivateStore {
        val places = LinkedHashMap<Pair<SavedOwner, String>, SavedPlace>()
        val trips = LinkedHashMap<Pair<SavedOwner, String>, SavedTrip>()
        val recordings = HashMap<SavedOwner, Triple<String, String, MutableList<TripPoint>>>()
        var imports = 0
        var onAccess: () -> Unit = {}
        override suspend fun places(owner: SavedOwner) = places.filterKeys { it.first == owner }.values.toList().also { onAccess() }
        override suspend fun stationFavorite(owner: SavedOwner, stationId: String) = places(owner).firstOrNull { it.stationId == stationId }
        override suspend fun savePlace(owner: SavedOwner, place: SavedPlace): SavedPlace { places[owner to place.id] = place; return place }
        override suspend fun removePlace(owner: SavedOwner, id: String) = places.remove(owner to id) != null
        override suspend fun trips(owner: SavedOwner) = trips.filterKeys { it.first == owner }.values.map { TripSummary(it.id, it.kind, it.name, it.points.size, it.avoid, it.createdAtMillis) }
        override suspend fun trip(owner: SavedOwner, id: String) = trips[owner to id]
        override suspend fun saveTrip(owner: SavedOwner, trip: SavedTrip) { trips[owner to trip.id] = trip }
        override suspend fun removeTrip(owner: SavedOwner, id: String) = trips.remove(owner to id) != null
        override suspend fun importContent(owner: SavedOwner, content: ExchangeContent, allowDeleted: Boolean, nowMillis: Long): ImportReport {
            imports++
            content.places.forEach { places[owner to it.id] = it }
            content.trips.forEach { trips[owner to it.id] = it }
            return ImportReport(content.places.size, content.trips.size, 0, false)
        }
        override suspend fun startRecording(owner: SavedOwner, id: String, name: String, nowMillis: Long): Boolean {
            if (recordings.containsKey(owner)) return false
            recordings[owner] = Triple(id, name, mutableListOf()); return true
        }
        override suspend fun recording(owner: SavedOwner) = recordings[owner]?.let { RecordingState(it.first, it.second, 0, it.third.size) }
        override suspend fun appendRecording(owner: SavedOwner, id: String, points: List<TripPoint>): Int {
            val r = recordings[owner]?.takeIf { it.first == id } ?: throw SavedValidationException("recording-missing")
            r.third += points; return r.third.size
        }
        override suspend fun finishRecording(owner: SavedOwner, id: String, save: Boolean, nowMillis: Long): SavedTrip? {
            val r = recordings.remove(owner) ?: return null
            if (!save || r.third.isEmpty()) return null
            return SavedTrip(id, SavedTripKind.RECORDED_TRACK, r.second, r.third, createdAtMillis = nowMillis).also { trips[owner to id] = it }
        }
        override suspend fun clearOwner(owner: SavedOwner) = Unit
    }

    private val store = FakeStore()

    @Test fun `favorite toggles on and off and belongs to the current owner only`() = runTest {
        val toggle = ToggleStationFavoriteUseCase(store, owners, { 5 }, ::id)
        assertTrue(toggle(station, "Posto", -25.4295, -49.2719, SavedSourceKind.ANP, "ANP-9"))
        assertTrue(IsStationFavoriteUseCase(store, owners)(station))
        account = "acct-b"
        assertFalse(IsStationFavoriteUseCase(store, owners)(station))
        assertTrue(ListSavedUseCase(store, owners)().places.isEmpty())
        account = null
        assertEquals(1, ListSavedUseCase(store, owners)().places.size)
        assertFalse(toggle(station, "Posto", -25.4295, -49.2719, SavedSourceKind.ANP, "ANP-9"))
        assertTrue(ListSavedUseCase(store, owners)().places.isEmpty())
    }

    @Test fun `an account switch during a call refuses to hand the result to the new owner`() = runTest {
        account = "acct-a"
        store.savePlace(owners.current(), SavedPlace(id(), SavedPlaceKind.PLACE, "Casa", GeoPoint(1, 1), null, SavedSourceKind.MANUAL, null, null, 0))
        store.onAccess = { account = "acct-b" }
        try { ListSavedUseCase(store, owners)(); fail("owner changed") } catch (e: SavedValidationException) { assertEquals("owner-scope-changed", e.code) }
    }

    @Test fun `route plans save only the plan and validate points`() = runTest {
        val saved = SaveRoutePlanUseCase(store, owners, { 7 }, ::id)("Casa → Trabalho", listOf(-25.4295 to -49.2719, -25.4222 to -49.264), setOf("TOLL"))
        val trip = store.trips.values.single()
        assertEquals(saved, trip.id)
        assertEquals(SavedTripKind.ROUTE_PLAN, trip.kind)
        assertEquals(setOf("TOLL"), trip.avoid)
        assertTrue(trip.points.all { it.timeMillis == null && it.elevationCm == null })
        try { SaveRoutePlanUseCase(store, owners)("x", listOf(-25.0 to -49.0), emptySet()); fail() } catch (e: SavedValidationException) { assertEquals("route-plan-points-invalid", e.code) }
    }

    @Test fun `a malicious file is refused before storage is touched`() = runTest {
        val import = ImportExchangeUseCase(store, owners, { 1 }, ::id)
        val xxe = """<?xml version="1.0"?><!DOCTYPE a [<!ENTITY x SYSTEM "file:///etc/passwd">]><gpx><wpt lat="1" lon="1"><name>&x;</name></wpt></gpx>"""
        try { import(xxe.toByteArray()); fail() } catch (e: SavedValidationException) { assertEquals("xml-doctype-refused", e.code) }
        assertEquals(0, store.imports)
        val ok = GeoExchange.writeGpx(listOf(SavedPlace(id(), SavedPlaceKind.PLACE, "Casa", GeoPoint(1, 1), null, SavedSourceKind.MANUAL, null, null, 0)), emptyList())
        assertEquals(1, import(ok).importedPlaces)
        assertEquals(1, store.imports)
    }

    @Test fun `import ignores any owner claim in the file and lands in the current owner`() = runTest {
        account = "acct-a"
        val file = """<gpx><wpt lat="1" lon="1"><name>x</name><extensions><abv:owner>acct-b</abv:owner><abv:id>${id()}</abv:id></extensions></wpt></gpx>"""
        ImportExchangeUseCase(store, owners, { 1 }, ::id)(file.toByteArray())
        assertTrue(store.places.keys.all { it.first.owner == "acct-a" })
    }

    @Test fun `export covers only the current owner and refuses when empty`() = runTest {
        account = "acct-a"
        val toggle = ToggleStationFavoriteUseCase(store, owners, { 5 }, ::id)
        toggle(station, "Posto A", -25.4295, -49.2719, SavedSourceKind.ANP, null)
        account = "acct-b"
        toggle(station, "Posto B", -25.0, -49.0, SavedSourceKind.COMMUNITY, null)
        val export = ExportExchangeUseCase(store, owners)
        val gpxB = export(ExchangeFormat.GPX).decodeToString()
        assertTrue(gpxB.contains("Posto B"))
        assertFalse(gpxB.contains("Posto A"))
        account = "acct-c"
        try { export(ExchangeFormat.KML); fail() } catch (e: SavedValidationException) { assertEquals("nothing-to-export", e.code) }
    }

    private fun fix(lat: Double, acc: Float = 5f, now: Long = 10_000_000_000L) = NavigationFix(lat, -49.27, acc, 1_790_000_000_000, now)

    @Test fun `recording needs an explicit start gates fixes and saves or discards on stop`() = runTest {
        val recorder = TrackRecordingUseCase(store, owners, { 9 }, ::id)
        val now = 10_000_000_000L
        assertNull(recorder.onFix(fix(-25.0), now)) // not started: nothing is recorded
        assertTrue(recorder.start("  Passeio  "))
        assertFalse(recorder.start("again"))
        assertEquals(1, recorder.onFix(fix(-25.0), now))
        assertNull(recorder.onFix(fix(-25.00001), now)) // under 10 m
        assertNull(recorder.onFix(fix(-25.0001, acc = 80f), now)) // poor accuracy
        assertEquals(2, recorder.onFix(fix(-25.0001), now))
        val saved = recorder.stop(save = true)!!
        assertEquals("Passeio", saved.name)
        assertEquals(2, saved.points.size)
        assertNull(recorder.state())
        assertTrue(recorder.start("x"))
        recorder.onFix(fix(-26.0), now)
        assertNull(recorder.stop(save = false))
        assertEquals(1, store.trips.size)
    }
}
