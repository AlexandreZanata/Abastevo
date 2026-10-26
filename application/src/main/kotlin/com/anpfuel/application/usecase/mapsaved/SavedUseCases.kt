package com.anpfuel.application.usecase.mapsaved

import com.anpfuel.application.portable.AuthSessionStore
import com.anpfuel.domain.map.saved.*
import com.anpfuel.domain.routing.NavigationFix
import java.util.UUID

/** Resolves the current owner at call time; a file or caller never chooses it (B-BR-MAP25-01). */
class SavedOwnerResolver(private val sessions: AuthSessionStore, private val origin: String) {
    fun current(): SavedOwner = SavedOwner(sessions.load()?.accountId ?: SavedOwner.GUEST, origin)
}

private suspend fun <T> SavedOwnerResolver.scoped(work: suspend (SavedOwner) -> T): T {
    val owner = current()
    val result = work(owner)
    // An account switch during the call must not expose the result to the new owner.
    if (current() != owner) throw SavedValidationException("owner-scope-changed")
    return result
}

data class SavedOverview(val owner: SavedOwner, val places: List<SavedPlace>, val trips: List<TripSummary>, val recording: RecordingState?)

class ListSavedUseCase(private val store: SavedPrivateStore, private val owners: SavedOwnerResolver) {
    suspend operator fun invoke(): SavedOverview = owners.scoped { o ->
        SavedOverview(o, store.places(o), store.trips(o), store.recording(o))
    }
}

class IsStationFavoriteUseCase(private val store: SavedPrivateStore, private val owners: SavedOwnerResolver) {
    suspend operator fun invoke(stationId: String): Boolean = owners.scoped { store.stationFavorite(it, stationId) != null }
}

/** Toggles a station favorite; returns true when the station is a favorite afterwards. */
class ToggleStationFavoriteUseCase(
    private val store: SavedPrivateStore,
    private val owners: SavedOwnerResolver,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) {
    suspend operator fun invoke(
        stationId: String, name: String, latitude: Double, longitude: Double,
        source: SavedSourceKind, sourceId: String?,
    ): Boolean = owners.scoped { o ->
        val existing = store.stationFavorite(o, stationId)
        if (existing != null) {
            store.removePlace(o, existing.id)
            false
        } else {
            store.savePlace(
                o,
                SavedPlace(newId(), SavedPlaceKind.STATION, name, GeoPoint.fromDouble(latitude, longitude), stationId, source, sourceId, null, nowMillis()),
            )
            true
        }
    }
}

class SaveRoutePlanUseCase(
    private val store: SavedPrivateStore,
    private val owners: SavedOwnerResolver,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) {
    /** Stores only the plan (origin, stops, destination, avoid options), never computed geometry. */
    suspend operator fun invoke(name: String, points: List<Pair<Double, Double>>, avoid: Set<String>): String = owners.scoped { o ->
        val trip = SavedTrip(
            newId(), SavedTripKind.ROUTE_PLAN, name,
            points.map { TripPoint(GeoPoint.fromDouble(it.first, it.second)) }, avoid, null, null, nowMillis(),
        )
        store.saveTrip(o, trip)
        trip.id
    }
}

class DeleteSavedUseCase(private val store: SavedPrivateStore, private val owners: SavedOwnerResolver) {
    suspend fun place(id: String): Boolean = owners.scoped { store.removePlace(it, id) }
    suspend fun trip(id: String): Boolean = owners.scoped { store.removeTrip(it, id) }
}

class GetSavedTripUseCase(private val store: SavedPrivateStore, private val owners: SavedOwnerResolver) {
    suspend operator fun invoke(id: String): SavedTrip? = owners.scoped { store.trip(it, id) }
}

enum class ExchangeFormat { GPX, KML }

class ImportExchangeUseCase(
    private val store: SavedPrivateStore,
    private val owners: SavedOwnerResolver,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) {
    /** Parsing is pure and finishes (or refuses) before any storage; the whole file lands in the current owner only. */
    suspend operator fun invoke(bytes: ByteArray, confirmDeleted: Boolean = false): ImportReport {
        val content = GeoExchange.parse(bytes, nowMillis(), newId)
        return owners.scoped { store.importContent(it, content, confirmDeleted, nowMillis()) }
    }
}

class ExportExchangeUseCase(private val store: SavedPrivateStore, private val owners: SavedOwnerResolver) {
    suspend operator fun invoke(format: ExchangeFormat, tripId: String? = null, placeId: String? = null): ByteArray = owners.scoped { o ->
        val places = if (tripId != null) emptyList() else store.places(o).filter { placeId == null || it.id == placeId }
        val trips = when {
            placeId != null -> emptyList()
            tripId != null -> listOfNotNull(store.trip(o, tripId))
            else -> store.trips(o).mapNotNull { store.trip(o, it.id) }
        }
        if (places.isEmpty() && trips.isEmpty()) throw SavedValidationException("nothing-to-export")
        when (format) {
            ExchangeFormat.GPX -> GeoExchange.writeGpx(places, trips)
            ExchangeFormat.KML -> GeoExchange.writeKml(places, trips)
        }
    }
}

/**
 * Explicit recorder (B-BR-MAP25-05). start/stop are user actions; [onFix] only appends
 * quality-gated, spaced points to the owner's recording; nothing starts it implicitly.
 */
class TrackRecordingUseCase(
    private val store: SavedPrivateStore,
    private val owners: SavedOwnerResolver,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val newId: () -> String = { UUID.randomUUID().toString() },
) {
    private var last: TripPoint? = null

    suspend fun start(name: String): Boolean = owners.scoped { o ->
        last = null
        store.startRecording(o, newId(), name.trim().ifEmpty { "Track" }.take(SavedBounds.MAX_NAME), nowMillis())
    }

    suspend fun state(): RecordingState? = owners.scoped { store.recording(it) }

    /** Returns the stored point count, or null when the fix was rejected or nothing is recording. */
    suspend fun onFix(fix: NavigationFix, nowNanos: Long): Int? = owners.scoped { o ->
        val recording = store.recording(o) ?: return@scoped null
        val point = TrackRecordingPolicy.accept(fix, nowNanos, last) ?: return@scoped null
        val count = try { store.appendRecording(o, recording.id, listOf(point)) } catch (e: SavedValidationException) {
            if (e.code == "track-points-invalid") return@scoped null else throw e
        }
        last = point
        count
    }

    suspend fun stop(save: Boolean): SavedTrip? = owners.scoped { o ->
        val recording = store.recording(o) ?: return@scoped null
        last = null
        store.finishRecording(o, recording.id, save, nowMillis())
    }
}
