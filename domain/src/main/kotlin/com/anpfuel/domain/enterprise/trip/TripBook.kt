package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncJson
import java.time.Instant

/*
 * The device's local trip index (EMP-TRIP-005, B-BR-EMP-376/377, EDR-242): one sealed record per partition,
 * written only together with the operation it indexes. It decides nothing on the server.
 */

/** Local refusals before queueing (B-BR-EMP-377); never a server outcome. */
enum class TripLocalFailure {
    /** The asset already has an unfinished, not refused trip of this partition on this device. */
    ASSET_HAS_UNFINISHED_TRIP,
    /** The trip is unknown to this partition's book. */
    UNKNOWN_TRIP,
    /** Events and the finish need an unfinished, not refused trip. */
    TRIP_NOT_OPEN,
    /** "Ready to send" needs a finished trip. */
    NOT_FINISHED,
    /** The trip was already marked ready to send. */
    ALREADY_SUBMITTED,
    /** An event or finish declared before the trip's start instant (the device clock went back). */
    BEFORE_START,
    /** The identifier already exists in the book. */
    DUPLICATE,
    /** The book reached its bound. */
    FULL,
    /** The book belongs to another membership or enrollment. */
    FOREIGN,
}

class TripLocalException(val failure: TripLocalFailure) : IllegalStateException("trip local: ${failure.name.lowercase()}")

enum class TripLocalState { IN_PROGRESS, FINISHED, START_REFUSED }

/** The picker's display label of the asset at the start (B-BR-EMP-352 bounds); display only. */
class TripAssetLabel(val name: String, val fleetNumber: String, val plate: String) {
    override fun equals(other: Any?): Boolean = other is TripAssetLabel && name == other.name && fleetNumber == other.fleetNumber && plate == other.plate
    override fun hashCode(): Int = 31 * (31 * name.hashCode() + fleetNumber.hashCode()) + plate.hashCode()
    override fun toString(): String = "EnterpriseTripAssetLabel(redacted)"
}

/** One trip of the book: its asset, start instant and the identifiers of its operations. */
class LocalTrip(
    val tripId: String,
    val assetId: String,
    val label: TripAssetLabel,
    val reviewWarningAccepted: Boolean,
    val startOperationId: String,
    val startedAt: Instant,
    eventOperationIds: List<String> = emptyList(),
    val finishOperationId: String? = null,
    val finishedAt: Instant? = null,
    val submitOperationId: String? = null,
    val startRefused: Boolean = false,
) {
    private val events = eventOperationIds.toList()
    val eventOperationIds: List<String> get() = events.toList()
    init {
        for (id in listOf(tripId, assetId, startOperationId) + events + listOfNotNull(finishOperationId, submitOperationId)) EnterprisePartition.requireId(id)
        require(events.size <= TripBook.MAX_EVENTS && (finishOperationId == null) == (finishedAt == null) &&
            (submitOperationId == null || finishOperationId != null) && (finishedAt == null || !finishedAt.isBefore(startedAt)))
        val ids = operationIds
        require(ids.distinct().size == ids.size)
        require(label.name.codePointCount(0, label.name.length) <= 80 && label.fleetNumber.codePointCount(0, label.fleetNumber.length) <= 40 &&
            label.plate.codePointCount(0, label.plate.length) <= 40)
    }
    val state: TripLocalState get() = when {
        startRefused -> TripLocalState.START_REFUSED
        finishOperationId != null -> TripLocalState.FINISHED
        else -> TripLocalState.IN_PROGRESS
    }
    /** Every operation identifier of the trip, start first. */
    val operationIds: List<String> get() = listOf(startOperationId) + events + listOfNotNull(finishOperationId, submitOperationId)
    internal fun copy(events: List<String> = this.events, finishOperationId: String? = this.finishOperationId, finishedAt: Instant? = this.finishedAt,
        submitOperationId: String? = this.submitOperationId, startRefused: Boolean = this.startRefused) = LocalTrip(tripId, assetId, label,
        reviewWarningAccepted, startOperationId, startedAt, events, finishOperationId, finishedAt, submitOperationId, startRefused)
    override fun toString(): String = "EnterpriseLocalTrip(redacted)"
}

/** The versioned `trip-book` index of one partition (EDR-242). */
class TripBook private constructor(val membershipId: String, val enrollmentId: String, trips: List<LocalTrip>) {
    private val values = trips.toList()
    val trips: List<LocalTrip> get() = values.toList()
    init {
        EnterprisePartition.requireId(membershipId); EnterprisePartition.requireId(enrollmentId)
        require(values.size <= MAX_TRIPS && values.map { it.tripId }.distinct().size == values.size)
        val ids = values.flatMap { it.operationIds }
        require(ids.size <= MAX_OPERATIONS && ids.distinct().size == ids.size)
    }
    fun trip(tripId: String): LocalTrip? = values.firstOrNull { it.tripId == tripId }

