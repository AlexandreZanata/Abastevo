package com.anpfuel.domain.map.saved

data class TripSummary(
    val id: String,
    val kind: SavedTripKind,
    val name: String,
    val pointCount: Int,
    val avoid: Set<String>,
    val createdAtMillis: Long,
)

data class RecordingState(val id: String, val name: String, val startedAtMillis: Long, val pointCount: Int)

data class ImportReport(
    val importedPlaces: Int,
    val importedTrips: Int,
    val skippedExisting: Int,
    val alreadyImported: Boolean,
)

/**
 * MAP-25 owner-scoped private store. Every call carries the [SavedOwner]; there is no
 * operation that lists, reads or deletes across owners (B-BR-MAP25-01).
 */
interface SavedPrivateStore {
    suspend fun places(owner: SavedOwner): List<SavedPlace>
    suspend fun stationFavorite(owner: SavedOwner, stationId: String): SavedPlace?

    /** Idempotent for the same station: returns the stored favorite. Quotas refuse, never evict. */
    suspend fun savePlace(owner: SavedOwner, place: SavedPlace): SavedPlace
    suspend fun removePlace(owner: SavedOwner, id: String): Boolean

    suspend fun trips(owner: SavedOwner): List<TripSummary>
    suspend fun trip(owner: SavedOwner, id: String): SavedTrip?
    suspend fun saveTrip(owner: SavedOwner, trip: SavedTrip)
    suspend fun removeTrip(owner: SavedOwner, id: String): Boolean

    /**
     * All-or-nothing. Idempotent by content hash. A record id deleted earlier is refused with
     * `deleted-records-present` unless [allowDeleted] is true (explicit second confirmation).
     */
    suspend fun importContent(owner: SavedOwner, content: ExchangeContent, allowDeleted: Boolean, nowMillis: Long): ImportReport

    suspend fun startRecording(owner: SavedOwner, id: String, name: String, nowMillis: Long): Boolean
    suspend fun recording(owner: SavedOwner): RecordingState?
    suspend fun appendRecording(owner: SavedOwner, id: String, points: List<TripPoint>): Int

    /** Saves a RECORDED_TRACK (null when it has no point) or discards it; the recording row is gone either way. */
    suspend fun finishRecording(owner: SavedOwner, id: String, save: Boolean, nowMillis: Long): SavedTrip?

    suspend fun clearOwner(owner: SavedOwner)
}
