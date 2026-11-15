package com.anpfuel.domain.enterprise.tracking

import com.anpfuel.domain.enterprise.trip.MembershipRef
import com.anpfuel.domain.enterprise.trip.TripDomainException
import com.anpfuel.domain.enterprise.trip.TripFailure
import com.anpfuel.domain.enterprise.trip.TripRef
import com.anpfuel.domain.enterprise.trip.TripState
import java.io.File
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import java.time.Duration
import java.time.Instant
import org.junit.jupiter.api.Assertions.assertEquals

/** The fixture grammar of session-v1 (same rules as the Go harness). */
internal object TrackingFixture {
    private val aliases = mapOf("A" to 1, "B" to 2, "S" to 11, "S2" to 12, "T" to 21, "T2" to 22, "M" to 31, "M2" to 32)
    val base: Instant = Instant.parse("2026-11-15T10:00:00Z")
    private val defaultCause = mapOf("SUSPEND" to "TRIP_PAUSED", "INTERRUPT" to "PROCESS_STOPPED", "REDUCE" to "PRECISION_REDUCED",
        "END" to "TRIP_COMPLETED")
    private val openOmittable = setOf("session", "trip", "driver", "state", "author", "origin", "policy", "at")
    private val actionOmittable = setOf("trip", "driver", "state", "author", "origin", "permission", "at")
    private val eventOmittable = setOf("trip", "driver", "state", "author", "cause", "at")
    private val followOmittable = setOf("trip", "driver", "state", "at")
    val collectOmittable = setOf("trip", "driver", "state")

