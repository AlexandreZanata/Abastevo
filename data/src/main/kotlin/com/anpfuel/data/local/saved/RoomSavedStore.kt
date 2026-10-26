package com.anpfuel.data.local.saved

import androidx.room.withTransaction
import com.anpfuel.data.local.AnpFuelDatabase
import com.anpfuel.domain.map.saved.*

/**
 * Room implementation of the MAP-25 owner-scoped store. Every statement filters by
 * owner + origin; composite primary keys mean two owners can hold the same record id
 * (for example the same imported file) without ever colliding or leaking.
 */
class RoomSavedStore(private val db: AnpFuelDatabase) : SavedPrivateStore {
    private val dao get() = db.savedDao()

    private fun SavedPlaceEntity.toDomain() = SavedPlace(
        id = placeId, kind = SavedPlaceKind.valueOf(kind), name = name, point = GeoPoint(latMicro, lonMicro),
        stationId = stationId, sourceKind = SavedSourceKind.valueOf(sourceKind), sourceId = sourceId, note = note,
        createdAtMillis = createdAtMillis,
    )

    private fun entity(owner: SavedOwner, p: SavedPlace) = SavedPlaceEntity(
        p.id, owner.owner, owner.origin, p.kind.name, p.name, p.point.latMicro, p.point.lonMicro, p.stationId,
        p.sourceKind.name, p.sourceId, p.note, p.createdAtMillis,
    )

    private fun avoidOf(text: String): Set<String> = text.split(',').filter { it.isNotEmpty() }.toSet()

    private fun SavedTripEntity.summary() = TripSummary(
        tripId, SavedTripKind.valueOf(kind), name, pointCount, avoidOf(avoid), createdAtMillis,
    )

    private fun tripEntity(owner: SavedOwner, t: SavedTrip, state: String) = SavedTripEntity(
        t.id, owner.owner, owner.origin, state, t.kind.name, t.name, t.avoid.sorted().joinToString(","), t.sourceId, t.note,
        t.points.size, t.createdAtMillis,
    )

    private fun pointRows(owner: SavedOwner, id: String, first: Int, points: List<TripPoint>) =
        points.mapIndexed { i, p ->
            SavedTripPointEntity(owner.owner, owner.origin, id, first + i, p.point.latMicro, p.point.lonMicro, p.elevationCm, p.timeMillis)
        }

    override suspend fun places(owner: SavedOwner) = dao.places(owner.owner, owner.origin).map { it.toDomain() }

    override suspend fun stationFavorite(owner: SavedOwner, stationId: String) =
        dao.placeByStation(owner.owner, owner.origin, stationId)?.toDomain()

    override suspend fun savePlace(owner: SavedOwner, place: SavedPlace): SavedPlace = db.withTransaction {
        place.stationId?.let { station ->
            dao.placeByStation(owner.owner, owner.origin, station)?.let { return@withTransaction it.toDomain() }
        }
        dao.place(owner.owner, owner.origin, place.id)?.let { return@withTransaction it.toDomain() }
        if (dao.placeCount(owner.owner, owner.origin) >= SavedBounds.MAX_PLACES) throw SavedValidationException("place-quota")
        dao.insertPlace(entity(owner, place))
        dao.clearTombstones(owner.owner, owner.origin, listOf(place.id))
        place
    }

    override suspend fun removePlace(owner: SavedOwner, id: String): Boolean = db.withTransaction {
        val removed = dao.deletePlace(owner.owner, owner.origin, id) > 0
        if (removed) dao.tombstone(SavedTombstoneEntity(owner.owner, owner.origin, id, System.currentTimeMillis()))
        removed
    }

    override suspend fun trips(owner: SavedOwner) = dao.trips(owner.owner, owner.origin).map { it.summary() }

    override suspend fun trip(owner: SavedOwner, id: String): SavedTrip? {
        val row = dao.tripAnyState(owner.owner, owner.origin, id)?.takeIf { it.state == "COMPLETE" } ?: return null
        val points = dao.points(owner.owner, owner.origin, id).map { TripPoint(GeoPoint(it.latMicro, it.lonMicro), it.elevationCm, it.timeMillis) }
        return SavedTrip(
            row.tripId, SavedTripKind.valueOf(row.kind), row.name, points, avoidOf(row.avoid), row.sourceId, row.note, row.createdAtMillis,
        )
    }

    override suspend fun saveTrip(owner: SavedOwner, trip: SavedTrip) {
        db.withTransaction {
            if (dao.tripAnyState(owner.owner, owner.origin, trip.id) != null) throw SavedValidationException("trip-exists")
            if (dao.tripCount(owner.owner, owner.origin) >= SavedBounds.MAX_TRIPS) throw SavedValidationException("trip-quota")
            insertTripLocked(owner, trip, "COMPLETE")
        }
    }

