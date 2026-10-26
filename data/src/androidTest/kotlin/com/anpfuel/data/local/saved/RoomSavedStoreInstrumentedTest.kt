package com.anpfuel.data.local.saved

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.domain.map.saved.*
import java.util.UUID
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.*
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/** MAP-25 on real Room/SQLite: isolation, quotas, atomic import, tombstones, recording, restart. Synthetic data only. */
@RunWith(AndroidJUnit4::class)
class RoomSavedStoreInstrumentedTest {
    private lateinit var context: Context
    private lateinit var name: String
    private var db: AnpFuelDatabase? = null
    private val origin = "https://api.example.test"
    private val a = SavedOwner("acct-a", origin)
    private val b = SavedOwner("acct-b", origin)
    private val guest = SavedOwner(SavedOwner.GUEST, origin)
    private var seq = 0

    private fun id() = String.format("00000000-0000-4000-8000-%012d", ++seq)
    private fun open(): RoomSavedStore {
        db?.close()
        db = Room.databaseBuilder(context, AnpFuelDatabase::class.java, name).build()
        return RoomSavedStore(db!!)
    }

    @Before fun prepare() { context = ApplicationProvider.getApplicationContext(); name = "map25-" + UUID.randomUUID() + ".db" }
    @After fun finish() { db?.close(); context.deleteDatabase(name) }

    private fun station(station: String = id(), n: String = "Posto") =
        SavedPlace(id(), SavedPlaceKind.STATION, n, GeoPoint.parse("-25.4295", "-49.2719"), station, SavedSourceKind.ANP, "ANP-1", "nota", 1)

    private fun track(points: Int, trackId: String = id()) = SavedTrip(
        trackId, SavedTripKind.IMPORTED_TRACK, "Trilha",
        List(points) { TripPoint(GeoPoint(it.toLong(), -it.toLong()), it, 1_000L + it) }, createdAtMillis = 2,
    )

    private fun content(places: List<SavedPlace> = emptyList(), trips: List<SavedTrip> = emptyList(), sha: String = id()) =
        ExchangeContent(places, trips, sha.replace("-", "").padEnd(64, '0'), 0, 0)

    private suspend fun refusal(code: String, work: suspend () -> Unit) {
        try { work(); fail("expected $code") } catch (e: SavedValidationException) { assertEquals(code, e.code) }
    }

    @Test fun favorite_is_idempotent_persists_restart_and_is_isolated_per_owner() = runBlocking {
        val store = open()
        val favorite = station("33333333-3333-4333-8333-333333333333")
        assertEquals(favorite, store.savePlace(a, favorite))
        // Same station again (different record id) converges on the stored row.
        val again = store.savePlace(a, favorite.copy(id = id(), name = "Outro nome"))
        assertEquals(favorite, again)
        assertEquals(1, store.places(a).size)
        assertTrue(store.places(b).isEmpty())
        assertTrue(store.places(guest).isEmpty())
        assertNull(store.stationFavorite(b, favorite.stationId!!))
        assertFalse("another owner cannot delete it", store.removePlace(b, favorite.id))
        assertFalse(store.removePlace(guest, favorite.id))
        // The same station and record id for another owner is a separate row.
        assertEquals(favorite, store.savePlace(b, favorite))
        assertEquals(1, store.places(b).size)
        val restarted = open()
        assertEquals(listOf(favorite), restarted.places(a))
        assertEquals(favorite, restarted.stationFavorite(b, favorite.stationId!!))
        assertTrue(restarted.removePlace(a, favorite.id))
        assertEquals(1, restarted.places(b).size)
        assertTrue(restarted.places(a).isEmpty())
    }

    @Test fun concurrent_saves_of_the_same_station_leave_exactly_one_row() = runBlocking {
        val store = open()
        val station = "44444444-4444-4444-8444-444444444444"
        (1..8).map { async(Dispatchers.Default) { store.savePlace(a, station(station, "n$it")) } }.awaitAll()
        assertEquals(1, store.places(a).size)
    }

    @Test fun quotas_refuse_without_evicting_and_import_is_all_or_nothing() = runBlocking {
        val store = open()
        val many = List(SavedBounds.MAX_PLACES) { SavedPlace(id(), SavedPlaceKind.PLACE, "p$it", GeoPoint(it.toLong() % 1000, 0), null, SavedSourceKind.IMPORT, null, null, 0) }
        assertEquals(SavedBounds.MAX_PLACES, store.importContent(a, content(many), false, 5).importedPlaces)
        refusal("place-quota") { store.savePlace(a, station()) }
        assertEquals(SavedBounds.MAX_PLACES, store.places(a).size)
        // A file with one more place and valid trips changes nothing.
        refusal("place-quota") { store.importContent(a, content(listOf(station()), listOf(track(3))), false, 6) }
        assertEquals(SavedBounds.MAX_PLACES, store.places(a).size)
        assertTrue(store.trips(a).isEmpty())
        // The quota is per owner.
        assertEquals(1, store.importContent(b, content(listOf(station())), false, 7).importedPlaces)
    }

    @Test fun ten_thousand_point_trip_round_trips_and_delete_cascades_without_touching_other_owners() = runBlocking {
        val store = open()
        val big = track(SavedBounds.MAX_POINTS_PER_TRIP)
        store.saveTrip(a, big)
        store.saveTrip(b, big)
        val back = store.trip(a, big.id)!!
        assertEquals(big.points, back.points)
        assertEquals(SavedBounds.MAX_POINTS_PER_TRIP, store.trips(a).single().pointCount)
        assertNull(store.trip(guest, big.id))
        refusal("trip-exists") { store.saveTrip(a, big) }
        assertFalse(store.removeTrip(guest, big.id))
        assertTrue(store.removeTrip(a, big.id))
        assertNull(store.trip(a, big.id))
        assertEquals(big.points, store.trip(b, big.id)!!.points)
        db!!.openHelper.readableDatabase.query("SELECT COUNT(*) FROM map_saved_trip_point WHERE owner_account = 'acct-a'").use {
            it.moveToFirst(); assertEquals(0, it.getInt(0))
        }
    }

