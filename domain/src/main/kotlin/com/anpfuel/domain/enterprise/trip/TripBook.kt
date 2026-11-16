package com.anpfuel.domain.enterprise.trip

import com.anpfuel.domain.enterprise.EnterprisePartition
import com.anpfuel.domain.enterprise.EnterpriseTime
import com.anpfuel.domain.enterprise.sync.CanonicalJson
import com.anpfuel.domain.enterprise.sync.SyncJson
import java.time.Instant

/*
 * The device's local trip index (EMP-TRIP-005, B-BR-EMP-376/377, EDR-242): one sealed record per partition,
 * written only together with the operation it indexes. It decides nothing on the server. Version 2
 * (EMP-TRIP-021, B-BR-EMP-529/530, EDR-344) adds each trip's mission and its lifecycle commands and reads v1.
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
    /** A pause needs a running trip (EMP-TRIP-011). */
    NOT_RUNNING,
    /** A resume needs a paused trip. */
    NOT_PAUSED,
    /** The mission already has a trip on this device whose start was not refused (B-BR-EMP-529). */
    MISSION_ALREADY_STARTED,
}

class TripLocalException(val failure: TripLocalFailure) : IllegalStateException("trip local: ${failure.name.lowercase()}")

enum class TripLocalState { IN_PROGRESS, PAUSED, FINISHED, CANCELLED, START_REFUSED }