    private suspend fun insertTripLocked(owner: SavedOwner, trip: SavedTrip, state: String) {
        dao.insertTrip(tripEntity(owner, trip, state))
        trip.points.chunked(500).forEachIndexed { index, chunk ->
            dao.insertPoints(pointRows(owner, trip.id, index * 500, chunk))
        }
        dao.clearTombstones(owner.owner, owner.origin, listOf(trip.id))
    }

    override suspend fun removeTrip(owner: SavedOwner, id: String): Boolean = db.withTransaction {
        val row = dao.tripAnyState(owner.owner, owner.origin, id)
        val removed = row != null && dao.deleteTrip(owner.owner, owner.origin, id) > 0
        if (removed) dao.tombstone(SavedTombstoneEntity(owner.owner, owner.origin, id, System.currentTimeMillis()))
        removed
    }

    override suspend fun importContent(owner: SavedOwner, content: ExchangeContent, allowDeleted: Boolean, nowMillis: Long): ImportReport =
        db.withTransaction {
            if (dao.importSeen(owner.owner, owner.origin, content.contentSha256) > 0) return@withTransaction ImportReport(0, 0, 0, true)
            val ids = (content.places.map { it.id } + content.trips.map { it.id })
            val deleted = ids.chunked(500).flatMap { dao.tombstoned(owner.owner, owner.origin, it) }
            if (deleted.isNotEmpty() && !allowDeleted) throw SavedValidationException("deleted-records-present")
            val newPlaces = content.places.filter { dao.place(owner.owner, owner.origin, it.id) == null }
                .filter { p -> p.stationId.let { station -> station == null || dao.placeByStation(owner.owner, owner.origin, station) == null } }
            val newTrips = content.trips.filter { dao.tripAnyState(owner.owner, owner.origin, it.id) == null }
            if (dao.placeCount(owner.owner, owner.origin) + newPlaces.size > SavedBounds.MAX_PLACES) throw SavedValidationException("place-quota")
            if (dao.tripCount(owner.owner, owner.origin) + newTrips.size > SavedBounds.MAX_TRIPS) throw SavedValidationException("trip-quota")
            newPlaces.forEach { dao.insertPlace(entity(owner, it)) }
            newTrips.forEach { insertTripLocked(owner, it, "COMPLETE") }
            if (deleted.isNotEmpty()) deleted.chunked(500).forEach { dao.clearTombstones(owner.owner, owner.origin, it) }
            dao.recordImport(SavedImportEntity(owner.owner, owner.origin, content.contentSha256, nowMillis))
            ImportReport(newPlaces.size, newTrips.size, ids.size - newPlaces.size - newTrips.size, false)
        }

    override suspend fun startRecording(owner: SavedOwner, id: String, name: String, nowMillis: Long): Boolean = db.withTransaction {
        if (dao.recording(owner.owner, owner.origin) != null || dao.tripAnyState(owner.owner, owner.origin, id) != null) return@withTransaction false
        dao.insertTrip(
            SavedTripEntity(id, owner.owner, owner.origin, "RECORDING", SavedTripKind.RECORDED_TRACK.name, name, "", null, null, 0, nowMillis),
        )
        true
    }

    override suspend fun recording(owner: SavedOwner): RecordingState? =
        dao.recording(owner.owner, owner.origin)?.let { RecordingState(it.tripId, it.name, it.createdAtMillis, it.pointCount) }

    override suspend fun appendRecording(owner: SavedOwner, id: String, points: List<TripPoint>): Int = db.withTransaction {
        val row = dao.tripAnyState(owner.owner, owner.origin, id)?.takeIf { it.state == "RECORDING" }
            ?: throw SavedValidationException("recording-missing")
        if (row.pointCount + points.size > SavedBounds.MAX_POINTS_PER_TRIP) throw SavedValidationException("track-points-invalid")
        dao.insertPoints(pointRows(owner, id, row.pointCount, points))
        val count = row.pointCount + points.size
        dao.setPointCount(owner.owner, owner.origin, id, count)
        count
    }

    override suspend fun finishRecording(owner: SavedOwner, id: String, save: Boolean, nowMillis: Long): SavedTrip? = db.withTransaction {
        val row = dao.tripAnyState(owner.owner, owner.origin, id)?.takeIf { it.state == "RECORDING" } ?: return@withTransaction null
        if (!save || row.pointCount == 0) {
            dao.deleteTrip(owner.owner, owner.origin, id)
            return@withTransaction null
        }
        if (dao.tripCount(owner.owner, owner.origin) >= SavedBounds.MAX_TRIPS) throw SavedValidationException("trip-quota")
        dao.completeRecording(owner.owner, owner.origin, id, row.pointCount)
        trip(owner, id)
    }

    override suspend fun clearOwner(owner: SavedOwner) {
        db.withTransaction {
            dao.clearTrips(owner.owner, owner.origin)
            dao.clearPlaces(owner.owner, owner.origin)
            dao.clearAllTombstones(owner.owner, owner.origin)
            dao.clearImports(owner.owner, owner.origin)
        }
    }
}