    private val operationCount: Int get() = values.sumOf { it.operationIds.size }
    private fun known(operationId: String) = values.any { operationId in it.operationIds }
    private fun replace(next: LocalTrip) = TripBook(membershipId, enrollmentId, values.map { if (it.tripId == next.tripId) next else it })
    private fun open(tripId: String, operationId: String, at: Instant): LocalTrip {
        val trip = trip(tripId) ?: throw TripLocalException(TripLocalFailure.UNKNOWN_TRIP)
        if (known(operationId)) throw TripLocalException(TripLocalFailure.DUPLICATE)
        if (trip.state != TripLocalState.IN_PROGRESS) throw TripLocalException(TripLocalFailure.TRIP_NOT_OPEN)
        if (at.isBefore(trip.startedAt)) throw TripLocalException(TripLocalFailure.BEFORE_START)
        if (operationCount >= MAX_OPERATIONS) throw TripLocalException(TripLocalFailure.FULL)
        return trip
    }

    /** A new trip; refused when its asset already has an unfinished, not refused trip here. */
    fun withStart(trip: LocalTrip): TripBook {
        require(trip.eventOperationIds.isEmpty() && trip.finishOperationId == null && trip.submitOperationId == null && !trip.startRefused)
        if (trip(trip.tripId) != null || known(trip.startOperationId)) throw TripLocalException(TripLocalFailure.DUPLICATE)
        if (values.any { it.assetId == trip.assetId && it.state == TripLocalState.IN_PROGRESS }) throw TripLocalException(TripLocalFailure.ASSET_HAS_UNFINISHED_TRIP)
        if (values.size >= MAX_TRIPS || operationCount >= MAX_OPERATIONS) throw TripLocalException(TripLocalFailure.FULL)
        return TripBook(membershipId, enrollmentId, values + trip)
    }
    fun withEvent(tripId: String, operationId: String, at: Instant): TripBook {
        val trip = open(tripId, operationId, at)
        if (trip.eventOperationIds.size >= MAX_EVENTS) throw TripLocalException(TripLocalFailure.FULL)
        return replace(trip.copy(events = trip.eventOperationIds + operationId))
    }
    fun withFinish(tripId: String, operationId: String, at: Instant): TripBook {
        val trip = open(tripId, operationId, at)
        return replace(trip.copy(finishOperationId = operationId, finishedAt = at))
    }
    fun withSubmission(tripId: String, operationId: String): TripBook {
        val trip = trip(tripId) ?: throw TripLocalException(TripLocalFailure.UNKNOWN_TRIP)
        if (known(operationId)) throw TripLocalException(TripLocalFailure.DUPLICATE)
        if (trip.submitOperationId != null) throw TripLocalException(TripLocalFailure.ALREADY_SUBMITTED)
        if (trip.finishOperationId == null) throw TripLocalException(TripLocalFailure.NOT_FINISHED)
        if (trip.startRefused) throw TripLocalException(TripLocalFailure.TRIP_NOT_OPEN)
        if (operationCount >= MAX_OPERATIONS) throw TripLocalException(TripLocalFailure.FULL)
        return replace(trip.copy(submitOperationId = operationId))
    }
    /** The projection of a final REJECTED start receipt; idempotent. */
    fun withStartRefused(tripId: String, startOperationId: String): TripBook {
        val trip = trip(tripId)?.takeIf { it.startOperationId == startOperationId } ?: throw TripLocalException(TripLocalFailure.UNKNOWN_TRIP)
        return if (trip.startRefused) this else replace(trip.copy(startRefused = true))
    }
    /** Refuses a book bound to another membership or enrollment. */
    fun requireBinding(membershipId: String, enrollmentId: String): TripBook {
        if (membershipId != this.membershipId || enrollmentId != this.enrollmentId) throw TripLocalException(TripLocalFailure.FOREIGN)
        return this
    }
    val canonicalBytes: ByteArray get() = CanonicalJson.encode(SyncJson.Object(mapOf(
        "format" to SyncJson.Text(FORMAT), "version" to SyncJson.Integer(VERSION.toString()),
        "membership_id" to SyncJson.Text(membershipId), "enrollment_id" to SyncJson.Text(enrollmentId),
        "trips" to SyncJson.Array(values.map(::tripJson)),
    )))
    override fun toString(): String = "EnterpriseTripBook(redacted)"