    @Test fun import_is_idempotent_and_a_deleted_record_cannot_resurrect_without_confirmation() = runBlocking {
        val store = open()
        val p = station()
        val t = track(5)
        val file = content(listOf(p), listOf(t), "file-1")
        val first = store.importContent(a, file, false, 10)
        assertEquals(1, first.importedPlaces)
        assertEquals(1, first.importedTrips)
        assertTrue(store.importContent(a, file, false, 11).alreadyImported)
        assertEquals(1, store.places(a).size)
        assertTrue(store.removePlace(a, p.id))
        assertTrue(store.removeTrip(a, t.id))
        // The same bytes are recognised; a different file naming the deleted ids is refused.
        assertTrue(store.importContent(a, file, false, 12).alreadyImported)
        val other = content(listOf(p), listOf(t), "file-2")
        refusal("deleted-records-present") { store.importContent(a, other, false, 13) }
        assertTrue(store.places(a).isEmpty())
        assertTrue(store.trips(a).isEmpty())
        // Another owner's tombstones do not apply to this one.
        assertEquals(1, store.importContent(b, other, false, 14).importedPlaces)
        // Explicit second confirmation recreates and clears the tombstones.
        assertEquals(1, store.importContent(a, other, true, 15).importedPlaces)
        assertEquals(1, store.places(a).size)
        assertTrue(store.removePlace(a, p.id))
        assertFalse(store.removePlace(a, p.id))
        // Tombstones hold ids only.
        db!!.openHelper.readableDatabase.query("SELECT * FROM map_saved_tombstone").use {
            assertEquals(listOf("owner_account", "origin", "record_id", "deleted_at_millis"), it.columnNames.toList())
        }
    }

    @Test fun existing_ids_are_skipped_never_overwritten_by_an_import() = runBlocking {
        val store = open()
        val mine = station("55555555-5555-4555-8555-555555555555", "Meu nome")
        store.savePlace(a, mine)
        val report = store.importContent(a, content(listOf(mine.copy(name = "Nome do arquivo", note = "x"))), false, 20)
        assertEquals(0, report.importedPlaces)
        assertEquals(1, report.skippedExisting)
        assertEquals("Meu nome", store.places(a).single().name)
    }

    @Test fun recording_is_explicit_survives_restart_and_saves_or_discards_per_owner() = runBlocking {
        val store = open()
        val rid = id()
        assertTrue(store.startRecording(a, rid, "Passeio", 100))
        assertFalse("one recording at a time", store.startRecording(a, id(), "Outro", 101))
        assertTrue("another owner has its own recording", store.startRecording(b, id(), "Do B", 102))
        assertEquals(2, store.appendRecording(a, rid, listOf(TripPoint(GeoPoint(1, 1), null, 5), TripPoint(GeoPoint(2, 2), null, 6))))
        assertEquals(3, store.appendRecording(a, rid, listOf(TripPoint(GeoPoint(3, 3), null, 7))))
        assertTrue("a recording is not a saved trip", store.trips(a).isEmpty())
        assertNull(store.trip(a, rid))
        val restarted = open()
        assertEquals(RecordingState(rid, "Passeio", 100, 3), restarted.recording(a))
        refusal("recording-missing") { restarted.appendRecording(guest, rid, listOf(TripPoint(GeoPoint(9, 9)))) }
        val saved = restarted.finishRecording(a, rid, save = true, nowMillis = 200)!!
        assertEquals(SavedTripKind.RECORDED_TRACK, saved.kind)
        assertEquals(listOf(1L, 2L, 3L), saved.points.map { it.point.latMicro })
        assertNull(restarted.recording(a))
        assertEquals(1, restarted.trips(a).size)
        val other = restarted.recording(b)!!
        assertNull(restarted.finishRecording(b, other.id, save = false, nowMillis = 201))
        assertNull(restarted.recording(b))
        assertTrue(restarted.trips(b).isEmpty())
    }

    @Test fun recording_refuses_beyond_the_track_limit_and_empty_recordings_save_nothing() = runBlocking {
        val store = open()
        val rid = id()
        store.startRecording(a, rid, "Longa", 1)
        store.appendRecording(a, rid, List(SavedBounds.MAX_POINTS_PER_TRIP) { TripPoint(GeoPoint(it.toLong(), 0)) })
        refusal("track-points-invalid") { store.appendRecording(a, rid, listOf(TripPoint(GeoPoint(1, 1)))) }
        assertEquals(SavedBounds.MAX_POINTS_PER_TRIP, store.recording(a)!!.pointCount)
        assertNotNull(store.finishRecording(a, rid, true, 2))
        val empty = id()
        store.startRecording(a, empty, "Vazia", 3)
        assertNull(store.finishRecording(a, empty, true, 4))
        assertEquals(1, store.trips(a).size)
    }

    @Test fun clearing_one_owner_leaves_everyone_else_untouched() = runBlocking {
        val store = open()
        val p = station()
        store.savePlace(a, p); store.savePlace(b, p); store.saveTrip(a, track(3)); store.saveTrip(guest, track(3, id()))
        store.clearOwner(a)
        assertTrue(store.places(a).isEmpty())
        assertTrue(store.trips(a).isEmpty())
        assertEquals(1, store.places(b).size)
        assertEquals(1, store.trips(guest).size)
    }
}