    fun hex(raw: String): String = raw.toByteArray(Charsets.UTF_8).joinToString("") { "%02x".format(it.toInt() and 255) }
    fun id(index: Int) = "9b000000-0000-4000-8000-" + index.toString().padStart(12, '0')
    fun alias(raw: String): String = aliases[raw]?.let(::id) ?: raw
    fun label(raw: String): String = aliases.entries.firstOrNull { id(it.value) == raw }?.key ?: raw
    fun fields(raw: String): Map<String, String> {
        val pairs = raw.split(';').map { val p = it.split('=', limit = 2); require(p.size == 2 && p[0].isNotBlank()); p[0] to p[1] }
        require(pairs.map { it.first }.distinct().size == pairs.size)
        return pairs.toMap()
    }
    fun decode(raw: String): String {
        require(raw.length % 2 == 0 && raw.all { it in '0'..'9' || it in 'a'..'f' })
        return Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT).onUnmappableCharacter(CodingErrorAction.REPORT)
            .decode(ByteBuffer.wrap(raw.chunked(2).map { it.toInt(16).toByte() }.toByteArray())).toString()
    }
    fun omitted(raw: String?, allowed: Set<String>): Set<String> {
        if (raw.isNullOrEmpty()) return emptySet()
        val keys = raw.split(',')
        require(keys.all { it in allowed } && keys.distinct().size == keys.size)
        return keys.toSet()
    }
    fun flag(raw: String): Boolean = when (raw) { "true" -> true; "false" -> false; else -> error("Malformed fixture flag") }

    /** A whole-minute offset from the fixture base, or "zero" for the unset instant. */
    fun instant(raw: String): Instant = if (raw == "zero") Instant.parse("0001-01-01T00:00:00Z") else base.plus(Duration.ofMinutes(raw.toLong()))
    fun minute(at: Instant): String = Duration.between(base, at).toMinutes().toString()

    /** Reads a corpus after asserting its exact hash and header; rows are id, operation, args, expected. */
    fun corpus(name: String, hash: String): List<List<String>> {
        val file = File("../contracts/testdata/enterprise/tracking/$name")
        assertEquals(hash, MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it.toInt() and 255) })
        val lines = file.readLines()
        assertEquals("id|operation|args|expected", lines.first())
        val rows = lines.drop(1).map { it.split('|') }
        require(rows.all { it.size == 4 && it[0].isNotBlank() })
        assertEquals(rows.size, rows.map { it[0] }.distinct().size)
        return rows
    }

    private fun derivedTripState(op: String, cause: String?): String = when {
        op == "SUSPEND" && cause == "TRIP_PAUSED" -> "PAUSED"
        op == "END" && cause == "TRIP_COMPLETED" -> "COMPLETED"
        op == "END" && cause == "TRIP_CANCELLED" -> "CANCELLED"
        else -> "IN_PROGRESS"
    }

    fun commandFields(op: String, args: Map<String, String>): Map<String, String> {
        val f = mutableMapOf("scope" to "A", "trip" to "T", "trip_org" to "A", "driver" to "M", "driver_org" to "A", "author" to "M",
            "author_org" to "A", "origin" to "TRIP_SCREEN", "permission" to "PRECISE", "notification" to "true", "at" to "100")
        defaultCause[op]?.let { f["cause"] = it }
        f.putAll(args)
        f.putIfAbsent("trip_state", derivedTripState(op, f["cause"]))
        return f
    }

    private fun openFields(args: Map<String, String>): Map<String, String> {
        val f = mutableMapOf("org" to "A", "session" to "S", "trip" to "T", "driver" to "M", "trip_state" to "IN_PROGRESS", "author" to "M",
            "origin" to "TRIP_SCREEN", "policy" to "EXPECTED", "at" to "0", "existing" to "none")
        f.putAll(args)
        for (key in listOf("scope", "session_org", "trip_org", "driver_org", "author_org")) f.putIfAbsent(key, f.getValue("org"))
        return f
    }

    fun facts(f: Map<String, String>, omit: Set<String>): TrackingTripFacts = TrackingTripFacts(
        reference = if ("trip" in omit) null else TripRef(alias(f.getValue("trip_org")), alias(f.getValue("trip"))),
        driver = if ("driver" in omit) null else MembershipRef(alias(f.getValue("driver_org")), alias(f.getValue("driver"))),
        state = if ("state" in omit) null else TripState.parse(f.getValue("trip_state")),
    )

    private fun author(f: Map<String, String>, omit: Set<String>): MembershipRef? =
        if ("author" in omit) null else MembershipRef(alias(f.getValue("author_org")), alias(f.getValue("author")))

    private fun existing(kind: String): TrackingSession? = when (kind) {
        "none" -> null
        "open" -> mustOpen(mapOf("session" to "S2"))
        "ended" -> runCommand(mustOpen(mapOf("session" to "S2")), "END", commandFields("END", mapOf("cause" to "MEMBERSHIP_ENDED", "at" to "0")))
        "other_trip" -> mustOpen(mapOf("session" to "S2", "trip" to "T2"))
        "foreign" -> mustOpen(mapOf("session" to "S2", "org" to "B"))
        else -> error("Unknown existing fixture session")
    }

    private fun mustOpen(args: Map<String, String>): TrackingSession = open(openFields(args))

    fun openVector(args: Map<String, String>): TrackingSession = open(openFields(args))

    private fun open(f: Map<String, String>): TrackingSession {
        val omit = omitted(f["omit"], openOmittable)
        val existing = existing(f.getValue("existing"))
        val session = if ("session" in omit) null else TrackingSessionRef(alias(f.getValue("session_org")), alias(f.getValue("session")))
        val trip = facts(f, omit)
        val author = author(f, omit)
        val origin = if ("origin" in omit) null else TrackingOrigin.parse(f.getValue("origin"))
        val policy = if ("policy" in omit) null else TrackingPolicy.parse(f.getValue("policy"))
        val at = if ("at" in omit) null else instant(f.getValue("at"))
        return TrackingSession.open(alias(f.getValue("scope")), TrackingOpenInput(session, trip, author, origin, policy, at, existing))
    }

    /** Builds the command input in the documented order and applies it. */
    fun runCommand(session: TrackingSession, op: String, f: Map<String, String>): TrackingSession {
        val scope = alias(f.getValue("scope"))
        return when (op) {
            "START", "RESUME" -> {
                val omit = omitted(f["omit"], actionOmittable)
                val trip = facts(f, omit)
                val author = author(f, omit)
                val origin = if ("origin" in omit) null else TrackingOrigin.parse(f.getValue("origin"))
                val permission = if ("permission" in omit) null else LocationPermission.parse(f.getValue("permission"))
                val device = TrackingDevice(permission, flag(f.getValue("notification")))
                val action = TrackingDriverAction(trip, author, origin, device, if ("at" in omit) null else instant(f.getValue("at")))
                if (op == "START") session.start(scope, action) else session.resume(scope, action)
            }
            "SUSPEND", "INTERRUPT", "REDUCE", "END" -> {
                val omit = omitted(f["omit"], eventOmittable)
                val trip = facts(f, omit)
                val author = author(f, omit)
                val cause = if ("cause" in omit) null else TrackingCause.parse(f.getValue("cause"))
                val event = TrackingEvent(trip, cause, author, if ("at" in omit) null else instant(f.getValue("at")))
                when (op) {
                    "SUSPEND" -> session.suspend(scope, event)
                    "INTERRUPT" -> session.interrupt(scope, event)
                    "REDUCE" -> session.reducePrecision(scope, event)
                    else -> session.end(scope, event)
                }
            }
            "FOLLOW" -> {
                val omit = omitted(f["omit"], followOmittable)
                val trip = facts(f, omit)
                session.follow(scope, trip, if ("at" in omit) null else instant(f.getValue("at")))
            }
            else -> error("Unknown session command")
        }
    }

    /** Opens the default session at minute 0 and applies each prior step at minutes 10, 20, ...; a failing step fails the loader. */
    fun priorSession(raw: String?): TrackingSession {
        var session = mustOpen(emptyMap())
        if (raw.isNullOrEmpty()) return session
        raw.split(',').forEachIndexed { i, step ->
            val parts = step.split(':', limit = 2)
            val op = parts[0]
            val arg = parts.getOrElse(1) { "" }
            val f = mutableMapOf("at" to (10 * (i + 1)).toString())
            when (op) {
                "START", "RESUME" -> when (arg) {
                    "", "PRECISE" -> Unit
                    "APPROXIMATE", "NONE" -> f["permission"] = arg
                    "BLOCKED" -> f["notification"] = "false"
                    else -> error("Unknown prior permission")
                }
                "SUSPEND", "INTERRUPT", "END" -> f["cause"] = arg
                "REDUCE" -> require(arg.isEmpty())
                "FOLLOW" -> f["trip_state"] = arg
                else -> error("Unknown prior step")
            }
            session = try { runCommand(session, op, commandFields(op, f)) } catch (e: IllegalArgumentException) { error("Prior fixture step failed: $step") }
        }
        return session
    }

    fun sessionFields(session: TrackingSession): Map<String, String> {
        val history = session.history.joinToString(",") {
            "${it.state.name}:${it.cause?.name ?: "?"}:${it.actor.name}:${it.permission?.name ?: "?"}@${minute(it.at)}"
        }
        val periods = session.recordingPeriods.joinToString(",") { "${minute(it.start)}~${it.end?.let(::minute) ?: "?"}~${it.precision.name}" }
        val org = session.organizationId
        return mapOf("org" to (if (session.trip.organizationId == org && session.owner.organizationId == org) label(org) else "MIXED"),
            "id" to label(session.reference.id), "trip" to label(session.trip.id), "owner" to label(session.owner.id),
            "policy" to session.policy.name, "state" to session.state.name, "cause" to (session.cause?.name ?: "?"),
            "precision" to (session.precision?.name ?: "?"), "coarse" to session.coarse.toString(), "history" to history,
            "periods" to periods.ifEmpty { "?" })
    }

    /** Maps the tracking vocabulary; fixture-built trip references keep their own INVALID/SCOPE meaning. */
    fun failure(error: Throwable): Map<String, String> = when (error) {
        is TrackingDomainException -> mapOf("error" to error.failure.name)
        is TripDomainException -> when (error.failure) {
            TripFailure.INVALID, TripFailure.SCOPE -> mapOf("error" to error.failure.name)
            else -> throw error
        }
        else -> throw error
    }
}