    companion object {
        const val FORMAT = "trip-book"
        const val VERSION = 1L
        const val MAX_TRIPS = 1000
        const val MAX_EVENTS = 500
        /** The queue's own entry bound (OFFL-003). */
        const val MAX_OPERATIONS = 10_000
        const val MAX_BYTES = 2 * 1024 * 1024
        private val TRIP_KEYS = setOf("trip_id", "asset_id", "label", "review_warning_accepted", "start_operation_id", "started_at",
            "event_operation_ids", "finish_operation_id", "finished_at", "submit_operation_id", "start_refused")
        fun empty(membershipId: String, enrollmentId: String): TripBook = TripBook(membershipId, enrollmentId, emptyList())

        /** Strict decode; an unknown format or version is refused, never guessed. */
        fun decode(raw: ByteArray): TripBook {
            val root = obj(CanonicalJson.parseLarge(raw, MAX_BYTES), setOf("format", "version", "membership_id", "enrollment_id", "trips"))
            require(text(root, "format") == FORMAT && (root.fields["version"] as? SyncJson.Integer)?.lexeme == VERSION.toString())
            val trips = (root.fields["trips"] as? SyncJson.Array ?: throw IllegalArgumentException()).values.map { item ->
                val trip = obj(item, TRIP_KEYS)
                val label = obj(trip.fields["label"], setOf("name", "fleet_number", "plate"))
                LocalTrip(text(trip, "trip_id"), text(trip, "asset_id"), TripAssetLabel(text(label, "name"), text(label, "fleet_number"), text(label, "plate")),
                    bool(trip, "review_warning_accepted"), text(trip, "start_operation_id"), instant(text(trip, "started_at")),
                    (trip.fields["event_operation_ids"] as? SyncJson.Array ?: throw IllegalArgumentException()).values.map {
                        (it as? SyncJson.Text)?.value ?: throw IllegalArgumentException() },
                    nullable(trip, "finish_operation_id"), nullable(trip, "finished_at")?.let(::instant), nullable(trip, "submit_operation_id"),
                    bool(trip, "start_refused"))
            }
            val book = TripBook(text(root, "membership_id"), text(root, "enrollment_id"), trips)
            // Only the exact canonical spelling is a stored book.
            require(book.canonicalBytes.contentEquals(raw))
            return book
        }

        private fun obj(value: SyncJson?, keys: Set<String>): SyncJson.Object {
            val result = value as? SyncJson.Object ?: throw IllegalArgumentException("Invalid trip book")
            require(result.fields.keys == keys) { "Invalid trip book" }
            return result
        }
        private fun text(value: SyncJson.Object, key: String): String = (value.fields[key] as? SyncJson.Text)?.value ?: throw IllegalArgumentException()
        private fun nullable(value: SyncJson.Object, key: String): String? = value.fields[key].let { if (it === SyncJson.Null) null else text(value, key) }
        private fun bool(value: SyncJson.Object, key: String): Boolean = (value.fields[key] as? SyncJson.Bool)?.value ?: throw IllegalArgumentException()
        private fun instant(raw: String): Instant {
            val at = EnterpriseTime.parseInstant(raw)
            require(EnterpriseTime.formatInstant(at) == raw)
            return at
        }
        private fun optional(value: String?): SyncJson = value?.let { SyncJson.Text(it) } ?: SyncJson.Null
        private fun tripJson(trip: LocalTrip): SyncJson = SyncJson.Object(mapOf(
            "trip_id" to SyncJson.Text(trip.tripId), "asset_id" to SyncJson.Text(trip.assetId),
            "label" to SyncJson.Object(mapOf("name" to SyncJson.Text(trip.label.name), "fleet_number" to SyncJson.Text(trip.label.fleetNumber),
                "plate" to SyncJson.Text(trip.label.plate))),
            "review_warning_accepted" to SyncJson.Bool(trip.reviewWarningAccepted),
            "start_operation_id" to SyncJson.Text(trip.startOperationId),
            "started_at" to SyncJson.Text(EnterpriseTime.formatInstant(trip.startedAt)),
            "event_operation_ids" to SyncJson.Array(trip.eventOperationIds.map { SyncJson.Text(it) }),
            "finish_operation_id" to optional(trip.finishOperationId),
            "finished_at" to optional(trip.finishedAt?.let(EnterpriseTime::formatInstant)),
            "submit_operation_id" to optional(trip.submitOperationId),
            "start_refused" to SyncJson.Bool(trip.startRefused),
        ))
    }
}