/** One lifecycle command of a local trip, in the order it was recorded (EMP-TRIP-011). */
class TripLifecycleEntry(val command: TripCommand, val operationId: String) {
    init { require(command == TripCommand.PAUSE || command == TripCommand.RESUME || command == TripCommand.CANCEL); EnterprisePartition.requireId(operationId) }
    override fun equals(other: Any?): Boolean = other is TripLifecycleEntry && command == other.command && operationId == other.operationId
    override fun hashCode(): Int = 31 * command.hashCode() + operationId.hashCode()
    override fun toString(): String = "EnterpriseTripLifecycleEntry(redacted)"
}

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
    /** The mission this trip executes (the start's EMP-TRIP-023 reference), absent for an ordinary trip. */
    val missionId: String? = null,
    lifecycle: List<TripLifecycleEntry> = emptyList(),
) {
    private val events = eventOperationIds.toList()
    val eventOperationIds: List<String> get() = events.toList()
    private val commands = lifecycle.toList()
    /** The lifecycle commands in recorded order. */
    val lifecycle: List<TripLifecycleEntry> get() = commands.toList()
    /** The revision the next lifecycle command expects: 1 after the start, one more per earlier command (EDR-344). */
    val nextRevision: Long get() = 1L + commands.size
    init {
        for (id in listOf(tripId, assetId, startOperationId) + events + listOfNotNull(finishOperationId, submitOperationId, missionId)) EnterprisePartition.requireId(id)
        // Commands alternate PAUSE/RESUME from a running trip; a cancellation is the last one and excludes a finish.
        require(commands.size <= TripBook.MAX_EVENTS && commands.withIndex().all { (index, entry) -> when (entry.command) {
            TripCommand.PAUSE -> index % 2 == 0
            TripCommand.RESUME -> index % 2 == 1
            else -> index == commands.lastIndex && finishOperationId == null
        } })
        require(events.size <= TripBook.MAX_EVENTS && (finishOperationId == null) == (finishedAt == null) &&
            (submitOperationId == null || finishOperationId != null) && (finishedAt == null || !finishedAt.isBefore(startedAt)))
        val ids = operationIds
        require(ids.distinct().size == ids.size)
        require(label.name.codePointCount(0, label.name.length) <= 80 && label.fleetNumber.codePointCount(0, label.fleetNumber.length) <= 40 &&
            label.plate.codePointCount(0, label.plate.length) <= 40)
    }
    val state: TripLocalState get() = when {
        startRefused -> TripLocalState.START_REFUSED
        commands.lastOrNull()?.command == TripCommand.CANCEL -> TripLocalState.CANCELLED
        finishOperationId != null -> TripLocalState.FINISHED
        commands.lastOrNull()?.command == TripCommand.PAUSE -> TripLocalState.PAUSED
        else -> TripLocalState.IN_PROGRESS
    }
    /** Running or paused: it holds its asset and admits events and the finish (EMP-TRIP-011). */
    val open: Boolean get() = state == TripLocalState.IN_PROGRESS || state == TripLocalState.PAUSED
    /** Every operation identifier of the trip, start first. */
    val operationIds: List<String> get() = listOf(startOperationId) + events + commands.map { it.operationId } +
        listOfNotNull(finishOperationId, submitOperationId)
    internal fun copy(events: List<String> = this.events, finishOperationId: String? = this.finishOperationId, finishedAt: Instant? = this.finishedAt,
        submitOperationId: String? = this.submitOperationId, startRefused: Boolean = this.startRefused,
        lifecycle: List<TripLifecycleEntry> = this.commands) = LocalTrip(tripId, assetId, label,
        reviewWarningAccepted, startOperationId, startedAt, events, finishOperationId, finishedAt, submitOperationId, startRefused, missionId, lifecycle)
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
        if (!trip.open) throw TripLocalException(TripLocalFailure.TRIP_NOT_OPEN)
        if (at.isBefore(trip.startedAt)) throw TripLocalException(TripLocalFailure.BEFORE_START)
        if (operationCount >= MAX_OPERATIONS) throw TripLocalException(TripLocalFailure.FULL)
        return trip
    }

    /** A new trip; refused when its asset already has an unfinished, not refused trip here. */
    fun withStart(trip: LocalTrip): TripBook {
        require(trip.eventOperationIds.isEmpty() && trip.finishOperationId == null && trip.submitOperationId == null && !trip.startRefused)
        if (trip(trip.tripId) != null || known(trip.startOperationId)) throw TripLocalException(TripLocalFailure.DUPLICATE)
        if (values.any { it.assetId == trip.assetId && it.open }) throw TripLocalException(TripLocalFailure.ASSET_HAS_UNFINISHED_TRIP)
        if (trip.missionId != null && missionTrip(trip.missionId) != null) throw TripLocalException(TripLocalFailure.MISSION_ALREADY_STARTED)
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
    /** EMP-TRIP-011: a running trip is paused; the command is declared at or after the start. */
    fun withPause(tripId: String, operationId: String, at: Instant): TripBook = command(tripId, operationId, at, TripCommand.PAUSE)
    /** A paused trip is resumed. */
    fun withResume(tripId: String, operationId: String, at: Instant): TripBook = command(tripId, operationId, at, TripCommand.RESUME)
    /** A running or paused trip is cancelled; it then holds its asset no more on this device. */
    fun withCancel(tripId: String, operationId: String, at: Instant): TripBook = command(tripId, operationId, at, TripCommand.CANCEL)
    /** The trip of this book, if any, that executes [missionId] and whose start was not refused (B-BR-EMP-529). */
    fun missionTrip(missionId: String): LocalTrip? = values.lastOrNull { it.missionId == missionId && !it.startRefused }

    private fun command(tripId: String, operationId: String, at: Instant, command: TripCommand): TripBook {
        val trip = open(tripId, operationId, at)
        when (command) {
            TripCommand.PAUSE -> if (trip.state != TripLocalState.IN_PROGRESS) throw TripLocalException(TripLocalFailure.NOT_RUNNING)
            TripCommand.RESUME -> if (trip.state != TripLocalState.PAUSED) throw TripLocalException(TripLocalFailure.NOT_PAUSED)
            else -> Unit
        }
        if (trip.lifecycle.size >= MAX_EVENTS) throw TripLocalException(TripLocalFailure.FULL)
        return replace(trip.copy(lifecycle = trip.lifecycle + TripLifecycleEntry(command, operationId)))
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
    val canonicalBytes: ByteArray get() = encode(VERSION)
    private fun encode(version: Long): ByteArray = CanonicalJson.encode(SyncJson.Object(mapOf(
        "format" to SyncJson.Text(FORMAT), "version" to SyncJson.Integer(version.toString()),
        "membership_id" to SyncJson.Text(membershipId), "enrollment_id" to SyncJson.Text(enrollmentId),
        "trips" to SyncJson.Array(values.map { tripJson(it, version) }),
    )))
    override fun toString(): String = "EnterpriseTripBook(redacted)"

    companion object {
        const val FORMAT = "trip-book"
        /** Version 2 (EMP-TRIP-021, EDR-344) adds `mission_id` and `lifecycle`; version 1 is still read. */
        const val VERSION = 2L
        private const val VERSION_1 = 1L
        const val MAX_TRIPS = 1000
        const val MAX_EVENTS = 500
        /** The queue's own entry bound (OFFL-003). */
        const val MAX_OPERATIONS = 10_000
        const val MAX_BYTES = 2 * 1024 * 1024
        private val TRIP_KEYS = setOf("trip_id", "asset_id", "label", "review_warning_accepted", "start_operation_id", "started_at",
            "event_operation_ids", "finish_operation_id", "finished_at", "submit_operation_id", "start_refused")
        private val TRIP_KEYS_V2 = TRIP_KEYS + setOf("mission_id", "lifecycle")
        fun empty(membershipId: String, enrollmentId: String): TripBook = TripBook(membershipId, enrollmentId, emptyList())

        /** Strict decode; an unknown format or version is refused, never guessed. */
        fun decode(raw: ByteArray): TripBook {
            val root = obj(CanonicalJson.parseLarge(raw, MAX_BYTES), setOf("format", "version", "membership_id", "enrollment_id", "trips"))
            val version = when ((root.fields["version"] as? SyncJson.Integer)?.lexeme) {
                VERSION.toString() -> VERSION
                VERSION_1.toString() -> VERSION_1
                else -> throw IllegalArgumentException("Invalid trip book")
            }
            require(text(root, "format") == FORMAT)
            val trips = (root.fields["trips"] as? SyncJson.Array ?: throw IllegalArgumentException()).values.map { item ->
                val trip = obj(item, if (version == VERSION) TRIP_KEYS_V2 else TRIP_KEYS)
                val lifecycle = if (version == VERSION_1) emptyList() else
                    (trip.fields["lifecycle"] as? SyncJson.Array ?: throw IllegalArgumentException()).values.map {
                        val entry = obj(it, setOf("command", "operation_id"))
                        TripLifecycleEntry(try { TripCommand.parse(text(entry, "command")) } catch (_: TripDomainException) {
                            throw IllegalArgumentException("Invalid trip book") }, text(entry, "operation_id"))
                    }
                val label = obj(trip.fields["label"], setOf("name", "fleet_number", "plate"))
                LocalTrip(text(trip, "trip_id"), text(trip, "asset_id"), TripAssetLabel(text(label, "name"), text(label, "fleet_number"), text(label, "plate")),
                    bool(trip, "review_warning_accepted"), text(trip, "start_operation_id"), instant(text(trip, "started_at")),
                    (trip.fields["event_operation_ids"] as? SyncJson.Array ?: throw IllegalArgumentException()).values.map {
                        (it as? SyncJson.Text)?.value ?: throw IllegalArgumentException() },
                    nullable(trip, "finish_operation_id"), nullable(trip, "finished_at")?.let(::instant), nullable(trip, "submit_operation_id"),
                    bool(trip, "start_refused"), if (version == VERSION_1) null else nullable(trip, "mission_id"), lifecycle)
            }
            val book = TripBook(text(root, "membership_id"), text(root, "enrollment_id"), trips)
            // Only the exact canonical spelling of its own version is a stored book.
            require(book.encode(version).contentEquals(raw))
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
        private fun tripJson(trip: LocalTrip, version: Long): SyncJson = SyncJson.Object(mapOf(
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
        ) + if (version == VERSION_1) emptyMap() else mapOf(
            "mission_id" to optional(trip.missionId),
            "lifecycle" to SyncJson.Array(trip.lifecycle.map {
                SyncJson.Object(mapOf("command" to SyncJson.Text(it.command.name), "operation_id" to SyncJson.Text(it.operationId)))
            }),
        ))
    }
}
